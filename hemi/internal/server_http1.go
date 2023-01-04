// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 server implementation.

// Both HTTP/1.0 and HTTP/1.1 are supported. For simplicity, HTTP/1.1 pipelining is recognized but not optimized.

package internal

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterServer("httpxServer", func(name string, stage *Stage) Server {
		s := new(httpxServer)
		s.onCreate(name, stage)
		return s
	})
}

// httpxServer is the HTTP/1 and HTTP/2 server.
type httpxServer struct {
	// Mixins
	httpServer_
	// States
	forceScheme  int8 // scheme that is must used
	strictScheme bool // use https scheme for TLS and http scheme for TCP?
	enableHTTP2  bool // enable HTTP/2?
	h2cMode      bool // if true, TCP runs HTTP/2 only. TLS is not affected. requires enableHTTP2
}

func (s *httpxServer) onCreate(name string, stage *Stage) {
	s.httpServer_.onCreate(name, stage)
	s.forceScheme = -1 // not force
}
func (s *httpxServer) OnShutdown() {
	// We don't use s.Shutdown() here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *httpxServer) OnConfigure() {
	s.httpServer_.onConfigure()
	var forceScheme string
	// forceScheme
	s.ConfigureString("forceScheme", &forceScheme, func(value string) bool {
		return value == "http" || value == "https"
	}, "")
	if forceScheme == "http" {
		s.forceScheme = SchemeHTTP
	} else if forceScheme == "https" {
		s.forceScheme = SchemeHTTPS
	}
	// strictScheme
	s.ConfigureBool("strictScheme", &s.strictScheme, false)
	// enableHTTP2
	s.ConfigureBool("enableHTTP2", &s.enableHTTP2, false) // TODO: change to true after HTTP/2 is fully implemented
	// h2cMode
	s.ConfigureBool("h2cMode", &s.h2cMode, false)
}
func (s *httpxServer) OnPrepare() {
	s.httpServer_.onPrepare()
	if s.tlsMode {
		var nextProtos []string
		if s.enableHTTP2 {
			nextProtos = []string{"h2", "http/1.1"}
		} else {
			nextProtos = []string{"http/1.1"}
		}
		s.tlsConfig.NextProtos = nextProtos
	} else if !s.enableHTTP2 {
		s.h2cMode = false
	}
}

