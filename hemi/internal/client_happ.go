// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP client implementation.

package internal

import (
	"net"
	"sync"
	"time"
)

func init() {
	registerFixture(signHAPPOutgate)
	RegisterBackend("happBackend", func(name string, stage *Stage) Backend {
		b := new(HAPPBackend)
		b.onCreate(name, stage)
		return b
	})
}

const signHAPPOutgate = "happOutgate"

func createHAPPOutgate(stage *Stage) *HAPPOutgate {
	happ := new(HAPPOutgate)
	happ.onCreate(stage)
	happ.setShell(happ)
	return happ
}

// HAPPOutgate
type HAPPOutgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HAPPOutgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHAPPOutgate, stage)
}

func (f *HAPPOutgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HAPPOutgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HAPPOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("happOutgate done")
	}
	f.stage.SubDone()
}

// HAPPBackend
type HAPPBackend struct {
	// Mixins
	webBackend_[*happNode]
	// States
}

func (b *HAPPBackend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HAPPBackend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HAPPBackend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HAPPBackend) createNode(id int32) *happNode {
	node := new(happNode)
	node.init(id, b)
	return node
}

// happNode
type happNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *HAPPBackend
	// States
}

func (n *happNode) init(id int32, backend *HAPPBackend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *happNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("happNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

// poolPExchan
var poolPExchan sync.Pool

func getPExchan(node *happNode, id int32) *PExchan {
	// TODO
	return nil
}
func putPExchan(exchan *PExchan) {
	exchan.onEnd()
	poolPExchan.Put(exchan)
}

// PExchan
type PExchan struct {
	// Mixins
	clientStream_
	// Assocs
	request  PRequest
	response PResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	node *happNode
	id   int32
	// Exchan states (zeros)
	pExchan0 // all values must be zero by default in this struct!
}
type pExchan0 struct { // for fast reset, entirely
}

func (x *PExchan) onUse(node *happNode, id int32) { // for non-zeros
	x.clientStream_.onUse()
	x.node = node
	x.id = id
	x.request.onUse(Version2)
	x.response.onUse(Version2)
}
func (x *PExchan) onEnd() { // for zeros
	x.response.onEnd()
	x.request.onEnd()
	x.node = nil
	x.pExchan0 = pExchan0{}
	x.clientStream_.onEnd()
}

func (x *PExchan) webAgent() webAgent { return nil }
func (x *PExchan) peerAddr() net.Addr { return nil }

func (x *PExchan) Request() *PRequest   { return &x.request }
func (x *PExchan) Response() *PResponse { return &x.response }

func (x *PExchan) ExecuteNormal() error { // request & response
	// TODO
	return nil
}

func (x *PExchan) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (x *PExchan) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (x *PExchan) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	// TODO
	return
}

func (x *PExchan) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (x *PExchan) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (x *PExchan) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (x *PExchan) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (x *PExchan) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (x *PExchan) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (x *PExchan) isBroken() bool { return false } // TODO: limit the breakage in the exchan
func (x *PExchan) markBroken()    {}               // TODO: limit the breakage in the exchan

// PRequest is the client-side HAPP request.
type PRequest struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *PRequest) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :uri
	return false
}
func (r *PRequest) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	// TODO: set :authority
	return false
}

func (r *PRequest) addHeader(name []byte, value []byte) bool   { return r.addHeaderP(name, value) }
func (r *PRequest) header(name []byte) (value []byte, ok bool) { return r.headerP(name) }
func (r *PRequest) hasHeader(name []byte) bool                 { return r.hasHeaderP(name) }
func (r *PRequest) delHeader(name []byte) (deleted bool)       { return r.delHeaderP(name) }
func (r *PRequest) delHeaderAt(o uint8)                        { r.delHeaderAtP(o) }

func (r *PRequest) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *PRequest) copyCookies(req Request) bool { // used by proxies. merge into one "cookie" header?
	// TODO: one by one?
	return true
}

func (r *PRequest) sendChain() error { return r.sendChainP() }

func (r *PRequest) echoHeaders() error { return r.writeHeadersP() }
func (r *PRequest) echoChain() error   { return r.echoChainP() }

func (r *PRequest) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerP(name, value)
}
func (r *PRequest) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerP(name)
}

func (r *PRequest) passHeaders() error       { return r.writeHeadersP() }
func (r *PRequest) passBytes(p []byte) error { return r.passBytesP(p) }

func (r *PRequest) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *PRequest) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *PRequest) addedHeaders() []byte { return nil } // TODO
func (r *PRequest) fixedHeaders() []byte { return nil } // TODO

// PResponse is the client-side HAPP response.
type PResponse struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *PResponse) readContent() (p []byte, err error) { return r.readContentP() }
