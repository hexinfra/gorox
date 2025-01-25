// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 backend implementation. See RFC 9113 and RFC 7541.

package hemi

import (
	"encoding/binary"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("http2Backend", func(compName string, stage *Stage) Backend {
		b := new(HTTP2Backend)
		b.onCreate(compName, stage)
		return b
	})
}

// HTTP2Backend
type HTTP2Backend struct {
	// Parent
	httpBackend_[*http2Node]
	// States
}

func (b *HTTP2Backend) onCreate(compName string, stage *Stage) {
	b.httpBackend_.onCreate(compName, stage)
}

func (b *HTTP2Backend) OnConfigure() {
	b.httpBackend_.onConfigure()

	b.ConfigureNodes()
}
func (b *HTTP2Backend) OnPrepare() {
	b.httpBackend_.onPrepare()

	b.PrepareNodes()
}

func (b *HTTP2Backend) CreateNode(compName string) Node {
	node := new(http2Node)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *HTTP2Backend) FetchStream(req ServerRequest) (BackendStream, error) {
	return b.nodes[b.nodeIndexGet()].fetchStream()
}
func (b *HTTP2Backend) StoreStream(backStream BackendStream) {
	backStream2 := backStream.(*backend2Stream)
	backStream2.conn.node.storeStream(backStream2)
}

// http2Node
type http2Node struct {
	// Parent
	httpNode_[*HTTP2Backend]
	// States
	connPool struct {
		sync.Mutex
		head *backend2Conn
		tail *backend2Conn
		qnty int
	}
}

func (n *http2Node) onCreate(compName string, stage *Stage, backend *HTTP2Backend) {
	n.httpNode_.onCreate(compName, stage, backend)
}

func (n *http2Node) OnConfigure() {
	n.httpNode_.onConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"h2"}
	}
}
func (n *http2Node) OnPrepare() {
	n.httpNode_.onPrepare()
}

func (n *http2Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http2Node=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *http2Node) fetchStream() (*backend2Stream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	return nil, nil
}
func (n *http2Node) _dialUDS() (*backend2Conn, error) {
	// TODO
	return nil, nil
}
func (n *http2Node) _dialTLS() (*backend2Conn, error) {
	// TODO
	return nil, nil
}
func (n *http2Node) _dialTCP() (*backend2Conn, error) {
	// TODO
	return nil, nil
}
func (n *http2Node) storeStream(backStream *backend2Stream) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
}

func (n *http2Node) pullConn() *backend2Conn {
	// TODO
	return nil
}
func (n *http2Node) pushConn(conn *backend2Conn) {
	// TODO
}
func (n *http2Node) closeIdle() int {
	// TODO
	return 0
}

// backend2Conn is the backend-side HTTP/2 connection.
type backend2Conn struct {
	// Parent
	http2Conn_
	// Mixins
	_backendConn_[*http2Node]
	// Assocs
	next *backend2Conn // the linked-list
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_backend2Conn0 // all values in this struct must be zero by default!
}
type _backend2Conn0 struct { // for fast reset, entirely
}

var poolBackend2Conn sync.Pool

func getBackend2Conn(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *backend2Conn {
	var backConn *backend2Conn
	if x := poolBackend2Conn.Get(); x == nil {
		backConn = new(backend2Conn)
	} else {
		backConn = x.(*backend2Conn)
	}
	backConn.onGet(id, node, netConn, rawConn)
	return backConn
}
func putBackend2Conn(backConn *backend2Conn) {
	backConn.onPut()
	poolBackend2Conn.Put(backConn)
}

func (c *backend2Conn) onGet(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.http2Conn_.onGet(id, node, netConn, rawConn)
	c._backendConn_.onGet(node)
}
func (c *backend2Conn) onPut() {
	c._backend2Conn0 = _backend2Conn0{}

	c._backendConn_.onPut()
	c.node = nil // put here due to Go's limitation
	c.http2Conn_.onPut()
}

func (c *backend2Conn) fetchStream() (*backend2Stream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO: incRef, backStream.onUse()
	return nil, nil
}
func (c *backend2Conn) storeStream(backStream *backend2Stream) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	//backStream.onEnd()
}

