// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 client implementation. See RFC 9114 and 9204.

// For simplicity, HTTP/3 Server Push is not supported.

package internal

import (
	"net"
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

func init() {
	registerFixture(signHTTP3Outgate)
	RegisterBackend("http3Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP3Backend)
		b.onCreate(name, stage)
		return b
	})
}

const signHTTP3Outgate = "http3Outgate"

func createHTTP3Outgate(stage *Stage) *HTTP3Outgate {
	http3 := new(HTTP3Outgate)
	http3.onCreate(stage)
	http3.setShell(http3)
	return http3
}

// HTTP3Outgate
type HTTP3Outgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HTTP3Outgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHTTP3Outgate, stage)
}

func (f *HTTP3Outgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HTTP3Outgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HTTP3Outgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("http3Outgate done")
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
	webBackend_[*http3Node]
	// States
}

func (b *HTTP3Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HTTP3Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HTTP3Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HTTP3Backend) createNode(id int32) *http3Node {
	node := new(http3Node)
	node.init(id, b)
	return node
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
	webNode_
	// Assocs
	backend *HTTP3Backend
	// States
}

func (n *http3Node) init(id int32, backend *HTTP3Backend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *http3Node) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("http3Node=%d done\n", n.id)
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
func (n *http3Node) storeConn(h3Conn *H3Conn) {
	// Note: An H3Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolH3Conn is the client-side HTTP/3 connection pool.
var poolH3Conn sync.Pool

func getH3Conn(id int64, client webClient, node *http3Node, quicConnection *quix.Connection) *H3Conn {
	var conn *H3Conn
	if x := poolH3Conn.Get(); x == nil {
		conn = new(H3Conn)
	} else {
		conn = x.(*H3Conn)
	}
	conn.onGet(id, client, node, quicConnection)
	return conn
}
func putH3Conn(conn *H3Conn) {
	conn.onPut()
	poolH3Conn.Put(conn)
}

// H3Conn
type H3Conn struct {
	// Mixins
	clientConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node           *http3Node
	quicConnection *quix.Connection // the underlying quic connection
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *H3Conn) onGet(id int64, client webClient, node *http3Node, quicConnection *quix.Connection) {
	c.clientConn_.onGet(id, client)
	c.node = node
	c.quicConnection = quicConnection
}
func (c *H3Conn) onPut() {
	c.clientConn_.onPut()
	c.node = nil
	c.quicConnection = nil
	c.activeStreams = 0
}

func (c *H3Conn) FetchStream() *H3Stream {
	// TODO: stream.onUse()
	return nil
}
func (c *H3Conn) StoreStream(stream *H3Stream) {
	// TODO
	stream.onEnd()
}

func (c *H3Conn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *H3Conn) closeConn() { c.quicConnection.Close() } // used by codes which use fetch/store

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
	clientStream_
	// Assocs
	request  H3Request
	response H3Response
	socket   *H3Socket
	// Stream states (stocks)
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
	s.clientStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *H3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.socket = nil
	s.conn = nil
	s.quicStream = nil
	s.h3Stream0 = h3Stream0{}
	s.clientStream_.onEnd()
}

func (s *H3Stream) webBroker() webBroker { return s.conn.getClient() }
func (s *H3Stream) remoteAddr() net.Addr { return nil } // TODO

func (s *H3Stream) Request() *H3Request   { return &s.request }
func (s *H3Stream) Response() *H3Response { return &s.response }

func (s *H3Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *H3Stream) ExecuteSocket() *H3Socket { // see RFC 9220
	// TODO, use s.startSocket()
	return s.socket
}
func (s *H3Stream) ExecuteTCPTun() { // CONNECT method
	// TODO, use s.startTCPTun()
}
func (s *H3Stream) ExecuteUDPTun() { // see RFC 9298
	// TODO, use s.startUDPTun()
}

func (s *H3Stream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *H3Stream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *H3Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *H3Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *H3Stream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

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

func (s *H3Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *H3Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// H3Request is the client-side HTTP/3 request.
type H3Request struct { // outgoing. needs building
	// Mixins
	clientRequest_
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
func (r *H3Request) copyCookies(req Request) bool { // used by proxies. DO NOT merge into one "cookie" header
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

// H3Response is the client-side HTTP/3 response.
type H3Response struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H3Response) readContent() (p []byte, err error) { return r.readContent3() }

// poolH3Socket
var poolH3Socket sync.Pool

// H3Socket is the client-side HTTP/3 websocket.
type H3Socket struct {
	// Mixins
	clientSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
