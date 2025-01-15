// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 server and backend implementation. See RFC 9114 and RFC 9204.

// Server Push is not supported because it's rarely used. Chrome and Firefox even removed it.

package hemi

import (
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
)

//////////////////////////////////////// HTTP/3 general implementation ////////////////////////////////////////

// http3Conn collects shared methods between *server3Conn and *backend3Conn.
type http3Conn interface {
	// Imports
	httpConn
	// Methods
}

// http3Conn_ is the parent for server3Conn and backend3Conn.
type http3Conn_ struct {
	// Parent
	httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *tcp2.Conn        // the quic connection
	inBuffer *http3InBuffer    // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	activeStreams [http3MaxConcurrentStreams]http3Stream // active (open, remoteClosed, localClosed) streams
	_http3Conn0                                          // all values in this struct must be zero by default!
}
type _http3Conn0 struct { // for fast reset, entirely
	inBufferEdge uint32 // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	partBack     uint32 // incoming frame part (header or payload) begins from c.inBuffer.buf[c.partBack]
	partFore     uint32 // incoming frame part (header or payload) ends at c.inBuffer.buf[c.partFore]
}

func (c *http3Conn_) onGet(id int64, stage *Stage, udsMode bool, tlsMode bool, quicConn *tcp2.Conn, readTimeout time.Duration, writeTimeout time.Duration) {
	c.httpConn_.onGet(id, stage, udsMode, tlsMode, readTimeout, writeTimeout)

	c.quicConn = quicConn
	if c.inBuffer == nil {
		c.inBuffer = getHTTP3InBuffer()
		c.inBuffer.incRef()
	}
}
func (c *http3Conn_) onPut() {
	// c.inBuffer is reserved
	// c.table is reserved
	c.activeStreams = [http3MaxConcurrentStreams]http3Stream{}
	c.quicConn = nil

	c.httpConn_.onPut()
}

func (c *http3Conn_) remoteAddr() net.Addr { return nil } // TODO

// http3Stream collects shared methods between *server3Stream and *backend3Stream.
type http3Stream interface {
	// Imports
	httpStream
	// Methods
}

// http3Stream_ is the parent for server3Stream and backend3Stream.
type http3Stream_[C http3Conn] struct {
	// Parent
	httpStream_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       C            // the http/3 connection
	quicStream *tcp2.Stream // the quic stream
	// Stream states (zeros)
	_http3Stream0 // all values in this struct must be zero by default!
}
type _http3Stream0 struct { // for fast reset, entirely
}

func (s *http3Stream_[C]) onUse(conn C, quicStream *tcp2.Stream) {
	s.httpStream_.onUse()

	s.conn = conn
	s.quicStream = quicStream
}
func (s *http3Stream_[C]) onEnd() {
	s._http3Stream0 = _http3Stream0{}

	// s.conn will be set as nil by upper code
	s.quicStream = nil
	s.httpStream_.onEnd()
}

func (s *http3Stream_[C]) Conn() httpConn       { return s.conn }
func (s *http3Stream_[C]) remoteAddr() net.Addr { return s.conn.remoteAddr() }

func (s *http3Stream_[C]) markBroken()    {}               // TODO
func (s *http3Stream_[C]) isBroken() bool { return false } // TODO

func (s *http3Stream_[C]) setReadDeadline() error {
	// TODO
	return nil
}
func (s *http3Stream_[C]) setWriteDeadline() error {
	// TODO
	return nil
}

