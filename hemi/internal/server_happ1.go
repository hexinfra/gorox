// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP/1 server implementation.

// HAPP/1 is a binary HTTP/1.1 without WebSocket, TCP Tunnel, and UDP Tunnel support.

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
	RegisterServer("happ1Server", func(name string, stage *Stage) Server {
		s := new(happ1Server)
		s.onCreate(name, stage)
		return s
	})
}

// happ1Server is the HAPP/1 server.
type happ1Server struct {
	// Mixins
	webServer_
	// States
}

func (s *happ1Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}
func (s *happ1Server) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *happ1Server) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *happ1Server) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *happ1Server) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(happ1Gate)
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

// happ1Gate is a gate of happServer.
type happ1Gate struct {
	// Mixins
	webGate_
	// Assocs
	server *happ1Server
	// States
	gate *net.TCPListener // the real gate. set after open
}

func (g *happ1Gate) init(server *happ1Server, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *happ1Gate) open() error {
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
func (g *happ1Gate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *happ1Gate) serve() { // goroutine
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
			happ1Conn := getHAPP1Conn(connID, g.server, g, tcpConn, rawConn)
			go happ1Conn.serve() // happ1Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("happGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}

func (g *happ1Gate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHAPP1Conn is the server-side HAPP/1 connection pool.
var poolHAPP1Conn sync.Pool

func getHAPP1Conn(id int64, server *happ1Server, gate *happ1Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) serverConn {
	var conn *happ1Conn
	if x := poolHAPP1Conn.Get(); x == nil {
		conn = new(happ1Conn)
		stream := &conn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		conn = x.(*happ1Conn)
	}
	conn.onGet(id, server, gate, tcpConn, rawConn)
	return conn
}
func putHAPP1Conn(conn *happ1Conn) {
	conn.onPut()
	poolHAPP1Conn.Put(conn)
}

// happ1Conn is the server-side HAPP/1 connection.
type happ1Conn struct {
	// Mixins
	serverConn_
	// Assocs
	stream happ1Stream // an happ1Conn has exactly one stream at a time, so just embed it
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn   *net.TCPConn    // the connection
	rawConn   syscall.RawConn // for syscall
	keepConn  bool            // keep the connection after current stream? true by default
	closeSafe bool            // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

func (c *happ1Conn) onGet(id int64, server *happ1Server, gate *happ1Gate, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.serverConn_.onGet(id, server, gate)
	req := &c.stream.request
	req.input = req.stockInput[:] // input is conn scoped but put in stream scoped c.request for convenience
	c.tcpConn = tcpConn
	c.rawConn = rawConn
	c.keepConn = true
	c.closeSafe = true
}
func (c *happ1Conn) onPut() {
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

func (c *happ1Conn) serve() { // goroutine
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
	putHAPP1Conn(c)
}

func (c *happ1Conn) closeConn() {
	if !c.closeSafe {
		c.tcpConn.CloseWrite()
		time.Sleep(time.Second)
	}
	c.tcpConn.Close()
	c.gate.onConnectionClosed()
}

// happ1Stream is the server-side HAPP/1 stream.
type happ1Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  happ1Request  // the server-side HAPP/1 request.
	response happ1Response // the server-side HAPP/1 response.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *happ1Conn // associated conn
	// Stream states (zeros)
}

func (s *happ1Stream) execute(conn *happ1Conn) {
	s.onUse(conn)
	defer s.onEnd()

	req, resp := &s.request, &s.response

	req.recvHead()
	if req.headResult != StatusOK { // receiving request error
		s.serveAbnormal(req, resp)
		return
	}

	server := conn.server.(*happ1Server)

	app := server.findApp(req.UnsafeHostname())

	if app == nil || (!app.isDefault && !bytes.Equal(req.UnsafeColonPort(), server.ColonPortBytes())) {
		req.headResult, req.failReason = StatusNotFound, "target app is not found in this server"
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

func (s *happ1Stream) onUse(conn *happ1Conn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *happ1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.webStream_.onEnd()
}

func (s *happ1Stream) webAgent() webAgent { return s.conn.getServer() }
func (s *happ1Stream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *happ1Stream) writeContinue() bool { // 100 continue
	return false
}
func (s *happ1Stream) executeWebApp(app *App, req *happ1Request, resp *happ1Response) { // request & response
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
func (s *happ1Stream) serveAbnormal(req *happ1Request, resp *happ1Response) { // 4xx & 5xx
}

func (s *happ1Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *happ1Stream) setReadDeadline(deadline time.Time) error {
	return nil
}
func (s *happ1Stream) setWriteDeadline(deadline time.Time) error {
	return nil
}

func (s *happ1Stream) read(p []byte) (int, error)     { return 0, nil }
func (s *happ1Stream) readFull(p []byte) (int, error) { return 0, nil }
func (s *happ1Stream) write(p []byte) (int, error)    { return 0, nil }
func (s *happ1Stream) writev(vector *net.Buffers) (int64, error) {
	return 0, nil
}

func (s *happ1Stream) isBroken() bool { return s.conn.isBroken() }
func (s *happ1Stream) markBroken()    { s.conn.markBroken() }

// happ1Request is the server-side HAPP/1 request.
type happ1Request struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *happ1Request) recvHead() {
}
func (r *happ1Request) recvControl() bool {
	return false
}
func (r *happ1Request) cleanInput() {
}

func (r *happ1Request) readContent() (p []byte, err error) { return r.readContentB1() }

// happ1Response is the server-side HAPP/1 response.
type happ1Response struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *happ1Response) control() []byte {
	return nil
}

func (r *happ1Response) addHeader(name []byte, value []byte) bool   { return false }
func (r *happ1Response) header(name []byte) (value []byte, ok bool) { return nil, false }
func (r *happ1Response) hasHeader(name []byte) bool                 { return false }
func (r *happ1Response) delHeader(name []byte) (deleted bool)       { return false }
func (r *happ1Response) delHeaderAt(o uint8)                        {}

func (r *happ1Response) AddHTTPSRedirection(authority string) bool {
	return false
}
func (r *happ1Response) AddHostnameRedirection(hostname string) bool {
	return false
}
func (r *happ1Response) AddDirectoryRedirection() bool {
	return false
}
func (r *happ1Response) setConnectionClose() {
}

func (r *happ1Response) SetCookie(cookie *Cookie) bool {
	return false
}

func (r *happ1Response) sendChain() error { return nil }

func (r *happ1Response) echoHeaders() error { return nil }
func (r *happ1Response) echoChain() error   { return r.echoChainB1() }

func (r *happ1Response) addTrailer(name []byte, value []byte) bool {
	return false
}
func (r *happ1Response) trailer(name []byte) (value []byte, ok bool) { return nil, false }

func (r *happ1Response) pass1xx(resp clientResponse) bool { // used by proxies
	return true
}
func (r *happ1Response) passHeaders() error       { return r.writeHeadersB1() }
func (r *happ1Response) passBytes(p []byte) error { return r.passBytesB1(p) }

func (r *happ1Response) finalizeHeaders() { // add at most 256 bytes
}
func (r *happ1Response) finalizeUnsized() error {
	return nil
}

func (r *happ1Response) addedHeaders() []byte { return nil }
func (r *happ1Response) fixedHeaders() []byte { return nil }
