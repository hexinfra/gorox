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

// poolPStream
var poolPStream sync.Pool

func getPStream(node *happNode, id int32) *PStream {
	var stream *PStream
	if x := poolPStream.Get(); x == nil {
		stream = new(PStream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		stream = x.(*PStream)
	}
	stream.onUse(node, id)
	return stream
}
func putPStream(stream *PStream) {
	stream.onEnd()
	poolPStream.Put(stream)
}

// PStream
type PStream struct {
	// Mixins
	webStream_
	// Assocs
	request  PRequest
	response PResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	node *happNode
	id   int32
	// Stream states (zeros)
	b2Stream0 // all values must be zero by default in this struct!
}
type b2Stream0 struct { // for fast reset, entirely
}

func (s *PStream) onUse(node *happNode, id int32) { // for non-zeros
	s.webStream_.onUse()
	s.node = node
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *PStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.node = nil
	s.b2Stream0 = b2Stream0{}
	s.webStream_.onEnd()
}

func (s *PStream) webAgent() webAgent { return nil }
func (s *PStream) peerAddr() net.Addr { return nil }

func (s *PStream) Request() *PRequest   { return &s.request }
func (s *PStream) Response() *PResponse { return &s.response }

func (s *PStream) ExecuteNormal() error { // request & response
	// TODO
	return nil
}

func (s *PStream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}
func (s *PStream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) {
	// TODO
}

func (s *PStream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	// TODO
	return
}

func (s *PStream) setWriteDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}
func (s *PStream) setReadDeadline(deadline time.Time) error { // for content i/o only?
	return nil
}

func (s *PStream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *PStream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *PStream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *PStream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (s *PStream) isBroken() bool { return false } // TODO: limit the breakage in the stream
func (s *PStream) markBroken()    {}               // TODO: limit the breakage in the stream

// PRequest is the client-side HAPP request.
type PRequest struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
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
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *PResponse) readContent() (p []byte, err error) { return r.readContentP() }
