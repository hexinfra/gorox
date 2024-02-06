// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB server implementation.

// Only exchan mode is supported.

package internal

import (
	"net"
	"sync"
	"time"
)

func init() {
	RegisterServer("hwebServer", func(name string, stage *Stage) Server {
		s := new(hwebServer)
		s.onCreate(name, stage)
		return s
	})
}

// hwebServer is the HWEB server.
type hwebServer struct {
	// Mixins
	webServer_
	// States
}

func (s *hwebServer) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *hwebServer) OnShutdown() {
	// Notify gates. We don't close(s.ShutChan) here.
	for _, gate := range s.gates {
		gate.shut()
	}
}

func (s *hwebServer) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *hwebServer) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *hwebServer) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(hwebGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		s.IncSub(1)
		go gate.serve()
	}
	s.WaitSubs() // gates
	if Debug() >= 2 {
		Printf("hwebServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// hwebGate is a gate of hwebServer.
type hwebGate struct {
	// Mixins
	Gate_
	// Assocs
	server *hwebServer
	// States
}

func (g *hwebGate) init(server *hwebServer, id int32) {
	g.Gate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *hwebGate) open() error {
	// TODO
	return nil
}
func (g *hwebGate) shut() error {
	g.MarkShut()
	// TODO
	return nil
}

func (g *hwebGate) serve() { // runner
	// TODO
}

// poolHWEBConn is the server-side HWEB connection pool.
var poolHWEBConn sync.Pool

func getHWEBConn(id int64, server *hwebServer, gate *hwebGate, tcpConn *net.TCPConn) *hwebConn {
	var httpConn *hwebConn
	if x := poolHWEBConn.Get(); x == nil {
		httpConn = new(hwebConn)
	} else {
		httpConn = x.(*hwebConn)
	}
	httpConn.onGet(id, server, gate, tcpConn)
	return httpConn
}
func putHWEBConn(httpConn *hwebConn) {
	httpConn.onPut()
	poolHWEBConn.Put(httpConn)
}

// hwebConn is the server-side HWEB connection.
type hwebConn struct {
	// Mixins
	serverConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn *net.TCPConn // the underlying tcp conn
	// Conn states (zeros)
	hwebConn0 // all values must be zero by default in this struct!
}
type hwebConn0 struct { // for fast reset, entirely
}

func (c *hwebConn) onGet(id int64, server *hwebServer, gate *hwebGate, tcpConn *net.TCPConn) {
	c.serverConn_.onGet(id, server, gate)
	c.tcpConn = tcpConn
}
func (c *hwebConn) onPut() {
	c.serverConn_.onPut()
	c.tcpConn = nil
	c.hwebConn0 = hwebConn0{}
}

func (c *hwebConn) serve() { // runner
	// TODO
	// use go c.receive()?
}
func (c *hwebConn) receive() { // runner
	// TODO
}

func (c *hwebConn) setReadDeadline(deadline time.Time) error {
	// TODO
	return nil
}
func (c *hwebConn) setWriteDeadline(deadline time.Time) error {
	// TODO
	return nil
}

func (c *hwebConn) closeConn() {
	c.tcpConn.Close()
	c.gate.onConnClosed()
}

// poolHWEBExchan
var poolHWEBExchan sync.Pool

func getHWEBExchan(gate *hwebGate, id uint32) *hwebExchan {
	// TODO
	return nil
}
func putHWEBExchan(exchan *hwebExchan) {
	// TODO
}

// hwebExchan is the server-side HWEB exchan.
type hwebExchan struct {
	// Mixins
	serverStream_
	// Assocs
	request  hwebRequest
	response hwebResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	gate *hwebGate
	// Exchan states (zeros)
	hwebExchan0 // all values must be zero by default in this struct!
}
type hwebExchan0 struct { // for fast reset, entirely
}

func (x *hwebExchan) onUse(gate *hwebGate) { // for non-zeros
	x.serverStream_.onUse()
	x.gate = gate
	x.request.onUse(Version2)
	x.response.onUse(Version2)
}
func (x *hwebExchan) onEnd() { // for zeros
	x.response.onEnd()
	x.request.onEnd()
	x.serverStream_.onEnd()
	x.gate = nil
	x.hwebExchan0 = hwebExchan0{}
}

func (x *hwebExchan) execute() { // runner
	// TODO
	putHWEBExchan(x)
}

func (x *hwebExchan) webBroker() webBroker { return nil } // TODO
func (x *hwebExchan) webConn() webConn     { return nil } // TODO
func (x *hwebExchan) remoteAddr() net.Addr { return nil } // TODO

func (x *hwebExchan) writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (x *hwebExchan) executeExchan(webapp *Webapp, req *hwebRequest, resp *hwebResponse) { // request & response
	// TODO
	webapp.exchanDispatch(req, resp)
}
func (x *hwebExchan) serveAbnormal(req *hwebRequest, resp *hwebResponse) { // 4xx & 5xx
	// TODO
}

func (x *hwebExchan) makeTempName(p []byte, unixTime int64) int {
	// TODO
	return 0
}

func (x *hwebExchan) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (x *hwebExchan) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (x *hwebExchan) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (x *hwebExchan) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (x *hwebExchan) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (x *hwebExchan) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

func (x *hwebExchan) isBroken() bool { return false } // TODO: limit the breakage in the exchan
func (x *hwebExchan) markBroken()    {}               // TODO: limit the breakage in the exchan

// hwebRequest is the server-side HWEB request.
type hwebRequest struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *hwebRequest) readContent() (p []byte, err error) { return r.readContentH() }

// hwebResponse is the server-side HWEB response.
type hwebResponse struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

func (r *hwebResponse) addHeader(name []byte, value []byte) bool   { return r.addHeaderH(name, value) }
func (r *hwebResponse) header(name []byte) (value []byte, ok bool) { return r.headerH(name) }
func (r *hwebResponse) hasHeader(name []byte) bool                 { return r.hasHeaderH(name) }
func (r *hwebResponse) delHeader(name []byte) (deleted bool)       { return r.delHeaderH(name) }
func (r *hwebResponse) delHeaderAt(i uint8)                        { r.delHeaderAtH(i) }

func (r *hwebResponse) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *hwebResponse) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *hwebResponse) AddDirectoryRedirection() bool {
	// TODO
	return false
}
func (r *hwebResponse) setConnectionClose() { BugExitln("not used in HWEB") }

func (r *hwebResponse) SetCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *hwebResponse) sendChain() error { return r.sendChainH() }

func (r *hwebResponse) echoHeaders() error { return r.writeHeadersH() }
func (r *hwebResponse) echoChain() error   { return r.echoChainH() }

func (r *hwebResponse) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerH(name, value)
}
func (r *hwebResponse) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerH(name)
}

func (r *hwebResponse) pass1xx(resp response) bool { // used by proxies
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
func (r *hwebResponse) passHeaders() error       { return r.writeHeadersH() }
func (r *hwebResponse) passBytes(p []byte) error { return r.passBytesH(p) }

func (r *hwebResponse) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *hwebResponse) finalizeVague() error {
	// TODO
	return nil
}

func (r *hwebResponse) addedHeaders() []byte { return nil } // TODO
func (r *hwebResponse) fixedHeaders() []byte { return nil } // TODO
