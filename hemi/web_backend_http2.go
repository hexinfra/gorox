// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 backend implementation. See RFC 9113 and 7541.

// Server Push is not supported.

package hemi

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("http2Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP2Backend)
		b.onCreate(name, stage)
		return b
	})
}

// HTTP2Backend
type HTTP2Backend struct {
	// Parent
	webBackend_[*http2Node]
	// States
}

func (b *HTTP2Backend) CreateNode(name string) Node {
	node := new(http2Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP2Backend) FetchStream() (BackendStream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP2Backend) StoreStream(stream BackendStream) {
	node := stream.webConn().(*Backend2Conn).http2Node()
	node.storeStream(stream)
}

// http2Node
type http2Node struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *http2Node) onCreate(name string, backend *HTTP2Backend) {
	n.Node_.OnCreate(name, backend)
}

func (n *http2Node) OnConfigure() {
	n.Node_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"h2"}
	}
}
func (n *http2Node) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *http2Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http2Node=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *http2Node) fetchStream() (BackendStream, error) {
	// Note: A Backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *http2Node) storeStream(stream BackendStream) {
	// Note: A Backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

/*
func (n *http2Node) fetchConn() (*Backend2Conn, error) {
	// Note: A Backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getBackend2Conn(connID, n, netConn, rawConn), nil
}
func (n *http2Node) _dialTCP() (*Backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialTLS() (*Backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialUDS() (*Backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) storeConn(conn *Backend2Conn) {
	// Note: A Backend2Conn can be used concurrently, limited by maxStreams.
	// TODO: decRef
	if conn.nStreams.Add(-1) > 0 {
		return
	}
}
*/

// poolBackend2Conn is the backend-side HTTP/2 connection pool.
var poolBackend2Conn sync.Pool

func getBackend2Conn(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *Backend2Conn {
	var conn *Backend2Conn
	if x := poolBackend2Conn.Get(); x == nil {
		conn = new(Backend2Conn)
	} else {
		conn = x.(*Backend2Conn)
	}
	conn.onGet(id, node, netConn, rawConn)
	return conn
}
func putBackend2Conn(conn *Backend2Conn) {
	conn.onPut()
	poolBackend2Conn.Put(conn)
}

// Backend2Conn
type Backend2Conn struct {
	// Parent
	BackendConn_
	// Mixins
	_webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn net.Conn // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	nStreams      atomic.Int32                           // concurrent streams
	streams       [http2MaxActiveStreams]*Backend2Stream // active (open, remoteClosed, localClosed) streams
	backend2Conn0                                        // all values must be zero by default in this struct!
}
type backend2Conn0 struct { // for fast reset, entirely
}

func (c *Backend2Conn) onGet(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c._webConn_.onGet()
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *Backend2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.nStreams.Store(0)
	c.streams = [http2MaxActiveStreams]*Backend2Stream{}
	c.backend2Conn0 = backend2Conn0{}
	c._webConn_.onPut()
	c.BackendConn_.OnPut()
}

func (c *Backend2Conn) WebBackend() WebBackend { return c.Backend().(WebBackend) }
func (c *Backend2Conn) http2Node() *http2Node  { return c.Node().(*http2Node) }

func (c *Backend2Conn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.WebBackend().MaxStreamsPerConn()
}

func (c *Backend2Conn) fetchStream() (BackendStream, error) {
	// Note: A Backend2Conn can be used concurrently, limited by maxStreams.
	// TODO: incRef, stream.onUse()
	return nil, nil
}
func (c *Backend2Conn) storeStream(stream BackendStream) {
	// Note: A Backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *Backend2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *Backend2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *Backend2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *Backend2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}
func (c *Backend2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}

func (c *Backend2Conn) Close() error {
	netConn := c.netConn
	putBackend2Conn(c)
	return netConn.Close()
}

// poolBackend2Stream
var poolBackend2Stream sync.Pool

func getBackend2Stream(conn *Backend2Conn, id uint32) *Backend2Stream {
	var stream *Backend2Stream
	if x := poolBackend2Stream.Get(); x == nil {
		stream = new(Backend2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*Backend2Stream)
	}
	stream.onUse(conn, id)
	return stream
}
func putBackend2Stream(stream *Backend2Stream) {
	stream.onEnd()
	poolBackend2Stream.Put(stream)
}

// Backend2Stream
type Backend2Stream struct {
	// Mixins
	_webStream_
	// Assocs
	request  Backend2Request
	response Backend2Response
	socket   *Backend2Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *Backend2Conn
	id   uint32
	// Stream states (zeros)
}

func (s *Backend2Stream) onUse(conn *Backend2Conn, id uint32) { // for non-zeros
	s._webStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *Backend2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s._webStream_.onEnd()
}

func (s *Backend2Stream) Request() BackendRequest   { return &s.request }
func (s *Backend2Stream) Response() BackendResponse { return &s.response }
func (s *Backend2Stream) Socket() BackendSocket     { return nil } // TODO

func (s *Backend2Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *Backend2Stream) ExecuteSocket() error { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
	return nil
}

func (s *Backend2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *Backend2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *Backend2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *Backend2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

func (s *Backend2Stream) webKeeper() webKeeper { return s.conn.WebBackend() }
func (s *Backend2Stream) webConn() webConn     { return s.conn }
func (s *Backend2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *Backend2Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *Backend2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *Backend2Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *Backend2Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

// Backend2Request is the backend-side HTTP/2 request.
type Backend2Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *Backend2Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *Backend2Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *Backend2Request) addHeader(name []byte, value []byte) bool   { return r.addHeader2(name, value) }
func (r *Backend2Request) header(name []byte) (value []byte, ok bool) { return r.header2(name) }
func (r *Backend2Request) hasHeader(name []byte) bool                 { return r.hasHeader2(name) }
func (r *Backend2Request) delHeader(name []byte) (deleted bool)       { return r.delHeader2(name) }
func (r *Backend2Request) delHeaderAt(i uint8)                        { r.delHeaderAt2(i) }

func (r *Backend2Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *Backend2Request) proxyCopyCookies(req Request) bool { // DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *Backend2Request) sendChain() error { return r.sendChain2() }

func (r *Backend2Request) echoHeaders() error { return r.writeHeaders2() }
func (r *Backend2Request) echoChain() error   { return r.echoChain2() }

func (r *Backend2Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}
func (r *Backend2Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer2(name) }

func (r *Backend2Request) passHeaders() error       { return r.writeHeaders2() }
func (r *Backend2Request) passBytes(p []byte) error { return r.passBytes2(p) }

func (r *Backend2Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *Backend2Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *Backend2Request) addedHeaders() []byte { return nil } // TODO
func (r *Backend2Request) fixedHeaders() []byte { return nil } // TODO

// Backend2Response is the backend-side HTTP/2 response.
type Backend2Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *Backend2Response) recvHead() {
	// TODO
}

func (r *Backend2Response) readContent() (p []byte, err error) { return r.readContent2() }

// poolBackend2Socket
var poolBackend2Socket sync.Pool

func getBackend2Socket(stream *Backend2Stream) *Backend2Socket {
	return nil
}
func putBackend2Socket(socket *Backend2Socket) {
}

// Backend2Socket is the backend-side HTTP/2 websocket.
type Backend2Socket struct {
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *Backend2Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *Backend2Socket) onEnd() {
	s.backendSocket_.onEnd()
}
