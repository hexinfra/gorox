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
		b.init(name, stage)
		return b
	})
}

const signHTTP3 = "http3"

func createHTTP3(stage *Stage) *HTTP3Outgate {
	http3 := new(HTTP3Outgate)
	http3.init(stage)
	http3.setShell(http3)
	return http3
}

// HTTP3Outgate
type HTTP3Outgate struct {
	// Mixins
	httpOutgate_
	// States
}

func (f *HTTP3Outgate) init(stage *Stage) {
	f.httpOutgate_.init(signHTTP3, stage)
}

func (f *HTTP3Outgate) OnConfigure() {
	f.httpOutgate_.onConfigure()
}
func (f *HTTP3Outgate) OnPrepare() {
	f.httpOutgate_.onPrepare()
}
func (f *HTTP3Outgate) OnShutdown() {
	f.httpOutgate_.onShutdown()
}

func (f *HTTP3Outgate) run() { // goroutine
	for {
		time.Sleep(time.Second)
	}
}

func (f *HTTP3Outgate) FetchConn(address string) (*H3Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP3Outgate) StoreConn(conn *H3Conn) {
	// TODO
}

// HTTP3Backend
type HTTP3Backend struct {
	// Mixins
	httpBackend_
	// States
	nodes []*http3Node
}

func (b *HTTP3Backend) init(name string, stage *Stage) {
	b.httpBackend_.init(name, stage)
}

func (b *HTTP3Backend) OnConfigure() {
	b.httpBackend_.onConfigure()
}
func (b *HTTP3Backend) OnPrepare() {
	b.httpBackend_.onPrepare(len(b.nodes))
}
func (b *HTTP3Backend) OnShutdown() {
	b.httpBackend_.onShutdown()
}

func (b *HTTP3Backend) maintain() { // goroutine
	for _, node := range b.nodes {
		node.checkHealth()
		time.Sleep(time.Second)
	}
}

func (b *HTTP3Backend) FetchConn() (*H3Conn, error) {
	node := b.nodes[b.getIndex()]
	return node.fetchConn()
}
func (b *HTTP3Backend) StoreConn(conn *H3Conn) {
	conn.node.storeConn(conn)
}

// http3Node
type http3Node struct {
	// Mixins
	httpNode_
	// Assocs
	backend *HTTP3Backend
	// States
}

func (n *http3Node) init(id int32, backend *HTTP3Backend) {
	n.httpNode_.init(id)
	n.backend = backend
}

func (n *http3Node) checkHealth() {
	// TODO
	for {
		time.Sleep(time.Second)
	}
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

func (s *H3Stream) getHolder() holder {
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

func (s *H3Stream) makeTempName(p []byte, seconds int64) (from int, edge int) {
	return s.conn.makeTempName(p, seconds)
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

// H3Request
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
	return nil
}

func (r *H3Request) header(name []byte) (value []byte, ok bool) {
	return r.header3(name)
}
func (r *H3Request) addHeader(name []byte, value []byte) bool {
	return r.addHeader3(name, value)
}
func (r *H3Request) delHeader(name []byte) (deleted bool) {
	return r.delHeader3(name)
}
func (r *H3Request) addedHeaders() []byte {
	return nil
}
func (r *H3Request) fixedHeaders() []byte {
	return nil
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
func (r *H3Request) addTrailer(name []byte, value []byte) bool {
	// TODO
	return false
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

// H3Response
type H3Response struct {
	// Mixins
	hResponse_
	// Assocs
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H3Response) readContent() (p []byte, err error) {
	return r.readContent3()
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