func (s *httpxServer) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(httpxGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		s.IncSub(1)
		if s.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
	if Debug(2) {
		fmt.Printf("httpxServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// httpxGate is a gate of HTTP/1 and HTTP/2 server.
type httpxGate struct {
	// Mixins
	httpGate_
	// Assocs
	server *httpxServer
	// States
	gate *net.TCPListener
}

func (g *httpxGate) init(server *httpxServer, id int32) {
	g.httpGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *httpxGate) open() error {
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
func (g *httpxGate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *httpxGate) serveTCP() { // goroutine
	getHTTPConn := getHTTP1Conn
	if g.server.h2cMode {
		getHTTPConn = getHTTP2Conn
	}
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
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
				g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			httpConn := getHTTPConn(connID, g.server, g, tcpConn, rawConn)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns
	if Debug(2) {
		fmt.Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}
func (g *httpxGate) serveTLS() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.server.tlsConfig)
			// TODO: set deadline
			if err := tlsConn.Handshake(); err != nil {
				g.justClose(tcpConn)
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
	g.WaitSubs() // conns
	if Debug(2) {
		fmt.Printf("httpxGate=%d TLS done\n", g.id)
	}
	g.server.SubDone()
}

func (g *httpxGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolHTTP1Conn is the server-side HTTP/1 connection pool.
var poolHTTP1Conn sync.Pool

func getHTTP1Conn(id int64, server *httpxServer, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) httpConn {
	var conn *http1Conn
	if x := poolHTTP1Conn.Get(); x == nil {
		conn = new(http1Conn)
		stream := &conn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		conn = x.(*http1Conn)
	}
	conn.onGet(id, server, gate, netConn, rawConn)
	return conn
}
func putHTTP1Conn(conn *http1Conn) {
	conn.onPut()
	poolHTTP1Conn.Put(conn)
}

// http1Conn is the server-side HTTP/1 connection.
type http1Conn struct {
	// Mixins
	httpConn_
	// Assocs
	stream http1Stream // an http1Conn has exactly one stream at a time, so just embed it
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn   net.Conn        // the connection (TCP/TLS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	keepConn  bool            // keep the connection after current stream? true by default
	closeSafe bool            // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

func (c *http1Conn) onGet(id int64, server *httpxServer, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.httpConn_.onGet(id, server, gate)
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
		// request.input is conn scoped but put in stream scoped c.request for convenience
		PutNK(req.input)
		req.input = nil
	}
	req.inputNext, req.inputEdge = 0, 0 // inputNext and inputEdge are conn scoped but put in stream scoped c.request for convenience
	c.httpConn_.onPut()
}

func (c *http1Conn) serve() { // goroutine
	stream := &c.stream
	for { // each stream
		stream.execute(c)
		if !c.keepConn {
			break
		}
	}
	if stream.httpMode == httpModeNormal {
		c.closeConn()
	} else {
		// It's switcher's responsibility to closeConn()
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
		if c.server.TLSMode() {
			c.netConn.(*tls.Conn).CloseWrite()
		} else {
			c.netConn.(*net.TCPConn).CloseWrite()
		}
		time.Sleep(500 * time.Millisecond)
	}
	c.netConn.Close()
	c.gate.onConnectionClosed()
}

// http1Stream is the server-side HTTP/1 stream.
type http1Stream struct {
	// Mixins
	httpStream_
	// Assocs
	request  http1Request  // the server-side http/1 request.
	response http1Response // the server-side http/1 response.
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *http1Conn // associated conn
	// Stream states (zeros)
}

func (s *http1Stream) execute(conn *http1Conn) {
	s.onUse(conn)
	defer func() {
		if s.httpMode == httpModeNormal {
			s.onEnd()
		} else {
			// It's switcher's responsibility to call onEnd()
		}
	}()
	req, resp := &s.request, &s.response
	req.recvHead()
	if req.headResult != StatusOK { // receiving request error
		s.serveAbnormal(req, resp)
		return
	}

	if req.methodCode == MethodCONNECT { // tcp tunnel mode?
		// CONNECT does not allow content, so expectContinue is not allowed, and rejected.
		s.serveTCPTun()
		s.httpMode = httpModeTCPTun
		conn.keepConn = false // hijacked, so must close conn after s.serveTCPTun()
		return
	}

	server := conn.server.(*httpxServer)

	// RFC 7230 (section 5.5):
	// If the server's configuration (or outbound gateway) provides a
	// fixed URI scheme, that scheme is used for the effective request
	// URI.  Otherwise, if the request is received over a TLS-secured TCP
	// connection, the effective request URI's scheme is "https"; if not,
	// the scheme is "http".
	if forceScheme := server.forceScheme; forceScheme != -1 {
		req.schemeCode = uint8(forceScheme)
	} else if server.TLSMode() {
		if req.schemeCode == SchemeHTTP {
			req.schemeCode = SchemeHTTPS
		}
	} else if req.schemeCode == SchemeHTTPS && server.strictScheme {
		req.schemeCode = SchemeHTTP
	}

	app := server.findApp(req.UnsafeHostname())
	if app == nil || (!app.isDefault && !bytes.Equal(req.UnsafeColonPort(), server.ColonPortBytes())) {
		req.headResult, req.headReason = StatusNotFound, "app is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}
	req.app = app
	resp.app = app

	// TODO: upgradeUDPTun?
	if req.upgradeSocket { // socket mode?
		if req.expectContinue && !s.writeContinue() {
			return
		}
		s.serveSocket()
		s.httpMode = httpModeSocket
		conn.keepConn = false // hijacked, so must close conn after s.serveSocket()
		return
	}

	// Normal mode.
	if req.formKind == httpFormMultipart { // We allow a larger content size for uploading through multipart/form-data (large files are written to disk).
		req.maxContentSize = app.maxUploadContentSize
	} else { // Other content types, including application/x-www-form-urlencoded, are limited in a smaller size.
		req.maxContentSize = int64(app.maxMemoryContentSize)
	}
	if req.contentSize > req.maxContentSize {
		if req.expectContinue {
			req.headResult = StatusExpectationFailed
		} else {
			req.headResult = StatusContentTooLarge
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
		s.conn.keepConn = false // reaches limit
	}

	s.serveNormal(app, req, resp)

	if s.isBroken() {
		s.conn.keepConn = false // i/o error
	}
}

func (s *http1Stream) onUse(conn *http1Conn) { // for non-zeros
	s.httpStream_.onUse()
	s.conn = conn
	s.request.onUse()
	s.response.onUse()
}
func (s *http1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.httpStream_.onEnd()
}

func (s *http1Stream) getHolder() holder {
	return s.conn.getServer()
}

func (s *http1Stream) peerAddr() net.Addr {
	return s.conn.netConn.RemoteAddr()
}

func (s *http1Stream) writeContinue() bool { // 100 continue
	// This is an interim response, write directly.
	if s.setWriteDeadline(time.Now().Add(s.conn.server.WriteTimeout())) == nil {
		if _, err := s.write(http1BytesContinue); err == nil {
			return true
		}
	}
	s.conn.keepConn = false // i/o error
	return false
}
func (s *http1Stream) serveTCPTun() { // CONNECT method
	// TODO(diogin): implementation
	// NOTICE: use idle timeout
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nconnection: close\r\n\r\n"))
	s.conn.closeConn()
	s.onEnd()
}
func (s *http1Stream) serveUDPTun() { // upgrade: connect-udp
	// TODO(diogin): implementation (RFC 9298)
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nconnection: close\r\n\r\n"))
	s.conn.closeConn()
	s.onEnd()
}
func (s *http1Stream) serveSocket() { // upgrade: websocket
	// TODO(diogin): implementation (RFC 6455)
	// NOTICE: use idle timeout or clear read timeout
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nConnection: close\r\n\r\n"))
	s.conn.closeConn()
	s.onEnd()
}
func (s *http1Stream) serveNormal(app *App, req *http1Request, resp *http1Response) { // request & response
	app.dispatchHandlet(req, resp)
	if !resp.isSent { // only happens on sized content.
		resp.sendChain(resp.content)
	} else if resp.contentSize == -2 { // write last chunk and trailers (if exist)
		resp.finishChunked()
	}
	if !req.contentReceived {
		req.dropContent()
	}
}
func (s *http1Stream) serveAbnormal(req *http1Request, resp *http1Response) { // 4xx & 5xx
	conn := s.conn
	if Debug(2) {
		fmt.Printf("server=%s gate=%d conn=%d headResult=%d\n", conn.server.Name(), conn.gate.ID(), conn.id, s.request.headResult)
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
	if errorPage, ok := httpErrorPages[status]; !ok {
		content = http1Controls[status]
	} else if req.headReason == "" {
		content = errorPage
	} else {
		content = risky.ConstBytes(req.headReason)
	}
	// Use response as a dumb struct, don't use its methods (like Send) to send anything here!
	resp.status = status
	resp.addContentType(httpBytesTextHTML)
	resp.contentSize = int64(len(content))
	if status == StatusMethodNotAllowed {
		// Currently only WebSocket use this status in abnormal state, so GET is hard coded.
		resp.AddHeaderByBytes(httpBytesAllow, "GET")
	}
	resp.finalizeHeaders()
	if req.methodCode == MethodHEAD || resp.forbidContent { // yes, we follow the method semantic even we are in abnormal
		resp.vector = resp.fixedVector[0:3]
	} else {
		resp.vector = resp.fixedVector[0:4]
		resp.vector[3] = content
	}
	resp.vector[0] = resp.control()
	resp.vector[1] = resp.addedHeaders()
	resp.vector[2] = resp.fixedHeaders()
	// Ignore any error, as the connection is closed anyway.
	if s.setWriteDeadline(time.Now().Add(conn.server.WriteTimeout())) == nil {
		s.writev(&resp.vector)
	}
}

func (s *http1Stream) makeTempName(p []byte, seconds int64) (from int, edge int) {
	return s.conn.makeTempName(p, seconds)
}

func (s *http1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= conn.server.ReadTimeout()/4 {
		if err := conn.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}
func (s *http1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= conn.server.WriteTimeout()/4 {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}

func (s *http1Stream) read(p []byte) (int, error) {
	return s.conn.netConn.Read(p)
}
func (s *http1Stream) readFull(p []byte) (int, error) {
	return io.ReadFull(s.conn.netConn, p)
}
func (s *http1Stream) write(p []byte) (int, error) {
	return s.conn.netConn.Write(p)
}
func (s *http1Stream) writev(vector *net.Buffers) (int64, error) {
	return vector.WriteTo(s.conn.netConn)
}

func (s *http1Stream) isBroken() bool { return s.conn.isBroken() }
func (s *http1Stream) markBroken()    { s.conn.markBroken() }

// http1Request is the server-side HTTP/1 request.
type http1Request struct {
	// Mixins
	httpRequest_
	// Assocs
	// Stream states (non-zeros)
}

func (r *http1Request) recvHead() { // control + headers
	// The entire request head must be received in one timeout
	if err := r._prepareRead(&r.receiveTime); err != nil {
		r.headResult = -1
		return
	}
	if r.inputEdge == 0 && !r.growHead1() { // r.inputEdge == 0 means r.input is empty, so we must fill it
		// r.headResult is set.
		return
	}
	if !r.recvControl() || !r.recvHeaders1() || !r.checkHead() {
		// r.headResult is set.
		return
	}
	r._cleanInput()
	if Debug(2) {
		fmt.Printf("[http1Stream=%d]<------- [%s]\n", r.stream.(*http1Stream).conn.id, r.input[r.head.from:r.head.edge])
	}
}
func (r *http1Request) recvControl() bool { // method SP request-target SP HTTP-version CRLF
	r.pBack, r.pFore = 0, 0

	// method = token
	// token = 1*tchar
	hash := uint16(0)
	for {
		if b := r.input[r.pFore]; httpTchar[b] != 0 {
			hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		} else if b == ' ' {
			break
		} else {
			r.headResult, r.headReason = StatusBadRequest, "invalid character in method"
			return false
		}
	}
	if r.pBack == r.pFore {
		r.headResult, r.headReason = StatusBadRequest, "empty method"
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
			r.targetForm = httpTargetAbsolute
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
					r.headResult, r.headReason = StatusBadRequest, "bad scheme"
					return false
				}
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			}
			if scheme := r.input[r.pBack:r.pFore]; bytes.Equal(scheme, httpBytesHTTP) {
				r.schemeCode = SchemeHTTP
			} else if bytes.Equal(scheme, httpBytesHTTPS) {
				r.schemeCode = SchemeHTTPS
			} else {
				r.headResult, r.headReason = StatusBadRequest, "unknown scheme"
				return false
			}
			// Skip ':'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '/' {
				r.headResult, r.headReason = StatusBadRequest, "bad first slash"
				return false
			}
			// Skip '/'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '/' {
				r.headResult, r.headReason = StatusBadRequest, "bad second slash"
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
				r.headResult, r.headReason = StatusBadRequest, "empty authority is not allowed"
				return false
			}
			if !r.parseAuthority(r.pBack, r.pFore, true) {
				r.headResult, r.headReason = StatusBadRequest, "bad authority"
				return false
			}
			if b == ' ' { // ends of request-target
				// Don't treat this as httpTargetAsterisk! r.uri is empty but we fetch it through r.URI() or like which gives '/' if uri is empty.
				if r.methodCode == MethodOPTIONS {
					// OPTIONS http://www.example.org:8001 HTTP/1.1
					r.asteriskOptions = true
				} else {
					// GET http://www.example.org HTTP/1.1
					// Do nothing.
				}
				goto beforeVersion // request target is done, since origin-form always starts with '/'.
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
			query pair  // plain query in r.array[query.from:query.edge]
			qsOff int32 // offset of query string, if exists
		)
		query.setPlace(pairPlaceArray) // because queries are decoded

		// r.pFore is at '/'.
		for { // TODO: use a better algorithm to improve performance, state machine might be slow here.
			b := r.input[r.pFore]
			if b == ' ' { // end of request-target
				break
			}
			switch state {
			case 1: // in path
				if httpPchar[b] == 1 {
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x1f // '1' means from state 1
				} else if b == '?' {
					// Path is over, switch to query string parsing
					r.path = r.array[0:r.arrayEdge]
					r.queries.from = uint8(len(r.primes))
					r.queries.edge = r.queries.from
					query.nameFrom = r.arrayEdge
					qsOff = r.pFore - r.pBack
					state = 2
				} else {
					r.headResult, r.headReason = StatusBadRequest, "invalid path"
					return false
				}
			case 2: // in query string and expecting '=' to get a name
				if b == '=' {
					if size := r.arrayEdge - query.nameFrom; size <= 255 {
						query.nameSize = uint8(size)
					} else {
						r.headResult, r.headReason = StatusBadRequest, "query name too long"
						return false
					}
					query.value.from = r.arrayEdge
					state = 3
				} else if httpPchar[b] > 0 { // including '?'
					if b == '+' {
						b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
					}
					query.hash += uint16(b)
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x2f // '2' means from state 2
				} else {
					r.headResult, r.headReason = StatusBadRequest, "invalid query name"
					return false
				}
			case 3: // in query string and expecting '&' to get a value
				if b == '&' {
					query.value.edge = r.arrayEdge
					if query.nameSize > 0 && !r.addQuery(&query) {
						return false
					}
					query.hash = 0 // reset hash for next query
					query.nameFrom = r.arrayEdge
					state = 2
				} else if httpPchar[b] > 0 { // including '?'
					if b == '+' {
						b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
					}
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x3f // '3' means from state 3
				} else {
					r.headResult, r.headReason = StatusBadRequest, "invalid query value"
					return false
				}
			default: // in query string and expecting HEXDIG
				half, ok := byteFromHex(b)
				if !ok {
					r.headResult, r.headReason = StatusBadRequest, "invalid pct encoding"
					return false
				}
				if state&0xf == 0xf { // Expecting the first HEXDIG
					octet = half << 4
					state &= 0xf0 // this reserves last state and leads to the state of second HEXDIG
				} else { // Expecting the second HEXDIG
					octet |= half
					if state == 0x20 { // in name
						query.hash += uint16(octet)
					} else if state == 0x10 && octet == 0x00 { // For security reasons, we reject "\x00" in path.
						r.headResult, r.headReason = StatusBadRequest, "malformed path"
						return false
					}
					r.arrayPush(octet)
					state >>= 4 // restore last state
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
			if query.nameSize > 0 && !r.addQuery(&query) {
				return false
			}
		} else { // incomplete pct-encoded
			r.headResult, r.headReason = StatusBadRequest, "incomplete pct-encoded"
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
		r.targetForm = httpTargetAsterisk
		// RFC 7230 (section 5.3.4):
		// The asterisk-form of request-target is only used for a server-wide
		// OPTIONS request (Section 4.3.7 of [RFC7231]).
		if r.methodCode != MethodOPTIONS {
			r.headResult, r.headReason = StatusBadRequest, "asterisk-form is only used by OPTIONS method"
			return false
		}
		// Skip '*'. We don't use it as uri! Instead, we use '/'. To test OPTIONS *, test r.asteriskOptions set below.
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
		r.asteriskOptions = true
		// Expect SP
		if r.input[r.pFore] != ' ' {
			r.headResult, r.headReason = StatusBadRequest, "malformed asterisk-form"
			return false
		}
		// RFC 7230 (section 5.5):
		// If the request-target is in authority-form or asterisk-form, the
		// effective request URI's combined path and query component is empty.
	} else { // r.methodCode == MethodCONNECT, authority-form
		r.targetForm = httpTargetAuthority
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
			r.headResult, r.headReason = StatusBadRequest, "empty authority is not allowed"
			return false
		}
		if !r.parseAuthority(r.pBack, r.pFore, true) {
			r.headResult, r.headReason = StatusBadRequest, "invalid authority"
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
	if version := r.input[r.pBack:r.pFore]; bytes.Equal(version, httpBytesHTTP1_1) {
		r.versionCode = Version1_1
	} else if bytes.Equal(version, httpBytesHTTP1_0) {
		r.versionCode = Version1_0
	} else {
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}
	if r.input[r.pFore] == '\r' {
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
	}
	if r.input[r.pFore] != '\n' {
		r.headResult, r.headReason = StatusBadRequest, "bad eol of start line"
		return false
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	return true
}
func (r *http1Request) _cleanInput() {
	// r.pFore is at the beginning of content (if exists) or next request (if exists and is pipelined).
	if r.contentSize == -1 { // no content
		r.contentReceived = true
		r.formReceived = true      // set anyway
		if r.pFore < r.inputEdge { // still has data, stream is pipelined
			r.inputNext = r.pFore // mark the beginning of next request
		} else { // r.pFore == r.inputEdge, no data anymore
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		return
	}
	// content exists (sized or chunked)
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
			r.sizeReceived = r.contentSize        // content is received entirely.
			r.contentBlob = r.input[r.pFore:edge] // exact.
			r.contentBlobKind = httpContentBlobInput
		}
		if r.contentSize == 0 {
			r.formReceived = true // no content means no form
		}
	} else { // chunked mode
		// We don't know the length of chunked content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *http1Request) readContent() (p []byte, err error) {
	return r.readContent1()
}

// http1Response is the server-side HTTP/1 response.
type http1Response struct {
	// Mixins
	httpResponse_
	// Stream states (controlled)
	start [32]byte // exactly 32 bytes for "HTTP/1.1 xxx Mysterious Status\r\n"
	// Stream states (non-zeros)
}

func (r *http1Response) control() []byte {
	var start []byte
	if r.status >= int16(len(http1Controls)) || http1Controls[r.status] == nil {
		r.start = http1Mysterious
		r.start[9] = byte(r.status/100 + '0')
		r.start[10] = byte(r.status/10%10 + '0')
		r.start[11] = byte(r.status%10 + '0')
		start = r.start[:]
	} else {
		start = http1Controls[r.status]
	}
	return start
}

func (r *http1Response) header(name []byte) (value []byte, ok bool) {
	return r.header1(name)
}
func (r *http1Response) addHeader(name []byte, value []byte) bool {
	return r.addHeader1(name, value)
}
func (r *http1Response) delHeader(name []byte) (deleted bool) {
	return r.delHeader1(name)
}
func (r *http1Response) addedHeaders() []byte {
	return r.fields[0:r.fieldsEdge]
}
func (r *http1Response) fixedHeaders() []byte {
	return http1BytesFixedResponseHeaders
}

func (r *http1Response) AddHTTPSRedirection(authority string) bool {
	size := len(http1BytesLocationHTTPS)
	if authority == "" {
		size += len(r.request.UnsafeAuthority())
	} else {
		size += len(authority)
	}
	size += len(r.request.UnsafeURI()) + len(httpBytesCRLF)
	if from, _, ok := r.growHeader(size); ok {
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
	size := len(prefix)
	// TODO: remove colonPort if colonPort is default?
	colonPort := r.request.UnsafeColonPort()
	size += len(hostname) + len(colonPort) + len(r.request.UnsafeURI()) + len(httpBytesCRLF)
	if from, _, ok := r.growHeader(size); ok {
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
	size := len(prefix)
	size += len(req.UnsafeAuthority()) + len(req.UnsafeURI()) + 1 + len(httpBytesCRLF)
	if from, _, ok := r.growHeader(size); ok {
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

func (r *http1Response) AddCookie(cookie *Cookie) bool {
	if cookie.name == "" || cookie.invalid {
		return false
	}
	size := len(httpBytesSetCookie) + len(httpBytesColonSpace) + cookie.size() + len(httpBytesCRLF) // set-cookie: cookie\r\n
	if from, _, ok := r.growHeader(size); ok {
		from += copy(r.fields[from:], httpBytesSetCookie)
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

func (r *http1Response) sendChain(chain Chain) error { // TODO: if r.conn is TLS, don't use writev as it uses many Write() which might be slower than make+copy+write.
	return r.sendChain1(chain)
}

func (r *http1Response) pushHeaders() error { // headers are sent immediately upon pushing chunks.
	return r.writeHeaders1()
}
func (r *http1Response) pushChain(chain Chain) error {
	return r.pushChain1(chain, r.request.VersionCode() == Version1_1)
}

func (r *http1Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer1(name)
}
func (r *http1Response) addTrailer(name []byte, value []byte) bool {
	if r.request.VersionCode() == Version1_0 { // HTTP/1.0 doesn't support trailer.
		return true
	}
	return r.addTrailer1(name, value)
}

func (r *http1Response) pass1xx(resp response) bool { // used by proxies
	r.status = resp.Status()
	resp.delHopHeaders()
	if !resp.walkHeaders(func(name []byte, value []byte) bool {
		return r.addHeader(name, value)
	}, true) { // for proxy
		return false
	}
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.control()
	r.vector[1] = r.addedHeaders()
	r.vector[2] = httpBytesCRLF
	// 1xx has no content.
	if r.writeVector1(&r.vector) != nil {
		return false
	}
	// For next use.
	r.onEnd()
	r.onUse()
	return true
}
func (r *http1Response) passHeaders() error {
	return r.writeHeaders1()
}
func (r *http1Response) passBytes(p []byte) error {
	return r.passBytes1(p)
}

func (r *http1Response) finalizeHeaders() { // add at most 256 bytes
	// date: Sun, 06 Nov 1994 08:49:37 GMT
	if !r.dateCopied {
		r.stream.getHolder().Stage().clock.writeDate(r.fields[r.fieldsEdge : r.fieldsEdge+uint16(clockDateSize)])
		r.fieldsEdge += uint16(clockDateSize)
	}
	// last-modified: Sun, 06 Nov 1994 08:49:37 GMT
	if r.lastModified != -1 && !r.lastModifiedCopied {
		clockWriteLastModified(r.fields[r.fieldsEdge:r.fieldsEdge+uint16(clockLastModifiedSize)], r.lastModified)
		r.fieldsEdge += uint16(clockLastModifiedSize)
	}
	// etag: "xxxx-xxxx"
	if r.nETag > 0 && !r.etagCopied {
		r._addFixedHeader1(httpBytesETag, r.etag[0:r.nETag])
	}
	if r.contentSize != -1 && !r.forbidFraming {
		if r.contentSize != -2 { // content-length: >= 0
			lengthBuffer := r.stream.smallStack() // stack is enough for length
			from, edge := i64ToDec(r.contentSize, lengthBuffer)
			r._addFixedHeader1(httpBytesContentLength, lengthBuffer[from:edge])
		} else if r.request.VersionCode() != Version1_0 { // transfer-encoding: chunked
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
		} else {
			// RFC 7230 (section 3.3.1): A server MUST NOT send a
			// response containing Transfer-Encoding unless the corresponding
			// request indicates HTTP/1.1 (or later).
			r.stream.(*http1Stream).conn.keepConn = false // close conn for HTTP/1.0
		}
		// content-type: text/html; charset=utf-8
		if !r.contentTypeAdded {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeTextHTML))
		}
	}
	if r.stream.(*http1Stream).conn.keepConn { // connection: keep-alive
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionKeepAlive))
	} else { // connection: close
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionClose))
	}
	// accept-ranges: bytes
	if r.acceptBytesRange {
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesAcceptRangesBytes))
	}
}
func (r *http1Response) finalizeChunked() error {
	if r.request.VersionCode() == Version1_0 {
		return nil
	}
	return r.finalizeChunked1()
}

// http1Socket is the server-side HTTP/1 websocket.
type http1Socket struct {
	// Mixins
	httpSocket_
	// Stream states (zeros)
}

func (s *http1Socket) onUse() {
	s.httpSocket_.onUse()
}
func (s *http1Socket) onEnd() {
	s.httpSocket_.onEnd()
}
