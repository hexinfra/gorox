// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 backend implementation. See RFC 9113 and 7541.

// For simplicity, HTTP/2 Server Push is not supported.

package internal

import (
	"io"
	"net"
	"sync"
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
	// Mixins
	webBackend_[*http2Node]
	// States
}

func (b *HTTP2Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HTTP2Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HTTP2Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HTTP2Backend) createNode(id int32) *http2Node {
	node := new(http2Node)
	node.init(id, b)
	return node
}

func (b *HTTP2Backend) FetchConn() (*H2Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *HTTP2Backend) StoreConn(conn *H2Conn) {
	conn.node.storeConn(conn)
}

// http2Node
type http2Node struct {
	// Mixins
	Node_
	// Assocs
	backend *HTTP2Backend
	// States
}

func (n *http2Node) init(id int32, backend *HTTP2Backend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *http2Node) setTLSMode() {
	n.Node_.setTLSMode()
	n.tlsConfig.InsecureSkipVerify = true
	n.tlsConfig.NextProtos = []string{"h2"}
}

func (n *http2Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("http2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *http2Node) fetchConn() (*H2Conn, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getH2Conn(connID, false, false, n.backend, n, netConn, rawConn), nil
}
func (n *http2Node) storeConn(h2Conn *H2Conn) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolH2Conn is the backend-side HTTP/2 connection pool.
var poolH2Conn sync.Pool

func getH2Conn(id int64, udsMode bool, tlsMode bool, backend *HTTP2Backend, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *H2Conn {
	var h2Conn *H2Conn
	if x := poolH2Conn.Get(); x == nil {
		h2Conn = new(H2Conn)
	} else {
		h2Conn = x.(*H2Conn)
	}
	h2Conn.onGet(id, udsMode, tlsMode, backend, node, netConn, rawConn)
	return h2Conn
}
func putH2Conn(h2Conn *H2Conn) {
	h2Conn.onPut()
	poolH2Conn.Put(h2Conn)
}

// H2Conn
type H2Conn struct {
	// Mixins
	backendWebConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *http2Node // associated node
	netConn net.Conn   // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *H2Conn) onGet(id int64, udsMode, tlsMode bool, backend *HTTP2Backend, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.backendWebConn_.onGet(id, udsMode, tlsMode, backend)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *H2Conn) onPut() {
	c.backendWebConn_.onPut()
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *H2Conn) FetchStream() *H2Stream {
	// TODO: stream.onUse()
	return nil
}
func (c *H2Conn) StoreStream(stream *H2Stream) {
	// TODO
	stream.onEnd()
}

func (c *H2Conn) Close() error { // only used by clients of dial
	// TODO
	return nil
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

func (c *H2Conn) closeConn() { c.netConn.Close() } // used by codes which use fetch/store

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
	backendStream_
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
	h2Stream0 // all values must be zero by default in this struct!
}
type h2Stream0 struct { // for fast reset, entirely
}

func (s *H2Stream) onUse(conn *H2Conn, id uint32) { // for non-zeros
	s.backendStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *H2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.socket = nil
	s.conn = nil
	s.h2Stream0 = h2Stream0{}
	s.backendStream_.onEnd()
}

func (s *H2Stream) webBroker() webBroker { return s.conn.webBackend() }
func (s *H2Stream) webConn() webConn     { return s.conn }
func (s *H2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *H2Stream) Request() *H2Request   { return &s.request }
func (s *H2Stream) Response() *H2Response { return &s.response }

func (s *H2Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *H2Stream) ExecuteSocket() *H2Socket { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO, use s.startSocket()
	return s.socket
}
func (s *H2Stream) ExecuteTCPTun() { // CONNECT method
	// TODO, use s.startTCPTun()
}
func (s *H2Stream) ExecuteUDPTun() { // see RFC 9298: https://datatracker.ietf.org/doc/html/rfc9298
	// TODO, use s.startUDPTun()
}

func (s *H2Stream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *H2Stream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *H2Stream) makeTempName(p []byte, unixTime int64) int {
	return s.conn.makeTempName(p, unixTime)
}

func (s *H2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *H2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

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

func (s *H2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *H2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// H2Request is the backend-side HTTP/2 request.
type H2Request struct { // outgoing. needs building
	// Mixins
	backendRequest_
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
func (r *H2Request) copyCookies(req Request) bool { // used by proxies. DO NOT merge into one "cookie" header
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
	// Mixins
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Response) readContent() (p []byte, err error) { return r.readContent2() }

// poolH2Socket
var poolH2Socket sync.Pool

// H2Socket is the backend-side HTTP/2 websocket.
type H2Socket struct {
	// Mixins
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
