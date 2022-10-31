// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 client implementation.

// For simplicity, HTTP/2 Server Push is not supported.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerBackend("http2Backend", func(name string, stage *Stage) backend {
		b := new(HTTP2Backend)
		b.init(name, stage)
		return b
	})
}

// HTTP2Backend
type HTTP2Backend struct {
	// Mixins
	httpBackend_
	// States
	nodes []*http2Node
}

func (b *HTTP2Backend) init(name string, stage *Stage) {
	b.httpBackend_.init(name, stage)
}

func (b *HTTP2Backend) OnConfigure() {
	b.httpBackend_.onConfigure()
}
func (b *HTTP2Backend) OnPrepare() {
	b.httpBackend_.onPrepare(len(b.nodes))
}
func (b *HTTP2Backend) OnShutdown() {
	b.httpBackend_.onShutdown()
}

func (b *HTTP2Backend) maintain() { // blocking
	for _, node := range b.nodes {
		node.checkHealth()
		time.Sleep(time.Second)
	}
}

func (b *HTTP2Backend) FetchConn() (*H2Conn, error) {
	node := b.nodes[b.getIndex()]
	return node.fetchConn()
}
func (b *HTTP2Backend) StoreConn(conn *H2Conn) {
	conn.node.storeConn(conn)
}

// http2Node
type http2Node struct {
	// Mixins
	httpNode_
	// Assocs
	backend *HTTP2Backend
	// States
}

func (n *http2Node) init(id int32, backend *HTTP2Backend) {
	n.httpNode_.init(id)
	n.backend = backend
}

func (n *http2Node) checkHealth() {
	// TODO
	for {
		time.Sleep(time.Second)
	}
}

func (n *http2Node) fetchConn() (*H2Conn, error) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
	var netConn net.Conn
	var rawConn syscall.RawConn
	connID := n.backend.nextConnID()
	return getH2Conn(connID, n.backend, n, netConn, rawConn), nil
}
func (n *http2Node) storeConn(hConn *H2Conn) {
	// Note: An H2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolH2Conn is the client-side HTTP/2 connection pool.
var poolH2Conn sync.Pool

func getH2Conn(id int64, client httpClient, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *H2Conn {
	var conn *H2Conn
	if x := poolH2Conn.Get(); x == nil {
		conn = new(H2Conn)
	} else {
		conn = x.(*H2Conn)
	}
	conn.onGet(id, client, node, netConn, rawConn)
	return conn
}
func putH2Conn(conn *H2Conn) {
	conn.onPut()
	poolH2Conn.Put(conn)
}

// H2Conn
type H2Conn struct {
	// Mixins
	hConn_
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *http2Node // belonging node
	netConn net.Conn   // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *H2Conn) onGet(id int64, client httpClient, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.hConn_.onGet(id, client)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *H2Conn) onPut() {
	c.hConn_.onPut()
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *H2Conn) FetchStream() *H2Stream {
	// TODO
	return nil
}
func (c *H2Conn) StoreStream(stream *H2Stream) {
	// TODO
}

func (c *H2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= c.client.WriteTimeout()/4 {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *H2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= c.client.ReadTimeout()/4 {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *H2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *H2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}
func (c *H2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}

func (c *H2Conn) closeConn() { c.netConn.Close() }

// poolH2Stream
var poolH2Stream sync.Pool

func getH2Stream(conn *H2Conn, id uint32) *H2Stream {
	var stream *H2Stream
	if x := poolH2Stream.Get(); x == nil {
		stream = new(H2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*H2Stream)
	}
	stream.onUse(conn, id)
	return stream
}
func putH2Stream(stream *H2Stream) {
	stream.onEnd()
	poolH2Stream.Put(stream)
}

// H2Stream
type H2Stream struct {
	// Mixins
	hStream_
	// Assocs
	request  H2Request
	response H2Response
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *H2Conn
	id   uint32
	// Stream states (zeros)
	h2Stream0 // all values must be zero by default in this struct!
}
type h2Stream0 struct { // for fast reset, entirely
}

func (s *H2Stream) onUse(conn *H2Conn, id uint32) { // for non-zeros
	s.hStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse()
	s.response.versionCode = Version2 // explicitly set
	s.response.onUse()
}
func (s *H2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.hStream_.onEnd()
	s.conn = nil
	s.h2Stream0 = h2Stream0{}
}

func (s *H2Stream) forwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *H2Stream) reverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *H2Stream) Request() *H2Request   { return &s.request }
func (s *H2Stream) Response() *H2Response { return &s.response }
func (s *H2Stream) Socket() *H2Socket {
	// TODO
	return nil
}

func (s *H2Stream) getHolder() holder { return s.conn.getClient() }

func (s *H2Stream) peerAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *H2Stream) makeTempName(p []byte, seconds int64) (from int, edge int) {
	return s.conn.makeTempName(p, seconds)
}

func (s *H2Stream) setWriteDeadline(deadline time.Time) error { // for content only
	return nil
}
func (s *H2Stream) setReadDeadline(deadline time.Time) error { // for content only
	return nil
}

func (s *H2Stream) write(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *H2Stream) writev(vector *net.Buffers) (int64, error) { // for content only
	return 0, nil
}
func (s *H2Stream) read(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *H2Stream) readFull(p []byte) (int, error) { // for content only
	return 0, nil
}

func (s *H2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?
func (s *H2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?

// H2Request
type H2Request struct {
	// Mixins
	hRequest_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Request) setControl(method []byte, uri []byte, hasContent bool) bool {
	// TODO
	return false
}
func (r *H2Request) control() []byte {
	return nil
}

func (r *H2Request) header(name []byte) (value []byte, ok bool) {
	return r.header2(name)
}
func (r *H2Request) addHeader(name []byte, value []byte) bool {
	return r.addHeader2(name, value)
}
func (r *H2Request) delHeader(name []byte) (deleted bool) {
	return r.delHeader2(name)
}
func (r *H2Request) addedHeaders() []byte {
	return nil
}
func (r *H2Request) fixedHeaders() []byte {
	return nil
}

func (r *H2Request) doSend(chain Chain) error {
	// TODO
	return nil
}

func (r *H2Request) pushHeaders() error {
	// TODO
	return nil
}
func (r *H2Request) doPush(chain Chain) error {
	// TODO
	return nil
}
func (r *H2Request) addTrailer(name []byte, value []byte) bool {
	// TODO
	return false
}

func (r *H2Request) passHeaders() error {
	return nil
}
func (r *H2Request) doPass(p []byte) error {
	return nil
}

func (r *H2Request) finalizeHeaders() {
	// TODO
}
func (r *H2Request) finalizeChunked() error {
	// TODO
	return nil
}

// H2Response
type H2Response struct {
	// Mixins
	hResponse_
	// Assocs
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H2Response) joinHeaders(p []byte) bool {
	// TODO
	return false
}

func (r *H2Response) readContent() (p []byte, err error) {
	return r.readContent2()
}

func (r *H2Response) joinTrailers(p []byte) bool {
	// TODO
	return false
}

// H2Socket is the client-side HTTP/2 websocket.
type H2Socket struct {
	// Mixins
	hSocket_
	// Stream states (zeros)
}

func (s *H2Socket) onUse() {
	s.hSocket_.onUse()
}
func (s *H2Socket) onEnd() {
	s.hSocket_.onEnd()
}