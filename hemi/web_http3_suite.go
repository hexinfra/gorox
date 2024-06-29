// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 server and backend. See RFC 9114 and 9204.

// Server Push is not supported because it's rarely used.

package hemi

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/quic"
)

func init() {
	RegisterServer("http3Server", func(name string, stage *Stage) Server {
		s := new(http3Server)
		s.onCreate(name, stage)
		return s
	})
	RegisterBackend("http3Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP3Backend)
		b.onCreate(name, stage)
		return b
	})
}

// http3Server is the HTTP/3 server.
type http3Server struct {
	// Parent
	httpServer_[*http3Gate]
	// States
}

func (s *http3Server) onCreate(name string, stage *Stage) {
	s.httpServer_.onCreate(name, stage)
	s.tlsConfig = new(tls.Config) // tls mode is always enabled in http/3
}

func (s *http3Server) OnConfigure() {
	s.httpServer_.onConfigure()
}
func (s *http3Server) OnPrepare() {
	s.httpServer_.onPrepare()
}

func (s *http3Server) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(http3Gate)
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub() // gate
		go gate.serve()
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("http3Server=%s done\n", s.Name())
	}
	s.stage.DecSub() // server
}

// http3Gate is a gate of http3Server.
type http3Gate struct {
	// Parent
	Gate_
	// Assocs
	server *http3Server
	// States
	listener *quic.Listener // the real gate. set after open
}

func (g *http3Gate) init(id int32, server *http3Server) {
	g.Gate_.Init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *http3Gate) Server() Server  { return g.server }
func (g *http3Gate) Address() string { return g.server.Address() }
func (g *http3Gate) IsTLS() bool     { return g.server.IsTLS() }
func (g *http3Gate) IsUDS() bool     { return g.server.IsUDS() }

func (g *http3Gate) Open() error {
	listener := quic.NewListener(g.Address())
	if err := listener.Open(); err != nil {
		return err
	}
	g.listener = listener
	return nil
}
func (g *http3Gate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *http3Gate) serve() { // runner
	connID := int64(0)
	for {
		quicConn, err := g.listener.Accept()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.justClose(quicConn)
		} else {
			server3Conn := getServer3Conn(connID, g, quicConn)
			go server3Conn.serve() // server3Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http3Gate=%d done\n", g.id)
	}
	g.server.DecSub() // gate
}

func (g *http3Gate) justClose(quicConn *quic.Conn) {
	quicConn.Close()
	g.DecActives()
	g.DecConn()
}

// poolServer3Conn is the server-side HTTP/3 connection pool.
var poolServer3Conn sync.Pool

func getServer3Conn(id int64, gate *http3Gate, quicConn *quic.Conn) *server3Conn {
	var serverConn *server3Conn
	if x := poolServer3Conn.Get(); x == nil {
		serverConn = new(server3Conn)
	} else {
		serverConn = x.(*server3Conn)
	}
	serverConn.onGet(id, gate, quicConn)
	return serverConn
}
func putServer3Conn(serverConn *server3Conn) {
	serverConn.onPut()
	poolServer3Conn.Put(serverConn)
}

// server3Conn is the server-side HTTP/3 connection.
type server3Conn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	gate     *http3Gate
	quicConn *quic.Conn        // the quic connection
	buffer   *http3Buffer      // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	usedStreams  atomic.Int32                          // accumulated num of streams served or fired
	broken       atomic.Bool                           // is conn broken?
	counter      atomic.Int64                          // can be used to generate a random number
	lastRead     time.Time                             // deadline of last read operation
	lastWrite    time.Time                             // deadline of last write operation
	streams      [http3MaxActiveStreams]*server3Stream // active (open, remoteClosed, localClosed) streams
	server3Conn0                                       // all values must be zero by default in this struct!
}
type server3Conn0 struct { // for fast reset, entirely
	bufferEdge uint32 // incoming data ends at c.buffer.buf[c.bufferEdge]
	pBack      uint32 // incoming frame part (header or payload) begins from c.buffer.buf[c.pBack]
	pFore      uint32 // incoming frame part (header or payload) ends at c.buffer.buf[c.pFore]
}

func (c *server3Conn) onGet(id int64, gate *http3Gate, quicConn *quic.Conn) {
	c.id = id
	c.gate = gate
	c.quicConn = quicConn
	if c.buffer == nil {
		c.buffer = getHTTP3Buffer()
		c.buffer.incRef()
	}
}
func (c *server3Conn) onPut() {
	c.quicConn = nil
	// c.buffer is reserved
	// c.table is reserved
	c.streams = [http3MaxActiveStreams]*server3Stream{}
	c.server3Conn0 = server3Conn0{}
	c.gate = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *server3Conn) ID() int64 { return c.id }

