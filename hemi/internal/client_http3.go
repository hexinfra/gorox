// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 client implementation.

// For simplicity, HTTP/3 Server Push is not supported.

package internal

import (
	"github.com/hexinfra/gorox/hemi/libraries/quix"
	"net"
	"sync"
	"time"
)

func init() {
	registerFixture(signHTTP3)
	registerBackend("http3Backend", func(name string, stage *Stage) backend {
		b := new(HTTP3Backend)
		b.onCreate(name, stage)
		return b
	})
}

const signHTTP3 = "http3"

func createHTTP3(stage *Stage) *HTTP3Outgate {
	http3 := new(HTTP3Outgate)
	http3.onCreate(stage)
	http3.setShell(http3)
	return http3
}

// HTTP3Outgate
type HTTP3Outgate struct {
	// Mixins
	httpOutgate_
	// States
}

func (f *HTTP3Outgate) onCreate(stage *Stage) {
	f.httpOutgate_.onCreate(signHTTP3, stage)
}
func (f *HTTP3Outgate) OnShutdown() {
	f.Shutdown()
}

func (f *HTTP3Outgate) OnConfigure() {
	f.httpOutgate_.onConfigure(f)
}
func (f *HTTP3Outgate) OnPrepare() {
	f.httpOutgate_.onPrepare(f)
}

func (f *HTTP3Outgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("http3 done")
	}
	f.stage.SubDone()
}

func (f *HTTP3Outgate) FetchConn(address string, tlsMode bool) (*H3Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP3Outgate) StoreConn(conn *H3Conn) {
	// TODO
}

// HTTP3Backend
type HTTP3Backend struct {
	// Mixins
	httpBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) onCreate(name string, stage *Stage) {
	b.httpBackend_.onCreate(name, stage, b)
}
func (b *HTTP3Backend) OnShutdown() {
	b.Shutdown()
}

func (b *HTTP3Backend) OnConfigure() {
	b.httpBackend_.onConfigure(b)
}
func (b *HTTP3Backend) OnPrepare() {
	b.httpBackend_.onPrepare(b, len(b.nodes))
}

func (b *HTTP3Backend) createNode(id int32) *http3Node {
	n := new(http3Node)
	n.init(id, b)
	return n
}

func (b *HTTP3Backend) FetchConn() (*H3Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *HTTP3Backend) StoreConn(conn *H3Conn) {
	conn.node.storeConn(conn)
}

// http3Node
type http3Node struct {
	// Mixins
	node_
	// Assocs
	backend *HTTP3Backend
	// States
}

func (n *http3Node) init(id int32, backend *HTTP3Backend) {
	n.node_.init(id)
	n.backend = backend
}

func (n *http3Node) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("http3Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *http3Node) fetchConn() (*H3Conn, error) {
	// Note: An H3Conn can be used concurrently, limited by maxStreams.
	// TODO
	conn, err := quix.DialTimeout(n.address, n.backend.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := n.backend.nextConnID()
	return getH3Conn(connID, n.backend, n, conn), nil
}
func (n *http3Node) storeConn(hConn *H3Conn) {
	// Note: An H3Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolH3Conn is the client-side HTTP/3 connection pool.
var poolH3Conn sync.Pool

func getH3Conn(id int64, client httpClient, node *http3Node, quicConn *quix.Conn) *H3Conn {
	var conn *H3Conn
	if x := poolH3Conn.Get(); x == nil {
		conn = new(H3Conn)
	} else {
		conn = x.(*H3Conn)
	}
	conn.onGet(id, client, node, quicConn)
	return conn
}
func putH3Conn(conn *H3Conn) {
	conn.onPut()
	poolH3Conn.Put(conn)
}

// H3Conn
type H3Conn struct {
	// Mixins
	hConn_
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node     *http3Node
	quicConn *quix.Conn // the underlying quic conn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *H3Conn) onGet(id int64, client httpClient, node *http3Node, quicConn *quix.Conn) {
	c.hConn_.onGet(id, client)
	c.node = node
	c.quicConn = quicConn
}
func (c *H3Conn) onPut() {
	c.hConn_.onPut()
	c.node = nil
	c.quicConn = nil
	c.activeStreams = 0
}

func (c *H3Conn) FetchStream() *H3Stream {
	// TODO
	return nil
}
func (c *H3Conn) StoreStream(stream *H3Stream) {
	// TODO
}

func (c *H3Conn) closeConn() { c.quicConn.Close() }

// poolH3Stream
var poolH3Stream sync.Pool

func getH3Stream(conn *H3Conn, quicStream *quix.Stream) *H3Stream {
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
	hStream_
	// Assocs
	request  H3Request
	response H3Response
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *H3Conn
	quicStream *quix.Stream // the underlying quic stream
	// Stream states (zeros)
	h3Stream0 // all values must be zero by default in this struct!
}
type h3Stream0 struct { // for fast reset, entirely
}

func (s *H3Stream) onUse(conn *H3Conn, quicStream *quix.Stream) { // for non-zeros
	s.hStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse()
	s.response.versionCode = Version3 // explicitly set
	s.response.onUse()
}
func (s *H3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.hStream_.onEnd()
	s.conn = nil
	s.quicStream = nil
	s.h3Stream0 = h3Stream0{}
}

func (s *H3Stream) holder() holder {
	return s.conn.getClient()
}
func (s *H3Stream) peerAddr() net.Addr {
	// TODO
	return nil
}

func (s *H3Stream) Request() *H3Request   { return &s.request }
func (s *H3Stream) Response() *H3Response { return &s.response }
func (s *H3Stream) Socket() *H3Socket {
	// TODO
	return nil
}

func (s *H3Stream) forwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *H3Stream) reverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *H3Stream) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return s.conn.makeTempName(p, stamp)
}

