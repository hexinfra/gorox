// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 backend implementation. See RFC 9113 and RFC 7541.

package hemi

import (
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("http2Backend", func(compName string, stage *Stage) Backend {
		b := new(HTTP2Backend)
		b.OnCreate(compName, stage)
		return b
	})
}

// HTTP2Backend
type HTTP2Backend struct {
	// Parent
	httpBackend_[*http2Node]
	// States
}

func (b *HTTP2Backend) CreateNode(compName string) Node {
	node := new(http2Node)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *HTTP2Backend) AcquireStream(servReq ServerRequest) (BackendStream, error) {
	return b.nodes[b.nodeIndexGet()].fetchStream()
}
func (b *HTTP2Backend) ReleaseStream(backStream BackendStream) {
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
	n.backend.DecNode()
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
	http2Conn_[*backend2Stream]
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
	c.http2Conn_.onPut()
}

func (c *backend2Conn) newStream() (*backend2Stream, error) { // used by http2Node
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO: incRef, backStream.onUse()
	return nil, nil
}
func (c *backend2Conn) delStream(backStream *backend2Stream) { // used by http2Node
	// Note: A backend2Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	//backStream.onEnd()
}

var backend2PrefaceAndMore = []byte{
	// prism
	'P', 'R', 'I', ' ', '*', ' ', 'H', 'T', 'T', 'P', '/', '2', '.', '0', '\r', '\n', '\r', '\n', 'S', 'M', '\r', '\n', '\r', '\n',

	// client preface settings
	0, 0, 36, // length=36
	4,          // kind=http2FrameSettings
	0,          // flags=
	0, 0, 0, 0, // streamID=0
	0, 1, 0x00, 0x00, 0x10, 0x00, // maxHeaderTableSize=4K
	0, 2, 0x00, 0x00, 0x00, 0x00, // enablePush=0
	0, 3, 0x00, 0x00, 0x00, 0x7f, // maxConcurrentStreams=127
	0, 4, 0x00, 0x00, 0xff, 0xff, // initialWindowSize=64K1
	0, 5, 0x00, 0x00, 0x40, 0x00, // maxFrameSize=16K
	0, 6, 0x00, 0x00, 0x40, 0x00, // maxHeaderListSize=16K

	// window update for the entire connection
	0, 0, 4, // length=4
	8,          // kind=http2FrameWindowUpdate
	0,          // flags=
	0, 0, 0, 0, // streamID=0
	0x7f, 0xff, 0x00, 0x00, // windowSize=2G1-64K1
}

func (c *backend2Conn) manage() { // runner
	// setWriteDeadline()
	// write client preface

	// go c.receive(false)

	// read server preface
	// c._updatePeerSettings(true)

	// setWriteDeadline()
	// ack server preface
}

var backend2InFrameProcessors = [http2NumFrameKinds]func(*backend2Conn, *http2InFrame) error{
	(*backend2Conn).processDataInFrame,
	(*backend2Conn).processFieldsInFrame,
	(*backend2Conn).processPriorityInFrame,
	(*backend2Conn).processResetStreamInFrame,
	(*backend2Conn).processSettingsInFrame,
	(*backend2Conn).processPushPromiseInFrame,
	(*backend2Conn).processPingInFrame,
	(*backend2Conn).processGoawayInFrame,
	(*backend2Conn).processWindowUpdateInFrame,
	(*backend2Conn).processContinuationInFrame,
}

func (c *backend2Conn) processFieldsInFrame(fieldsInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *backend2Conn) processSettingsInFrame(settingsInFrame *http2InFrame) error {
	// TODO: server sent a new settings
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
	response backend2Response // the backend-side http/2 response
	request  backend2Request  // the backend-side http/2 request
	socket   *backend2Socket  // the backend-side http/2 webSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backend2Stream0 // all values in this struct must be zero by default!
}
type _backend2Stream0 struct { // for fast reset, entirely
}

var poolBackend2Stream sync.Pool

func getBackend2Stream(conn *backend2Conn, id uint32, remoteWindow int32) *backend2Stream {
	var backStream *backend2Stream
	if x := poolBackend2Stream.Get(); x == nil {
		backStream = new(backend2Stream)
		backResp, backReq := &backStream.response, &backStream.request
		backResp.stream = backStream
		backResp.in = backResp
		backReq.stream = backStream
		backReq.out = backReq
		backReq.response = backResp
	} else {
		backStream = x.(*backend2Stream)
	}
	backStream.onUse(conn, id, remoteWindow)
	return backStream
}
func putBackend2Stream(backStream *backend2Stream) {
	backStream.onEnd()
	poolBackend2Stream.Put(backStream)
}

func (s *backend2Stream) onUse(conn *backend2Conn, id uint32, remoteWindow int32) { // for non-zeros
	s.http2Stream_.onUse(conn, id)
	s._backendStream_.onUse()

	s.localWindow = _64K1         // max size of r.bodyWindow
	s.remoteWindow = remoteWindow // may be changed by the peer
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
}

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

func (r *backend2Request) proxySetMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *backend2Request) proxySetAuthority(hostname []byte, colonport []byte) bool {
	// TODO: set :authority
	return false
}

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