func (c *server3Conn) IsTLS() bool { return c.gate.IsTLS() }
func (c *server3Conn) IsUDS() bool { return c.gate.IsUDS() }

func (c *server3Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.gate.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *server3Conn) markBroken()    { c.broken.Store(true) }
func (c *server3Conn) isBroken() bool { return c.broken.Load() }

func (c *server3Conn) serve() { // runner
	// TODO
	// use go c.receive()?
}

func (c *server3Conn) receive() { // runner
	// TODO
}

func (c *server3Conn) closeConn() {
	c.quicConn.Close()
	c.gate.DecActives()
	c.gate.DecConn()
}

// poolServer3Stream is the server-side HTTP/3 stream pool.
var poolServer3Stream sync.Pool

func getServer3Stream(conn *server3Conn, quicStream *quic.Stream) *server3Stream {
	var stream *server3Stream
	if x := poolServer3Stream.Get(); x == nil {
		stream = new(server3Stream)
		req, resp := &stream.request, &stream.response
		req.message = req
		req.stream = stream
		resp.message = resp
		resp.stream = stream
		resp.request = req
	} else {
		stream = x.(*server3Stream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putServer3Stream(stream *server3Stream) {
	stream.onEnd()
	poolServer3Stream.Put(stream)
}

// server3Stream is the server-side HTTP/3 stream.
type server3Stream struct {
	// Assocs
	request  server3Request  // the http/3 request.
	response server3Response // the http/3 response.
	socket   *server3Socket  // ...
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *server3Conn
	quicStream *quic.Stream // the underlying quic stream
	region     Region       // a region-based memory pool
	// Stream states (zeros)
	server3Stream0 // all values must be zero by default in this struct!
}
type server3Stream0 struct { // for fast reset, entirely
	index uint8
	state uint8
	reset bool
}

func (s *server3Stream) onUse(conn *server3Conn, quicStream *quic.Stream) { // for non-zeros
	s.region.Init()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *server3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s.quicStream = nil
	s.server3Stream0 = server3Stream0{}
	s.region.Free()
}

func (s *server3Stream) execute() { // runner
	// TODO ...
	putServer3Stream(s)
}

func (s *server3Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *server3Stream) executeExchan(webapp *Webapp, req *server3Request, resp *server3Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server3Stream) serveAbnormal(req *server3Request, resp *server3Response) { // 4xx & 5xx
	// TODO
}

func (s *server3Stream) executeSocket() { // see RFC 9220
	// TODO
}

func (s *server3Stream) Serend() httpSerend   { return s.conn.gate.server }
func (s *server3Stream) Conn() httpConn       { return s.conn }
func (s *server3Stream) remoteAddr() net.Addr { return nil } // TODO

func (s *server3Stream) markBroken()    {}               // TODO
func (s *server3Stream) isBroken() bool { return false } // TODO

func (s *server3Stream) setReadDeadline() error { // for content i/o only
	// TODO
	return nil
}
func (s *server3Stream) setWriteDeadline() error { // for content i/o only
	// TODO
	return nil
}

func (s *server3Stream) read(p []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *server3Stream) readFull(p []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *server3Stream) write(p []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *server3Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	// TODO
	return 0, nil
}

func (s *server3Stream) buffer256() []byte          { return s.stockBuffer[:] }
func (s *server3Stream) unsafeMake(size int) []byte { return s.region.Make(size) }

// server3Request is the server-side HTTP/3 request.
type server3Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Request) readContent() (p []byte, err error) { return r.readContent3() }

// server3Response is the server-side HTTP/3 response.
type server3Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Response) control() []byte { // :status xxx
	var start []byte
	if r.status >= int16(len(http3Controls)) || http3Controls[r.status] == nil {
		copy(r.start[:], http3Template[:])
		r.start[8] = byte(r.status/100 + '0')
		r.start[9] = byte(r.status/10%10 + '0')
		r.start[10] = byte(r.status%10 + '0')
		start = r.start[:len(http3Template)]
	} else {
		start = http3Controls[r.status]
	}
	return start
}

func (r *server3Response) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *server3Response) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *server3Response) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *server3Response) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *server3Response) delHeaderAt(i uint8)                        { r.delHeaderAt3(i) }

func (r *server3Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *server3Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *server3Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}

func (r *server3Response) AddCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *server3Response) sendChain() error { return r.sendChain3() }

func (r *server3Response) echoHeaders() error { return r.writeHeaders3() }
func (r *server3Response) echoChain() error   { return r.echoChain3() }

func (r *server3Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}
func (r *server3Response) trailer(name []byte) (value []byte, ok bool) { return r.trailer3(name) }

