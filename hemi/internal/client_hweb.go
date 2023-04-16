// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB client implementation.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

const signHWEBOutgate = "hwebOutgate"

func createHWEBOutgate(stage *Stage) *HWEBOutgate {
	hweb := new(HWEBOutgate)
	hweb.onCreate(stage)
	hweb.setShell(hweb)
	return hweb
}

// HWEBOutgate
type HWEBOutgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HWEBOutgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHWEBOutgate, stage)
}

func (f *HWEBOutgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HWEBOutgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HWEBOutgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("hwebOutgate done")
	}
	f.stage.SubDone()
}

func (f *HWEBOutgate) FetchConn(address string, tlsMode bool) (*H2Conn, error) {
	// TODO
	return nil, nil
}
func (f *HWEBOutgate) StoreConn(conn *H2Conn) {
	// TODO
}

// hwebBackend
type hwebBackend struct {
	// Mixins
	webBackend_[*hwebNode]
}

func (b *hwebBackend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *hwebBackend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *hwebBackend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *hwebBackend) createNode(id int32) *hwebNode {
	node := new(hwebNode)
	node.init(id, b)
	return node
}

func (b *hwebBackend) FetchConn() (*hConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *hwebBackend) StoreConn(conn *hConn) {
	conn.node.storeConn(conn)
}

// hwebNode
type hwebNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *hwebBackend
	// States
}

func (n *hwebNode) init(id int32, backend *hwebBackend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hwebNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("hwebNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *hwebNode) fetchConn() (*hConn, error) {
	// Note: An hConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *hwebNode) storeConn(wConn *hConn) {
	// Note: An hConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolHConn is the client-side HWEB connection pool.
var poolHConn sync.Pool

func getHConn(id int64, client webClient, node *hwebNode, tcpConn *net.TCPConn, rawConn syscall.RawConn) *hConn {
	var conn *hConn
	if x := poolHConn.Get(); x == nil {
		conn = new(hConn)
	} else {
		conn = x.(*hConn)
	}
	conn.onGet(id, client, node, tcpConn, rawConn)
	return conn
}
func putHConn(conn *hConn) {
	conn.onPut()
	poolHConn.Put(conn)
}

// hConn
type hConn struct {
	// Mixins
	wConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hwebNode    // associated node
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *hConn) onGet(id int64, client webClient, node *hwebNode, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.wConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *hConn) onPut() {
	c.wConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *hConn) FetchStream() *hStream {
	// TODO: stream.onUse()
	return nil
}
func (c *hConn) StoreStream(stream *hStream) {
	// TODO
	stream.onEnd()
}

func (c *hConn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *hConn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.tcpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *hConn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.tcpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *hConn) write(p []byte) (int, error) { return c.tcpConn.Write(p) }
func (c *hConn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.tcpConn)
}
func (c *hConn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.tcpConn, p, n)
}

func (c *hConn) closeConn() { c.tcpConn.Close() } // used by codes other than dial

// poolHStream
var poolHStream sync.Pool

func getHStream(conn *hConn, id int32) *hStream {
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
	wStream_
	// Assocs
	request  hRequest
	response hResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *hConn
	id   int32
	// Stream states (zeros)
	hStream0 // all values must be zero by default in this struct!
}
type hStream0 struct { // for fast reset, entirely
}

func (s *hStream) onUse(conn *hConn, id int32) { // for non-zeros
	s.wStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version255)
	s.response.onUse(Version255)
}
func (s *hStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.hStream0 = hStream0{}
	s.wStream_.onEnd()
}

func (s *hStream) keeper() keeper     { return s.conn.getClient() }
func (s *hStream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *hStream) Request() *hRequest   { return &s.request }
func (s *hStream) Response() *hResponse { return &s.response }

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

// hRequest is the client-side HWEB request.
type hRequest struct { // outgoing. needs building
	// Mixins
	wRequest_
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

// hResponse is the client-side HWEB response.
type hResponse struct { // incoming. needs parsing
	// Mixins
	wResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hResponse) readContent() (p []byte, err error) { return r.readContentH() }
