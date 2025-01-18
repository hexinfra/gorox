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

	// sub components
	b.ConfigureNodes()
}
func (b *HTTP2Backend) OnPrepare() {
	b.httpBackend_.onPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *HTTP2Backend) CreateNode(compName string) Node {
	node := new(http2Node)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *HTTP2Backend) FetchStream(req Request) (backendStream, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.fetchStream()
}
func (b *HTTP2Backend) StoreStream(stream backendStream) {
	stream2 := stream.(*backend2Stream)
	stream2.conn.node.storeStream(stream2)
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
func (n *http2Node) storeStream(stream *backend2Stream) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
}

func (n *http2Node) _dialUDS() (*backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialTLS() (*backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialTCP() (*backend2Conn, error) {
	return nil, nil
}

func (n *http2Node) pullConn() *backend2Conn {
	return nil
}
func (n *http2Node) pushConn(conn *backend2Conn) {
}
func (n *http2Node) closeFree() int {
	return 0
}

// backend2Conn is the backend-side HTTP/2 connection.
type backend2Conn struct {
	// Parent
	http2Conn_
	// Mixins
	_backendConn_
	// Assocs
	next *backend2Conn // the linked-list
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node *http2Node // the node to which the connection belongs
	// Conn states (zeros)
	_backend2Conn0 // all values in this struct must be zero by default!
}
type _backend2Conn0 struct { // for fast reset, entirely
}

var poolBackend2Conn sync.Pool

func getBackend2Conn(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *backend2Conn {
	var backendConn *backend2Conn
	if x := poolBackend2Conn.Get(); x == nil {
		backendConn = new(backend2Conn)
	} else {
		backendConn = x.(*backend2Conn)
	}
	backendConn.onGet(id, node, netConn, rawConn)
	return backendConn
}
func putBackend2Conn(backendConn *backend2Conn) {
	backendConn.onPut()
	poolBackend2Conn.Put(backendConn)
}

func (c *backend2Conn) onGet(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.http2Conn_.onGet(id, node.Stage(), node.UDSMode(), node.TLSMode(), netConn, rawConn, node.ReadTimeout(), node.WriteTimeout())
	c._backendConn_.onGet(time.Now().Add(node.idleTimeout))

	c.node = node
}
func (c *backend2Conn) onPut() {
	c._backend2Conn0 = _backend2Conn0{}
	c.node = nil

	c._backendConn_.onPut()
	c.http2Conn_.onPut()
}

func (c *backend2Conn) ranOut() bool {
	return c.cumulativeStreams.Add(1) > c.node.MaxCumulativeStreamsPerConn()
}
func (c *backend2Conn) fetchStream() (*backend2Stream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO: incRef, stream.onUse()
	return nil, nil
}
func (c *backend2Conn) storeStream(stream *backend2Stream) {
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	//stream.onEnd()
}

var backend2InFrameProcessors = [http2NumFrameKinds]func(*backend2Conn, *http2InFrame) error{
	(*backend2Conn).onDataInFrame,
	(*backend2Conn).onHeadersInFrame,
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
func (c *backend2Conn) onHeadersInFrame(headersInFrame *http2InFrame) error {
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

// backend2Stream
type backend2Stream struct {
	// Parent
	http2Stream_[*backend2Conn]
	// Assocs
	request  backend2Request
	response backend2Response
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
	var backendStream *backend2Stream
	if x := poolBackend2Stream.Get(); x == nil {
		backendStream = new(backend2Stream)
		req, resp := &backendStream.request, &backendStream.response
		req.stream = backendStream
		req.outMessage = req
		req.response = resp
		resp.stream = backendStream
		resp.inMessage = resp
	} else {
		backendStream = x.(*backend2Stream)
	}
	backendStream.onUse(id, conn)
	return backendStream
}
func putBackend2Stream(backendStream *backend2Stream) {
	backendStream.onEnd()
	poolBackend2Stream.Put(backendStream)
}

func (s *backend2Stream) onUse(id uint32, conn *backend2Conn) { // for non-zeros
	s.http2Stream_.onUse(id, conn)

	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *backend2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._backend2Stream0 = _backend2Stream0{}

	s.http2Stream_.onEnd()
	s.conn = nil // we can't do this in http2Stream_.onEnd() due to Go's limit, so put here
}

func (s *backend2Stream) Holder() httpHolder { return s.conn.node }

func (s *backend2Stream) Request() backendRequest   { return &s.request }
func (s *backend2Stream) Response() backendResponse { return &s.response }
func (s *backend2Stream) Socket() backendSocket     { return nil } // TODO. See RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441

// backend2Request is the backend-side HTTP/2 request.
type backend2Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend2Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *backend2Request) proxySetAuthority(hostname []byte, colonport []byte) bool {
	// TODO: set :authority
	return false
}

func (r *backend2Request) addHeader(name []byte, value []byte) bool   { return r.addHeader2(name, value) }
func (r *backend2Request) header(name []byte) (value []byte, ok bool) { return r.header2(name) }
func (r *backend2Request) hasHeader(name []byte) bool                 { return r.hasHeader2(name) }
func (r *backend2Request) delHeader(name []byte) (deleted bool)       { return r.delHeader2(name) }
func (r *backend2Request) delHeaderAt(i uint8)                        { r.delHeaderAt2(i) }

func (r *backend2Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *backend2Request) proxyCopyCookies(foreReq Request) bool { // NOTE: DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *backend2Request) sendChain() error { return r.sendChain2() }

func (r *backend2Request) echoHeaders() error { return r.writeHeaders2() }
func (r *backend2Request) echoChain() error   { return r.echoChain2() }

func (r *backend2Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}
func (r *backend2Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer2(name) }

func (r *backend2Request) proxyPassHeaders() error          { return r.writeHeaders2() }
func (r *backend2Request) proxyPassBytes(data []byte) error { return r.proxyPassBytes2(data) }

func (r *backend2Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *backend2Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *backend2Request) addedHeaders() []byte { return nil } // TODO
func (r *backend2Request) fixedHeaders() []byte { return nil } // TODO

// backend2Response is the backend-side HTTP/2 response.
type backend2Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend2Response) recvHead() {
	// TODO
}

func (r *backend2Response) readContent() (data []byte, err error) { return r.readContent2() }

// backend2Socket is the backend-side HTTP/2 webSocket.
type backend2Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
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
}
func (s *backend2Socket) onEnd() {
	s.backendSocket_.onEnd()
}
