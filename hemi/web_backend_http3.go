// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 backend implementation. See RFC 9114 and 9204.

// Server Push is not supported.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quic"
)

func init() {
	RegisterBackend("http3Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP3Backend)
		b.onCreate(name, stage)
		return b
	})
}

// HTTP3Backend
type HTTP3Backend struct {
	// Parent
	webBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage)
}

func (b *HTTP3Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HTTP3Backend) OnPrepare() {
	b.webBackend_.onPrepare(b)
}

func (b *HTTP3Backend) CreateNode(name string) Node {
	node := new(http3Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP3Backend) FetchStream() (WebBackendStream, error) {
	return nil, nil
}
func (b *HTTP3Backend) StoreStream(stream WebBackendStream) {
}

// http3Node
type http3Node struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *http3Node) onCreate(name string, backend *HTTP3Backend) {
	n.Node_.OnCreate(name, backend)
}

func (n *http3Node) OnConfigure() {
	n.Node_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
	}
}
func (n *http3Node) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *http3Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("http3Node=%s done\n", n.name)
	}
	n.backend.DecSub()
}

/*
func (n *http3Node) fetchConn() (WebBackendConn, error) {
	// TODO: dynamic address names?
	// TODO
	conn, err := quic.DialTimeout(n.address, n.backend.DialTimeout())
	if err != nil {
		return nil, err
	}
	connID := n.backend.nextConnID()
	return getH3Conn(connID, n, conn), nil
}
func (n *http3Node) storeConn(conn WebBackendConn) {
	// TODO
}
*/

func (n *http3Node) fetchStream() (WebBackendStream, error) {
	return nil, nil
}
func (n *http3Node) storeStream(stream WebBackendStream) {
}

// poolH3Conn is the backend-side HTTP/3 connection pool.
var poolH3Conn sync.Pool

func getH3Conn(id int64, node *http3Node, quicConn *quic.Conn) *H3Conn {
	var h3Conn *H3Conn
	if x := poolH3Conn.Get(); x == nil {
		h3Conn = new(H3Conn)
	} else {
		h3Conn = x.(*H3Conn)
	}
	h3Conn.onGet(id, node, quicConn)
	return h3Conn
}
func putH3Conn(h3Conn *H3Conn) {
	h3Conn.onPut()
	poolH3Conn.Put(h3Conn)
}

// H3Conn
type H3Conn struct {
	// Parent
	BackendConn_
	// Mixins
	_webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *quic.Conn // the underlying quic connection
	// Conn states (zeros)
	nStreams atomic.Int32                     // concurrent streams
	streams  [http3MaxActiveStreams]*H3Stream // active (open, remoteClosed, localClosed) streams
	h3Conn0                                   // all values must be zero by default in this struct!
}
type h3Conn0 struct { // for fast reset, entirely
}

func (c *H3Conn) onGet(id int64, node *http3Node, quicConn *quic.Conn) {
	c.BackendConn_.OnGet(id, node)
	c._webConn_.onGet()
	c.quicConn = quicConn
}
func (c *H3Conn) onPut() {
	c.quicConn = nil
	c.nStreams.Store(0)
	c.streams = [http3MaxActiveStreams]*H3Stream{}
	c.h3Conn0 = h3Conn0{}
	c._webConn_.onPut()
	c.BackendConn_.OnPut()
}

func (c *H3Conn) WebBackend() WebBackend { return c.Backend().(WebBackend) }
func (c *H3Conn) WebNode() WebNode       { return c.Node().(WebNode) }
func (c *H3Conn) makeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.Backend().Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *H3Conn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.WebBackend().MaxStreamsPerConn()
}

func (c *H3Conn) FetchStream() (WebBackendStream, error) {
	// Note: An H3Conn can be used concurrently, limited by maxStreams.
	// TODO: stream.onUse()
	return nil, nil
}
func (c *H3Conn) StoreStream(stream WebBackendStream) {
	// Note: An H3Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *H3Conn) Close() error {
	quicConn := c.quicConn
	putH3Conn(c)
	return quicConn.Close()
}

// poolH3Stream
var poolH3Stream sync.Pool

func getH3Stream(conn *H3Conn, quicStream *quic.Stream) *H3Stream {
	var stream *H3Stream
	if x := poolH3Stream.Get(); x == nil {
		stream = new(H3Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*H3Stream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putH3Stream(stream *H3Stream) {
	stream.onEnd()
	poolH3Stream.Put(stream)
}

// H3Stream
type H3Stream struct {
	// Mixins
	_webStream_
	// Assocs
	request  H3Request
	response H3Response
	socket   *H3Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *H3Conn
	quicStream *quic.Stream // the underlying quic stream
	// Stream states (zeros)
}

func (s *H3Stream) onUse(conn *H3Conn, quicStream *quic.Stream) { // for non-zeros
	s._webStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *H3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s.quicStream = nil
	s._webStream_.onEnd()
}

func (s *H3Stream) Request() WebBackendRequest   { return &s.request }
func (s *H3Stream) Response() WebBackendResponse { return &s.response }

func (s *H3Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *H3Stream) ExecuteSocket() *H3Socket { // see RFC 9220
	// TODO, use s.startSocket()
	return s.socket
}

func (s *H3Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *H3Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *H3Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *H3Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

func (s *H3Stream) webAgent() webAgent   { return s.conn.WebBackend() }
func (s *H3Stream) webConn() webConn     { return s.conn }
func (s *H3Stream) remoteAddr() net.Addr { return nil } // TODO

func (s *H3Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *H3Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *H3Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *H3Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

// H3Request is the backend-side HTTP/3 request.
type H3Request struct { // outgoing. needs building
	// Parent
	webBackendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H3Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *H3Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *H3Request) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *H3Request) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *H3Request) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *H3Request) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *H3Request) delHeaderAt(i uint8)                        { r.delHeaderAt3(i) }

func (r *H3Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *H3Request) proxyCopyCookies(req Request) bool { // DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *H3Request) sendChain() error { return r.sendChain3() }

func (r *H3Request) echoHeaders() error { return r.writeHeaders3() }
func (r *H3Request) echoChain() error   { return r.echoChain3() }

func (r *H3Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}
func (r *H3Request) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer3(name)
}

func (r *H3Request) passHeaders() error       { return r.writeHeaders3() }
func (r *H3Request) passBytes(p []byte) error { return r.passBytes3(p) }

func (r *H3Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *H3Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *H3Request) addedHeaders() []byte { return nil } // TODO
func (r *H3Request) fixedHeaders() []byte { return nil } // TODO

// H3Response is the backend-side HTTP/3 response.
type H3Response struct { // incoming. needs parsing
	// Parent
	webBackendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H3Response) recvHead() {
	// TODO
}

func (r *H3Response) readContent() (p []byte, err error) { return r.readContent3() }

// poolH3Socket
var poolH3Socket sync.Pool

func getH3Socket(stream *H3Stream) *H3Socket {
	return nil
}
func putH3Socket(socket *H3Socket) {
}

// H3Socket is the backend-side HTTP/3 websocket.
type H3Socket struct {
	// Parent
	webBackendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *H3Socket) onUse() {
	s.webBackendSocket_.onUse()
}
func (s *H3Socket) onEnd() {
	s.webBackendSocket_.onEnd()
}
