// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/2 client implementation.

// HWEB/2 is a simplified HTTP/2.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerFixture(signHWEB2Outgate)
	registerBackend("hweb2Backend", func(name string, stage *Stage) backend {
		b := new(hweb2Backend)
		b.onCreate(name, stage)
		return b
	})
}

const signHWEB2Outgate = "hweb2Outgate"

func createHWEB2Outgate(stage *Stage) *HWEB2Outgate {
	hweb2 := new(HWEB2Outgate)
	hweb2.onCreate(stage)
	hweb2.setShell(hweb2)
	return hweb2
}

// HWEB2Outgate
type HWEB2Outgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HWEB2Outgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHWEB2Outgate, stage)
}

func (f *HWEB2Outgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HWEB2Outgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HWEB2Outgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("hweb2Outgate done")
	}
	f.stage.SubDone()
}

func (f *HWEB2Outgate) FetchConn(address string, tlsMode bool) (*b2Conn, error) {
	// TODO
	return nil, nil
}
func (f *HWEB2Outgate) StoreConn(conn *b2Conn) {
	// TODO
}

// hweb2Backend
type hweb2Backend struct {
	// Mixins
	webBackend_[*hweb2Node]
	// States
}

func (b *hweb2Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *hweb2Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *hweb2Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *hweb2Backend) createNode(id int32) *hweb2Node {
	node := new(hweb2Node)
	node.init(id, b)
	return node
}

func (b *hweb2Backend) FetchConn() (*b2Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *hweb2Backend) StoreConn(conn *b2Conn) {
	conn.node.storeConn(conn)
}

// hweb2Node
type hweb2Node struct {
	// Mixins
	webNode_
	// Assocs
	backend *hweb2Backend
	// States
}

func (n *hweb2Node) init(id int32, backend *hweb2Backend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hweb2Node) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("hweb2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *hweb2Node) fetchConn() (*b2Conn, error) {
	// Note: An b2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var tcpConn *net.TCPConn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getB2Conn(connID, n.backend, n, tcpConn, rawConn), nil
}
func (n *hweb2Node) storeConn(wConn *b2Conn) {
	// Note: An b2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolB2Conn is the client-side HWEB/2 connection pool.
var poolB2Conn sync.Pool

func getB2Conn(id int64, client webClient, node *hweb2Node, tcpConn *net.TCPConn, rawConn syscall.RawConn) *b2Conn {
	var conn *b2Conn
	if x := poolB2Conn.Get(); x == nil {
		conn = new(b2Conn)
	} else {
		conn = x.(*b2Conn)
	}
	conn.onGet(id, client, node, tcpConn, rawConn)
	return conn
}
func putB2Conn(conn *b2Conn) {
	conn.onPut()
	poolB2Conn.Put(conn)
}

// b2Conn
type b2Conn struct {
	// Mixins
	wConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hweb2Node   // associated node
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *b2Conn) onGet(id int64, client webClient, node *hweb2Node, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.wConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *b2Conn) onPut() {
	c.wConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *b2Conn) FetchStream() *hStream {
	// TODO: stream.onUse()
	return nil
}
func (c *b2Conn) StoreStream(stream *hStream) {
	// TODO
	stream.onEnd()
}

func (c *b2Conn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *b2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.tcpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *b2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.tcpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *b2Conn) write(p []byte) (int, error) { return c.tcpConn.Write(p) }
func (c *b2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.tcpConn)
}
func (c *b2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.tcpConn, p, n)
}

func (c *b2Conn) closeConn() { c.tcpConn.Close() } // used by codes other than dial

// poolHStream
var poolHStream sync.Pool

func getHStream(conn *b2Conn, id int32) *hStream {
	var stream *hStream
	if x := poolHStream.Get(); x == nil {
		stream = new(hStream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*hStream)
	}
	stream.onUse(conn, id)
	return stream
}
func putHStream(stream *hStream) {
	stream.onEnd()
	poolHStream.Put(stream)
}

// hStream
type hStream struct {
	// Mixins
	webStream_
	// Assocs
	request  hRequest
	response hResponse
	socket   *hSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *b2Conn
	id   int32
	// Stream states (zeros)
	hStream0 // all values must be zero by default in this struct!
}
type hStream0 struct { // for fast reset, entirely
}

func (s *hStream) onUse(conn *b2Conn, id int32) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *hStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.socket = nil
	s.conn = nil
	s.hStream0 = hStream0{}
	s.webStream_.onEnd()
}

func (s *hStream) keeper() webKeeper  { return s.conn.getClient() }
func (s *hStream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *hStream) Request() *hRequest   { return &s.request }
func (s *hStream) Response() *hResponse { return &s.response }

func (s *hStream) ExecuteNormal() error { // request & response
	// TODO
	return nil
}
func (s *hStream) ExecuteSocket() *hSocket { // see RFC 8441: https://www.rfc-editor.org/rfc/rfc8441.html
	// TODO
	return s.socket
}
func (s *hStream) ExecuteTCPTun() { // CONNECT method
	// TODO
}
func (s *hStream) ExecuteUDPTun() { // see RFC 9298: https://www.rfc-editor.org/rfc/rfc9298.html
	// TODO
}

func (s *hStream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *hStream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *hStream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *hStream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *hStream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *hStream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *hStream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *hStream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *hStream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (s *hStream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *hStream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// hRequest is the client-side HWEB/2 request.
type hRequest struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hRequest) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :uri = uri
	// TODO: set :method and :uri
	return false
}
func (r *hRequest) setAuthority(hostname []byte, colonPort []byte) bool { // used by agents
	// TODO: set :authority
	return false
}

func (r *hRequest) addHeader(name []byte, value []byte) bool   { return r.addHeaderH(name, value) }
func (r *hRequest) header(name []byte) (value []byte, ok bool) { return r.headerH(name) }
func (r *hRequest) hasHeader(name []byte) bool                 { return r.hasHeaderH(name) }
func (r *hRequest) delHeader(name []byte) (deleted bool)       { return r.delHeaderH(name) }
func (r *hRequest) delHeaderAt(o uint8)                        { r.delHeaderAtH(o) }

func (r *hRequest) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *hRequest) copyCookies(req Request) bool { // used by agents. merge into one "cookie" header?
	// TODO: one by one?
	return true
}

func (r *hRequest) sendChain() error { return r.sendChainH() }

func (r *hRequest) echoHeaders() error {
	// TODO
	return nil
}
func (r *hRequest) echoChain() error { return r.echoChainH() }

func (r *hRequest) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerH(name)
}
func (r *hRequest) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerH(name, value)
}

func (r *hRequest) passHeaders() error {
	// TODO
	return nil
}
func (r *hRequest) passBytes(p []byte) error { return r.passBytesH(p) }

func (r *hRequest) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *hRequest) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *hRequest) addedHeaders() []byte { return nil }
func (r *hRequest) fixedHeaders() []byte { return nil }

// hResponse is the client-side HWEB/2 response.
type hResponse struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hResponse) readContent() (p []byte, err error) { return r.readContentH() }

// poolHSocket
var poolHSocket sync.Pool

// hSocket is the client-side HWEB/2 websocket.
type hSocket struct {
	// Mixins
	clientSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
