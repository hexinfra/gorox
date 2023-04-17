// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB server implementation.

// HWEB is a simplified HTTP/2.

package internal

import (
	"context"
	"github.com/hexinfra/gorox/hemi/common/system"
	"net"
	"sync"
	"syscall"
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
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *hwebServer) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *hwebServer) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *hwebServer) Serve() { // goroutine
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
	if IsDebug(2) {
		Debugf("hwebServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// hwebGate is a gate of hwebServer.
type hwebGate struct {
	// Mixins
	webGate_
	// Assocs
	server *hwebServer
	// States
	gate *net.TCPListener
}

func (g *hwebGate) init(server *hwebServer, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *hwebGate) open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		if err := system.SetReusePort(rawConn); err != nil {
			return err
		}
		return system.SetDeferAccept(rawConn)
	}
	gate, err := listenConfig.Listen(context.Background(), "tcp", g.address)
	if err == nil {
		g.gate = gate.(*net.TCPListener)
	}
	return err
}
func (g *hwebGate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *hwebGate) serve() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("hwebServer[%s] hwebGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				tcpConn.Close()
				g.stage.Logf("hwebServer[%s] hwebGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			webConn := getHWEBConn(connID, g.server, g, tcpConn, rawConn)
			go webConn.serve() // webConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("hwebGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}

func (g *hwebGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHWEBConn is the server-side HWEB connection pool.
var poolHWEBConn sync.Pool

func getHWEBConn(id int64, server *hwebServer, gate *hwebGate, netConn net.Conn, rawConn syscall.RawConn) webConn {
	var conn *hwebConn
	if x := poolHWEBConn.Get(); x == nil {
		conn = new(hwebConn)
	} else {
		conn = x.(*hwebConn)
	}
	conn.onGet(id, server, gate, netConn, rawConn)
	return conn
}
func putHWEBConn(conn *hwebConn) {
	conn.onPut()
	poolHWEBConn.Put(conn)
}

// hwebConn is the server-side HWEB connection.
type hwebConn struct {
	// Mixins
	webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	hwebConn0 // all values must be zero by default in this struct!
}
type hwebConn0 struct { // for fast reset, entirely
}

func (c *hwebConn) onGet(id int64, server *hwebServer, gate *hwebGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.webConn_.onGet(id, server, gate)
	// TODO
}
func (c *hwebConn) onPut() {
	c.webConn_.onPut()
	// TODO
}

func (c *hwebConn) serve() { // goroutine
	// TODO
}
func (c *hwebConn) receive() { // goroutine
	// TODO
}

func (c *hwebConn) closeConn() {
	// TODO
}

// poolHWEBStream is the server-side HWEB stream pool.
var poolHWEBStream sync.Pool

func getHWEBStream(conn *hwebConn, id uint32) *hwebStream {
	// TODO
	return nil
}
func putHWEBStream(stream *hwebStream) {
	// TODO
}

// hwebStream is the server-side HWEB stream.
type hwebStream struct {
	// Mixins
	webStream_
	// Assocs
	request  hwebRequest
	response hwebResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *hwebConn
	// Stream states (zeros)
	hwebStream0 // all values must be zero by default in this struct!
}
type hwebStream0 struct { // for fast reset, entirely
}

func (s *hwebStream) onUse(conn *hwebConn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(VersionH)
	s.response.onUse(VersionH)
}
func (s *hwebStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webStream_.onEnd()
	s.conn = nil
	s.hwebStream0 = hwebStream0{}
}

func (s *hwebStream) execute() { // goroutine
	// TODO
	putHWEBStream(s)
}

func (s *hwebStream) keeper() webKeeper  { return nil }
func (s *hwebStream) peerAddr() net.Addr { return nil }

func (s *hwebStream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *hwebStream) executeWebApp(app *App, req *hwebRequest, resp *hwebResponse) { // request & response
	// TODO
	//app.dispatchHandlet(req, resp)
}
func (s *hwebStream) executeRPCSvc(svc *Svc, req *hwebRequest, resp *hwebResponse) { // request & response
	// TODO
	svc.dispatchHRPC(req, resp)
}
func (s *hwebStream) serveAbnormal(req *hwebRequest, resp *hwebResponse) { // 4xx & 5xx
	// TODO
}
func (s *hwebStream) executeSocket() { // see RFC 8441: https://www.rfc-editor.org/rfc/rfc8441.html
	// TODO
}
func (s *hwebStream) executeTCPTun() { // CONNECT method
	// TODO
}
func (s *hwebStream) executeUDPTun() { // see RFC 9298: https://www.rfc-editor.org/rfc/rfc9298.html
	// TODO
}

// hwebRequest is the server-side HWEB request.
type hwebRequest struct { // incoming. needs parsing
	// Mixins
	webRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hwebRequest) readContent() (p []byte, err error) { return r.readContentH() }

// hwebResponse is the server-side HWEB response.
type hwebResponse struct { // outgoing. needs building
	// Mixins
	webResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hwebResponse) addHeader(name []byte, value []byte) bool   { return r.addHeaderH(name, value) }
func (r *hwebResponse) header(name []byte) (value []byte, ok bool) { return r.headerH(name) }
func (r *hwebResponse) hasHeader(name []byte) bool                 { return r.hasHeaderH(name) }
func (r *hwebResponse) delHeader(name []byte) (deleted bool)       { return r.delHeaderH(name) }
func (r *hwebResponse) delHeaderAt(o uint8)                        { r.delHeaderAtH(o) }

func (r *hwebResponse) addedHeaders() []byte { return nil }
func (r *hwebResponse) fixedHeaders() []byte { return nil }

// poolHWEBSocket
var poolHWEBSocket sync.Pool

// hwebSocket is the server-side HWEB websocket.
type hwebSocket struct {
	// Mixins
	webSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