func (s *http3Stream_[C]) read(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) readFull(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) write(src []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) writev(srcVec *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

//////////////////////////////////////// HTTP/3 incoming implementation ////////////////////////////////////////

func (r *httpIn_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *httpIn_) readContent3() (data []byte, err error) {
	// TODO
	return
}

// http3InFrame is the HTTP/3 incoming frame.
type http3InFrame struct {
	// TODO
}

func (f *http3InFrame) zero() { *f = http3InFrame{} }

// http3InBuffer
type http3InBuffer struct {
	buf [_16K]byte // header + payload
	ref atomic.Int32
}

var poolHTTP3InBuffer sync.Pool

func getHTTP3InBuffer() *http3InBuffer {
	var inBuffer *http3InBuffer
	if x := poolHTTP3InBuffer.Get(); x == nil {
		inBuffer = new(http3InBuffer)
	} else {
		inBuffer = x.(*http3InBuffer)
	}
	return inBuffer
}
func putHTTP3InBuffer(inBuffer *http3InBuffer) { poolHTTP3InBuffer.Put(inBuffer) }

func (b *http3InBuffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http3InBuffer) getRef() int32 { return b.ref.Load() }
func (b *http3InBuffer) incRef()       { b.ref.Add(1) }
func (b *http3InBuffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DebugLevel() >= 1 {
			Printf("putHTTP3InBuffer ref=%d\n", b.ref.Load())
		}
		putHTTP3InBuffer(b)
	}
}

//////////////////////////////////////// HTTP/3 outgoing implementation ////////////////////////////////////////

func (r *httpOut_) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *httpOut_) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *httpOut_) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *httpOut_) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *httpOut_) delHeaderAt3(i uint8) {
	// TODO
}

func (r *httpOut_) sendChain3() error {
	// TODO
	return nil
}
func (r *httpOut_) _sendEntireChain3() error {
	// TODO
	return nil
}
func (r *httpOut_) _sendSingleRange3() error {
	// TODO
	return nil
}
func (r *httpOut_) _sendMultiRanges3() error {
	// TODO
	return nil
}

func (r *httpOut_) echoChain3() error {
	// TODO
	return nil
}

func (r *httpOut_) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *httpOut_) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *httpOut_) trailers3() []byte {
	// TODO
	return nil
}

func (r *httpOut_) proxyPassBytes3(data []byte) error { return r.writeBytes3(data) }

func (r *httpOut_) finalizeVague3() error {
	// TODO
	if r.numTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *httpOut_) writeHeaders3() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *httpOut_) writePiece3(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *httpOut_) _writeTextPiece3(piece *Piece) error {
	// TODO
	return nil
}
func (r *httpOut_) _writeFilePiece3(piece *Piece) error {
	// TODO
	return nil
}
func (r *httpOut_) writeVector3() error {
	// TODO
	return nil
}
func (r *httpOut_) writeBytes3(data []byte) error {
	// TODO
	return nil
}

// http3OutFrame is the HTTP/3 outgoing frame.
type http3OutFrame struct {
	// TODO
}

func (f *http3OutFrame) zero() { *f = http3OutFrame{} }

//////////////////////////////////////// HTTP/3 socket implementation ////////////////////////////////////////

func (s *httpSocket_) todo3() {
}

//////////////////////////////////////// HTTP/3 server implementation ////////////////////////////////////////

func init() {
	RegisterServer("http3Server", func(compName string, stage *Stage) Server {
		s := new(http3Server)
		s.onCreate(compName, stage)
		return s
	})
}

// http3Server is the HTTP/3 server. An http3Server has many http3Gates.
type http3Server struct {
	// Parent
	httpServer_[*http3Gate]
	// States
}

func (s *http3Server) onCreate(compName string, stage *Stage) {
	s.httpServer_.onCreate(compName, stage)
	s.tlsConfig = new(tls.Config) // currently tls mode is always enabled in http/3
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
		gate.onNew(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub() // gate
		if s.UDSMode() {
			go gate.serveUDS()
		} else {
			go gate.serveTLS()
		}
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("http3Server=%s done\n", s.CompName())
	}
	s.stage.DecSub() // server
}

// http3Gate is a gate of http3Server.
type http3Gate struct {
	// Parent
	httpGate_[*http3Server]
	// States
	listener *tcp2.Listener // the real gate. set after open
}

func (g *http3Gate) onNew(server *http3Server, id int32) {
	g.httpGate_.onNew(server, id)
}

