// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 backend implementation. See RFC 9113 and 7541.

// For simplicity, HTTP/2 Server Push is not supported.

package hemi

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("h2Backend", func(name string, stage *Stage) Backend {
		b := new(H2Backend)
		b.onCreate(name, stage)
		return b
	})
}

// H2Backend
type H2Backend struct {
	// Mixins
	webBackend_[*h2Node]
	// States
}

func (b *H2Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *H2Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *H2Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *H2Backend) createNode(id int32) *h2Node {
	node := new(h2Node)
	node.init(id, b)
	return node
}

func (b *H2Backend) FetchConn() (*H2Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *H2Backend) StoreConn(conn *H2Conn) {
	conn.node.(*h2Node).storeConn(conn)
}

// h2Node
type h2Node struct {
	// Mixins
	Node_
	// Assocs
	backend *H2Backend
	// States
}

func (n *h2Node) init(id int32, backend *H2Backend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *h2Node) setTLS() {
	n.Node_.setTLS()
	n.tlsConfig.InsecureSkipVerify = true
	n.tlsConfig.NextProtos = []string{"h2"}
}

func (n *h2Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("h2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *h2Node) fetchConn() (*H2Conn, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getH2Conn(connID, n.backend, n, netConn, rawConn), nil
}
func (n *h2Node) storeConn(h2Conn *H2Conn) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolH2Conn is the backend-side HTTP/2 connection pool.
var poolH2Conn sync.Pool

func getH2Conn(id int64, backend *H2Backend, node *h2Node, netConn net.Conn, rawConn syscall.RawConn) *H2Conn {
	var h2Conn *H2Conn
	if x := poolH2Conn.Get(); x == nil {
		h2Conn = new(H2Conn)
	} else {
		h2Conn = x.(*H2Conn)
	}
	h2Conn.onGet(id, backend, node, netConn, rawConn)
	return h2Conn
}
func putH2Conn(h2Conn *H2Conn) {
	h2Conn.onPut()
	poolH2Conn.Put(h2Conn)
}

// H2Conn
type H2Conn struct {
	// Mixins
	webBackendConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn net.Conn // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *H2Conn) onGet(id int64, backend *H2Backend, node *h2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.webBackendConn_.onGet(id, backend, node)
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *H2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.activeStreams = 0
	c.webBackendConn_.onPut()
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
	webBackendStream_
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
	s.webBackendStream_.onUse()
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
	s.webBackendStream_.onEnd()
}

func (s *H2Stream) webBroker() webBroker { return s.conn.Backend() }
func (s *H2Stream) webConn() webConn     { return s.conn }
func (s *H2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *H2Stream) Request() *H2Request   { return &s.request }
func (s *H2Stream) Response() *H2Response { return &s.response }

func (s *H2Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *H2Stream) ReverseExchan(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *H2Stream) ExecuteSocket() *H2Socket { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO, use s.startSocket()
	return s.socket
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
	webBackendResponse_
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
	webBackendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
