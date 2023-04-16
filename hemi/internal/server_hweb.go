// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB server implementation.

package internal

import (
	"bytes"
	"context"
	"github.com/hexinfra/gorox/hemi/common/system"
	"net"
	"sync"
	"syscall"
)

// hwebServer is the HWEB server.
type hwebServer struct {
	// Mixins
	webServer_
	// Assocs
	// States
	hrpcMode   bool                // works as HRPC server and dispatches to svcs instead of apps?
	forSvcs    []string            // for what svcs
	exactSvcs  []*hostnameTo[*Svc] // like: ("example.com")
	suffixSvcs []*hostnameTo[*Svc] // like: ("*.example.com")
	prefixSvcs []*hostnameTo[*Svc] // like: ("www.example.*")
}

func (s *hwebServer) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *hwebServer) OnShutdown() {
	// TODO
}

func (s *hwebServer) OnConfigure() {
	s.webServer_.onConfigure(s)
	// hrpcMode
	s.ConfigureBool("hrpcMode", &s.hrpcMode, false)
	// forSvcs
	s.ConfigureStringList("forSvcs", &s.forSvcs, nil, []string{})
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

func (s *hwebServer) LinkSvcs() {
	for _, svcName := range s.forSvcs {
		svc := s.stage.Svc(svcName)
		if svc == nil {
			continue
		}
		svc.linkHRPC(s)
		// TODO: use hash table?
		for _, hostname := range svc.exactHostnames {
			s.exactSvcs = append(s.exactSvcs, &hostnameTo[*Svc]{hostname, svc})
		}
		// TODO: use radix trie?
		for _, hostname := range svc.suffixHostnames {
			s.suffixSvcs = append(s.suffixSvcs, &hostnameTo[*Svc]{hostname, svc})
		}
		// TODO: use radix trie?
		for _, hostname := range svc.prefixHostnames {
			s.prefixSvcs = append(s.prefixSvcs, &hostnameTo[*Svc]{hostname, svc})
		}
	}
}
func (s *hwebServer) findSvc(hostname []byte) *Svc {
	// TODO: use hash table?
	for _, exactMap := range s.exactSvcs {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixSvcs {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixSvcs {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return nil
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
	return nil
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

// hwebConn
type hwebConn struct {
	webConn_
}

func (c *hwebConn) onGet(id int64, server *hwebServer, gate *hwebGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.webConn_.onGet(id, server, gate)
}
func (c *hwebConn) onPut() {
	c.webConn_.onPut()
}

func (c *hwebConn) serve() { // goroutine
}
func (c *hwebConn) receive() { // goroutine
}

// poolHWEBStream is the server-side HWEB stream pool.
var poolHWEBStream sync.Pool

func getHWEBStream(conn *hwebConn, id uint32) *hwebStream {
	return nil
}
func putHWEBStream(stream *hwebStream) {
}

// hwebStream
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
	s.request.onUse(Version255)
	s.response.onUse(Version255)
}
func (s *hwebStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webStream_.onEnd()
	s.conn = nil
	s.hwebStream0 = hwebStream0{}
}

func (s *hwebStream) execute() { // goroutine
}

func (s *hwebStream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *hwebStream) executeWebApp(app *App, req *hwebRequest, resp *hwebResponse) { // request & response
	// TODO
	//app.dispatchHandlet(req, resp)
}
func (s *hwebStream) executeRPCSvc(svc *Svc, req *hwebRequest, resp *hwebResponse) {
	// TODO
	svc.dispatchHRPC(req, resp)
}
func (s *hwebStream) serveAbnormal(req *hwebRequest, resp *hwebResponse) { // 4xx & 5xx
	// TODO
}
func (s *hwebStream) executeSocket() { // upgrade: websocket
	// TODO
}
func (s *hwebStream) executeTCPTun() { // CONNECT method
	// TODO
}
func (s *hwebStream) executeUDPTun() { // upgrade: connect-udp
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