func (r *server3Response) proxyPass1xx(resp backendResponse) bool {
	resp.delHopHeaders()
	r.status = resp.Status()
	if !resp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.hash, name, value)
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse(Version3)
	return false
}
func (r *server3Response) passHeaders() error       { return r.writeHeaders3() }
func (r *server3Response) passBytes(p []byte) error { return r.passBytes3(p) }

func (r *server3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			clock := r.stream.(*server3Stream).conn.gate.server.stage.clock
			r.fieldsEdge += uint16(clock.writeDate3(r.fields[r.fieldsEdge:]))
		}
	*/
}
func (r *server3Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *server3Response) addedHeaders() []byte { return nil }
func (r *server3Response) fixedHeaders() []byte { return nil }

// poolServer3Socket
var poolServer3Socket sync.Pool

func getServer3Socket(stream *server3Stream) *server3Socket {
	return nil
}
func putServer3Socket(socket *server3Socket) {
}

// server3Socket is the server-side HTTP/3 websocket.
type server3Socket struct {
	// Parent
	serverSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server3Socket) onUse() {
	s.serverSocket_.onUse()
}
func (s *server3Socket) onEnd() {
	s.serverSocket_.onEnd()
}

// HTTP3Backend
type HTTP3Backend struct {
	// Parent
	httpBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) onCreate(name string, stage *Stage) {
	b.httpBackend_.OnCreate(name, stage)
}

func (b *HTTP3Backend) OnConfigure() {
	b.httpBackend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *HTTP3Backend) OnPrepare() {
	b.httpBackend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *HTTP3Backend) CreateNode(name string) Node {
	node := new(http3Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP3Backend) FetchStream() (backendStream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP3Backend) StoreStream(stream backendStream) {
	stream3 := stream.(*backend3Stream)
	stream3.conn.node.storeStream(stream3)
}

// http3Node
type http3Node struct {
	// Parent
	httpNode_
	// Assocs
	backend *HTTP3Backend
	// States
}

func (n *http3Node) onCreate(name string, backend *HTTP3Backend) {
	n.httpNode_.OnCreate(name)
	n.backend = backend
}

func (n *http3Node) OnConfigure() {
	n.httpNode_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
	}

	// keepAliveConns
	n.ConfigureInt32("keepAliveConns", &n.keepAliveConns, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad keepAliveConns in node")
	}, 10)
}
func (n *http3Node) OnPrepare() {
	n.httpNode_.OnPrepare()
}

func (n *http3Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http3Node=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *http3Node) fetchStream() (*backend3Stream, error) {
	// TODO
	return nil, nil
}
func (n *http3Node) storeStream(stream *backend3Stream) {
	// TODO
}

// poolBackend3Conn is the backend-side HTTP/3 connection pool.
var poolBackend3Conn sync.Pool

func getBackend3Conn(id int64, node *http3Node, quicConn *quic.Conn) *backend3Conn {
	var backendConn *backend3Conn
	if x := poolBackend3Conn.Get(); x == nil {
		backendConn = new(backend3Conn)
	} else {
		backendConn = x.(*backend3Conn)
	}
	backendConn.onGet(id, node, quicConn)
	return backendConn
}
func putBackend3Conn(backendConn *backend3Conn) {
	backendConn.onPut()
	poolBackend3Conn.Put(backendConn)
}

// backend3Conn
type backend3Conn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64 // the conn id
	node     *http3Node
	quicConn *quic.Conn // the underlying quic connection
	expire   time.Time  // when the conn is considered expired
	// Conn states (zeros)
	usedStreams   atomic.Int32                           // accumulated num of streams served or fired
	broken        atomic.Bool                            // is conn broken?
	counter       atomic.Int64                           // can be used to generate a random number
	lastWrite     time.Time                              // deadline of last write operation
	lastRead      time.Time                              // deadline of last read operation
	nStreams      atomic.Int32                           // concurrent streams
	streams       [http3MaxActiveStreams]*backend3Stream // active (open, remoteClosed, localClosed) streams
	backend3Conn0                                        // all values must be zero by default in this struct!
}
type backend3Conn0 struct { // for fast reset, entirely
}

func (c *backend3Conn) onGet(id int64, node *http3Node, quicConn *quic.Conn) {
	c.id = id
	c.node = node
	c.quicConn = quicConn
	c.expire = time.Now().Add(node.backend.aliveTimeout)
}
func (c *backend3Conn) onPut() {
	c.quicConn = nil
	c.nStreams.Store(0)
	c.streams = [http3MaxActiveStreams]*backend3Stream{}
	c.backend3Conn0 = backend3Conn0{}
	c.node = nil
	c.expire = time.Time{}
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *backend3Conn) IsTLS() bool { return c.node.IsTLS() }
func (c *backend3Conn) IsUDS() bool { return c.node.IsUDS() }