func (s *H3Stream) setWriteDeadline(deadline time.Time) error { // for content only
	return nil
}
func (s *H3Stream) setReadDeadline(deadline time.Time) error { // for content only
	return nil
}

func (s *H3Stream) write(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *H3Stream) writev(vector *net.Buffers) (int64, error) { // for content only
	return 0, nil
}
func (s *H3Stream) read(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *H3Stream) readFull(p []byte) (int, error) { // for content only
	return 0, nil
}

func (s *H3Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?
func (s *H3Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?

// H3Request is the client-side HTTP/3 request.
type H3Request struct {
	// Mixins
	hRequest_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H3Request) setControl(method []byte, uri []byte, hasContent bool) bool {
	// TODO
	return false
}
func (r *H3Request) control() []byte {
	// TODO
	return nil
}
func (r *H3Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO
	return false
}

func (r *H3Request) header(name []byte) (value []byte, ok bool) {
	return r.header3(name)
}
func (r *H3Request) hasHeader(name []byte) bool {
	return r.hasHeader3(name)
}
func (r *H3Request) addHeader(name []byte, value []byte) bool {
	return r.addHeader3(name, value)
}
func (r *H3Request) delHeader(name []byte) (deleted bool) {
	return r.delHeader3(name)
}
func (r *H3Request) delHeaderAt(o uint8) {
	r.delHeaderAt3(o)
}
func (r *H3Request) addedHeaders() []byte {
	return nil
}
func (r *H3Request) fixedHeaders() []byte {
	return nil
}

func (r *H3Request) AddCookie(name string, value string) bool {
	// TODO
	return false
}
func (r *H3Request) copyCookies(req Request) bool { // used by proxies
	// TODO: one by one?
	return true
}

func (r *H3Request) sendChain(chain Chain) error {
	// TODO
	return nil
}

func (r *H3Request) pushHeaders() error {
	// TODO
	return nil
}
func (r *H3Request) pushChain(chain Chain) error {
	// TODO
	return nil
}

func (r *H3Request) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer3(name)
}
func (r *H3Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}

func (r *H3Request) passHeaders() error {
	return nil
}
func (r *H3Request) passBytes(p []byte) error {
	return nil
}

func (r *H3Request) finalizeHeaders() {
	// TODO
}
func (r *H3Request) finalizeChunked() error {
	// TODO
	return nil
}

// H3Response is the client-side HTTP/3 response.
type H3Response struct {
	// Mixins
	hResponse_
	// Assocs
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H3Response) joinHeaders(p []byte) bool {
	// TODO
	return false
}

func (r *H3Response) readContent() (p []byte, err error) {
	return r.readContent3()
}

func (r *H3Response) joinTrailers(p []byte) bool {
	// TODO
	return false
}

// H3Socket is the client-side HTTP/3 websocket.
type H3Socket struct {
	// Mixins
	hSocket_
	// Stream states (zeros)
}

func (s *H3Socket) onUse() {
	s.hSocket_.onUse()
}
func (s *H3Socket) onEnd() {
	s.hSocket_.onEnd()
}
