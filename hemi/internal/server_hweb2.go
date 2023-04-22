// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/2 server implementation.

// HWEB/2 is a simplified HTTP/2 without WebSocket, TCP Tunnel, and UDP Tunnel support.

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
	RegisterServer("hweb2Server", func(name string, stage *Stage) Server {
		s := new(hweb2Server)
		s.onCreate(name, stage)
		return s
	})
}

// hweb2Server is the HWEB/2 server.
type hweb2Server struct {
	// Mixins
	webServer_
	// States
}

func (s *hweb2Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *hweb2Server) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *hweb2Server) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *hweb2Server) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *hweb2Server) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(hweb2Gate)
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
		Debugf("hweb2Server=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// hweb2Gate is a gate of hweb2Server.
type hweb2Gate struct {
	// Mixins
	webGate_
	// Assocs
	server *hweb2Server
	// States
	gate *net.TCPListener // the real gate. set after open
}

func (g *hweb2Gate) init(server *hweb2Server, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *hweb2Gate) open() error {
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
func (g *hweb2Gate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *hweb2Gate) serve() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("hweb2Server[%s] hweb2Gate[%d]: accept error: %v\n", g.server.name, g.id, err)
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
				g.stage.Logf("hweb2Server[%s] hweb2Gate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			hweb2Conn := getHWEB2Conn(connID, g.server, g, tcpConn, rawConn)
			go hweb2Conn.serve() // hweb2Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("hweb2Gate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}

func (g *hweb2Gate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHWEB2Conn is the server-side HWEB/2 connection pool.
var poolHWEB2Conn sync.Pool

func getHWEB2Conn(id int64, server *hweb2Server, gate *hweb2Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) serverConn {
	var conn *hweb2Conn
	if x := poolHWEB2Conn.Get(); x == nil {
		conn = new(hweb2Conn)
	} else {
		conn = x.(*hweb2Conn)
	}
	conn.onGet(id, server, gate, tcpConn, rawConn)
	return conn
}
func putHWEB2Conn(conn *hweb2Conn) {
	conn.onPut()
	poolHWEB2Conn.Put(conn)
}

// hweb2Conn is the server-side HWEB/2 connection.
type hweb2Conn struct {
	// Mixins
	serverConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	hweb2Conn0 // all values must be zero by default in this struct!
}
type hweb2Conn0 struct { // for fast reset, entirely
}

func (c *hweb2Conn) onGet(id int64, server *hweb2Server, gate *hweb2Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.serverConn_.onGet(id, server, gate)
	// TODO
}
func (c *hweb2Conn) onPut() {
	c.serverConn_.onPut()
	// TODO
}

func (c *hweb2Conn) serve() { // goroutine
	// TODO
}
func (c *hweb2Conn) receive() { // goroutine
	// TODO
}

func (c *hweb2Conn) setReadDeadline(deadline time.Time) error {
	// TODO
	return nil
}
func (c *hweb2Conn) setWriteDeadline(deadline time.Time) error {
	// TODO
	return nil
}

func (c *hweb2Conn) readAtLeast(p []byte, n int) (int, error) {
	// TODO
	return 0, nil
}
func (c *hweb2Conn) write(p []byte) (int, error) {
	// TODO
	return 0, nil
}
func (c *hweb2Conn) writev(vector *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

func (c *hweb2Conn) closeConn() {
	// TODO
}

// poolHWEB2Stream is the server-side HWEB/2 stream pool.
var poolHWEB2Stream sync.Pool

func getHWEB2Stream(conn *hweb2Conn, id uint32) *hweb2Stream {
	// TODO
	return nil
}
func putHWEB2Stream(stream *hweb2Stream) {
	// TODO
}

// hweb2Stream is the server-side HWEB/2 stream.
type hweb2Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  hweb2Request
	response hweb2Response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *hweb2Conn
	// Stream states (zeros)
	hweb2Stream0 // all values must be zero by default in this struct!
}
type hweb2Stream0 struct { // for fast reset, entirely
}

func (s *hweb2Stream) onUse(conn *hweb2Conn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *hweb2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webStream_.onEnd()
	s.conn = nil
	s.hweb2Stream0 = hweb2Stream0{}
}

func (s *hweb2Stream) execute() { // goroutine
	// TODO
	putHWEB2Stream(s)
}

func (s *hweb2Stream) keeper() webKeeper  { return nil }
func (s *hweb2Stream) peerAddr() net.Addr { return nil }

func (s *hweb2Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *hweb2Stream) executeWebApp(app *App, req *hweb2Request, resp *hweb2Response) { // request & response
	// TODO
	//app.dispatchHandlet(req, resp)
}
func (s *hweb2Stream) executeRPCSvc(svc *Svc, req *hweb2Request, resp *hweb2Response) { // request & response
	// TODO
	svc.dispatchHRPC(req, resp)
}
func (s *hweb2Stream) serveAbnormal(req *hweb2Request, resp *hweb2Response) { // 4xx & 5xx
	// TODO
}

func (s *hweb2Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *hweb2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *hweb2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *hweb2Stream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *hweb2Stream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *hweb2Stream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *hweb2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

func (s *hweb2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *hweb2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// hweb2Request is the server-side HWEB/2 request.
type hweb2Request struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hweb2Request) readContent() (p []byte, err error) { return r.readContentB2() }

// hweb2Response is the server-side HWEB/2 response.
type hweb2Response struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hweb2Response) addHeader(name []byte, value []byte) bool   { return r.addHeaderB2(name, value) }
func (r *hweb2Response) header(name []byte) (value []byte, ok bool) { return r.headerB2(name) }
func (r *hweb2Response) hasHeader(name []byte) bool                 { return r.hasHeaderB2(name) }
func (r *hweb2Response) delHeader(name []byte) (deleted bool)       { return r.delHeaderB2(name) }
func (r *hweb2Response) delHeaderAt(o uint8)                        { r.delHeaderAtB2(o) }

func (r *hweb2Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *hweb2Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *hweb2Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}
func (r *hweb2Response) setConnectionClose() { BugExitln("not used in HWEB/2") }

func (r *hweb2Response) SetCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *hweb2Response) sendChain() error { return r.sendChainB2() }

func (r *hweb2Response) echoHeaders() error { // headers are sent immediately upon echoing.
	// TODO
	return nil
}
func (r *hweb2Response) echoChain() error { return r.echoChainB2() }

func (r *hweb2Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerB2(name)
}
func (r *hweb2Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerB2(name, value)
}

func (r *hweb2Response) pass1xx(resp response) bool { // used by proxies
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
func (r *hweb2Response) passHeaders() error {
	// TODO
	return nil
}
func (r *hweb2Response) passBytes(p []byte) error { return r.passBytesB2(p) }

func (r *hweb2Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *hweb2Response) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *hweb2Response) addedHeaders() []byte { return nil } // TODO
func (r *hweb2Response) fixedHeaders() []byte { return nil } // TODO
