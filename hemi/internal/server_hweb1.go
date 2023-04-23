// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/1 server implementation.

// HWEB/1 is a binary HTTP/1.1 without WebSocket, TCP Tunnel, and UDP Tunnel support.

package internal

import (
	"bytes"
	"context"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
)

func init() {
	RegisterServer("hweb1Server", func(name string, stage *Stage) Server {
		s := new(hweb1Server)
		s.onCreate(name, stage)
		return s
	})
}

// hweb1Server is the HWEB/1 server.
type hweb1Server struct {
	// Mixins
	webServer_
	// States
}

func (s *hweb1Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *hweb1Server) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *hweb1Server) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *hweb1Server) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *hweb1Server) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(hweb1Gate)
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

// hweb1Gate is a gate of hwebServer.
type hweb1Gate struct {
	// Mixins
	webGate_
	// Assocs
	server *hweb1Server
	// States
	gate *net.TCPListener // the real gate. set after open
}

func (g *hweb1Gate) init(server *hweb1Server, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *hweb1Gate) open() error {
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
func (g *hweb1Gate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *hweb1Gate) serve() { // goroutine
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
			hweb1Conn := getHWEB1Conn(connID, g.server, g, tcpConn, rawConn)
			go hweb1Conn.serve() // hweb1Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("hwebGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}

func (g *hweb1Gate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHWEB1Conn is the server-side HWEB/1 connection pool.
var poolHWEB1Conn sync.Pool

func getHWEB1Conn(id int64, server *hweb1Server, gate *hweb1Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) serverConn {
	var conn *hweb1Conn
	if x := poolHWEB1Conn.Get(); x == nil {
		conn = new(hweb1Conn)
		stream := &conn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		conn = x.(*hweb1Conn)
	}
	conn.onGet(id, server, gate, tcpConn, rawConn)
	return conn
}
func putHWEB1Conn(conn *hweb1Conn) {
	conn.onPut()
	poolHWEB1Conn.Put(conn)
}

// hweb1Conn is the server-side HWEB/1 connection.
type hweb1Conn struct {
	// Mixins
	serverConn_
	// Assocs
	stream hweb1Stream // an hweb1Conn has exactly one stream at a time, so just embed it
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn   *net.TCPConn    // the connection
	rawConn   syscall.RawConn // for syscall
	keepConn  bool            // keep the connection after current stream? true by default
	closeSafe bool            // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

func (c *hweb1Conn) onGet(id int64, server *hweb1Server, gate *hweb1Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.serverConn_.onGet(id, server, gate)
	req := &c.stream.request
	req.input = req.stockInput[:] // input is conn scoped but put in stream scoped c.request for convenience
	c.tcpConn = tcpConn
	c.rawConn = rawConn
	c.keepConn = true
	c.closeSafe = true
}
func (c *hweb1Conn) onPut() {
	c.tcpConn = nil
	c.rawConn = nil
	req := &c.stream.request
	if cap(req.input) != cap(req.stockInput) { // fetched from pool
		// request.input is conn scoped but put in stream scoped c.request for convenience
		PutNK(req.input)
		req.input = nil
	}
	req.inputNext, req.inputEdge = 0, 0 // inputNext and inputEdge are conn scoped but put in stream scoped c.request for convenience
	c.serverConn_.onPut()
}

func (c *hweb1Conn) serve() { // goroutine
	stream := &c.stream
	for { // each stream
		stream.execute(c)
		if !c.keepConn {
			break
		}
	}
	if stream.mode == streamModeNormal {
		c.closeConn()
	} else {
		// It's switcher's responsibility to call c.closeConn()
	}
	putHWEB1Conn(c)
}

func (c *hweb1Conn) closeConn() {
	if !c.closeSafe {
		c.tcpConn.CloseWrite()
		time.Sleep(time.Second)
	}
	c.tcpConn.Close()
	c.gate.onConnectionClosed()
}

// hweb1Stream is the server-side HWEB/1 stream.
type hweb1Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  hweb1Request  // the server-side HWEB/1 request.
	response hweb1Response // the server-side HWEB/1 response.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *hweb1Conn // associated conn
	// Stream states (zeros)
}

func (s *hweb1Stream) execute(conn *hweb1Conn) {
	s.onUse(conn)
	defer s.onEnd()

	req, resp := &s.request, &s.response

	req.recvHead()
	if req.headResult != StatusOK { // receiving request error
		s.serveAbnormal(req, resp)
		return
	}

	server := conn.server.(*hweb1Server)

	// TODO
	if server.hrpcMode {
	} else {
	}

	app := server.findApp(req.UnsafeHostname())

	if app == nil || (!app.isDefault && !bytes.Equal(req.UnsafeColonPort(), server.ColonPortBytes())) {
		req.headResult, req.failReason = StatusNotFound, "target is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}
	req.app = app
	resp.app = app

	if req.formKind == httpFormMultipart { // we allow a larger content size for uploading through multipart/form-data (large files are written to disk).
		req.maxContentSize = app.maxUploadContentSize
	} else { // other content types, including application/x-www-form-urlencoded, are limited in a smaller size.
		req.maxContentSize = int64(app.maxMemoryContentSize)
	}
	if req.contentSize > req.maxContentSize {
		if req.expectContinue {
			req.headResult = StatusExpectationFailed
		} else {
			req.headResult, req.failReason = StatusContentTooLarge, "content size exceeds app's limit"
		}
		s.serveAbnormal(req, resp)
		return
	}

	// Prepare response according to request
	if req.methodCode == MethodHEAD {
		resp.forbidContent = true
	}

	if req.expectContinue && !s.writeContinue() {
		return
	}
	conn.usedStreams.Add(1)
	if maxStreams := server.MaxStreamsPerConn(); (maxStreams > 0 && conn.usedStreams.Load() == maxStreams) || req.keepAlive == 0 || s.conn.gate.IsShut() {
		s.conn.keepConn = false // reaches limit, or client told us to close, or gate is shut
	}
	s.executeWebApp(app, req, resp)

	if s.isBroken() {
		s.conn.keepConn = false // i/o error
	}
}

func (s *hweb1Stream) onUse(conn *hweb1Conn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *hweb1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.webStream_.onEnd()
}

func (s *hweb1Stream) keeper() webKeeper  { return s.conn.getServer() }
func (s *hweb1Stream) peerAddr() net.Addr { return nil }

func (s *hweb1Stream) writeContinue() bool { // 100 continue
	return false
}
func (s *hweb1Stream) executeWebApp(app *App, req *hweb1Request, resp *hweb1Response) { // request & response
	app.dispatchHandlet(req, resp)
	if !resp.IsSent() { // only happens on sized content because response must be sent on echo
		resp.sendChain()
	} else if resp.isUnsized() { // end unsized content and write trailers (if exist)
		resp.endUnsized()
	}
	if !req.contentReceived { // content exists but is not used, we receive and drop it here
		req.dropContent()
	}
}
func (s *hweb1Stream) executeRPCSvc(svc *Svc, req *hweb1Request, resp *hweb1Response) { // request & response
	// TODO
	svc.dispatchHRPC(req, resp)
}
func (s *hweb1Stream) serveAbnormal(req *hweb1Request, resp *hweb1Response) { // 4xx & 5xx
}

func (s *hweb1Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *hweb1Stream) setReadDeadline(deadline time.Time) error {
	return nil
}
func (s *hweb1Stream) setWriteDeadline(deadline time.Time) error {
	return nil
}

func (s *hweb1Stream) read(p []byte) (int, error)     { return 0, nil }
func (s *hweb1Stream) readFull(p []byte) (int, error) { return 0, nil }
func (s *hweb1Stream) write(p []byte) (int, error)    { return 0, nil }
func (s *hweb1Stream) writev(vector *net.Buffers) (int64, error) {
	return 0, nil
}

func (s *hweb1Stream) isBroken() bool { return s.conn.isBroken() }
func (s *hweb1Stream) markBroken()    { s.conn.markBroken() }

// hweb1Request is the server-side HWEB/1 request.
type hweb1Request struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hweb1Request) recvHead() {
}
func (r *hweb1Request) recvControl() bool {
	return false
}
func (r *hweb1Request) cleanInput() {
}

func (r *hweb1Request) readContent() (p []byte, err error) { return r.readContentB1() }

// hweb1Response is the server-side HWEB/1 response.
type hweb1Response struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hweb1Response) control() []byte {
	return nil
}

func (r *hweb1Response) addHeader(name []byte, value []byte) bool   { return false }
func (r *hweb1Response) header(name []byte) (value []byte, ok bool) { return nil, false }
func (r *hweb1Response) hasHeader(name []byte) bool                 { return false }
func (r *hweb1Response) delHeader(name []byte) (deleted bool)       { return false }
func (r *hweb1Response) delHeaderAt(o uint8)                        {}

func (r *hweb1Response) AddHTTPSRedirection(authority string) bool {
	return false
}
func (r *hweb1Response) AddHostnameRedirection(hostname string) bool {
	return false
}
func (r *hweb1Response) AddDirectoryRedirection() bool {
	return false
}
func (r *hweb1Response) setConnectionClose() {
}

func (r *hweb1Response) SetCookie(cookie *Cookie) bool {
	return false
}

func (r *hweb1Response) sendChain() error { return nil }

func (r *hweb1Response) echoHeaders() error { return nil }
func (r *hweb1Response) echoChain() error   { return r.echoChainB1() }

func (r *hweb1Response) trailer(name []byte) (value []byte, ok bool) { return nil, false }
func (r *hweb1Response) addTrailer(name []byte, value []byte) bool {
	return false
}

func (r *hweb1Response) pass1xx(resp response) bool { // used by proxies
	return true
}
func (r *hweb1Response) passHeaders() error       { return r.writeHeadersB1() }
func (r *hweb1Response) passBytes(p []byte) error { return r.passBytesB1(p) }

func (r *hweb1Response) finalizeHeaders() { // add at most 256 bytes
}
func (r *hweb1Response) finalizeUnsized() error {
	return nil
}

func (r *hweb1Response) addedHeaders() []byte { return nil }
func (r *hweb1Response) fixedHeaders() []byte { return nil }