func (g *http3Gate) Open() error {
	listener := tcp2.NewListener(g.Address())
	if err := listener.Open(); err != nil {
		return err
	}
	g.listener = listener
	return nil
}
func (g *http3Gate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *http3Gate) serveUDS() { // runner
	// TODO
}
func (g *http3Gate) serveTLS() { // runner
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
		g.IncSub() // conn
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			g.justClose(quicConn)
			continue
		}
		server3Conn := getServer3Conn(connID, g, quicConn)
		go server3Conn.manager() // server3Conn is put to pool in manager()
		connID++
	}
	g.WaitSubs() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http3Gate=%d done\n", g.id)
	}
	g.server.DecSub() // gate
}

func (g *http3Gate) justClose(quicConn *tcp2.Conn) {
	quicConn.Close()
	g.DecConcurrentConns()
	g.DecSub() // conn
}

// server3Conn is the server-side HTTP/3 connection.
type server3Conn struct {
	// Parent
	http3Conn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *http3Gate
	// Conn states (zeros)
	_server3Conn0 // all values in this struct must be zero by default!
}
type _server3Conn0 struct { // for fast reset, entirely
}

var poolServer3Conn sync.Pool

func getServer3Conn(id int64, gate *http3Gate, quicConn *tcp2.Conn) *server3Conn {
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

func (c *server3Conn) onGet(id int64, gate *http3Gate, quicConn *tcp2.Conn) {
	c.http3Conn_.onGet(id, gate.Stage(), gate.UDSMode(), gate.TLSMode(), quicConn, gate.ReadTimeout(), gate.WriteTimeout())

	c.gate = gate
}
func (c *server3Conn) onPut() {
	c._server3Conn0 = _server3Conn0{}
	c.gate = nil

	c.http3Conn_.onPut()
}

func (c *server3Conn) manager() { // runner
	// TODO
	// use go c.receiver()?
}

func (c *server3Conn) receiver() { // runner
	// TODO
}

func (c *server3Conn) closeConn() {
	c.quicConn.Close()
	c.gate.DecConcurrentConns()
	c.gate.DecSub()
}

// server3Stream is the server-side HTTP/3 stream.
type server3Stream struct {
	// Parent
	http3Stream_[*server3Conn]
	// Assocs
	request  server3Request  // the http/3 request.
	response server3Response // the http/3 response.
	socket   *server3Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_server3Stream0 // all values in this struct must be zero by default!
}
type _server3Stream0 struct { // for fast reset, entirely
	index uint8
	state uint8
	reset bool
}

var poolServer3Stream sync.Pool

func getServer3Stream(conn *server3Conn, quicStream *tcp2.Stream) *server3Stream {
	var serverStream *server3Stream
	if x := poolServer3Stream.Get(); x == nil {
		serverStream = new(server3Stream)
		req, resp := &serverStream.request, &serverStream.response
		req.stream = serverStream
		req.inMessage = req
		resp.stream = serverStream
		resp.outMessage = resp
		resp.request = req
	} else {
		serverStream = x.(*server3Stream)
	}
	serverStream.onUse(conn, quicStream)
	return serverStream
}
func putServer3Stream(serverStream *server3Stream) {
	serverStream.onEnd()
	poolServer3Stream.Put(serverStream)
}

func (s *server3Stream) onUse(conn *server3Conn, quicStream *tcp2.Stream) { // for non-zeros
	s.http3Stream_.onUse(conn, quicStream)

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
	s._server3Stream0 = _server3Stream0{}

	s.http3Stream_.onEnd()
	s.conn = nil // we can't do this in http3Stream_.onEnd() due to Go's limit, so put here
}

func (s *server3Stream) Holder() httpHolder { return s.conn.gate }

func (s *server3Stream) execute() { // runner
	// TODO ...
	putServer3Stream(s)
}
func (s *server3Stream) _serveAbnormal(req *server3Request, resp *server3Response) { // 4xx & 5xx
	// TODO
}
func (s *server3Stream) _writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *server3Stream) executeExchan(webapp *Webapp, req *server3Request, resp *server3Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server3Stream) executeSocket() { // see RFC 9220
	// TODO
}

// server3Request is the server-side HTTP/3 request.
type server3Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Request) readContent() (data []byte, err error) { return r.readContent3() }

