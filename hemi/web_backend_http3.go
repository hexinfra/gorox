// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 backend implementation. See RFC 9114 and 9204.

// Server Push is not supported.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quic"
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
	webBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) CreateNode(name string) Node {
	node := new(http3Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP3Backend) FetchStream() (BackendStream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP3Backend) StoreStream(stream BackendStream) {
	node := stream.webConn().(*Backend3Conn).http3Node()
	node.storeStream(stream)
}

// http3Node
type http3Node struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *http3Node) onCreate(name string, backend *HTTP3Backend) {
	n.Node_.OnCreate(name, backend)
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
	n.backend.DecSub()
}

func (n *http3Node) fetchStream() (BackendStream, error) {
	// TODO
	return nil, nil
}
func (n *http3Node) storeStream(stream BackendStream) {
	// TODO
}

/*
func (n *http3Node) fetchConn() (*Backend3Conn, error) {
	// TODO: dynamic address names?
	// TODO
	conn, err := quic.DialTimeout(n.address, n.backend.DialTimeout())
	if err != nil {
		return nil, err
	}
	connID := n.backend.nextConnID()
	return getBackend3Conn(connID, n, conn), nil
}
func (n *http3Node) storeConn(conn *Backend3Conn) {
	// TODO
}
*/

// poolBackend3Conn is the backend-side HTTP/3 connection pool.
var poolBackend3Conn sync.Pool

func getBackend3Conn(id int64, node *http3Node, quicConn *quic.Conn) *Backend3Conn {
	var conn *Backend3Conn
	if x := poolBackend3Conn.Get(); x == nil {
		conn = new(Backend3Conn)
	} else {
		conn = x.(*Backend3Conn)
	}
	conn.onGet(id, node, quicConn)
	return conn
}
func putBackend3Conn(conn *Backend3Conn) {
	conn.onPut()
	poolBackend3Conn.Put(conn)
}

// Backend3Conn
type Backend3Conn struct {
	// Parent
	BackendConn_
	// Mixins
	_webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *quic.Conn // the underlying quic connection
	// Conn states (zeros)
	nStreams      atomic.Int32                           // concurrent streams
	streams       [http3MaxActiveStreams]*Backend3Stream // active (open, remoteClosed, localClosed) streams
	backend3Conn0                                        // all values must be zero by default in this struct!
}
type backend3Conn0 struct { // for fast reset, entirely
}

func (c *Backend3Conn) onGet(id int64, node *http3Node, quicConn *quic.Conn) {
	c.BackendConn_.OnGet(id, node)
	c._webConn_.onGet()
	c.quicConn = quicConn
}
func (c *Backend3Conn) onPut() {
	c.quicConn = nil
	c.nStreams.Store(0)
	c.streams = [http3MaxActiveStreams]*Backend3Stream{}
	c.backend3Conn0 = backend3Conn0{}
	c._webConn_.onPut()
	c.BackendConn_.OnPut()
}

func (c *Backend3Conn) WebBackend() WebBackend { return c.Backend().(WebBackend) }
func (c *Backend3Conn) http3Node() *http3Node  { return c.Node().(*http3Node) }

func (c *Backend3Conn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.WebBackend().MaxStreamsPerConn()
}

func (c *Backend3Conn) fetchStream() (BackendStream, error) {
	// Note: A Backend3Conn can be used concurrently, limited by maxStreams.
	// TODO: stream.onUse()
	return nil, nil
}
func (c *Backend3Conn) storeStream(stream BackendStream) {
	// Note: A Backend3Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *Backend3Conn) Close() error {
	quicConn := c.quicConn
	putBackend3Conn(c)
	return quicConn.Close()
}

// poolBackend3Stream
var poolBackend3Stream sync.Pool

func getBackend3Stream(conn *Backend3Conn, quicStream *quic.Stream) *Backend3Stream {
	var stream *Backend3Stream
	if x := poolBackend3Stream.Get(); x == nil {
		stream = new(Backend3Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*Backend3Stream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putBackend3Stream(stream *Backend3Stream) {
	stream.onEnd()
	poolBackend3Stream.Put(stream)
}

// Backend3Stream
type Backend3Stream struct {
	// Mixins
	_webStream_
	// Assocs
	request  Backend3Request
	response Backend3Response
	socket   *Backend3Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *Backend3Conn
	quicStream *quic.Stream // the underlying quic stream
	// Stream states (zeros)
}

func (s *Backend3Stream) onUse(conn *Backend3Conn, quicStream *quic.Stream) { // for non-zeros
	s._webStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *Backend3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s.quicStream = nil
	s._webStream_.onEnd()
}

func (s *Backend3Stream) Request() BackendRequest   { return &s.request }
func (s *Backend3Stream) Response() BackendResponse { return &s.response }
func (s *Backend3Stream) Socket() BackendSocket     { return nil } // TODO

func (s *Backend3Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *Backend3Stream) ExecuteSocket() error { // see RFC 9220
	// TODO
	return nil
}

func (s *Backend3Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *Backend3Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *Backend3Stream) isBroken() bool { return false } // TODO
func (s *Backend3Stream) markBroken()    {}               // TODO

func (s *Backend3Stream) webKeeper() webKeeper { return s.conn.WebBackend() }
func (s *Backend3Stream) webConn() webConn     { return s.conn }
func (s *Backend3Stream) remoteAddr() net.Addr { return nil } // TODO

func (s *Backend3Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *Backend3Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *Backend3Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *Backend3Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

// Backend3Request is the backend-side HTTP/3 request.
type Backend3Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *Backend3Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *Backend3Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *Backend3Request) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *Backend3Request) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *Backend3Request) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *Backend3Request) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *Backend3Request) delHeaderAt(i uint8)                        { r.delHeaderAt3(i) }

func (r *Backend3Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *Backend3Request) proxyCopyCookies(req Request) bool { // DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *Backend3Request) sendChain() error { return r.sendChain3() }

func (r *Backend3Request) echoHeaders() error { return r.writeHeaders3() }
func (r *Backend3Request) echoChain() error   { return r.echoChain3() }

func (r *Backend3Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}
func (r *Backend3Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer3(name) }

func (r *Backend3Request) passHeaders() error       { return r.writeHeaders3() }
func (r *Backend3Request) passBytes(p []byte) error { return r.passBytes3(p) }

func (r *Backend3Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *Backend3Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *Backend3Request) addedHeaders() []byte { return nil } // TODO
func (r *Backend3Request) fixedHeaders() []byte { return nil } // TODO

// Backend3Response is the backend-side HTTP/3 response.
type Backend3Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *Backend3Response) recvHead() {
	// TODO
}

func (r *Backend3Response) readContent() (p []byte, err error) { return r.readContent3() }

// poolBackend3Socket
var poolBackend3Socket sync.Pool

func getBackend3Socket(stream *Backend3Stream) *Backend3Socket {
	return nil
}
func putBackend3Socket(socket *Backend3Socket) {
}

// Backend3Socket is the backend-side HTTP/3 websocket.
type Backend3Socket struct {
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *Backend3Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *Backend3Socket) onEnd() {
	s.backendSocket_.onEnd()
}
