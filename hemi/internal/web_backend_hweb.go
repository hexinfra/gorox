// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB backend implementation.

package internal

import (
	"net"
	"sync"
	"time"
)

func init() {
	RegisterBackend("hwebBackend", func(name string, stage *Stage) Backend {
		b := new(HWEBBackend)
		b.onCreate(name, stage)
		return b
	})
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
	Node_
	// Assocs
	backend *HWEBBackend
	// States
}

func (n *hwebNode) init(id int32, backend *HWEBBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *hwebNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("hwebNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

// poolHWConn is the backend-side HWEB connection pool.
var poolHWConn sync.Pool

func getHWConn(id int64, udsMode bool, tlsMode bool, backend webBackend, node *hwebNode, tcpConn *net.TCPConn) *HWConn {
	var hwConn *HWConn
	if x := poolHWConn.Get(); x == nil {
		hwConn = new(HWConn)
	} else {
		hwConn = x.(*HWConn)
	}
	hwConn.onGet(id, udsMode, tlsMode, backend, node, tcpConn)
	return hwConn
}
func putHWConn(hwConn *HWConn) {
	hwConn.onPut()
	poolHWConn.Put(hwConn)
}

// HWConn
type HWConn struct {
	// Mixins
	backendConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hwebNode
	tcpConn *net.TCPConn // the underlying tcp conn
	// Conn states (zeros)
	activeExchans int32 // concurrent exchans
}

func (c *HWConn) onGet(id int64, udsMode bool, tlsMode bool, backend webBackend, node *hwebNode, tcpConn *net.TCPConn) {
	c.backendConn_.onGet(id, udsMode, tlsMode, backend)
	c.node = node
	c.tcpConn = tcpConn
}
func (c *HWConn) onPut() {
	c.backendConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.activeExchans = 0
}

func (c *HWConn) FetchExchan() *HWExchan {
	// TODO: exchan.onUse()
	return nil
}
func (c *HWConn) StoreExchan(exchan *HWExchan) {
	// TODO
	exchan.onEnd()
}

func (c *HWConn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *HWConn) closeConn() { c.tcpConn.Close() } // used by codes which use fetch/store

// poolHWExchan
var poolHWExchan sync.Pool

func getHWExchan(node *hwebNode, id int32) *HWExchan {
	// TODO
	return nil
}
func putHWExchan(exchan *HWExchan) {
	exchan.onEnd()
	poolHWExchan.Put(exchan)
}

// HWExchan is the backend-side HWEB exchan.
type HWExchan struct {
	// Mixins
	backendStream_
	// Assocs
	request  HWRequest
	response HWResponse
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

func (x *HWExchan) onUse(node *hwebNode, id int32) { // for non-zeros
	x.backendStream_.onUse()
	x.node = node
	x.id = id
	x.request.onUse(Version2)
	x.response.onUse(Version2)
}
func (x *HWExchan) onEnd() { // for zeros
	x.response.onEnd()
	x.request.onEnd()
	x.node = nil
	x.hExchan0 = hExchan0{}
	x.backendStream_.onEnd()
}

func (x *HWExchan) webBroker() webBroker { return nil } // TODO
func (x *HWExchan) webConn() webConn     { return nil } // TODO
func (x *HWExchan) remoteAddr() net.Addr { return nil } // TODO

func (x *HWExchan) Request() *HWRequest   { return &x.request }
func (x *HWExchan) Response() *HWResponse { return &x.response }

func (x *HWExchan) ExecuteExchan() error { // request & response
	// TODO
	return nil
}

func (x *HWExchan) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (x *HWExchan) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (x *HWExchan) makeTempName(p []byte, unixTime int64) int {
	// TODO
	return 0
}

func (x *HWExchan) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (x *HWExchan) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (x *HWExchan) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (x *HWExchan) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (x *HWExchan) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (x *HWExchan) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (x *HWExchan) isBroken() bool { return false } // TODO: limit the breakage in the exchan
func (x *HWExchan) markBroken()    {}               // TODO: limit the breakage in the exchan

// HWRequest is the backend-side HWEB request.
type HWRequest struct { // outgoing. needs building
	// Mixins
	backendRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *HWRequest) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :target = uri
	// TODO: set :method and :uri
	return false
}
func (r *HWRequest) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *HWRequest) addHeader(name []byte, value []byte) bool   { return r.addHeaderH(name, value) }
func (r *HWRequest) header(name []byte) (value []byte, ok bool) { return r.headerH(name) }
func (r *HWRequest) hasHeader(name []byte) bool                 { return r.hasHeaderH(name) }
func (r *HWRequest) delHeader(name []byte) (deleted bool)       { return r.delHeaderH(name) }
func (r *HWRequest) delHeaderAt(i uint8)                        { r.delHeaderAtH(i) }

func (r *HWRequest) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *HWRequest) copyCookies(req Request) bool { // used by proxies. merge into one "cookie" header?
	// TODO: one by one?
	return true
}

func (r *HWRequest) sendChain() error { return r.sendChainH() }

func (r *HWRequest) echoHeaders() error { return r.writeHeadersH() }
func (r *HWRequest) echoChain() error   { return r.echoChainH() }

func (r *HWRequest) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerH(name, value)
}
func (r *HWRequest) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerH(name)
}

func (r *HWRequest) passHeaders() error       { return r.writeHeadersH() }
func (r *HWRequest) passBytes(p []byte) error { return r.passBytesH(p) }

func (r *HWRequest) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *HWRequest) finalizeVague() error {
	// TODO
	return nil
}

func (r *HWRequest) addedHeaders() []byte { return nil } // TODO
func (r *HWRequest) fixedHeaders() []byte { return nil } // TODO

// HWResponse is the backend-side HWEB response.
type HWResponse struct { // incoming. needs parsing
	// Mixins
	backendResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *HWResponse) readContent() (p []byte, err error) { return r.readContentH() }
