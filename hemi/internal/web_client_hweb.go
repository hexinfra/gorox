// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB client implementation.

package internal

import (
	"net"
	"sync"
	"time"
)

func init() {
	registerFixture(signHWEBOutgate)
	RegisterBackend("hwebBackend", func(name string, stage *Stage) Backend {
		b := new(HWEBBackend)
		b.onCreate(name, stage)
		return b
	})
}

const signHWEBOutgate = "hwebOutgate"

func createHWEBOutgate(stage *Stage) *HWEBOutgate {
	hweb := new(HWEBOutgate)
	hweb.onCreate(stage)
	hweb.setShell(hweb)
	return hweb
}

// HWEBOutgate
type HWEBOutgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HWEBOutgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHWEBOutgate, stage)
}

func (f *HWEBOutgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HWEBOutgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HWEBOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("hwebOutgate done")
	}
	f.stage.SubDone()
}

// HWEBBackend
type HWEBBackend struct {
	// Mixins
	webBackend_[*hwebNode]
	// States
}

func (b *HWEBBackend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HWEBBackend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HWEBBackend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HWEBBackend) createNode(id int32) *hwebNode {
	node := new(hwebNode)
	node.init(id, b)
	return node
}

// hwebNode
type hwebNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *HWEBBackend
	// States
}

func (n *hwebNode) init(id int32, backend *HWEBBackend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hwebNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("hwebNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

// poolHConn is the client-side HWEB connection pool.
var poolHConn sync.Pool

func getHConn(id int64, client webClient, node *hwebNode, tcpConn *net.TCPConn) *HConn {
	var conn *HConn
	if x := poolHConn.Get(); x == nil {
		conn = new(HConn)
	} else {
		conn = x.(*HConn)
	}
	conn.onGet(id, client, node, tcpConn)
	return conn
}
func putHConn(conn *HConn) {
	conn.onPut()
	poolHConn.Put(conn)
}

// HConn
type HConn struct {
	// Mixins
	clientConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hwebNode
	tcpConn *net.TCPConn // the underlying tcp conn
	// Conn states (zeros)
	activeExchans int32 // concurrent exchans
}

func (c *HConn) onGet(id int64, client webClient, node *hwebNode, tcpConn *net.TCPConn) {
	c.clientConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
}
func (c *HConn) onPut() {
	c.clientConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.activeExchans = 0
}

func (c *HConn) FetchExchan() *HExchan {
	// TODO: exchan.onUse()
	return nil
}
func (c *HConn) StoreExchan(exchan *HExchan) {
	// TODO
	exchan.onEnd()
}

func (c *HConn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *HConn) closeConn() { c.tcpConn.Close() } // used by codes which use fetch/store

// poolHExchan
var poolHExchan sync.Pool

func getHExchan(node *hwebNode, id int32) *HExchan {
	// TODO
	return nil
}
func putHExchan(exchan *HExchan) {
	exchan.onEnd()
	poolHExchan.Put(exchan)
}

// HExchan is the client-side HWEB exchan.
type HExchan struct {
	// Mixins
	clientStream_
	// Assocs
	request  HRequest
	response HResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	node *hwebNode
	id   int32
	// Exchan states (zeros)
	hExchan0 // all values must be zero by default in this struct!
}
type hExchan0 struct { // for fast reset, entirely
}

func (x *HExchan) onUse(node *hwebNode, id int32) { // for non-zeros
	x.clientStream_.onUse()
	x.node = node
	x.id = id
	x.request.onUse(Version2)
	x.response.onUse(Version2)
}
func (x *HExchan) onEnd() { // for zeros
	x.response.onEnd()
	x.request.onEnd()
	x.node = nil
	x.hExchan0 = hExchan0{}
	x.clientStream_.onEnd()
}

func (x *HExchan) webKeeper() webKeeper { return nil }
func (x *HExchan) peerAddr() net.Addr   { return nil }

func (x *HExchan) Request() *HRequest   { return &x.request }
func (x *HExchan) Response() *HResponse { return &x.response }

func (x *HExchan) ExecuteExchan() error { // request & response
	// TODO
	return nil
}

func (x *HExchan) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (x *HExchan) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (x *HExchan) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	// TODO
	return
}

func (x *HExchan) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (x *HExchan) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (x *HExchan) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (x *HExchan) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (x *HExchan) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (x *HExchan) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (x *HExchan) isBroken() bool { return false } // TODO: limit the breakage in the exchan
func (x *HExchan) markBroken()    {}               // TODO: limit the breakage in the exchan

// HRequest is the client-side HWEB request.
type HRequest struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *HRequest) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :uri
	return false
}
func (r *HRequest) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *HRequest) addHeader(name []byte, value []byte) bool   { return r.addHeaderH(name, value) }
func (r *HRequest) header(name []byte) (value []byte, ok bool) { return r.headerH(name) }
func (r *HRequest) hasHeader(name []byte) bool                 { return r.hasHeaderH(name) }
func (r *HRequest) delHeader(name []byte) (deleted bool)       { return r.delHeaderH(name) }
func (r *HRequest) delHeaderAt(o uint8)                        { r.delHeaderAtH(o) }

func (r *HRequest) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *HRequest) copyCookies(req Request) bool { // used by proxies. merge into one "cookie" header?
	// TODO: one by one?
	return true
}

func (r *HRequest) sendChain() error { return r.sendChainH() }

func (r *HRequest) echoHeaders() error { return r.writeHeadersH() }
func (r *HRequest) echoChain() error   { return r.echoChainH() }

func (r *HRequest) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerH(name, value)
}
func (r *HRequest) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerH(name)
}

func (r *HRequest) passHeaders() error       { return r.writeHeadersH() }
func (r *HRequest) passBytes(p []byte) error { return r.passBytesH(p) }

func (r *HRequest) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *HRequest) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *HRequest) addedHeaders() []byte { return nil } // TODO
func (r *HRequest) fixedHeaders() []byte { return nil } // TODO

// HResponse is the client-side HWEB response.
type HResponse struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *HResponse) readContent() (p []byte, err error) { return r.readContentH() }
