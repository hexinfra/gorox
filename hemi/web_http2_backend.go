// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 backend implementation. See RFC 9113 and 7541.

// Server Push is not supported.

package hemi

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("http2Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP2Backend)
		b.onCreate(name, stage)
		return b
	})
}

// HTTP2Backend
type HTTP2Backend struct {
	// Parent
	Backend_[*http2Node]
	// Mixins
	_httpServend_
	// States
}

func (b *HTTP2Backend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *HTTP2Backend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._httpServend_.onConfigure(b, 60*time.Second, 60*time.Second, 1000, TmpDir()+"/web/backends/"+b.name)

	// sub components
	b.ConfigureNodes()
}
func (b *HTTP2Backend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._httpServend_.onPrepare(b)

	// sub components
	b.PrepareNodes()
}

func (b *HTTP2Backend) CreateNode(name string) Node {
	node := new(http2Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP2Backend) FetchStream() (backendStream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP2Backend) StoreStream(stream backendStream) {
	conn := stream.httpConn().(*backend2Conn)
	conn.node.storeStream(stream)
}

// http2Node
type http2Node struct {
	// Parent
	Node_
	// Assocs
	backend *HTTP2Backend
	// States
}

func (n *http2Node) onCreate(name string, backend *HTTP2Backend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *http2Node) OnConfigure() {
	n.Node_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"h2"}
	}
}
func (n *http2Node) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *http2Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http2Node=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *http2Node) fetchStream() (backendStream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *http2Node) storeStream(stream backendStream) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

/*
func (n *http2Node) fetchConn() (*backend2Conn, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getBackend2Conn(connID, n, netConn, rawConn), nil
}
func (n *http2Node) _dialTLS() (*backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialUDS() (*backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) _dialTCP() (*backend2Conn, error) {
	return nil, nil
}
func (n *http2Node) storeConn(conn *backend2Conn) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO: decRef
	if conn.nStreams.Add(-1) > 0 {
		return
	}
}
*/

// poolBackend2Conn is the backend-side HTTP/2 connection pool.
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

// backend2Conn
type backend2Conn struct {
	// Mixins
	_httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64     // the conn id
	expire  time.Time // when the conn is considered expired
	node    *http2Node
	netConn net.Conn // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	counter       atomic.Int64                           // can be used to generate a random number
	lastWrite     time.Time                              // deadline of last write operation
	lastRead      time.Time                              // deadline of last read operation
	nStreams      atomic.Int32                           // concurrent streams
	streams       [http2MaxActiveStreams]*backend2Stream // active (open, remoteClosed, localClosed) streams
	backend2Conn0                                        // all values must be zero by default in this struct!
}
type backend2Conn0 struct { // for fast reset, entirely
}

func (c *backend2Conn) onGet(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c._httpConn_.onGet()

	c.id = id
	c.expire = time.Now().Add(node.backend.aliveTimeout)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *backend2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.nStreams.Store(0)
	c.streams = [http2MaxActiveStreams]*backend2Stream{}
	c.backend2Conn0 = backend2Conn0{}
	c.node = nil
	c.expire = time.Time{}
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}

	c._httpConn_.onPut()
}

func (c *backend2Conn) IsTLS() bool { return c.node.IsTLS() }
func (c *backend2Conn) IsUDS() bool { return c.node.IsUDS() }

func (c *backend2Conn) ID() int64 { return c.id }

func (c *backend2Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.node.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *backend2Conn) HTTPBackend() HTTPBackend { return c.node.backend }

func (c *backend2Conn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.HTTPBackend().MaxStreamsPerConn()
}

func (c *backend2Conn) fetchStream() (backendStream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO: incRef, stream.onUse()
	return nil, nil
}
func (c *backend2Conn) storeStream(stream backendStream) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *backend2Conn) setWriteDeadline() error {
	deadline := time.Now().Add(c.node.backend.WriteTimeout())
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *backend2Conn) setReadDeadline() error {
	deadline := time.Now().Add(c.node.backend.ReadTimeout())
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *backend2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *backend2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}
func (c *backend2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}

func (c *backend2Conn) Close() error {
	netConn := c.netConn
	putBackend2Conn(c)
	return netConn.Close()
}

// poolBackend2Stream
var poolBackend2Stream sync.Pool

func getBackend2Stream(conn *backend2Conn, id uint32) *backend2Stream {
	var stream *backend2Stream
	if x := poolBackend2Stream.Get(); x == nil {
		stream = new(backend2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*backend2Stream)
	}
	stream.onUse(conn, id)
	return stream
}
func putBackend2Stream(stream *backend2Stream) {
	stream.onEnd()
	poolBackend2Stream.Put(stream)
}

// backend2Stream
type backend2Stream struct {
	// Mixins
	_httpStream_
	// Assocs
	request  backend2Request
	response backend2Response
	socket   *backend2Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *backend2Conn
	id   uint32
	// Stream states (zeros)
}

func (s *backend2Stream) onUse(conn *backend2Conn, id uint32) { // for non-zeros
	s._httpStream_.onUse()
	s.conn = conn
	s.id = id
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
	s.conn = nil
	s._httpStream_.onEnd()
}

func (s *backend2Stream) Request() backendRequest   { return &s.request }
func (s *backend2Stream) Response() backendResponse { return &s.response }
func (s *backend2Stream) Socket() backendSocket     { return nil } // TODO

func (s *backend2Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *backend2Stream) ExecuteSocket() error { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
	return nil
}

func (s *backend2Stream) httpServend() httpServend { return s.conn.HTTPBackend() }
func (s *backend2Stream) httpConn() httpConn       { return s.conn }
func (s *backend2Stream) remoteAddr() net.Addr     { return s.conn.netConn.RemoteAddr() }

func (s *backend2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream
func (s *backend2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream

func (s *backend2Stream) setWriteDeadline() error { // for content i/o only?
	// TODO
	return nil
}
func (s *backend2Stream) setReadDeadline() error { // for content i/o only?
	// TODO
	return nil
}

func (s *backend2Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *backend2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *backend2Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *backend2Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

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
func (r *backend2Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
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
func (r *backend2Request) proxyCopyCookies(req Request) bool { // DO NOT merge into one "cookie" header!
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

func (r *backend2Request) passHeaders() error       { return r.writeHeaders2() }
func (r *backend2Request) passBytes(p []byte) error { return r.passBytes2(p) }

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

func (r *backend2Response) readContent() (p []byte, err error) { return r.readContent2() }

// poolBackend2Socket
var poolBackend2Socket sync.Pool

func getBackend2Socket(stream *backend2Stream) *backend2Socket {
	return nil
}
func putBackend2Socket(socket *backend2Socket) {
}

// backend2Socket is the backend-side HTTP/2 websocket.
type backend2Socket struct {
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *backend2Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *backend2Socket) onEnd() {
	s.backendSocket_.onEnd()
}
