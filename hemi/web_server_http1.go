// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 server implementation. See RFC 9112.

// Both HTTP/1.0 and HTTP/1.1 are supported. For simplicity, HTTP/1.1 pipelining, which is rarely used, is supported but not optimized.

package hemi

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
	"github.com/hexinfra/gorox/hemi/common/system"
)

func init() {
	RegisterServer("httpServer", func(name string, stage *Stage) Server {
		s := new(httpServer)
		s.onCreate(name, stage)
		return s
	})
}

// httpServer is the HTTP/1 and HTTP/2 server.
type httpServer struct {
	// Mixins
	webServer_[*httpGate]
	// States
	forceScheme  int8 // scheme (http/https) that must be used
	adjustScheme bool // use https scheme for TLS and http scheme for TCP?
	enableHTTP2  bool // enable HTTP/2 support?
	http2Only    bool // if true, server runs HTTP/2 *only*. requires enableHTTP2 to be true, otherwise ignored
}

func (s *httpServer) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
	s.forceScheme = -1 // not forced
}
func (s *httpServer) OnShutdown() {
	s.webServer_.onShutdown()
}

func (s *httpServer) OnConfigure() {
	s.webServer_.onConfigure(s)

	var scheme string
	// forceScheme
	s.ConfigureString("forceScheme", &scheme, func(value string) error {
		if value == "http" || value == "https" {
			return nil
		}
		return errors.New(".forceScheme has an invalid value")
	}, "")
	if scheme == "http" {
		s.forceScheme = SchemeHTTP
	} else if scheme == "https" {
		s.forceScheme = SchemeHTTPS
	}

	// adjustScheme
	s.ConfigureBool("adjustScheme", &s.adjustScheme, true)

	if Debug() >= 2 { // remove this condition after HTTP/2 server has been fully implemented
		// enableHTTP2
		s.ConfigureBool("enableHTTP2", &s.enableHTTP2, true)
		// http2Only
		s.ConfigureBool("http2Only", &s.http2Only, false)
	}
}
func (s *httpServer) OnPrepare() {
	s.webServer_.onPrepare(s)
	if s.tlsMode {
		var nextProtos []string
		if !s.enableHTTP2 {
			nextProtos = []string{"http/1.1"}
		} else if s.http2Only {
			nextProtos = []string{"h2"}
		} else {
			nextProtos = []string{"h2", "http/1.1"}
		}
		s.tlsConfig.NextProtos = nextProtos
	} else if !s.enableHTTP2 {
		s.http2Only = false
	}
}

