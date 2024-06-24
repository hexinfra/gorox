// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 backend implementation. See RFC 9114 and 9204.

// Server Push is not supported.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/quic"
)

func init() {
	RegisterBackend("http3Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP3Backend)
		b.onCreate(name, stage)
		return b
	})
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
	Node_
	// Assocs
	backend *HTTP3Backend
	// States
}

func (n *http3Node) onCreate(name string, backend *HTTP3Backend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *http3Node) OnConfigure() {
	n.Node_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
	}
}
func (n *http3Node) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *http3Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
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

/*
func (n *http3Node) fetchConn() (*backend3Conn, error) {
	// TODO: dynamic address names?
	// TODO
	conn, err := quic.DialTimeout(n.address, n.backend.DialTimeout())
	if err != nil {
		return nil, err
	}
	connID := n.backend.nextConnID()
	return getBackend3Conn(connID, n, conn), nil
}
func (n *http3Node) storeConn(conn *backend3Conn) {
	// TODO
}
*/

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

func (s *backend3Stream) servend() httpServend { return s.conn.node.backend }
func (s *backend3Stream) httpConn() httpConn   { return s.conn }
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