var backend2InFrameProcessors = [http2NumFrameKinds]func(*backend2Conn, *http2InFrame) error{
	(*backend2Conn).onDataInFrame,
	(*backend2Conn).onFieldsInFrame,
	(*backend2Conn).onPriorityInFrame,
	(*backend2Conn).onRSTStreamInFrame,
	(*backend2Conn).onSettingsInFrame,
	nil, // pushPromise frames are rejected priorly
	(*backend2Conn).onPingInFrame,
	nil, // goaway frames are hijacked by c.receiver()
	(*backend2Conn).onWindowUpdateInFrame,
	nil, // discrete continuation frames are rejected priorly
}

func (c *backend2Conn) onDataInFrame(dataInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *backend2Conn) onFieldsInFrame(fieldsInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *backend2Conn) onPriorityInFrame(priorityInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *backend2Conn) onRSTStreamInFrame(rstStreamInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *backend2Conn) onSettingsInFrame(settingsInFrame *http2InFrame) error {
	// TODO: server sent a new settings
	return nil
}
func (c *backend2Conn) _updatePeerSettings(settingsInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *backend2Conn) _adjustStreamWindows(delta int32) {
	// TODO
}
func (c *backend2Conn) onPingInFrame(pingInFrame *http2InFrame) error {
	pongOutFrame := &c.outFrame
	pongOutFrame.length = 8
	pongOutFrame.streamID = 0
	pongOutFrame.kind = http2FramePing
	pongOutFrame.ack = true
	pongOutFrame.payload = pingInFrame.effective() // TODO: copy()?
	err := c.sendOutFrame(pongOutFrame)
	pongOutFrame.zero()
	return err
}
func (c *backend2Conn) onWindowUpdateInFrame(windowUpdateInFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(windowUpdateInFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		return http2ErrorProtocol
	}
	// TODO
	c.inWindow = int32(windowSize)
	Printf("conn=%d stream=%d windowUpdate=%d\n", c.id, windowUpdateInFrame.streamID, windowSize)
	return nil
}

func (c *backend2Conn) Close() error {
	netConn := c.netConn
	putBackend2Conn(c)
	return netConn.Close()
}

// backend2Stream is the backend-side HTTP/2 stream.
type backend2Stream struct {
	// Parent
	http2Stream_[*backend2Conn]
	// Mixins
	_backendStream_
	// Assocs
	response backend2Response
	request  backend2Request
	socket   *backend2Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backend2Stream0 // all values in this struct must be zero by default!
}
type _backend2Stream0 struct { // for fast reset, entirely
}

var poolBackend2Stream sync.Pool

func getBackend2Stream(conn *backend2Conn, id uint32) *backend2Stream {
	var backStream *backend2Stream
	if x := poolBackend2Stream.Get(); x == nil {
		backStream = new(backend2Stream)
		resp, req := &backStream.response, &backStream.request
		resp.stream = backStream
		resp.inMessage = resp
		req.stream = backStream
		req.outMessage = req
		req.response = resp
	} else {
		backStream = x.(*backend2Stream)
	}
	backStream.onUse(id, conn)
	return backStream
}
func putBackend2Stream(backStream *backend2Stream) {
	backStream.onEnd()
	poolBackend2Stream.Put(backStream)
}

func (s *backend2Stream) onUse(id uint32, conn *backend2Conn) { // for non-zeros
	s.http2Stream_.onUse(id, conn)
	s._backendStream_.onUse()

	s.response.onUse()
	s.request.onUse()
}
func (s *backend2Stream) onEnd() { // for zeros
	s.request.onEnd()
	s.response.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._backend2Stream0 = _backend2Stream0{}

	s._backendStream_.onEnd()
	s.http2Stream_.onEnd()
	s.conn = nil // we can't do this in http2Stream_.onEnd() due to Go's limit, so put here
}