func (s *httpServer) Serve() { // runner
	if s.udsMode {
		s.serveUDS()
	} else {
		s.serveTCP()
	}
}
func (s *httpServer) serveUDS() {
	gate := new(httpGate)
	gate.init(s, 0)
	if err := gate.Open(); err != nil {
		EnvExitln(err.Error())
	}
	s.AddGate(gate)
	s.IncSub(1)
	go gate.serveUDS()
	s.WaitSubs() // gates
	if Debug() >= 2 {
		Printf("httpServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}
func (s *httpServer) serveTCP() {
	for id := int32(0); id < s.numGates; id++ {
		gate := new(httpGate)
		gate.init(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub(1)
		if s.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
	if Debug() >= 2 {
		Printf("httpServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// httpGate is a gate of httpServer.
type httpGate struct {
	// Mixins
	Gate_
	// Assocs
	server *httpServer
	// States
	gate net.Listener // the real gate. set after open
}

func (g *httpGate) init(server *httpServer, id int32) {
	g.Gate_.Init(server.stage, id, server.udsMode, server.tlsMode, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *httpGate) Open() error {
	if g.udsMode {
		return g.openUDS()
	} else {
		return g.openTCP()
	}
}
func (g *httpGate) openUDS() error {
	gate, err := net.Listen("unix", g.address)
	if err == nil {
		g.gate = gate.(*net.UnixListener)
		if Debug() >= 1 {
			Printf("httpGate id=%d address=%s opened!\n", g.id, g.address)
		}
	}
	return err
}
func (g *httpGate) openTCP() error {
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
		if Debug() >= 1 {
			Printf("httpGate id=%d address=%s opened!\n", g.id, g.address)
		}
	}
	return err
}
func (g *httpGate) Shut() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *httpGate) serveUDS() { // runner
	getHTTPConn := getHTTP1Conn
	if g.server.http2Only {
		getHTTPConn = getHTTP2Conn
	}
	gate := g.gate.(*net.UnixListener)
	connID := int64(0)
	for {
		unixConn, err := gate.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpServer[%s] httpGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				//g.stage.Logf("httpServer[%s] httpGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			httpConn := getHTTPConn(connID, g.server, g, unixConn, rawConn)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("httpGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}
func (g *httpGate) serveTCP() { // runner
	getHTTPConn := getHTTP1Conn
	if g.server.http2Only {
		getHTTPConn = getHTTP2Conn
	}
	gate := g.gate.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpServer[%s] httpGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				//g.stage.Logf("httpServer[%s] httpGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			httpConn := getHTTPConn(connID, g.server, g, tcpConn, rawConn)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("httpGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}
func (g *httpGate) serveTLS() { // runner
	gate := g.gate.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpServer[%s] httpGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.server.tlsConfig)
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			connState := tlsConn.ConnectionState()
			getHTTPConn := getHTTP1Conn
			if connState.NegotiatedProtocol == "h2" {
				getHTTPConn = getHTTP2Conn
			}
			httpConn := getHTTPConn(connID, g.server, g, tlsConn, nil)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("httpGate=%d TLS done\n", g.id)
	}
	g.server.SubDone()
}

func (g *httpGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
}

// httpxConn is a *http1Conn or *http2Conn.
type httpxConn interface {
	serve() // runner
}

// poolHTTP1Conn is the server-side HTTP/1 connection pool.
var poolHTTP1Conn sync.Pool

func getHTTP1Conn(id int64, server *httpServer, gate *httpGate, netConn net.Conn, rawConn syscall.RawConn) httpxConn {
	var httpConn *http1Conn
	if x := poolHTTP1Conn.Get(); x == nil {
		httpConn = new(http1Conn)
		stream := &httpConn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		httpConn = x.(*http1Conn)
	}
	httpConn.onGet(id, server, gate, netConn, rawConn)
	return httpConn
}
func putHTTP1Conn(httpConn *http1Conn) {
	httpConn.onPut()
	poolHTTP1Conn.Put(httpConn)
}

// http1Conn is the server-side HTTP/1 connection.
type http1Conn struct {
	// Mixins
	webServerConn_
	// Assocs
	stream http1Stream // an http1Conn has exactly one stream at a time, so just embed it
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn   net.Conn        // the connection (UDS/TCP/TLS)
	rawConn   syscall.RawConn // for syscall, only when netConn is UDS/TCP
	keepConn  bool            // keep the connection after current stream? true by default
	closeSafe bool            // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

func (c *http1Conn) onGet(id int64, server *httpServer, gate *httpGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.webServerConn_.onGet(id, server, gate)
	req := &c.stream.request
	req.input = req.stockInput[:] // input is conn scoped but put in stream scoped c.request for convenience
	c.netConn = netConn
	c.rawConn = rawConn
	c.keepConn = true
	c.closeSafe = true
}
func (c *http1Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	req := &c.stream.request
	if cap(req.input) != cap(req.stockInput) { // fetched from pool
		// req.input is conn scoped but put in stream scoped c.request for convenience
		PutNK(req.input)
		req.input = nil
	}
	req.inputNext, req.inputEdge = 0, 0 // inputNext and inputEdge are conn scoped but put in stream scoped c.request for convenience
	c.webServerConn_.onPut()
}

func (c *http1Conn) serve() { // runner
	stream := &c.stream
	for c.keepConn { // each stream
		stream.onUse(c)
		stream.execute()
		if !stream.isSocket {
			stream.onEnd()
		} else {
			// It's switcher's responsibility to call stream.onEnd() and c.closeConn()
			break
		}
	}
	if !stream.isSocket {
		c.closeConn()
	}
	putHTTP1Conn(c)
}

func (c *http1Conn) closeConn() {
	// RFC 7230 (section 6.6):
	//
	// If a server performs an immediate close of a TCP connection, there is
	// a significant risk that the client will not be able to read the last
	// HTTP response.  If the server receives additional data from the
	// client on a fully closed connection, such as another request that was
	// sent by the client before receiving the server's response, the
	// server's TCP stack will send a reset packet to the client;
	// unfortunately, the reset packet might erase the client's
	// unacknowledged input buffers before they can be read and interpreted
	// by the client's HTTP parser.
	//
	// To avoid the TCP reset problem, servers typically close a connection
	// in stages.  First, the server performs a half-close by closing only
	// the write side of the read/write connection.  The server then
	// continues to read from the connection until it receives a
	// corresponding close by the client, or until the server is reasonably
	// certain that its own TCP stack has received the client's
	// acknowledgement of the packet(s) containing the server's last
	// response.  Finally, the server fully closes the connection.
	if !c.closeSafe {
		if c.IsUDS() {
			c.netConn.(*net.UnixConn).CloseWrite()
		} else if c.IsTLS() {
			c.netConn.(*tls.Conn).CloseWrite()
		} else {
			c.netConn.(*net.TCPConn).CloseWrite()
		}
		time.Sleep(time.Second)
	}
	c.netConn.Close()
	c.gate.OnConnClosed()
}

// http1Stream is the server-side HTTP/1 stream.
type http1Stream struct {
	// Mixins
	webServerStream_
	// Assocs
	request  http1Request  // the server-side http/1 request.
	response http1Response // the server-side http/1 response.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *http1Conn // associated conn
	// Stream states (zeros)
}

func (s *http1Stream) execute() {
	req, resp := &s.request, &s.response

	req.recvHead()

	if req.headResult != StatusOK { // receiving request error
		s.serveAbnormal(req, resp)
		return
	}

	if req.methodCode == MethodCONNECT {
		req.headResult, req.failReason = StatusNotImplemented, "tcp over http is not implemented here"
		s.serveAbnormal(req, resp)
		return
	}

	server := s.conn.server.(*httpServer)

	// RFC 7230 (section 5.5):
	// If the server's configuration (or outbound gateway) provides a
	// fixed URI scheme, that scheme is used for the effective request
	// URI.  Otherwise, if the request is received over a TLS-secured TCP
	// connection, the effective request URI's scheme is "https"; if not,
	// the scheme is "http".
	if server.forceScheme != -1 { // forceScheme is set explicitly
		req.schemeCode = uint8(server.forceScheme)
	} else if s.conn.IsTLS() {
		if req.schemeCode == SchemeHTTP && server.adjustScheme {
			req.schemeCode = SchemeHTTPS
		}
	} else if req.schemeCode == SchemeHTTPS && server.adjustScheme {
		req.schemeCode = SchemeHTTP
	}

	webapp := server.findWebapp(req.UnsafeHostname())

	if webapp == nil || (!webapp.isDefault && !bytes.Equal(req.UnsafeColonPort(), server.ColonPortBytes())) {
		req.headResult, req.failReason = StatusNotFound, "target webapp is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}
	req.webapp = webapp
	resp.webapp = webapp

	if req.upgradeSocket { // socket mode?
		if req.expectContinue && !s.writeContinue() {
			return
		}
		s.executeSocket()
		s.isSocket = true
	} else { // exchan mode.
		if req.formKind == webFormMultipart { // we allow a larger content size for uploading through multipart/form-data (large files are written to disk).
			req.maxContentSize = webapp.maxUploadContentSize
		} else { // other content types, including application/x-www-form-urlencoded, are limited in a smaller size.
			req.maxContentSize = int64(webapp.maxMemoryContentSize)
		}
		if req.contentSize > req.maxContentSize {
			if req.expectContinue {
				req.headResult = StatusExpectationFailed
			} else {
				req.headResult, req.failReason = StatusContentTooLarge, "content size exceeds webapp's limit"
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
		s.conn.usedStreams.Add(1)
		if maxStreams := server.MaxStreamsPerConn(); (maxStreams > 0 && s.conn.usedStreams.Load() == maxStreams) || req.keepAlive == 0 || s.conn.gate.IsShut() {
			s.conn.keepConn = false // reaches limit, or client told us to close, or gate was shut
		}
		s.executeExchan(webapp, req, resp)

		if s.isBroken() {
			s.conn.keepConn = false // i/o error
		}
	}
}

func (s *http1Stream) onUse(conn *http1Conn) { // for non-zeros
	s.webServerStream_.onUse()
	s.conn = conn
	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *http1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.webServerStream_.onEnd()
}

func (s *http1Stream) webBroker() webBroker { return s.conn.webServer() }
func (s *http1Stream) webConn() webConn     { return s.conn }
func (s *http1Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *http1Stream) writeContinue() bool { // 100 continue
	// This is an interim response, write directly.
	if s.setWriteDeadline(time.Now().Add(s.conn.server.WriteTimeout())) == nil {
		if _, err := s.write(http1BytesContinue); err == nil {
			return true
		}
	}
	// i/o error
	s.conn.keepConn = false
	return false
}

func (s *http1Stream) executeExchan(webapp *Webapp, req *http1Request, resp *http1Response) { // request & response
	webapp.exchanDispatch(req, resp)
	if !resp.isSent { // only happens on sized content because response must be sent on echo
		resp.sendChain()
	} else if resp.isVague() { // end vague content and write trailers (if exist)
		resp.endVague()
	}
	if !req.contentReceived { // content exists but is not used, we receive and drop it here
		req.dropContent()
	}
}
func (s *http1Stream) serveAbnormal(req *http1Request, resp *http1Response) { // 4xx & 5xx
	conn := s.conn
	if Debug() >= 2 {
		Printf("server=%s gate=%d conn=%d headResult=%d\n", conn.server.Name(), conn.gate.ID(), conn.id, s.request.headResult)
	}
	s.conn.keepConn = false // close anyway.
	status := req.headResult
	if status == -1 || (status == StatusRequestTimeout && !req.gotInput) {
		return // send nothing.
	}
	// So we need to send something...
	if status == StatusContentTooLarge || status == StatusURITooLong || status == StatusRequestHeaderFieldsTooLarge {
		// The receiving side may has data when we close the connection
		conn.closeSafe = false
	}
	var content []byte
	if errorPage, ok := webErrorPages[status]; !ok {
		content = http1Controls[status]
	} else if req.failReason == "" {
		content = errorPage
	} else {
		content = risky.ConstBytes(req.failReason)
	}
	// Use response as a dumb struct here, don't use its methods (like Send) to send anything!
	resp.status = status
	resp.AddHeaderBytes(bytesContentType, bytesTypeHTMLUTF8)
	resp.contentSize = int64(len(content))
	if status == StatusMethodNotAllowed {
		// Currently only WebSocket use this status in abnormal state, so GET is hard coded.
		resp.AddHeaderBytes(bytesAllow, bytesGET)
	}
	resp.finalizeHeaders()
	if req.methodCode == MethodHEAD || resp.forbidContent { // we follow the method semantic even we are in abnormal
		resp.vector = resp.fixedVector[0:3]
	} else {
		resp.vector = resp.fixedVector[0:4]
		resp.vector[3] = content
	}
	resp.vector[0] = resp.control()
	resp.vector[1] = resp.addedHeaders()
	resp.vector[2] = resp.fixedHeaders()
	// Ignore any error, as the connection will be closed anyway.
	if s.setWriteDeadline(time.Now().Add(conn.server.WriteTimeout())) == nil {
		s.writev(&resp.vector)
	}
}
func (s *http1Stream) executeSocket() { // upgrade: websocket
	// TODO(diogin): implementation (RFC 6455), use s.serveSocket()
	// NOTICE: use idle timeout or clear read timeout otherwise
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nConnection: close\r\n\r\n"))
	s.conn.closeConn()
	s.onEnd()
}

func (s *http1Stream) makeTempName(p []byte, unixTime int64) int {
	return s.conn.makeTempName(p, unixTime)
}

func (s *http1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= time.Second {
		if err := conn.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}
func (s *http1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= time.Second {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}

func (s *http1Stream) read(p []byte) (int, error)     { return s.conn.netConn.Read(p) }
func (s *http1Stream) readFull(p []byte) (int, error) { return io.ReadFull(s.conn.netConn, p) }
func (s *http1Stream) write(p []byte) (int, error)    { return s.conn.netConn.Write(p) }
func (s *http1Stream) writev(vector *net.Buffers) (int64, error) {
	return vector.WriteTo(s.conn.netConn)
}

func (s *http1Stream) isBroken() bool { return s.conn.isBroken() }
func (s *http1Stream) markBroken()    { s.conn.markBroken() }

// http1Request is the server-side HTTP/1 request.
type http1Request struct { // incoming. needs parsing
	// Mixins
	webServerRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http1Request) recvHead() { // control + headers
	// The entire request head must be received in one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if r.inputEdge == 0 && !r.growHead1() { // r.inputEdge == 0 means r.input is empty, so we must fill it
		// r.headResult is set.
		return
	}
	if !r.recvControl() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if Debug() >= 2 {
		Printf("[http1Stream=%d]<------- [%s]\n", r.stream.(*http1Stream).conn.id, r.input[r.head.from:r.head.edge])
	}
}
func (r *http1Request) recvControl() bool { // method SP request-target SP HTTP-version CRLF
	r.pBack, r.pFore = 0, 0

	// method = token
	// token = 1*tchar
	hash := uint16(0)
	for {
		if b := r.input[r.pFore]; webTchar[b] != 0 {
			hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		} else if b == ' ' {
			break
		} else {
			r.headResult, r.failReason = StatusBadRequest, "invalid character in method"
			return false
		}
	}
	if r.pBack == r.pFore {
		r.headResult, r.failReason = StatusBadRequest, "empty method"
		return false
	}
	r.gotInput = true
	r.method.set(r.pBack, r.pFore)
	r.recognizeMethod(r.input[r.pBack:r.pFore], hash)
	// Skip SP after method
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Now r.pFore is at request-target.
	r.pBack = r.pFore
	// request-target = absolute-form / origin-form / authority-form / asterisk-form
	if b := r.input[r.pFore]; b != '*' && r.methodCode != MethodCONNECT { // absolute-form / origin-form
		if b != '/' { // absolute-form
			r.targetForm = webTargetAbsolute
			// absolute-form = absolute-URI
			// absolute-URI = scheme ":" hier-part [ "?" query ]
			// scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )
			// hier-part = "//" authority path-abempty
			// authority = host [ ":" port ]
			// path-abempty = *( "/" segment)

			// Scheme
			for {
				if b := r.input[r.pFore]; b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '-' || b == '.' {
					// Do nothing
				} else if b >= 'A' && b <= 'Z' {
					// RFC 7230 (section 2.7.3.  http and https URI Normalization and Comparison):
					// The scheme and host are case-insensitive and normally provided in lowercase;
					// all other components are compared in a case-sensitive manner.
					r.input[r.pFore] = b + 0x20 // to lower
				} else if b == ':' {
					break
				} else {
					r.headResult, r.failReason = StatusBadRequest, "bad scheme"
					return false
				}
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			}
			if scheme := r.input[r.pBack:r.pFore]; bytes.Equal(scheme, bytesHTTP) {
				r.schemeCode = SchemeHTTP
			} else if bytes.Equal(scheme, bytesHTTPS) {
				r.schemeCode = SchemeHTTPS
			} else {
				r.headResult, r.failReason = StatusBadRequest, "unknown scheme"
				return false
			}
			// Skip ':'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad first slash"
				return false
			}
			// Skip '/'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad second slash"
				return false
			}
			// Skip '/'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			// authority = host [ ":" port ]
			// host = IP-literal / IPv4address / reg-name
			r.pBack = r.pFore
			for {
				if b = r.input[r.pFore]; b >= 'A' && b <= 'Z' {
					r.input[r.pFore] = b + 0x20 // to lower
				} else if b == '/' || b == ' ' {
					break
				}
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			}
			if r.pBack == r.pFore {
				r.headResult, r.failReason = StatusBadRequest, "empty authority is not allowed"
				return false
			}
			if !r.parseAuthority(r.pBack, r.pFore, true) { // save = true
				r.headResult, r.failReason = StatusBadRequest, "bad authority"
				return false
			}
			if b == ' ' { // ends of request-target
				// Don't treat this as webTargetAsterisk! r.uri is empty but we fetch it through r.URI() or like which gives '/' if uri is empty.
				if r.methodCode == MethodOPTIONS {
					// OPTIONS http://www.example.org:8001 HTTP/1.1
					r.asteriskOptions = true
				} else {
					// GET http://www.example.org HTTP/1.1
					// Do nothing.
				}
				goto beforeVersion // request target is done, since origin-form always starts with '/', while b is ' ' here.
			}
			r.pBack = r.pFore // at '/'.
		}
		// RFC 7230 (5.3.1.  origin-form)
		//
		// The most common form of request-target is the origin-form.
		//
		//   origin-form = absolute-path [ "?" query ]
		//       absolute-path = 1*( "/" segment )
		//           segment = *pchar
		//       query = *( pchar / "/" / "?" )
		//
		// When making a request directly to an origin server, other than a
		// CONNECT or server-wide OPTIONS request (as detailed below), a client
		// MUST send only the absolute path and query components of the target
		// URI as the request-target.  If the target URI's path component is
		// empty, the client MUST send "/" as the path within the origin-form of
		// request-target.  A Host header field is also sent, as defined in
		// Section 5.4.
		var (
			state = 1   // in path
			octet byte  // byte value of %xx
			qsOff int32 // offset of query string, if exists
		)
		query := &r.mainPair
		query.zero()
		query.kind = kindQuery
		query.place = placeArray // all received queries are placed in r.array because queries have been decoded

		// r.pFore is at '/'.
	uri:
		for { // TODO: use a better algorithm to improve performance, state machine might be slow here.
			b := r.input[r.pFore]
			switch state {
			case 1: // in path
				if webPchar[b] == 1 { // excluding '?'
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x1f // '1' means from state 1, 'f' means first HEXDIG
				} else if b == '?' {
					// Path is over, switch to query string parsing
					r.path = r.array[0:r.arrayEdge]
					r.queries.from = uint8(len(r.primes))
					r.queries.edge = r.queries.from
					query.nameFrom = r.arrayEdge
					qsOff = r.pFore - r.pBack
					state = 2
				} else if b == ' ' { // end of request-target
					break uri
				} else {
					r.headResult, r.failReason = StatusBadRequest, "invalid path"
					return false
				}
			case 2: // in query string and expecting '=' to get a name
				if b == '=' {
					if nameSize := r.arrayEdge - query.nameFrom; nameSize <= 255 {
						query.nameSize = uint8(nameSize)
						query.value.from = r.arrayEdge
					} else {
						r.headResult, r.failReason = StatusBadRequest, "query name too long"
						return false
					}
					state = 3
				} else if webPchar[b] > 0 { // including '?'
					if b == '+' {
						b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
					}
					query.hash += uint16(b)
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x2f // '2' means from state 2, 'f' means first HEXDIG
				} else if b == ' ' { // end of request-target
					break uri
				} else {
					r.headResult, r.failReason = StatusBadRequest, "invalid query name"
					return false
				}
			case 3: // in query string and expecting '&' to get a value
				if b == '&' {
					query.value.edge = r.arrayEdge
					if query.nameSize > 0 && !r.addQuery(query) {
						return false
					}
					query.hash = 0 // reset for next query
					query.nameFrom = r.arrayEdge
					state = 2
				} else if webPchar[b] > 0 { // including '?'
					if b == '+' {
						b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
					}
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x3f // '3' means from state 3, 'f' means first HEXDIG
				} else if b == ' ' { // end of request-target
					break uri
				} else {
					r.headResult, r.failReason = StatusBadRequest, "invalid query value"
					return false
				}
			default: // in query string and expecting HEXDIG
				if b == ' ' { // end of request-target
					break uri
				}
				nybble, ok := byteFromHex(b)
				if !ok {
					r.headResult, r.failReason = StatusBadRequest, "invalid pct encoding"
					return false
				}
				if state&0xf == 0xf { // Expecting the first HEXDIG
					octet = nybble << 4
					state &= 0xf0 // this reserves last state and leads to the state of second HEXDIG
				} else { // Expecting the second HEXDIG
					octet |= nybble
					if state == 0x20 { // in name
						query.hash += uint16(octet)
					} else if octet == 0x00 && state == 0x10 { // For security reasons, we reject "\x00" in path.
						r.headResult, r.failReason = StatusBadRequest, "malformed path"
						return false
					}
					r.arrayPush(octet)
					state >>= 4 // restore previous state
				}
			}
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		if state == 1 { // path ends without a '?'
			r.path = r.array[0:r.arrayEdge]
		} else if state == 2 { // in query string and no '=' found
			r.queryString.set(r.pBack+qsOff, r.pFore)
			// Since there is no '=', we ignore this query
		} else if state == 3 { // in query string and no '&' found
			r.queryString.set(r.pBack+qsOff, r.pFore)
			query.value.edge = r.arrayEdge
			if query.nameSize > 0 && !r.addQuery(query) {
				return false
			}
		} else { // incomplete pct-encoded
			r.headResult, r.failReason = StatusBadRequest, "incomplete pct-encoded"
			return false
		}

		r.uri.set(r.pBack, r.pFore)
		if qsOff == 0 {
			r.encodedPath = r.uri
		} else {
			r.encodedPath.set(r.pBack, r.pBack+qsOff)
		}
		r.cleanPath()
	} else if b == '*' { // OPTIONS *, asterisk-form
		r.targetForm = webTargetAsterisk
		// RFC 7230 (section 5.3.4):
		// The asterisk-form of request-target is only used for a server-wide
		// OPTIONS request (Section 4.3.7 of [RFC7231]).
		if r.methodCode != MethodOPTIONS {
			r.headResult, r.failReason = StatusBadRequest, "asterisk-form is only used by OPTIONS method"
			return false
		}
		// Skip '*'. We don't use it as uri! Instead, we use '/'. To test OPTIONS *, test r.asteriskOptions set below.
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
		r.asteriskOptions = true
		// Expect SP
		if r.input[r.pFore] != ' ' {
			r.headResult, r.failReason = StatusBadRequest, "malformed asterisk-form"
			return false
		}
		// RFC 7230 (section 5.5):
		// If the request-target is in authority-form or asterisk-form, the
		// effective request URI's combined path and query component is empty.
	} else { // r.methodCode == MethodCONNECT, authority-form
		r.targetForm = webTargetAuthority
		// RFC 7230 (section 5.3.3. authority-form:
		// The authority-form of request-target is only used for CONNECT
		// requests (Section 4.3.6 of [RFC7231]).
		//
		//   authority-form = authority
		//   authority      = host [ ":" port ]
		//
		// When making a CONNECT request to establish a tunnel through one or
		// more proxies, a client MUST send only the target URI's authority
		// component (excluding any userinfo and its "@" delimiter) as the
		// request-target.
		for {
			if b := r.input[r.pFore]; b >= 'A' && b <= 'Z' {
				r.input[r.pFore] = b + 0x20 // to lower
			} else if b == ' ' {
				break
			}
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		if r.pBack == r.pFore {
			r.headResult, r.failReason = StatusBadRequest, "empty authority is not allowed"
			return false
		}
		if !r.parseAuthority(r.pBack, r.pFore, true) { // save = true
			r.headResult, r.failReason = StatusBadRequest, "invalid authority"
			return false
		}
		// RFC 7230 (section 5.5):
		// If the request-target is in authority-form or asterisk-form, the
		// effective request URI's combined path and query component is empty.
	}

beforeVersion: // r.pFore is at ' '.
	// Skip SP before HTTP-version
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Now r.pFore is at HTTP-version.
	r.pBack = r.pFore
	// HTTP-version = HTTP-name "/" DIGIT "." DIGIT
	// HTTP-name = %x48.54.54.50 ; "HTTP", case-sensitive
	if have := r.inputEdge - r.pFore; have >= 9 {
		// r.pFore -> EOL
		// r.inputEdge -> after EOL or more
		r.pFore += 8
	} else { // have < 9, but len("HTTP/1.X\n") = 9.
		// r.pFore at 'H' -> EOL
		// r.inputEdge at "TTP/1.X\n" -> after EOL
		r.pFore = r.inputEdge - 1
		for i, n := int32(0), 9-have; i < n; i++ {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
	}
	if version := r.input[r.pBack:r.pFore]; bytes.Equal(version, bytesHTTP1_1) {
		r.versionCode = Version1_1
	} else if bytes.Equal(version, bytesHTTP1_0) {
		r.versionCode = Version1_0
	} else { // i don't believe there will be a HTTP/1.2 in the future.
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}
	if r.input[r.pFore] == '\r' {
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
	}
	if r.input[r.pFore] != '\n' {
		r.headResult, r.failReason = StatusBadRequest, "bad eol of start line"
		return false
	}
	r.receiving = webSectionHeaders
	// Skip '\n'
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	return true
}
func (r *http1Request) cleanInput() {
	// r.pFore is at the beginning of content (if exists) or next request (if exists and is pipelined).
	if r.contentSize == -1 { // no content
		r.contentReceived = true   // we treat it as "received"
		r.formReceived = true      // set anyway
		if r.pFore < r.inputEdge { // still has data, stream is pipelined
			r.inputNext = r.pFore // mark the beginning of the next request
		} else { // r.pFore == r.inputEdge, no data anymore
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		return
	}
	// content exists (sized or vague)
	r.imme.set(r.pFore, r.inputEdge)
	if r.contentSize >= 0 { // sized mode
		immeSize := int64(r.imme.size())
		if immeSize == 0 || immeSize <= r.contentSize {
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		if immeSize >= r.contentSize {
			r.contentReceived = true
			edge := r.pFore + int32(r.contentSize)
			if immeSize > r.contentSize { // still has data, stream is pipelined
				r.imme.set(r.pFore, edge)
				r.inputNext = edge // mark the beginning of next request
			}
			r.receivedSize = r.contentSize        // content is received entirely.
			r.contentText = r.input[r.pFore:edge] // exact.
			r.contentTextKind = webContentTextInput
		}
		if r.contentSize == 0 {
			r.formReceived = true // no content means no form, so mark it as "received"
		}
	} else { // vague mode
		// We don't know the size of vague content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *http1Request) readContent() (p []byte, err error) { return r.readContent1() }

// http1Response is the server-side HTTP/1 response.
type http1Response struct { // outgoing. needs building
	// Mixins
	webServerResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http1Response) control() []byte { // HTTP/1's own control(). HTTP/2 and HTTP/3 use general control() in webServerResponse_
	var start []byte
	if r.status >= int16(len(http1Controls)) || http1Controls[r.status] == nil {
		r.start = http1Template
		r.start[9] = byte(r.status/100 + '0')
		r.start[10] = byte(r.status/10%10 + '0')
		r.start[11] = byte(r.status%10 + '0')
		start = r.start[:]
	} else {
		start = http1Controls[r.status]
	}
	return start
}

func (r *http1Response) addHeader(name []byte, value []byte) bool   { return r.addHeader1(name, value) }
func (r *http1Response) header(name []byte) (value []byte, ok bool) { return r.header1(name) }
func (r *http1Response) hasHeader(name []byte) bool                 { return r.hasHeader1(name) }
func (r *http1Response) delHeader(name []byte) (deleted bool)       { return r.delHeader1(name) }
func (r *http1Response) delHeaderAt(i uint8)                        { r.delHeaderAt1(i) }

func (r *http1Response) AddHTTPSRedirection(authority string) bool {
	headerSize := len(http1BytesLocationHTTPS)
	if authority == "" {
		headerSize += len(r.request.UnsafeAuthority())
	} else {
		headerSize += len(authority)
	}
	headerSize += len(r.request.UnsafeURI()) + len(bytesCRLF)
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], http1BytesLocationHTTPS)
		if authority == "" {
			from += copy(r.fields[from:], r.request.UnsafeAuthority())
		} else {
			from += copy(r.fields[from:], authority)
		}
		from += copy(r.fields[from:], r.request.UnsafeURI())
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}
func (r *http1Response) AddHostnameRedirection(hostname string) bool {
	var prefix []byte
	if r.request.IsHTTPS() {
		prefix = http1BytesLocationHTTPS
	} else {
		prefix = http1BytesLocationHTTP
	}
	headerSize := len(prefix)
	// TODO: remove colonPort if colonPort is default?
	colonPort := r.request.UnsafeColonPort()
	headerSize += len(hostname) + len(colonPort) + len(r.request.UnsafeURI()) + len(bytesCRLF)
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], hostname) // this is almost always configured, not client provided
		from += copy(r.fields[from:], colonPort)
		from += copy(r.fields[from:], r.request.UnsafeURI()) // original uri, won't split the response
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}
func (r *http1Response) AddDirectoryRedirection() bool {
	var prefix []byte
	if r.request.IsHTTPS() {
		prefix = http1BytesLocationHTTPS
	} else {
		prefix = http1BytesLocationHTTP
	}
	req := r.request
	headerSize := len(prefix)
	headerSize += len(req.UnsafeAuthority()) + len(req.UnsafeURI()) + 1 + len(bytesCRLF)
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], req.UnsafeAuthority())
		from += copy(r.fields[from:], req.UnsafeEncodedPath())
		r.fields[from] = '/'
		from++
		if len(req.UnsafeQueryString()) > 0 {
			from += copy(r.fields[from:], req.UnsafeQueryString())
		}
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}
func (r *http1Response) setConnectionClose() {
	r.stream.(*http1Stream).conn.keepConn = false // explicitly
}

func (r *http1Response) SetCookie(cookie *Cookie) bool {
	if cookie.name == "" || cookie.invalid {
		return false
	}
	headerSize := len(bytesSetCookie) + len(bytesColonSpace) + cookie.size() + len(bytesCRLF) // set-cookie: cookie\r\n
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], bytesSetCookie)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += cookie.writeTo(r.fields[from:])
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}

func (r *http1Response) sendChain() error { return r.sendChain1() }

func (r *http1Response) echoHeaders() error { return r.writeHeaders1() }
func (r *http1Response) echoChain() error   { return r.echoChain1(r.request.IsHTTP1_1()) } // chunked only for HTTP/1.1

func (r *http1Response) addTrailer(name []byte, value []byte) bool {
	if r.request.VersionCode() == Version1_1 {
		return r.addTrailer1(name, value)
	}
	return true // HTTP/1.0 doesn't support trailer.
}
func (r *http1Response) trailer(name []byte) (value []byte, ok bool) { return r.trailer1(name) }

func (r *http1Response) pass1xx(resp response) bool { // used by proxies
	resp.delHopHeaders()
	r.status = resp.Status()
	if !resp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.hash, name, value)
	}) {
		return false
	}
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.control()
	r.vector[1] = r.addedHeaders()
	r.vector[2] = bytesCRLF
	// 1xx has no content.
	if r.writeVector1() != nil {
		return false
	}
	// For next use.
	r.onEnd()
	r.onUse(Version1_1)
	return true
}
func (r *http1Response) passHeaders() error       { return r.writeHeaders1() }
func (r *http1Response) passBytes(p []byte) error { return r.passBytes1(p) }

func (r *http1Response) finalizeHeaders() { // add at most 256 bytes
	// date: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.iDate == 0 {
		r.fieldsEdge += uint16(r.stream.webBroker().Stage().clock.writeDate1(r.fields[r.fieldsEdge:]))
	}
	// expires: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.expires >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesExpires, r.unixTimes.expires))
	}
	// last-modified: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.lastModified >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesLastModified, r.unixTimes.lastModified))
	}
	if r.contentSize != -1 { // with content
		if !r.forbidFraming {
			if !r.isVague() { // content-length: >=0\r\n
				sizeBuffer := r.stream.buffer256() // enough for content-length
				n := i64ToDec(r.contentSize, sizeBuffer)
				r._addFixedHeader1(bytesContentLength, sizeBuffer[:n])
			} else if r.request.VersionCode() == Version1_1 { // transfer-encoding: chunked\r\n
				r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
			} else {
				// RFC 7230 (section 3.3.1): A server MUST NOT send a
				// response containing Transfer-Encoding unless the corresponding
				// request indicates HTTP/1.1 (or later).
				r.stream.(*http1Stream).conn.keepConn = false // close conn anyway for HTTP/1.0
			}
		}
		// content-type: text/html; charset=utf-8\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeHTMLUTF8))
		}
	}
	if r.stream.(*http1Stream).conn.keepConn { // connection: keep-alive\r\n
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionKeepAlive))
	} else { // connection: close\r\n
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionClose))
	}
}
func (r *http1Response) finalizeVague() error {
	if r.request.VersionCode() == Version1_1 {
		return r.finalizeVague1()
	}
	return nil // HTTP/1.0 does nothing.
}

func (r *http1Response) addedHeaders() []byte { return r.fields[0:r.fieldsEdge] }
func (r *http1Response) fixedHeaders() []byte { return http1BytesFixedResponseHeaders }

// poolHTTP1Socket
var poolHTTP1Socket sync.Pool

// http1Socket is the server-side HTTP/1 websocket.
type http1Socket struct {
	// Mixins
	webServerSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}