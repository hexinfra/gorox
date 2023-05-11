// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP server implementation.

package internal

import (
	"net"
	"sync"
	"time"
)

func init() {
	RegisterServer("happServer", func(name string, stage *Stage) Server {
		s := new(happServer)
		s.onCreate(name, stage)
		return s
	})
}

// happServer is the HAPP server.
type happServer struct {
	// Mixins
	webServer_
	// States
}

func (s *happServer) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *happServer) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *happServer) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *happServer) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *happServer) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(happGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		s.IncSub(1)
		go gate.serve()
	}
	s.WaitSubs() // gates
	if IsDebug(2) {
		Debugf("happServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// happGate is a gate of happServer.
type happGate struct {
	// Mixins
	webGate_
	// Assocs
	server *happServer
	// States
}

func (g *happGate) init(server *happServer, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *happGate) open() error {
	// TODO
	return nil
}
func (g *happGate) shutdown() error {
	g.MarkShut()
	// TODO
	return nil
}

func (g *happGate) serve() { // goroutine
	// TODO
}

// poolHAPPExchan is the server-side HAPP exchan pool.
var poolHAPPExchan sync.Pool

func getHAPPExchan(gate *happGate, id uint32) *happExchan {
	// TODO
	return nil
}
func putHAPPExchan(exchan *happExchan) {
	// TODO
}

// happExchan is the server-side HAPP exchan.
type happExchan struct {
	// Mixins
	webStream_
	// Assocs
	request  happRequest
	response happResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	gate *happGate
	// Exchan states (zeros)
	happExchan0 // all values must be zero by default in this struct!
}
type happExchan0 struct { // for fast reset, entirely
}

func (x *happExchan) onUse(gate *happGate) { // for non-zeros
	x.webStream_.onUse()
	x.gate = gate
	x.request.onUse(Version2)
	x.response.onUse(Version2)
}
func (x *happExchan) onEnd() { // for zeros
	x.response.onEnd()
	x.request.onEnd()
	x.webStream_.onEnd()
	x.gate = nil
	x.happExchan0 = happExchan0{}
}

func (x *happExchan) execute() { // goroutine
	// TODO
	putHAPPExchan(x)
}

func (x *happExchan) webAgent() webAgent { return nil }
func (x *happExchan) peerAddr() net.Addr { return nil }

func (x *happExchan) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (x *happExchan) executeNormal(app *App, req *happRequest, resp *happResponse) { // request & response
	// TODO
	//app.dispatchHandlet(req, resp)
}
func (x *happExchan) serveAbnormal(req *happRequest, resp *happResponse) { // 4xx & 5xx
	// TODO
}

func (x *happExchan) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	// TODO
	return
}

func (x *happExchan) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (x *happExchan) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (x *happExchan) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (x *happExchan) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (x *happExchan) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (x *happExchan) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

func (x *happExchan) isBroken() bool { return false } // TODO: limit the breakage in the exchan
func (x *happExchan) markBroken()    {}               // TODO: limit the breakage in the exchan

// happRequest is the server-side HAPP request.
type happRequest struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *happRequest) readContent() (p []byte, err error) { return r.readContentP() }

// happResponse is the server-side HAPP response.
type happResponse struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *happResponse) addHeader(name []byte, value []byte) bool   { return r.addHeaderP(name, value) }
func (r *happResponse) header(name []byte) (value []byte, ok bool) { return r.headerP(name) }
func (r *happResponse) hasHeader(name []byte) bool                 { return r.hasHeaderP(name) }
func (r *happResponse) delHeader(name []byte) (deleted bool)       { return r.delHeaderP(name) }
func (r *happResponse) delHeaderAt(o uint8)                        { r.delHeaderAtP(o) }

func (r *happResponse) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *happResponse) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *happResponse) AddDirectoryRedirection() bool {
	// TODO
	return false
}
func (r *happResponse) setConnectionClose() { BugExitln("not used in HAPP") }

func (r *happResponse) SetCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *happResponse) sendChain() error { return r.sendChainP() }

func (r *happResponse) echoHeaders() error { return r.writeHeadersP() }
func (r *happResponse) echoChain() error   { return r.echoChainP() }

func (r *happResponse) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerP(name, value)
}
func (r *happResponse) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerP(name)
}

func (r *happResponse) pass1xx(resp clientResponse) bool { // used by proxies
	resp.delHopHeaders()
	r.status = resp.Status()
	if !resp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.hash, name, value)
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse(Version2)
	return false
}
func (r *happResponse) passHeaders() error       { return r.writeHeadersP() }
func (r *happResponse) passBytes(p []byte) error { return r.passBytesP(p) }

func (r *happResponse) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *happResponse) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *happResponse) addedHeaders() []byte { return nil } // TODO
func (r *happResponse) fixedHeaders() []byte { return nil } // TODO