// server3Response is the server-side HTTP/3 response.
type server3Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Response) control() []byte { // :status NNN
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

func (r *server3Response) proxyPass1xx(backResp backendResponse) bool {
	backResp.proxyDelHopHeaders()
	r.status = backResp.Status()
	if !backResp.proxyWalkHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.nameHash, name, value) // some headers are restricted
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse(Version3)
	return false
}
func (r *server3Response) proxyPassHeaders() error          { return r.writeHeaders3() }
func (r *server3Response) proxyPassBytes(data []byte) error { return r.proxyPassBytes3(data) }

func (r *server3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			clock := r.stream.(*server3Stream).conn.gate.stage.clock
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

// server3Socket is the server-side HTTP/3 webSocket.
type server3Socket struct { // incoming and outgoing
	// Parent
	serverSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolServer3Socket sync.Pool

func getServer3Socket(stream *server3Stream) *server3Socket {
	// TODO
	return nil
}
func putServer3Socket(socket *server3Socket) {
	// TODO
}

func (s *server3Socket) onUse() {
	s.serverSocket_.onUse()
}
func (s *server3Socket) onEnd() {
	s.serverSocket_.onEnd()
}

//////////////////////////////////////// HTTP/3 backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("http3Backend", func(compName string, stage *Stage) Backend {
		b := new(HTTP3Backend)
		b.onCreate(compName, stage)
		return b
	})
}

// HTTP3Backend
type HTTP3Backend struct {
	// Parent
	httpBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) onCreate(compName string, stage *Stage) {
	b.httpBackend_.OnCreate(compName, stage)
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

func (b *HTTP3Backend) CreateNode(compName string) Node {
	node := new(http3Node)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *HTTP3Backend) FetchStream() (backendStream, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.fetchStream()
}
func (b *HTTP3Backend) StoreStream(stream backendStream) {
	stream3 := stream.(*backend3Stream)
	stream3.conn.node.storeStream(stream3)
}

// http3Node
type http3Node struct {
	// Parent
	httpNode_[*HTTP3Backend]
	// States
}

func (n *http3Node) onCreate(compName string, stage *Stage, backend *HTTP3Backend) {
	n.httpNode_.onCreate(compName, stage, backend)
}

func (n *http3Node) OnConfigure() {
	n.httpNode_.onConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
	}
}
func (n *http3Node) OnPrepare() {
	n.httpNode_.onPrepare()
}

