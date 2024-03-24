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

func (b *HTTP2Backend) FetchStream() (WebBackendStream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP2Backend) StoreStream(stream WebBackendStream) {
	node := stream.webConn().(*H2Conn).webNode()
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
	if DbgLevel() >= 2 {
		Printf("http2Node=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *http2Node) fetchStream() (WebBackendStream, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *http2Node) storeStream(stream WebBackendStream) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

/*
func (n *http2Node) fetchConn() (*H2Conn, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getH2Conn(connID, n, netConn, rawConn), nil
}
func (n *http2Node) _dialTCP() (*H2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialTLS() (*H2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialUDS() (*H2Conn, error) {
	return nil, nil
}
func (n *http2Node) storeConn(conn *H2Conn) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO: decRef
	if conn.nStreams.Add(-1) > 0 {
		return
	}
}
*/

// poolH2Conn is the backend-side HTTP/2 connection pool.
var poolH2Conn sync.Pool

func getH2Conn(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *H2Conn {
	var h2Conn *H2Conn
	if x := poolH2Conn.Get(); x == nil {
		h2Conn = new(H2Conn)
	} else {
		h2Conn = x.(*H2Conn)
	}
	h2Conn.onGet(id, node, netConn, rawConn)
	return h2Conn
}
func putH2Conn(h2Conn *H2Conn) {
	h2Conn.onPut()
	poolH2Conn.Put(h2Conn)
}

// H2Conn
type H2Conn struct {
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
	nStreams atomic.Int32                     // concurrent streams
	streams  [http2MaxActiveStreams]*H2Stream // active (open, remoteClosed, localClosed) streams
	h2Conn0                                   // all values must be zero by default in this struct!
}
type h2Conn0 struct { // for fast reset, entirely
}

func (c *H2Conn) onGet(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c._webConn_.onGet()
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *H2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.nStreams.Store(0)
	c.streams = [http2MaxActiveStreams]*H2Stream{}
	c.h2Conn0 = h2Conn0{}
	c._webConn_.onPut()
	c.BackendConn_.OnPut()
}

func (c *H2Conn) WebBackend() WebBackend { return c.Backend().(WebBackend) }
func (c *H2Conn) webNode() *http2Node    { return c.Node().(*http2Node) }

func (c *H2Conn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.WebBackend().MaxStreamsPerConn()
}

func (c *H2Conn) fetchStream() (WebBackendStream, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO: incRef, stream.onUse()
	return nil, nil
}
func (c *H2Conn) storeStream(stream WebBackendStream) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *H2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *H2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *H2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *H2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}
func (c *H2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}

func (c *H2Conn) Close() error {
	netConn := c.netConn
	putH2Conn(c)
	return netConn.Close()
}

// poolH2Stream
var poolH2Stream sync.Pool

func getH2Stream(conn *H2Conn, id uint32) *H2Stream {
	var stream *H2Stream
	if x := poolH2Stream.Get(); x == nil {
		stream = new(H2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*H2Stream)
	}
	stream.onUse(conn, id)
	return stream
}
func putH2Stream(stream *H2Stream) {
	stream.onEnd()
	poolH2Stream.Put(stream)
}

// H2Stream
type H2Stream struct {
	// Mixins
	_webStream_
	// Assocs
	request  H2Request
	response H2Response
	socket   *H2Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *H2Conn
	id   uint32
	// Stream states (zeros)
}

func (s *H2Stream) onUse(conn *H2Conn, id uint32) { // for non-zeros
	s._webStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *H2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s._webStream_.onEnd()
}

func (s *H2Stream) Request() WebBackendRequest   { return &s.request }
func (s *H2Stream) Response() WebBackendResponse { return &s.response }
func (s *H2Stream) Socket() WebBackendSocket     { return nil } // TODO

func (s *H2Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *H2Stream) ExecuteSocket() error { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
	return nil
}

func (s *H2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *H2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *H2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *H2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

func (s *H2Stream) webAgent() webAgent   { return s.conn.WebBackend() }
func (s *H2Stream) webConn() webConn     { return s.conn }
func (s *H2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *H2Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *H2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *H2Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *H2Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

// H2Request is the backend-side HTTP/2 request.
type H2Request struct { // outgoing. needs building
	// Parent
	webBackendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *H2Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *H2Request) addHeader(name []byte, value []byte) bool   { return r.addHeader2(name, value) }
func (r *H2Request) header(name []byte) (value []byte, ok bool) { return r.header2(name) }
func (r *H2Request) hasHeader(name []byte) bool                 { return r.hasHeader2(name) }
func (r *H2Request) delHeader(name []byte) (deleted bool)       { return r.delHeader2(name) }
func (r *H2Request) delHeaderAt(i uint8)                        { r.delHeaderAt2(i) }

func (r *H2Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *H2Request) proxyCopyCookies(req Request) bool { // DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *H2Request) sendChain() error { return r.sendChain2() }

func (r *H2Request) echoHeaders() error { return r.writeHeaders2() }
func (r *H2Request) echoChain() error   { return r.echoChain2() }

func (r *H2Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}
func (r *H2Request) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer2(name)
}

func (r *H2Request) passHeaders() error       { return r.writeHeaders2() }
func (r *H2Request) passBytes(p []byte) error { return r.passBytes2(p) }

func (r *H2Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *H2Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *H2Request) addedHeaders() []byte { return nil } // TODO
func (r *H2Request) fixedHeaders() []byte { return nil } // TODO

// H2Response is the backend-side HTTP/2 response.
type H2Response struct { // incoming. needs parsing
	// Parent
	webBackendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Response) recvHead() {
	// TODO
}

func (r *H2Response) readContent() (p []byte, err error) { return r.readContent2() }

// poolH2Socket
var poolH2Socket sync.Pool

func getH2Socket(stream *H2Stream) *H2Socket {
	return nil
}
func putH2Socket(socket *H2Socket) {
}

// H2Socket is the backend-side HTTP/2 websocket.
type H2Socket struct {
	// Parent
	webBackendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *H2Socket) onUse() {
	s.webBackendSocket_.onUse()
}
func (s *H2Socket) onEnd() {
	s.webBackendSocket_.onEnd()
}
