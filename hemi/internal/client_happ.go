// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP client implementation.

// HAPP is a simplified HTTP/2 without WebSocket, TCP Tunnel, and UDP Tunnel support.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerFixture(signHAPPOutgate)
	RegisterBackend("happBackend", func(name string, stage *Stage) Backend {
		b := new(HAPPBackend)
		b.onCreate(name, stage)
		return b
	})
}

const signHAPPOutgate = "happOutgate"

func createHAPPOutgate(stage *Stage) *HAPPOutgate {
	happ := new(HAPPOutgate)
	happ.onCreate(stage)
	happ.setShell(happ)
	return happ
}

// HAPPOutgate
type HAPPOutgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HAPPOutgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHAPPOutgate, stage)
}

func (f *HAPPOutgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HAPPOutgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HAPPOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("happOutgate done")
	}
	f.stage.SubDone()
}

func (f *HAPPOutgate) FetchConn(address string, tlsMode bool) (*PConn, error) {
	// TODO
	return nil, nil
}
func (f *HAPPOutgate) StoreConn(conn *PConn) {
	// TODO
}

// HAPPBackend
type HAPPBackend struct {
	// Mixins
	webBackend_[*happNode]
	// States
}

func (b *HAPPBackend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HAPPBackend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HAPPBackend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HAPPBackend) createNode(id int32) *happNode {
	node := new(happNode)
	node.init(id, b)
	return node
}

func (b *HAPPBackend) FetchConn() (*PConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *HAPPBackend) StoreConn(conn *PConn) {
	conn.node.storeConn(conn)
}

// happNode
type happNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *HAPPBackend
	// States
}

func (n *happNode) init(id int32, backend *HAPPBackend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *happNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("happNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *happNode) fetchConn() (*PConn, error) {
	// Note: An PConn can be used concurrently, limited by maxStreams.
	// TODO
	var tcpConn *net.TCPConn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getPConn(connID, n.backend, n, tcpConn, rawConn), nil
}
func (n *happNode) storeConn(b2Conn *PConn) {
	// Note: An PConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolPConn is the client-side HAPP connection pool.
var poolPConn sync.Pool

func getPConn(id int64, client webClient, node *happNode, tcpConn *net.TCPConn, rawConn syscall.RawConn) *PConn {
	var conn *PConn
	if x := poolPConn.Get(); x == nil {
		conn = new(PConn)
	} else {
		conn = x.(*PConn)
	}
	conn.onGet(id, client, node, tcpConn, rawConn)
	return conn
}
func putPConn(conn *PConn) {
	conn.onPut()
	poolPConn.Put(conn)
}

// PConn
type PConn struct {
	// Mixins
	clientConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *happNode    // associated node
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *PConn) onGet(id int64, client webClient, node *happNode, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.clientConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *PConn) onPut() {
	c.clientConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *PConn) FetchStream() *PStream {
	// TODO: stream.onUse()
	return nil
}
func (c *PConn) StoreStream(stream *PStream) {
	// TODO
	stream.onEnd()
}

func (c *PConn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *PConn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.tcpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *PConn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.tcpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *PConn) write(p []byte) (int, error) { return c.tcpConn.Write(p) }
func (c *PConn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.tcpConn)
}
func (c *PConn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.tcpConn, p, n)
}

func (c *PConn) closeConn() { c.tcpConn.Close() } // used by codes other than dial

// poolPStream
var poolPStream sync.Pool

func getPStream(conn *PConn, id int32) *PStream {
	var stream *PStream
	if x := poolPStream.Get(); x == nil {
		stream = new(PStream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*PStream)
	}
	stream.onUse(conn, id)
	return stream
}
func putPStream(stream *PStream) {
	stream.onEnd()
	poolPStream.Put(stream)
}

// PStream
type PStream struct {
	// Mixins
	webStream_
	// Assocs
	request  PRequest
	response PResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *PConn
	id   int32
	// Stream states (zeros)
	b2Stream0 // all values must be zero by default in this struct!
}
type b2Stream0 struct { // for fast reset, entirely
}

func (s *PStream) onUse(conn *PConn, id int32) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *PStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.b2Stream0 = b2Stream0{}
	s.webStream_.onEnd()
}

func (s *PStream) webAgent() webAgent { return s.conn.getClient() }
func (s *PStream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *PStream) Request() *PRequest   { return &s.request }
func (s *PStream) Response() *PResponse { return &s.response }

func (s *PStream) ExecuteNormal() error { // request & response
	// TODO
	return nil
}

func (s *PStream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *PStream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *PStream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *PStream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *PStream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *PStream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *PStream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *PStream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *PStream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (s *PStream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *PStream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// PRequest is the client-side HAPP request.
type PRequest struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *PRequest) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :uri
	return false
}
func (r *PRequest) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *PRequest) addHeader(name []byte, value []byte) bool   { return r.addHeaderP(name, value) }
func (r *PRequest) header(name []byte) (value []byte, ok bool) { return r.headerP(name) }
func (r *PRequest) hasHeader(name []byte) bool                 { return r.hasHeaderP(name) }
func (r *PRequest) delHeader(name []byte) (deleted bool)       { return r.delHeaderP(name) }
func (r *PRequest) delHeaderAt(o uint8)                        { r.delHeaderAtP(o) }

func (r *PRequest) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *PRequest) copyCookies(req Request) bool { // used by proxies. merge into one "cookie" header?
	// TODO: one by one?
	return true
}

func (r *PRequest) sendChain() error { return r.sendChainP() }

func (r *PRequest) echoHeaders() error { return r.writeHeadersP() }
func (r *PRequest) echoChain() error   { return r.echoChainP() }

func (r *PRequest) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerP(name, value)
}
func (r *PRequest) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerP(name)
}

func (r *PRequest) passHeaders() error       { return r.writeHeadersP() }
func (r *PRequest) passBytes(p []byte) error { return r.passBytesP(p) }

func (r *PRequest) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *PRequest) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *PRequest) addedHeaders() []byte { return nil } // TODO
func (r *PRequest) fixedHeaders() []byte { return nil } // TODO

// PResponse is the client-side HAPP response.
type PResponse struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *PResponse) readContent() (p []byte, err error) { return r.readContentP() }