func (c *backend3Conn) ID() int64 { return c.id }

func (c *backend3Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.node.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *backend3Conn) runOut() bool {
	return c.usedStreams.Add(1) > c.node.backend.MaxStreamsPerConn()
}

func (c *backend3Conn) markBroken()    { c.broken.Store(true) }
func (c *backend3Conn) isBroken() bool { return c.broken.Load() }

func (c *backend3Conn) fetchStream() (*backend3Stream, error) {
	// Note: A backend3Conn can be used concurrently, limited by maxStreams.
	// TODO: stream.onUse()
	return nil, nil
}
func (c *backend3Conn) storeStream(stream *backend3Stream) {
	// Note: A backend3Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *backend3Conn) Close() error {
	quicConn := c.quicConn
	putBackend3Conn(c)
	return quicConn.Close()
}

// poolBackend3Stream
var poolBackend3Stream sync.Pool

func getBackend3Stream(conn *backend3Conn, quicStream *quic.Stream) *backend3Stream {
	var stream *backend3Stream
	if x := poolBackend3Stream.Get(); x == nil {
		stream = new(backend3Stream)
		req, resp := &stream.request, &stream.response
		req.message = req
		req.stream = stream
		req.response = resp
		resp.message = resp
		resp.stream = stream
	} else {
		stream = x.(*backend3Stream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putBackend3Stream(stream *backend3Stream) {
	stream.onEnd()
	poolBackend3Stream.Put(stream)
}

// backend3Stream
type backend3Stream struct {
	// Assocs
	request  backend3Request
	response backend3Response
	socket   *backend3Socket
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *backend3Conn
	quicStream *quic.Stream // the underlying quic stream
	region     Region       // a region-based memory pool
	// Stream states (zeros)
}

func (s *backend3Stream) onUse(conn *backend3Conn, quicStream *quic.Stream) { // for non-zeros
	s.region.Init()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *backend3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s.quicStream = nil
	s.region.Free()
}

func (s *backend3Stream) Request() backendRequest   { return &s.request }
func (s *backend3Stream) Response() backendResponse { return &s.response }
func (s *backend3Stream) Socket() backendSocket     { return nil } // TODO

func (s *backend3Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *backend3Stream) ExecuteSocket() error { // see RFC 9220
	// TODO
	return nil
}

func (s *backend3Stream) Serend() httpSerend   { return s.conn.node.backend }
func (s *backend3Stream) Conn() httpConn       { return s.conn }
func (s *backend3Stream) remoteAddr() net.Addr { return nil } // TODO

func (s *backend3Stream) markBroken()    {}               // TODO
func (s *backend3Stream) isBroken() bool { return false } // TODO

func (s *backend3Stream) setWriteDeadline() error { // for content i/o only?
	// TODO
	return nil
}
func (s *backend3Stream) setReadDeadline() error { // for content i/o only?
	// TODO
	return nil
}

func (s *backend3Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *backend3Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *backend3Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *backend3Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (s *backend3Stream) buffer256() []byte          { return s.stockBuffer[:] }
func (s *backend3Stream) unsafeMake(size int) []byte { return s.region.Make(size) }

// backend3Request is the backend-side HTTP/3 request.
type backend3Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend3Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *backend3Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *backend3Request) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *backend3Request) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *backend3Request) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *backend3Request) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *backend3Request) delHeaderAt(i uint8)                        { r.delHeaderAt3(i) }

func (r *backend3Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *backend3Request) proxyCopyCookies(req Request) bool { // DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *backend3Request) sendChain() error { return r.sendChain3() }

func (r *backend3Request) echoHeaders() error { return r.writeHeaders3() }
func (r *backend3Request) echoChain() error   { return r.echoChain3() }

func (r *backend3Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}
func (r *backend3Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer3(name) }

func (r *backend3Request) passHeaders() error       { return r.writeHeaders3() }
func (r *backend3Request) passBytes(p []byte) error { return r.passBytes3(p) }

func (r *backend3Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *backend3Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *backend3Request) addedHeaders() []byte { return nil } // TODO
func (r *backend3Request) fixedHeaders() []byte { return nil } // TODO

// backend3Response is the backend-side HTTP/3 response.
type backend3Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend3Response) recvHead() {
	// TODO
}

func (r *backend3Response) readContent() (p []byte, err error) { return r.readContent3() }

// poolBackend3Socket
var poolBackend3Socket sync.Pool

func getBackend3Socket(stream *backend3Stream) *backend3Socket {
	return nil
}
func putBackend3Socket(socket *backend3Socket) {
}

// backend3Socket is the backend-side HTTP/3 websocket.
type backend3Socket struct {
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *backend3Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *backend3Socket) onEnd() {
	s.backendSocket_.onEnd()
}
