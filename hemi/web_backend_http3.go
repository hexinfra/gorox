// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 backend implementation. See RFC 9114 and RFC 9204.

package hemi

import (
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/library/gotcp2"
)

func init() {
	RegisterBackend("http3Backend", func(compName string, stage *Stage) Backend {
		b := new(HTTP3Backend)
		b.OnCreate(compName, stage)
		return b
	})
}

// HTTP3Backend
type HTTP3Backend struct {
	// Parent
	httpBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) CreateNode(compName string) Node {
	node := new(http3Node)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *HTTP3Backend) AcquireStream(servReq ServerRequest) (BackendStream, error) {
	return b.nodes[b.nodeIndexGet()].fetchStream()
}
func (b *HTTP3Backend) ReleaseStream(backStream BackendStream) {
	backStream3 := backStream.(*backend3Stream)
	backStream3.conn.node.storeStream(backStream3)
}

// http3Node
type http3Node struct {
	// Parent
	httpNode_[*HTTP3Backend]
	// States
	connPool struct {
		sync.Mutex
		head *backend3Conn
		tail *backend3Conn
		qnty int
	}
}

func (n *http3Node) onCreate(compName string, stage *Stage, backend *HTTP3Backend) {
	n.httpNode_.onCreate(compName, stage, backend)
}

func (n *http3Node) OnConfigure() {
	n.httpNode_.onConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
	}
}
func (n *http3Node) OnPrepare() {
	n.httpNode_.onPrepare()
}

func (n *http3Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http3Node=%s done\n", n.compName)
	}
	n.backend.DecNode()
}

func (n *http3Node) fetchStream() (*backend3Stream, error) {
	// TODO
	return nil, nil
}
func (n *http3Node) _dialUDS() (*backend3Conn, error) {
	// TODO
	return nil, nil
}
func (n *http3Node) _dialTLS() (*backend3Conn, error) {
	// TODO
	return nil, nil
}
func (n *http3Node) storeStream(backStream *backend3Stream) {
	// TODO
}

func (n *http3Node) pullConn() *backend3Conn {
	// TODO
	return nil
}
func (n *http3Node) pushConn(conn *backend3Conn) {
	// TODO
}
func (n *http3Node) closeIdle() int {
	// TODO
	return 0
}

// backend3Conn is the backend-side HTTP/3 connection.
type backend3Conn struct {
	// Parent
	http3Conn_
	// Mixins
	_backendConn_[*http3Node]
	// Assocs
	next *backend3Conn // the linked-list
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_backend3Conn0 // all values in this struct must be zero by default!
}
type _backend3Conn0 struct { // for fast reset, entirely
}

var poolBackend3Conn sync.Pool

func getBackend3Conn(id int64, node *http3Node, quicConn *gotcp2.Conn) *backend3Conn {
	var backConn *backend3Conn
	if x := poolBackend3Conn.Get(); x == nil {
		backConn = new(backend3Conn)
	} else {
		backConn = x.(*backend3Conn)
	}
	backConn.onGet(id, node, quicConn)
	return backConn
}
func putBackend3Conn(backConn *backend3Conn) {
	backConn.onPut()
	poolBackend3Conn.Put(backConn)
}

func (c *backend3Conn) onGet(id int64, node *http3Node, quicConn *gotcp2.Conn) {
	c.http3Conn_.onGet(id, node, quicConn)
	c._backendConn_.onGet(node)
}
func (c *backend3Conn) onPut() {
	c._backend3Conn0 = _backend3Conn0{}

	c._backendConn_.onPut()
	c.http3Conn_.onPut()
}

func (c *backend3Conn) newStream() (*backend3Stream, error) { // used by http3Node
	// Note: A backend3Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO: backStream.onUse()
	return nil, nil
}
func (c *backend3Conn) delStream(backStream *backend3Stream) { // used by http3Node
	// Note: A backend3Conn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	//backStream.onEnd()
}

func (c *backend3Conn) Close() error {
	quicConn := c.quicConn
	putBackend3Conn(c)
	return quicConn.Close()
}

// backend3Stream is the backend-side HTTP/3 stream.
type backend3Stream struct {
	// Parent
	http3Stream_[*backend3Conn]
	// Mixins
	_backendStream_
	// Assocs
	response backend3Response // the backend-side http/3 response
	request  backend3Request  // the backend-side http/3 request
	socket   *backend3Socket  // the backend-side http/3 webSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backend3Stream0 // all values in this struct must be zero by default!
}
type _backend3Stream0 struct { // for fast reset, entirely
}

var poolBackend3Stream sync.Pool

