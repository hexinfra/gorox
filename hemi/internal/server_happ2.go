// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP/2 server implementation.

// HAPP/2 is a simplified HTTP/2 without WebSocket, TCP Tunnel, and UDP Tunnel support.

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
	RegisterServer("happ2Server", func(name string, stage *Stage) Server {
		s := new(happ2Server)
		s.onCreate(name, stage)
		return s
	})
}

// happ2Server is the HAPP/2 server.
type happ2Server struct {
	// Mixins
	webServer_
	// States
}

func (s *happ2Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *happ2Server) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *happ2Server) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *happ2Server) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *happ2Server) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(happ2Gate)
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
		Debugf("happ2Server=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// happ2Gate is a gate of happ2Server.
type happ2Gate struct {
	// Mixins
	webGate_
	// Assocs
	server *happ2Server
	// States
	gate *net.TCPListener // the real gate. set after open
}

func (g *happ2Gate) init(server *happ2Server, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *happ2Gate) open() error {
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
func (g *happ2Gate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *happ2Gate) serve() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("happ2Server[%s] happ2Gate[%d]: accept error: %v\n", g.server.name, g.id, err)
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
				g.stage.Logf("happ2Server[%s] happ2Gate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			happ2Conn := getHAPP2Conn(connID, g.server, g, tcpConn, rawConn)
			go happ2Conn.serve() // happ2Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("happ2Gate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}

func (g *happ2Gate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHAPP2Conn is the server-side HAPP/2 connection pool.
var poolHAPP2Conn sync.Pool

func getHAPP2Conn(id int64, server *happ2Server, gate *happ2Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) serverConn {
	var conn *happ2Conn
	if x := poolHAPP2Conn.Get(); x == nil {
		conn = new(happ2Conn)
	} else {
		conn = x.(*happ2Conn)
	}
	conn.onGet(id, server, gate, tcpConn, rawConn)
	return conn
}
func putHAPP2Conn(conn *happ2Conn) {
	conn.onPut()
	poolHAPP2Conn.Put(conn)
}

// happ2Conn is the server-side HAPP/2 connection.
type happ2Conn struct {
	// Mixins
	serverConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	happ2Conn0 // all values must be zero by default in this struct!
}
type happ2Conn0 struct { // for fast reset, entirely
}

func (c *happ2Conn) onGet(id int64, server *happ2Server, gate *happ2Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.serverConn_.onGet(id, server, gate)
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *happ2Conn) onPut() {
	c.serverConn_.onPut()
	c.tcpConn = nil
	c.rawConn = nil

	c.happ2Conn0 = happ2Conn0{}
}

func (c *happ2Conn) serve() { // goroutine
	// TODO
}
func (c *happ2Conn) receive() { // goroutine
	// TODO
}

func (c *happ2Conn) setReadDeadline(deadline time.Time) error {
	// TODO
	return nil
}
func (c *happ2Conn) setWriteDeadline(deadline time.Time) error {
	// TODO
	return nil
}

func (c *happ2Conn) readAtLeast(p []byte, n int) (int, error) {
	// TODO
	return 0, nil
}
func (c *happ2Conn) write(p []byte) (int, error) {
	// TODO
	return 0, nil
}
func (c *happ2Conn) writev(vector *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

func (c *happ2Conn) closeConn() {
	// TODO
}

// poolHAPP2Stream is the server-side HAPP/2 stream pool.
var poolHAPP2Stream sync.Pool

func getHAPP2Stream(conn *happ2Conn, id uint32) *happ2Stream {
	// TODO
	return nil
}
func putHAPP2Stream(stream *happ2Stream) {
	// TODO
}

// happ2Stream is the server-side HAPP/2 stream.
type happ2Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  happ2Request
	response happ2Response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *happ2Conn
	// Stream states (zeros)
	happ2Stream0 // all values must be zero by default in this struct!
}
type happ2Stream0 struct { // for fast reset, entirely
}

func (s *happ2Stream) onUse(conn *happ2Conn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *happ2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webStream_.onEnd()
	s.conn = nil
	s.happ2Stream0 = happ2Stream0{}
}

func (s *happ2Stream) execute() { // goroutine
	// TODO
	putHAPP2Stream(s)
}

func (s *happ2Stream) webAgent() webAgent { return s.conn.getServer() }
func (s *happ2Stream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *happ2Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *happ2Stream) executeWebApp(app *App, req *happ2Request, resp *happ2Response) { // request & response
	// TODO
	//app.dispatchHandlet(req, resp)
}
func (s *happ2Stream) serveAbnormal(req *happ2Request, resp *happ2Response) { // 4xx & 5xx
	// TODO
}

func (s *happ2Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *happ2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *happ2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *happ2Stream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *happ2Stream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *happ2Stream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *happ2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

func (s *happ2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *happ2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// happ2Request is the server-side HAPP/2 request.
type happ2Request struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *happ2Request) readContent() (p []byte, err error) { return r.readContentB2() }

// happ2Response is the server-side HAPP/2 response.
type happ2Response struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *happ2Response) addHeader(name []byte, value []byte) bool   { return r.addHeaderB2(name, value) }
func (r *happ2Response) header(name []byte) (value []byte, ok bool) { return r.headerB2(name) }
func (r *happ2Response) hasHeader(name []byte) bool                 { return r.hasHeaderB2(name) }
func (r *happ2Response) delHeader(name []byte) (deleted bool)       { return r.delHeaderB2(name) }
func (r *happ2Response) delHeaderAt(o uint8)                        { r.delHeaderAtB2(o) }

func (r *happ2Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *happ2Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *happ2Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}
func (r *happ2Response) setConnectionClose() { BugExitln("not used in HAPP/2") }

func (r *happ2Response) SetCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *happ2Response) sendChain() error { return r.sendChainB2() }

func (r *happ2Response) echoHeaders() error { return r.writeHeadersB2() }
func (r *happ2Response) echoChain() error   { return r.echoChainB2() }

func (r *happ2Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerB2(name, value)
}
func (r *happ2Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerB2(name)
}

func (r *happ2Response) pass1xx(resp clientResponse) bool { // used by proxies
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
func (r *happ2Response) passHeaders() error       { return r.writeHeadersB2() }
func (r *happ2Response) passBytes(p []byte) error { return r.passBytesB2(p) }

func (r *happ2Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *happ2Response) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *happ2Response) addedHeaders() []byte { return nil } // TODO
func (r *happ2Response) fixedHeaders() []byte { return nil } // TODO
