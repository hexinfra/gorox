// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP server implementation.

// HAPP is a simplified HTTP/2 without WebSocket, TCP Tunnel, and UDP Tunnel support.

package internal

import (
	"context"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
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
	gate *net.TCPListener // the real gate. set after open
}

func (g *happGate) init(server *happServer, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *happGate) open() error {
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
func (g *happGate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *happGate) serve() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("happServer[%s] happGate[%d]: accept error: %v\n", g.server.name, g.id, err)
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
				g.stage.Logf("happServer[%s] happGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			happConn := getHAPPConn(connID, g.server, g, tcpConn, rawConn)
			go happConn.serve() // happConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("happGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}

func (g *happGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHAPPConn is the server-side HAPP connection pool.
var poolHAPPConn sync.Pool

func getHAPPConn(id int64, server *happServer, gate *happGate, tcpConn *net.TCPConn, rawConn syscall.RawConn) serverConn {
	var conn *happConn
	if x := poolHAPPConn.Get(); x == nil {
		conn = new(happConn)
	} else {
		conn = x.(*happConn)
	}
	conn.onGet(id, server, gate, tcpConn, rawConn)
	return conn
}
func putHAPPConn(conn *happConn) {
	conn.onPut()
	poolHAPPConn.Put(conn)
}

// happConn is the server-side HAPP connection.
type happConn struct {
	// Mixins
	serverConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	happConn0 // all values must be zero by default in this struct!
}
type happConn0 struct { // for fast reset, entirely
}

func (c *happConn) onGet(id int64, server *happServer, gate *happGate, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.serverConn_.onGet(id, server, gate)
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *happConn) onPut() {
	c.serverConn_.onPut()
	c.tcpConn = nil
	c.rawConn = nil

	c.happConn0 = happConn0{}
}

func (c *happConn) serve() { // goroutine
	// TODO
}
func (c *happConn) receive() { // goroutine
	// TODO
}

func (c *happConn) setReadDeadline(deadline time.Time) error {
	// TODO
	return nil
}
func (c *happConn) setWriteDeadline(deadline time.Time) error {
	// TODO
	return nil
}

func (c *happConn) readAtLeast(p []byte, n int) (int, error) {
	// TODO
	return 0, nil
}
func (c *happConn) write(p []byte) (int, error) {
	// TODO
	return 0, nil
}
func (c *happConn) writev(vector *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

func (c *happConn) closeConn() {
	// TODO
}

// poolHAPPStream is the server-side HAPP stream pool.
var poolHAPPStream sync.Pool

func getHAPPStream(conn *happConn, id uint32) *happStream {
	// TODO
	return nil
}
func putHAPPStream(stream *happStream) {
	// TODO
}

// happStream is the server-side HAPP stream.
type happStream struct {
	// Mixins
	webStream_
	// Assocs
	request  happRequest
	response happResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *happConn
	// Stream states (zeros)
	happStream0 // all values must be zero by default in this struct!
}
type happStream0 struct { // for fast reset, entirely
}

func (s *happStream) onUse(conn *happConn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *happStream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webStream_.onEnd()
	s.conn = nil
	s.happStream0 = happStream0{}
}

func (s *happStream) execute() { // goroutine
	// TODO
	putHAPPStream(s)
}

func (s *happStream) webAgent() webAgent { return s.conn.getServer() }
func (s *happStream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *happStream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *happStream) executeNormal(app *App, req *happRequest, resp *happResponse) { // request & response
	// TODO
	//app.dispatchHandlet(req, resp)
}
func (s *happStream) serveAbnormal(req *happRequest, resp *happResponse) { // 4xx & 5xx
	// TODO
}

func (s *happStream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *happStream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *happStream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *happStream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *happStream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *happStream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *happStream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

func (s *happStream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *happStream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// happRequest is the server-side HAPP request.
type happRequest struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *happRequest) readContent() (p []byte, err error) { return r.readContentP() }

// happResponse is the server-side HAPP response.
type happResponse struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
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