func (s *backend2Stream) Holder() httpHolder { return s.conn.node }

func (s *backend2Stream) Response() BackendResponse { return &s.response }
func (s *backend2Stream) Request() BackendRequest   { return &s.request }
func (s *backend2Stream) Socket() BackendSocket     { return nil } // TODO. See RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441

// backend2Response is the backend-side HTTP/2 response.
type backend2Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Assocs
	in2 _http2In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend2Response) onUse() {
	r.backendResponse_.onUse(Version2)
	r.in2.onUse(&r._httpIn_)
}
func (r *backend2Response) onEnd() {
	r.backendResponse_.onEnd()
	r.in2.onEnd()
}

func (r *backend2Response) recvHead() { // control data + header section
	// TODO
}

func (r *backend2Response) readContent() (data []byte, err error) { return r.in2.readContent() }

// backend2Request is the backend-side HTTP/2 request.
type backend2Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Assocs
	out2 _http2Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend2Request) onUse() {
	r.backendRequest_.onUse(Version2)
	r.out2.onUse(&r._httpOut_)
}
func (r *backend2Request) onEnd() {
	r.backendRequest_.onEnd()
	r.out2.onEnd()
}

func (r *backend2Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *backend2Request) proxySetAuthority(hostname []byte, colonport []byte) bool {
	// TODO: set :authority
	return false
}

func (r *backend2Request) addHeader(name []byte, value []byte) bool {
	return r.out2.addHeader(name, value)
}
func (r *backend2Request) header(name []byte) (value []byte, ok bool) { return r.out2.header(name) }
func (r *backend2Request) hasHeader(name []byte) bool                 { return r.out2.hasHeader(name) }
func (r *backend2Request) delHeader(name []byte) (deleted bool)       { return r.out2.delHeader(name) }
func (r *backend2Request) delHeaderAt(i uint8)                        { r.out2.delHeaderAt(i) }

func (r *backend2Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *backend2Request) proxyCopyCookies(servReq ServerRequest) bool { // NOTE: DO NOT merge into one "cookie" header field!
	// TODO: one by one?
	return true
}

func (r *backend2Request) sendChain() error { return r.out2.sendChain() }

func (r *backend2Request) echoHeaders() error { return r.out2.writeHeaders() }
func (r *backend2Request) echoChain() error   { return r.out2.echoChain() }

func (r *backend2Request) addTrailer(name []byte, value []byte) bool {
	return r.out2.addTrailer(name, value)
}
func (r *backend2Request) trailer(name []byte) (value []byte, ok bool) { return r.out2.trailer(name) }

func (r *backend2Request) proxyPassHeaders() error          { return r.out2.writeHeaders() }
func (r *backend2Request) proxyPassBytes(data []byte) error { return r.out2.proxyPassBytes(data) }

func (r *backend2Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *backend2Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *backend2Request) addedHeaders() []byte { return nil } // TODO
func (r *backend2Request) fixedHeaders() []byte { return nil } // TODO

// backend2Socket is the backend-side HTTP/2 webSocket.
type backend2Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
	// Assocs
	so2 _http2Socket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolBackend2Socket sync.Pool

func getBackend2Socket(stream *backend2Stream) *backend2Socket {
	// TODO
	return nil
}
func putBackend2Socket(socket *backend2Socket) {
	// TODO
}

func (s *backend2Socket) onUse() {
	s.backendSocket_.onUse()
	s.so2.onUse(&s._httpSocket_)
}
func (s *backend2Socket) onEnd() {
	s.backendSocket_.onEnd()
	s.so2.onEnd()
}

func (s *backend2Socket) backendTodo2() {
	s.backendTodo()
	s.so2.todo2()
}
