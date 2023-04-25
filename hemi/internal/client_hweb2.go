// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/2 client implementation.

// HWEB/2 is a simplified HTTP/2 without WebSocket, TCP Tunnel, and UDP Tunnel support.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterHandlet("hweb2Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(hweb2Proxy)
		h.onCreate(name, stage, app)
		return h
	})
	registerFixture(signHWEB2Outgate)
	RegisterBackend("hweb2Backend", func(name string, stage *Stage) Backend {
		b := new(HWEB2Backend)
		b.onCreate(name, stage)
		return b
	})
}

// hweb2Proxy handlet passes requests to another/backend HWEB/2 servers and cache responses.
type hweb2Proxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *hweb2Proxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *hweb2Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *hweb2Proxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *hweb2Proxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *hweb2Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	return
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
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("hweb2Outgate done")
	}
	f.stage.SubDone()
}

func (f *HWEB2Outgate) FetchConn(address string, tlsMode bool) (*B2Conn, error) {
	// TODO
	return nil, nil
}
func (f *HWEB2Outgate) StoreConn(conn *B2Conn) {
	// TODO
}

// HWEB2Backend
type HWEB2Backend struct {
	// Mixins
	webBackend_[*hweb2Node]
	// States
}

func (b *HWEB2Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HWEB2Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HWEB2Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HWEB2Backend) createNode(id int32) *hweb2Node {
	node := new(hweb2Node)
	node.init(id, b)
	return node
}

func (b *HWEB2Backend) FetchConn() (*B2Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *HWEB2Backend) StoreConn(conn *B2Conn) {
	conn.node.storeConn(conn)
}

// hweb2Node
type hweb2Node struct {
	// Mixins
	webNode_
	// Assocs
	backend *HWEB2Backend
	// States
}

func (n *hweb2Node) init(id int32, backend *HWEB2Backend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hweb2Node) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("hweb2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *hweb2Node) fetchConn() (*B2Conn, error) {
	// Note: An B2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var tcpConn *net.TCPConn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getB2Conn(connID, n.backend, n, tcpConn, rawConn), nil
}
func (n *hweb2Node) storeConn(b2Conn *B2Conn) {
	// Note: An B2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolB2Conn is the client-side HWEB/2 connection pool.
var poolB2Conn sync.Pool

func getB2Conn(id int64, client webClient, node *hweb2Node, tcpConn *net.TCPConn, rawConn syscall.RawConn) *B2Conn {
	var conn *B2Conn
	if x := poolB2Conn.Get(); x == nil {
		conn = new(B2Conn)
	} else {
		conn = x.(*B2Conn)
	}
	conn.onGet(id, client, node, tcpConn, rawConn)
	return conn
}
func putB2Conn(conn *B2Conn) {
	conn.onPut()
	poolB2Conn.Put(conn)
}

// B2Conn
type B2Conn struct {
	// Mixins
	clientConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hweb2Node   // associated node
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *B2Conn) onGet(id int64, client webClient, node *hweb2Node, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.clientConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *B2Conn) onPut() {
	c.clientConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *B2Conn) FetchStream() *B2Stream {
	// TODO: stream.onUse()
	return nil
}
func (c *B2Conn) StoreStream(stream *B2Stream) {
	// TODO
	stream.onEnd()
}

func (c *B2Conn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *B2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.tcpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *B2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.tcpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *B2Conn) write(p []byte) (int, error) { return c.tcpConn.Write(p) }
func (c *B2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.tcpConn)
}
func (c *B2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.tcpConn, p, n)
}

func (c *B2Conn) closeConn() { c.tcpConn.Close() } // used by codes other than dial

// poolB2Stream
var poolB2Stream sync.Pool

func getB2Stream(conn *B2Conn, id int32) *B2Stream {
	var stream *B2Stream
	if x := poolB2Stream.Get(); x == nil {
		stream = new(B2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*B2Stream)
	}
	stream.onUse(conn, id)
	return stream
}
func putB2Stream(stream *B2Stream) {
	stream.onEnd()
	poolB2Stream.Put(stream)
}

// B2Stream
type B2Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  B2Request
	response B2Response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *B2Conn
	id   int32
	// Stream states (zeros)
	b2Stream0 // all values must be zero by default in this struct!
}
type b2Stream0 struct { // for fast reset, entirely
}

func (s *B2Stream) onUse(conn *B2Conn, id int32) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *B2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.b2Stream0 = b2Stream0{}
	s.webStream_.onEnd()
}

func (s *B2Stream) keeper() webKeeper  { return s.conn.getClient() }
func (s *B2Stream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *B2Stream) Request() *B2Request   { return &s.request }
func (s *B2Stream) Response() *B2Response { return &s.response }

func (s *B2Stream) ExecuteNormal() error { // request & response
	// TODO
	return nil
}

func (s *B2Stream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *B2Stream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *B2Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *B2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *B2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *B2Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *B2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *B2Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *B2Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (s *B2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *B2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// B2Request is the client-side HWEB/2 request.
type B2Request struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *B2Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :uri
	return false
}
func (r *B2Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *B2Request) addHeader(name []byte, value []byte) bool   { return r.addHeaderB2(name, value) }
func (r *B2Request) header(name []byte) (value []byte, ok bool) { return r.headerB2(name) }
func (r *B2Request) hasHeader(name []byte) bool                 { return r.hasHeaderB2(name) }
func (r *B2Request) delHeader(name []byte) (deleted bool)       { return r.delHeaderB2(name) }
func (r *B2Request) delHeaderAt(o uint8)                        { r.delHeaderAtB2(o) }

func (r *B2Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *B2Request) copyCookies(req Request) bool { // used by proxies. merge into one "cookie" header?
	// TODO: one by one?
	return true
}

func (r *B2Request) sendChain() error { return r.sendChainB2() }

func (r *B2Request) echoHeaders() error { return r.writeHeadersB2() }
func (r *B2Request) echoChain() error   { return r.echoChainB2() }

func (r *B2Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerB2(name, value)
}
func (r *B2Request) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerB2(name)
}

func (r *B2Request) passHeaders() error       { return r.writeHeadersB2() }
func (r *B2Request) passBytes(p []byte) error { return r.passBytesB2(p) }

func (r *B2Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *B2Request) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *B2Request) addedHeaders() []byte { return nil } // TODO
func (r *B2Request) fixedHeaders() []byte { return nil } // TODO

// B2Response is the client-side HWEB/2 response.
type B2Response struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *B2Response) readContent() (p []byte, err error) { return r.readContentB2() }