func getBackend3Stream(conn *backend3Conn, quicStream *gotcp2.Stream) *backend3Stream {
	var backStream *backend3Stream
	if x := poolBackend3Stream.Get(); x == nil {
		backStream = new(backend3Stream)
		backResp, backReq := &backStream.response, &backStream.request
		backResp.stream = backStream
		backResp.in = backResp
		backReq.stream = backStream
		backReq.out = backReq
		backReq.response = backResp
	} else {
		backStream = x.(*backend3Stream)
	}
	backStream.onUse(conn, quicStream)
	return backStream
}
func putBackend3Stream(backStream *backend3Stream) {
	backStream.onEnd()
	poolBackend3Stream.Put(backStream)
}

func (s *backend3Stream) onUse(conn *backend3Conn, quicStream *gotcp2.Stream) { // for non-zeros
	s.http3Stream_.onUse(conn, quicStream)
	s._backendStream_.onUse()

	s.response.onUse()
	s.request.onUse()
}
func (s *backend3Stream) onEnd() { // for zeros
	s.request.onEnd()
	s.response.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._backend3Stream0 = _backend3Stream0{}

	s._backendStream_.onEnd()
	s.http3Stream_.onEnd()
}

func (s *backend3Stream) Response() BackendResponse { return &s.response }
func (s *backend3Stream) Request() BackendRequest   { return &s.request }
func (s *backend3Stream) Socket() BackendSocket     { return nil } // TODO. See RFC 9220

// backend3Response is the backend-side HTTP/3 response.
type backend3Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Assocs
	in3 _http3In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend3Response) onUse() {
	r.backendResponse_.onUse(Version3)
	r.in3.onUse(&r._httpIn_)
}
func (r *backend3Response) onEnd() {
	r.backendResponse_.onEnd()
	r.in3.onEnd()
}

func (r *backend3Response) recvHead() { // control data + header section
	// TODO
}

func (r *backend3Response) readContent() (data []byte, err error) { return r.in3.readContent() }

// backend3Request is the backend-side HTTP/3 request.
type backend3Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Assocs
	out3 _http3Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend3Request) onUse() {
	r.backendRequest_.onUse(Version3)
	r.out3.onUse(&r._httpOut_)
}
func (r *backend3Request) onEnd() {
	r.backendRequest_.onEnd()
	r.out3.onEnd()
}

func (r *backend3Request) addHeader(name []byte, value []byte) bool {
	return r.out3.addHeader(name, value)
}
func (r *backend3Request) header(name []byte) (value []byte, ok bool) { return r.out3.header(name) }
func (r *backend3Request) hasHeader(name []byte) bool                 { return r.out3.hasHeader(name) }
func (r *backend3Request) delHeader(name []byte) (deleted bool)       { return r.out3.delHeader(name) }
func (r *backend3Request) delHeaderAt(i uint8)                        { r.out3.delHeaderAt(i) }

func (r *backend3Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *backend3Request) proxyCopyCookies(servReq ServerRequest) bool { // NOTE: DO NOT merge into one "cookie" header field!
	// TODO: one by one?
	return true
}

func (r *backend3Request) sendChain() error { return r.out3.sendChain() }

func (r *backend3Request) echoHeaders() error { return r.out3.writeHeaders() }
func (r *backend3Request) echoChain() error   { return r.out3.echoChain() }

func (r *backend3Request) addTrailer(name []byte, value []byte) bool {
	return r.out3.addTrailer(name, value)
}
func (r *backend3Request) trailer(name []byte) (value []byte, ok bool) { return r.out3.trailer(name) }

func (r *backend3Request) proxySetMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *backend3Request) proxySetAuthority(hostname []byte, colonport []byte) bool {
	// TODO: set :authority
	return false
}

func (r *backend3Request) proxyPassHeaders() error          { return r.out3.writeHeaders() }
func (r *backend3Request) proxyPassBytes(data []byte) error { return r.out3.proxyPassBytes(data) }

func (r *backend3Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *backend3Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *backend3Request) addedHeaders() []byte { return nil } // TODO
func (r *backend3Request) fixedHeaders() []byte { return nil } // TODO

// backend3Socket is the backend-side HTTP/3 webSocket.
type backend3Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
	// Assocs
	so3 _http3Socket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolBackend3Socket sync.Pool

func getBackend3Socket(stream *backend3Stream) *backend3Socket {
	// TODO
	return nil
}
func putBackend3Socket(socket *backend3Socket) {
	// TODO
}

func (s *backend3Socket) onUse() {
	s.backendSocket_.onUse()
	s.so3.onUse(&s._httpSocket_)
}
func (s *backend3Socket) onEnd() {
	s.backendSocket_.onEnd()
	s.so3.onEnd()
}

func (s *backend3Socket) backendTodo3() {
	s.backendTodo()
	s.so3.todo3()
}