func (n *http3Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http3Node=%s done\n", n.compName)
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

// backend3Conn
type backend3Conn struct {
	// Parent
	http3Conn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node       *http3Node
	expireTime time.Time // when the conn is considered expired
	// Conn states (zeros)
	_backend3Conn0 // all values in this struct must be zero by default!
}
type _backend3Conn0 struct { // for fast reset, entirely
}

var poolBackend3Conn sync.Pool

func getBackend3Conn(id int64, node *http3Node, quicConn *tcp2.Conn) *backend3Conn {
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

func (c *backend3Conn) onGet(id int64, node *http3Node, quicConn *tcp2.Conn) {
	c.http3Conn_.onGet(id, node.Stage(), node.UDSMode(), node.TLSMode(), quicConn, node.ReadTimeout(), node.WriteTimeout())

	c.node = node
	c.expireTime = time.Now().Add(node.idleTimeout)
}
func (c *backend3Conn) onPut() {
	c._backend3Conn0 = _backend3Conn0{}
	c.expireTime = time.Time{}
	c.node = nil

	c.http3Conn_.onPut()
}

func (c *backend3Conn) ranOut() bool {
	return c.cumulativeStreams.Add(1) > c.node.MaxCumulativeStreamsPerConn()
}
func (c *backend3Conn) fetchStream() (*backend3Stream, error) {
	// Note: A backend3Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO: stream.onUse()
	return nil, nil
}
func (c *backend3Conn) storeStream(stream *backend3Stream) {
	// Note: A backend3Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	//stream.onEnd()
}

func (c *backend3Conn) Close() error {
	quicConn := c.quicConn
	putBackend3Conn(c)
	return quicConn.Close()
}

// backend3Stream
type backend3Stream struct {
	// Parent
	http3Stream_[*backend3Conn]
	// Assocs
	request  backend3Request
	response backend3Response
	socket   *backend3Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backend3Stream0 // all values in this struct must be zero by default!
}
type _backend3Stream0 struct { // for fast reset, entirely
}

var poolBackend3Stream sync.Pool

func getBackend3Stream(conn *backend3Conn, quicStream *tcp2.Stream) *backend3Stream {
	var backendStream *backend3Stream
	if x := poolBackend3Stream.Get(); x == nil {
		backendStream = new(backend3Stream)
		req, resp := &backendStream.request, &backendStream.response
		req.stream = backendStream
		req.outMessage = req
		req.response = resp
		resp.stream = backendStream
		resp.inMessage = resp
	} else {
		backendStream = x.(*backend3Stream)
	}
	backendStream.onUse(conn, quicStream)
	return backendStream
}
func putBackend3Stream(backendStream *backend3Stream) {
	backendStream.onEnd()
	poolBackend3Stream.Put(backendStream)
}

func (s *backend3Stream) onUse(conn *backend3Conn, quicStream *tcp2.Stream) { // for non-zeros
	s.http3Stream_.onUse(conn, quicStream)

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
	s._backend3Stream0 = _backend3Stream0{}

	s.http3Stream_.onEnd()
	s.conn = nil // we can't do this in http3Stream_.onEnd() due to Go's limit, so put here
}

func (s *backend3Stream) Holder() httpHolder { return s.conn.node }

func (s *backend3Stream) Request() backendRequest   { return &s.request }
func (s *backend3Stream) Response() backendResponse { return &s.response }
func (s *backend3Stream) Socket() backendSocket     { return nil } // TODO. See RFC 9220

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
func (r *backend3Request) proxySetAuthority(hostname []byte, colonport []byte) bool {
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
func (r *backend3Request) proxyCopyCookies(foreReq Request) bool { // NOTE: DO NOT merge into one "cookie" header!
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

func (r *backend3Request) proxyPassHeaders() error          { return r.writeHeaders3() }
func (r *backend3Request) proxyPassBytes(data []byte) error { return r.proxyPassBytes3(data) }

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

func (r *backend3Response) readContent() (data []byte, err error) { return r.readContent3() }

// backend3Socket is the backend-side HTTP/3 webSocket.
type backend3Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolBackend3Socket sync.Pool

func getBackend3Socket(stream *backend3Stream) *backend3Socket {
	// TODO
	return nil
}
func putBackend3Socket(socket *backend3Socket) {
	// TODO
}

func (s *backend3Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *backend3Socket) onEnd() {
	s.backendSocket_.onEnd()
}

//////////////////////////////////////// HTTP/3 protocol elements ////////////////////////////////////////

const ( // HTTP/3 sizes and limits for both of our HTTP/3 server and HTTP/3 backend
	http3MaxTableSize         = _4K
	http3MaxConcurrentStreams = 127 // currently hardcoded
)

// http3StaticTable
var http3StaticTable = [99]pair{ // TODO
	/*
		0:  {1059, placeStatic3, 10, 0, span{0, 0}},
		1:  {487, placeStatic3, 5, 10, span{15, 16}},
		2:  {301, placeStatic3, 3, 16, span{19, 20}},
		3:  {2013, placeStatic3, 19, 20, span{0, 0}},
		4:  {1450, placeStatic3, 14, 39, span{53, 54}},
		5:  {634, placeStatic3, 6, 54, span{0, 0}},
		6:  {414, placeStatic3, 4, 60, span{0, 0}},
		7:  {417, placeStatic3, 4, 64, span{0, 0}},
		8:  {1660, placeStatic3, 17, 68, span{0, 0}},
		9:  {1254, placeStatic3, 13, 85, span{0, 0}},
		10: {1314, placeStatic3, 13, 98, span{0, 0}},
		11: {430, placeStatic3, 4, 111, span{0, 0}},
		12: {857, placeStatic3, 8, 115, span{0, 0}},
		13: {747, placeStatic3, 7, 123, span{0, 0}},
		14: {1011, placeStatic3, 10, 130, span{0, 0}},
		15: {699, placeStatic3, 7, 140, span{147, 154}},
		16: {699, placeStatic3, 7, 140, span{154, 160}},
		17: {699, placeStatic3, 7, 140, span{160, 163}},
		18: {699, placeStatic3, 7, 140, span{163, 167}},
		19: {699, placeStatic3, 7, 140, span{167, 174}},
		20: {699, placeStatic3, 7, 140, span{174, 178}},
		21: {699, placeStatic3, 7, 140, span{178, 181}},
		22: {687, placeStatic3, 7, 181, span{188, 192}},
		23: {687, placeStatic3, 7, 181, span{192, 197}},
		24: {734, placeStatic3, 7, 197, span{204, 207}},
		25: {734, placeStatic3, 7, 197, span{207, 210}},
		26: {734, placeStatic3, 7, 197, span{210, 213}},
		27: {734, placeStatic3, 7, 197, span{213, 216}},
		28: {734, placeStatic3, 7, 197, span{216, 219}},
		29: {624, placeStatic3, 6, 219, span{225, 228}},
		30: {624, placeStatic3, 6, 219, span{228, 251}},
		31: {1508, placeStatic3, 15, 251, span{266, 283}},
		32: {1309, placeStatic3, 13, 283, span{296, 301}},
		33: {2805, placeStatic3, 28, 301, span{329, 342}},
		34: {2805, placeStatic3, 28, 301, span{342, 354}},
		35: {2721, placeStatic3, 27, 354, span{381, 382}},
		36: {1314, placeStatic3, 13, 382, span{395, 404}},
		37: {1314, placeStatic3, 13, 382, span{404, 419}},
		38: {1314, placeStatic3, 13, 382, span{419, 433}},
		39: {1314, placeStatic3, 13, 382, span{433, 441}},
		40: {1314, placeStatic3, 13, 382, span{441, 449}},
		41: {1314, placeStatic3, 13, 382, span{449, 473}},
		42: {1647, placeStatic3, 16, 473, span{489, 491}},
		43: {1647, placeStatic3, 16, 473, span{491, 495}},
		44: {1258, placeStatic3, 12, 495, span{507, 530}},
		45: {1258, placeStatic3, 12, 495, span{530, 552}},
		46: {1258, placeStatic3, 12, 495, span{552, 568}},
		47: {1258, placeStatic3, 12, 495, span{568, 601}},
		48: {1258, placeStatic3, 12, 495, span{601, 610}},
		49: {1258, placeStatic3, 12, 495, span{610, 620}},
		50: {1258, placeStatic3, 12, 495, span{620, 629}},
		51: {1258, placeStatic3, 12, 495, span{629, 637}},
		52: {1258, placeStatic3, 12, 495, span{637, 661}},
		53: {1258, placeStatic3, 12, 495, span{661, 671}},
		54: {1258, placeStatic3, 12, 495, span{671, 695}},
		55: {525, placeStatic3, 5, 695, span{700, 708}},
		56: {2648, placeStatic3, 25, 708, span{733, 749}},
		57: {2648, placeStatic3, 25, 708, span{749, 784}},
		58: {2648, placeStatic3, 25, 708, span{784, 828}},
		59: {450, placeStatic3, 4, 828, span{832, 847}},
		60: {450, placeStatic3, 4, 828, span{847, 853}},
		61: {2248, placeStatic3, 22, 853, span{875, 882}},
		62: {1655, placeStatic3, 16, 882, span{898, 911}},
		63: {734, placeStatic3, 7, 911, span{918, 921}},
		64: {734, placeStatic3, 7, 911, span{921, 924}},
		65: {734, placeStatic3, 7, 911, span{924, 927}},
		66: {734, placeStatic3, 7, 911, span{927, 930}},
		67: {734, placeStatic3, 7, 911, span{930, 933}},
		68: {734, placeStatic3, 7, 911, span{933, 936}},
		69: {734, placeStatic3, 7, 911, span{936, 939}},
		70: {734, placeStatic3, 7, 911, span{939, 942}},
		71: {734, placeStatic3, 7, 911, span{942, 945}},
		72: {1505, placeStatic3, 15, 945, span{0, 0}},
		73: {3239, placeStatic3, 32, 960, span{992, 997}},
		74: {3239, placeStatic3, 32, 960, span{997, 1001}},
		75: {2805, placeStatic3, 28, 1001, span{1029, 1030}},
		76: {2829, placeStatic3, 28, 1030, span{1058, 1061}},
		77: {2829, placeStatic3, 28, 1030, span{1061, 1079}},
		78: {2829, placeStatic3, 28, 1030, span{1079, 1086}},
		79: {2922, placeStatic3, 29, 1086, span{1115, 1129}},
		80: {3039, placeStatic3, 30, 1129, span{1159, 1171}},
		81: {2948, placeStatic3, 29, 1171, span{1200, 1203}},
		82: {2948, placeStatic3, 29, 1171, span{1203, 1207}},
		83: {698, placeStatic3, 7, 1207, span{1214, 1219}},
		84: {1425, placeStatic3, 13, 1219, span{0, 0}},
		85: {2397, placeStatic3, 23, 1232, span{1255, 1308}},
		86: {996, placeStatic3, 10, 1308, span{1318, 1319}},
		87: {909, placeStatic3, 9, 1319, span{0, 0}},
		88: {958, placeStatic3, 9, 1328, span{0, 0}},
		89: {777, placeStatic3, 8, 1337, span{0, 0}},
		90: {648, placeStatic3, 6, 1345, span{0, 0}},
		91: {782, placeStatic3, 7, 1351, span{1358, 1366}},
		92: {663, placeStatic3, 6, 1366, span{0, 0}},
		93: {1929, placeStatic3, 19, 1372, span{1391, 1392}},
		94: {2588, placeStatic3, 25, 1392, span{1417, 1418}},
		95: {1019, placeStatic3, 10, 1418, span{0, 0}},
		96: {1495, placeStatic3, 15, 1428, span{0, 0}},
		97: {1513, placeStatic3, 15, 1443, span{1458, 1462}},
		98: {1513, placeStatic3, 15, 1443, span{1462, 1472}},
	*/
}

// http3TableEntry is a dynamic table entry.
type http3TableEntry struct { // 8 bytes
	nameFrom  uint16
	nameEdge  uint16 // nameEdge - nameFrom <= 255?
	valueEdge uint16
	totalSize uint16 // nameSize + valueSize + 32
}

// http3DynamicTable
type http3DynamicTable struct {
	entries [124]http3TableEntry
	content [_4K]byte
}

var http3Template = [11]byte{':', 's', 't', 'a', 't', 'u', 's', ' ', 'x', 'x', 'x'}
var http3Controls = [...][]byte{ // size: 512*24B=12K. keep sync with http1Control and http2Control!
	// 1XX
	StatusContinue:           []byte(":status 100"),
	StatusSwitchingProtocols: []byte(":status 101"),
	StatusProcessing:         []byte(":status 102"),
	StatusEarlyHints:         []byte(":status 103"),
	// 2XX
	StatusOK:                         []byte(":status 200"),
	StatusCreated:                    []byte(":status 201"),
	StatusAccepted:                   []byte(":status 202"),
	StatusNonAuthoritativeInfomation: []byte(":status 203"),
	StatusNoContent:                  []byte(":status 204"),
	StatusResetContent:               []byte(":status 205"),
	StatusPartialContent:             []byte(":status 206"),
	StatusMultiStatus:                []byte(":status 207"),
	StatusAlreadyReported:            []byte(":status 208"),
	StatusIMUsed:                     []byte(":status 226"),
	// 3XX
	StatusMultipleChoices:   []byte(":status 300"),
	StatusMovedPermanently:  []byte(":status 301"),
	StatusFound:             []byte(":status 302"),
	StatusSeeOther:          []byte(":status 303"),
	StatusNotModified:       []byte(":status 304"),
	StatusUseProxy:          []byte(":status 305"),
	StatusTemporaryRedirect: []byte(":status 307"),
	StatusPermanentRedirect: []byte(":status 308"),
	// 4XX
	StatusBadRequest:                  []byte(":status 400"),
	StatusUnauthorized:                []byte(":status 401"),
	StatusPaymentRequired:             []byte(":status 402"),
	StatusForbidden:                   []byte(":status 403"),
	StatusNotFound:                    []byte(":status 404"),
	StatusMethodNotAllowed:            []byte(":status 405"),
	StatusNotAcceptable:               []byte(":status 406"),
	StatusProxyAuthenticationRequired: []byte(":status 407"),
	StatusRequestTimeout:              []byte(":status 408"),
	StatusConflict:                    []byte(":status 409"),
	StatusGone:                        []byte(":status 410"),
	StatusLengthRequired:              []byte(":status 411"),
	StatusPreconditionFailed:          []byte(":status 412"),
	StatusContentTooLarge:             []byte(":status 413"),
	StatusURITooLong:                  []byte(":status 414"),
	StatusUnsupportedMediaType:        []byte(":status 415"),
	StatusRangeNotSatisfiable:         []byte(":status 416"),
	StatusExpectationFailed:           []byte(":status 417"),
	StatusMisdirectedRequest:          []byte(":status 421"),
	StatusUnprocessableEntity:         []byte(":status 422"),
	StatusLocked:                      []byte(":status 423"),
	StatusFailedDependency:            []byte(":status 424"),
	StatusTooEarly:                    []byte(":status 425"),
	StatusUpgradeRequired:             []byte(":status 426"),
	StatusPreconditionRequired:        []byte(":status 428"),
	StatusTooManyRequests:             []byte(":status 429"),
	StatusRequestHeaderFieldsTooLarge: []byte(":status 431"),
	StatusUnavailableForLegalReasons:  []byte(":status 451"),
	// 5XX
	StatusInternalServerError:           []byte(":status 500"),
	StatusNotImplemented:                []byte(":status 501"),
	StatusBadGateway:                    []byte(":status 502"),
	StatusServiceUnavailable:            []byte(":status 503"),
	StatusGatewayTimeout:                []byte(":status 504"),
	StatusHTTPVersionNotSupported:       []byte(":status 505"),
	StatusVariantAlsoNegotiates:         []byte(":status 506"),
	StatusInsufficientStorage:           []byte(":status 507"),
	StatusLoopDetected:                  []byte(":status 508"),
	StatusNotExtended:                   []byte(":status 510"),
	StatusNetworkAuthenticationRequired: []byte(":status 511"),
}

var ( // HTTP/3 byteses
	http3BytesStatic = []byte(":authority:path/age0content-dispositioncontent-length0cookiedateetagif-modified-sinceif-none-matchlast-modifiedlinklocationrefererset-cookie:methodCONNECTDELETEGETHEADOPTIONSPOSTPUT:schemehttphttps:status103200304404503accept*/*application/dns-messageaccept-encodinggzip, deflate, braccept-rangesbytesaccess-control-allow-headerscache-controlcontent-typeaccess-control-allow-origin*cache-controlmax-age=0max-age=2592000max-age=604800no-cacheno-storepublic, max-age=31536000content-encodingbrgzipcontent-typeapplication/dns-messageapplication/javascriptapplication/jsonapplication/x-www-form-urlencodedimage/gifimage/jpegimage/pngtext/csstext/html; charset=utf-8text/plaintext/plain;charset=utf-8rangebytes=0-strict-transport-securitymax-age=31536000max-age=31536000; includesubdomainsmax-age=31536000; includesubdomains; preloadvaryaccept-encodingoriginx-content-type-optionsnosniffx-xss-protection1; mode=block:status100204206302400403421425500accept-languageaccess-control-allow-credentialsFALSETRUEaccess-control-allow-headers*access-control-allow-methodsgetget, post, optionsoptionsaccess-control-expose-headerscontent-lengthaccess-control-request-headerscontent-typeaccess-control-request-methodgetpostalt-svcclearauthorizationcontent-security-policyscript-src 'none'; object-src 'none'; base-uri 'none'early-data1expect-ctforwardedif-rangeoriginpurposeprefetchservertiming-allow-origin*upgrade-insecure-requests1user-agentx-forwarded-forx-frame-optionsdenysameorigin") // DO NOT CHANGE THIS UNLESS YOU KNOW WHAT YOU ARE DOING
)
