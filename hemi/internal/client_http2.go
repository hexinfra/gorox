// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 client implementation.

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
	registerFixture(signHTTP2)
	registerBackend("http2Backend", func(name string, stage *Stage) backend {
		b := new(HTTP2Backend)
		b.onCreate(name, stage)
		return b
	})
}

const signHTTP2 = "http2"

func createHTTP2(stage *Stage) *HTTP2Outgate {
	http2 := new(HTTP2Outgate)
	http2.onCreate(stage)
	http2.setShell(http2)
	return http2
}

// HTTP2Outgate
type HTTP2Outgate struct {
	// Mixins
	httpOutgate_
	// States
}

func (f *HTTP2Outgate) onCreate(stage *Stage) {
	f.httpOutgate_.onCreate(signHTTP2, stage)
}

func (f *HTTP2Outgate) OnConfigure() {
	f.httpOutgate_.onConfigure(f)
}
func (f *HTTP2Outgate) OnPrepare() {
	f.httpOutgate_.onPrepare(f)
}

func (f *HTTP2Outgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("http2 done")
	}
	f.stage.SubDone()
}

func (f *HTTP2Outgate) FetchConn(address string, tlsMode bool) (*H2Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP2Outgate) StoreConn(conn *H2Conn) {
	// TODO
}

// HTTP2Backend
type HTTP2Backend struct {
	// Mixins
	httpBackend_[*http2Node]
	// States
}

func (b *HTTP2Backend) onCreate(name string, stage *Stage) {
	b.httpBackend_.onCreate(name, stage, b)
}

func (b *HTTP2Backend) OnConfigure() {
	b.httpBackend_.onConfigure(b)
}
func (b *HTTP2Backend) OnPrepare() {
	b.httpBackend_.onPrepare(b, len(b.nodes))
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
	node_
	// Assocs
	backend *HTTP2Backend
	// States
}

func (n *http2Node) init(id int32, backend *HTTP2Backend) {
	n.node_.init(id)
	n.backend = backend
}

func (n *http2Node) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("http2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *http2Node) fetchConn() (*H2Conn, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getH2Conn(connID, n.backend, n, netConn, rawConn), nil
}
func (n *http2Node) storeConn(hConn *H2Conn) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolH2Conn is the client-side HTTP/2 connection pool.
var poolH2Conn sync.Pool

func getH2Conn(id int64, client httpClient, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *H2Conn {
	var conn *H2Conn
	if x := poolH2Conn.Get(); x == nil {
		conn = new(H2Conn)
	} else {
		conn = x.(*H2Conn)
	}
	conn.onGet(id, client, node, netConn, rawConn)
	return conn
}
func putH2Conn(conn *H2Conn) {
	conn.onPut()
	poolH2Conn.Put(conn)
}

// H2Conn
type H2Conn struct {
	// Mixins
	hConn_
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *http2Node // associated node
	netConn net.Conn   // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *H2Conn) onGet(id int64, client httpClient, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.hConn_.onGet(id, client)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *H2Conn) onPut() {
	c.hConn_.onPut()
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *H2Conn) FetchStream() *H2Stream {
	// TODO
	return nil
}
func (c *H2Conn) StoreStream(stream *H2Stream) {
	// TODO
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

func (c *H2Conn) closeConn() { c.netConn.Close() }

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
	hStream_
	// Assocs
	request  H2Request
	response H2Response
	socket   *H2Socket
	// Stream states (buffers)
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
	s.hStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse()
	s.response.versionCode = Version2 // explicitly set
	s.response.onUse()
}
func (s *H2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.socket = nil
	s.conn = nil
	s.h2Stream0 = h2Stream0{}
	s.hStream_.onEnd()
}

func (s *H2Stream) keeper() keeper {
	return s.conn.getClient()
}
func (s *H2Stream) peerAddr() net.Addr {
	return s.conn.netConn.RemoteAddr()
}

func (s *H2Stream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *H2Stream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *H2Stream) Request() *H2Request   { return &s.request }
func (s *H2Stream) Response() *H2Response { return &s.response }
func (s *H2Stream) Socket() *H2Socket     { return s.socket }

func (s *H2Stream) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return s.conn.makeTempName(p, stamp)
}

func (s *H2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *H2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *H2Stream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *H2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}
func (s *H2Stream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *H2Stream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}

func (s *H2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *H2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// H2Request is the client-side HTTP/2 request.
type H2Request struct { // outgoing. needs building
	// Mixins
	hRequest_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Request) setControl(method []byte, uri []byte, hasContent bool) bool {
	// TODO
	return false
}
func (r *H2Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO
	return false
}

func (r *H2Request) header(name []byte) (value []byte, ok bool) {
	return r.header2(name)
}
func (r *H2Request) hasHeader(name []byte) bool {
	return r.hasHeader2(name)
}
func (r *H2Request) addHeader(name []byte, value []byte) bool {
	return r.addHeader2(name, value)
}
func (r *H2Request) delHeader(name []byte) (deleted bool) {
	return r.delHeader2(name)
}
func (r *H2Request) delHeaderAt(o uint8) {
	r.delHeaderAt2(o)
}
func (r *H2Request) addedHeaders() []byte {
	return nil
}
func (r *H2Request) fixedHeaders() []byte {
	return nil
}

func (r *H2Request) AddCookie(name string, value string) bool {
	// TODO
	return false
}
func (r *H2Request) copyCookies(req Request) bool { // used by proxies
	// TODO: one by one?
	return true
}

func (r *H2Request) sendChain(chain Chain) error {
	// TODO
	return nil
}

func (r *H2Request) pushHeaders() error {
	// TODO
	return nil
}
func (r *H2Request) pushChain(chain Chain) error {
	// TODO
	return nil
}

func (r *H2Request) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer2(name)
}
func (r *H2Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}

func (r *H2Request) syncHeaders() error {
	// TODO
	return nil
}
func (r *H2Request) syncBytes(p []byte) error {
	return r.syncBytes2(p)
}

func (r *H2Request) finalizeHeaders() {
	// TODO
}
func (r *H2Request) finalizeChunked() error {
	// TODO
	return nil
}

// H2Response is the client-side HTTP/2 response.
type H2Response struct { // incoming. needs parsing
	// Mixins
	hResponse_
	// Assocs
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Response) appendHeaders(p []byte) bool {
	// TODO
	return false
}

func (r *H2Response) readContent() (p []byte, err error) {
	return r.readContent2()
}

func (r *H2Response) appendTrailers(p []byte) bool {
	// TODO
	return false
}

// H2Socket is the client-side HTTP/2 websocket.
type H2Socket struct {
	// Mixins
	hSocket_
	// Stream states (zeros)
}

func (s *H2Socket) onUse() {
	s.hSocket_.onUse()
}
func (s *H2Socket) onEnd() {
	s.hSocket_.onEnd()
}
