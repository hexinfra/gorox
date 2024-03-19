// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 and HTTP/2 server implementation. See RFC 9112 for HTTP/1. See RFC 9113 and 7541 for HTTP/2.

// For HTTP/1, both HTTP/1.0 and HTTP/1.1 are supported. Pipelining is supported but not optimized because it's rarely used.
// For HTTP/2, Server Push is not supported because it's rarely used.

package hemi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
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
	// Parent
	webServer_[*httpxGate]
	// States
	forceScheme  int8 // scheme (http/https) that must be used
	adjustScheme bool // use https scheme for TLS and http scheme for TCP?
	enableHTTP2  bool // enable HTTP/2 support?
	http2Only    bool // if true, server runs HTTP/2 *only*. requires enableHTTP2 to be true, otherwise ignored
}

func (s *httpxServer) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
	s.forceScheme = -1 // not forced
}

func (s *httpxServer) OnConfigure() {
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

	if DbgLevel() >= 2 { // remove this condition after HTTP/2 server has been fully implemented
		// enableHTTP2
		s.ConfigureBool("enableHTTP2", &s.enableHTTP2, true)
		// http2Only
		s.ConfigureBool("http2Only", &s.http2Only, false)
	}
}
func (s *httpxServer) OnPrepare() {
	s.webServer_.onPrepare(s)
	if s.IsTLS() {
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

func (s *httpxServer) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(httpxGate)
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub()
		if s.IsUDS() {
			go gate.serveUDS()
		} else if s.IsTLS() {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
	if DbgLevel() >= 2 {
		Printf("httpxServer=%s done\n", s.Name())
	}
	s.stage.DecSub()
}

// httpxGate is a gate of httpxServer.
type httpxGate struct {
	// Parent
	Gate_
	// Assocs
	// States
	listener net.Listener // the real gate. set after open
}

func (g *httpxGate) init(id int32, server *httpxServer) {
	g.Gate_.Init(id, server)
}

func (g *httpxGate) Open() error {
	if g.IsUDS() {
		return g._openUnix()
	} else {
		return g._openInet()
	}
}
func (g *httpxGate) _openUnix() error {
	address := g.Address()
	// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
	// This affects graceful upgrading, maybe we can implement fd transfer in the future.
	os.Remove(address)
	listener, err := net.Listen("unix", address)
	if err == nil {
		g.listener = listener.(*net.UnixListener)
		if DbgLevel() >= 1 {
			Printf("httpxGate id=%d address=%s opened!\n", g.id, g.Address())
		}
	}
	return err
}
func (g *httpxGate) _openInet() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		if err := system.SetReusePort(rawConn); err != nil {
			return err
		}
		return system.SetDeferAccept(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.listener = listener.(*net.TCPListener)
		if DbgLevel() >= 1 {
			Printf("httpxGate id=%d address=%s opened!\n", g.id, g.Address())
		}
	}
	return err
}
func (g *httpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *httpxGate) serveUDS() { // runner
	getHTTPConn := getHTTP1Conn
	if g.server.(*httpxServer).http2Only {
		getHTTPConn = getHTTP2Conn
	}
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
	for {
		unixConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			httpConn := getHTTPConn(connID, g, unixConn, rawConn)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DbgLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub()
}
func (g *httpxGate) serveTLS() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			connState := tlsConn.ConnectionState()
			getHTTPConn := getHTTP1Conn
			if connState.NegotiatedProtocol == "h2" {
				getHTTPConn = getHTTP2Conn
			}
			httpConn := getHTTPConn(connID, g, tlsConn, nil)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DbgLevel() >= 2 {
		Printf("httpxGate=%d TLS done\n", g.id)
	}
	g.server.DecSub()
}
func (g *httpxGate) serveTCP() { // runner
	getHTTPConn := getHTTP1Conn
	if g.server.(*httpxServer).http2Only {
		getHTTPConn = getHTTP2Conn
	}
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			httpConn := getHTTPConn(connID, g, tcpConn, rawConn)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DbgLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub()
}

func (g *httpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
}

// poolHTTP1Conn is the server-side HTTP/1 connection pool.
var poolHTTP1Conn sync.Pool

func getHTTP1Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) webServerConn {
	var httpConn *http1Conn
	if x := poolHTTP1Conn.Get(); x == nil {
		httpConn = new(http1Conn)
		stream := httpConn
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		httpConn = x.(*http1Conn)
	}
	httpConn.onGet(id, gate, netConn, rawConn)
	return httpConn
}
func putHTTP1Conn(httpConn *http1Conn) {
	httpConn.onPut()
	poolHTTP1Conn.Put(httpConn)
}

// http1Conn is the server-side HTTP/1 connection.
type http1Conn struct {
	// Parent
	ServerConn_
	// Mixins
	_webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn   net.Conn        // the connection (UDS/TCP/TLS)
	rawConn   syscall.RawConn // for syscall, only when netConn is UDS/TCP
	closeSafe bool            // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)

	// Mixins
	_webStream_
	// Assocs
	request  http1Request  // the server-side http/1 request.
	response http1Response // the server-side http/1 response.
	socket   *http1Socket  // the server-side http/1 socket.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (c *http1Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c._webConn_.onGet()
	req := &c.request
	req.input = req.stockInput[:] // input is conn scoped but put in stream scoped request for convenience
	c.netConn = netConn
	c.rawConn = rawConn
	c.closeSafe = true
}
func (c *http1Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	req := &c.request
	if cap(req.input) != cap(req.stockInput) { // fetched from pool
		// req.input is conn scoped but put in stream scoped c.request for convenience
		PutNK(req.input)
		req.input = nil
	}
	req.inputNext, req.inputEdge = 0, 0 // inputNext and inputEdge are conn scoped but put in stream scoped request for convenience
	c._webConn_.onPut()
	c.ServerConn_.OnPut()
}

func (c *http1Conn) webServer() webServer { return c.Server().(webServer) }
func (c *http1Conn) makeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.Server().Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *http1Conn) serve() { // runner
	defer putHTTP1Conn(c)

	stream := c
	for c.persistent { // each queued stream
		stream.onUse()
		stream.execute()
		stream.onEnd()
	}

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
	netConn := c.netConn
	if !c.closeSafe {
		if c.IsUDS() {
			netConn.(*net.UnixConn).CloseWrite()
		} else if c.IsTLS() {
			netConn.(*tls.Conn).CloseWrite()
		} else {
			netConn.(*net.TCPConn).CloseWrite()
		}
		time.Sleep(time.Second)
	}
	netConn.Close()
	c.gate.OnConnClosed()
}

// http1Stream is the server-side HTTP/1 stream.
type http1Stream = http1Conn

func (s *http1Stream) onUse() { // for non-zeros
	s._webStream_.onUse()
	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *http1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._webStream_.onEnd()
}

func (s *http1Stream) execute() {
	req, resp := &s.request, &s.response

	req.recvHead()

	if req.HeadResult() != StatusOK { // receiving request error
		s.serveAbnormal(req, resp)
		return
	}

	if req.methodCode == MethodCONNECT {
		req.headResult, req.failReason = StatusNotImplemented, "tcp over http is not implemented here"
		s.serveAbnormal(req, resp)
		return
	}

	conn := s
	server := conn.Server().(*httpxServer)
	// RFC 9112:
	// If the server's configuration provides for a fixed URI scheme, or a
	// scheme is provided by a trusted outbound gateway, that scheme is
	// used for the target URI. This is common in large-scale deployments
	// because a gateway server will receive the client's connection context
	// and replace that with their own connection to the inbound server.
	// Otherwise, if the request is received over a secured connection, the
	// target URI's scheme is "https"; if not, the scheme is "http".
	if server.forceScheme != -1 { // forceScheme is set explicitly
		req.schemeCode = uint8(server.forceScheme)
	} else { // scheme is not forced
		if conn.IsTLS() {
			if req.schemeCode == SchemeHTTP && server.adjustScheme {
				req.schemeCode = SchemeHTTPS
			}
		} else { // not secured
			if req.schemeCode == SchemeHTTPS && server.adjustScheme {
				req.schemeCode = SchemeHTTP
			}
		}
	}

	webapp := server.findApp(req.UnsafeHostname())

	if webapp == nil {
		req.headResult, req.failReason = StatusNotFound, "target webapp is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}
	if !webapp.isDefault && !bytes.Equal(req.UnsafeColonPort(), server.ColonPortBytes()) {
		req.headResult, req.failReason = StatusNotFound, "authoritative webapp is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}

	req.webapp = webapp
	resp.webapp = webapp

	if !req.upgradeSocket { // exchan mode?
		if req.formKind == webFormMultipart { // we allow a larger content size for uploading through multipart/form-data (large files are written to disk).
			req.maxContentSizeAllowed = webapp.maxUpfileSize
		} else { // other content types, including application/x-www-form-urlencoded, are limited in a smaller size.
			req.maxContentSizeAllowed = int64(server.MaxMemoryContentSize())
		}
		if req.contentSize > req.maxContentSizeAllowed {
			if req.expectContinue {
				req.headResult = StatusExpectationFailed
			} else {
				req.headResult, req.failReason = StatusContentTooLarge, "content size exceeds webapp's limit"
			}
			s.serveAbnormal(req, resp)
			return
		}

		// Prepare the response according to the request
		if req.methodCode == MethodHEAD {
			resp.forbidContent = true
		}

		if req.expectContinue && !s.writeContinue() {
			return
		}
		conn.usedStreams.Add(1)
		if maxStreams := server.MaxStreamsPerConn(); (maxStreams > 0 && conn.usedStreams.Load() == maxStreams) || req.keepAlive == 0 || conn.gate.IsShut() {
			conn.persistent = false // reaches limit, or client told us to close, or gate was shut
		}

		s.executeExchan(webapp, req, resp)

		if s.isBroken() {
			conn.persistent = false // i/o error
		}
	} else { // socket mode.
		if req.expectContinue && !s.writeContinue() {
			return
		}

		s.executeSocket()

		conn.persistent = false // explicitly
	}
}

func (s *http1Stream) writeContinue() bool { // 100 continue
	conn := s
	// This is an interim response, write directly.
	if s.setWriteDeadline(time.Now().Add(conn.Server().WriteTimeout())) == nil {
		if _, err := s.write(http1BytesContinue); err == nil {
			return true
		}
	}
	// i/o error
	conn.persistent = false
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
	conn := s
	if DbgLevel() >= 2 {
		Printf("server=%s gate=%d conn=%d headResult=%d\n", conn.Server().Name(), conn.gate.ID(), conn.id, s.request.headResult)
	}
	conn.persistent = false // close anyway.
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
		content = ConstBytes(req.failReason)
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
	if s.setWriteDeadline(time.Now().Add(conn.Server().WriteTimeout())) == nil {
		s.writev(&resp.vector)
	}
}

func (s *http1Stream) executeSocket() { // upgrade: websocket
	// TODO(diogin): implementation (RFC 6455). use s.serveSocket()?
	// NOTICE: use idle timeout or clear read timeout otherwise
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nConnection: close\r\n\r\n"))
}

func (s *http1Stream) setReadDeadline(deadline time.Time) error {
	conn := s
	if deadline.Sub(conn.lastRead) >= time.Second {
		if err := conn.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}
func (s *http1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s
	if deadline.Sub(conn.lastWrite) >= time.Second {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}

func (c *http1Stream) webAgent() webAgent   { return c.webServer() }
func (c *http1Stream) webConn() webConn     { return c }
func (c *http1Stream) remoteAddr() net.Addr { return c.netConn.RemoteAddr() }

func (c *http1Stream) read(p []byte) (int, error)     { return c.netConn.Read(p) }
func (c *http1Stream) readFull(p []byte) (int, error) { return io.ReadFull(c.netConn, p) }
func (c *http1Stream) write(p []byte) (int, error)    { return c.netConn.Write(p) }
func (c *http1Stream) writev(vector *net.Buffers) (int64, error) {
	return vector.WriteTo(c.netConn)
}

// poolHTTP2Conn is the server-side HTTP/2 connection pool.
var poolHTTP2Conn sync.Pool

func getHTTP2Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) webServerConn {
	var httpConn *http2Conn
	if x := poolHTTP2Conn.Get(); x == nil {
		httpConn = new(http2Conn)
	} else {
		httpConn = x.(*http2Conn)
	}
	httpConn.onGet(id, gate, netConn, rawConn)
	return httpConn
}
func putHTTP2Conn(httpConn *http2Conn) {
	httpConn.onPut()
	poolHTTP2Conn.Put(httpConn)
}

// http2Conn is the server-side HTTP/2 connection.
type http2Conn struct {
	// Parent
	ServerConn_
	// Mixins
	_webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	outFrame http2OutFrame // used by c.serve() to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	netConn        net.Conn            // the connection (TCP/TLS)
	rawConn        syscall.RawConn     // for syscall. only usable when netConn is TCP
	buffer         *http2Buffer        // http2Buffer in use, for receiving incoming frames
	clientSettings http2Settings       // settings of remote client
	table          http2DynamicTable   // dynamic table
	incomingChan   chan any            // frames and errors generated by c.receive() and waiting for c.serve() to consume
	inWindow       int32               // connection-level window size for incoming DATA frames
	outWindow      int32               // connection-level window size for outgoing DATA frames
	outgoingChan   chan *http2OutFrame // frames generated by streams and waiting for c.serve() to send
	// Conn states (zeros)
	inFrame0    http2InFrame                        // incoming frame, http2Conn controlled
	inFrame1    http2InFrame                        // incoming frame, http2Conn controlled
	inFrame     *http2InFrame                       // current incoming frame, used by recvFrame(). refers to c.inFrame0 or c.inFrame1 in turn
	streams     [http2MaxActiveStreams]*http2Stream // active (open, remoteClosed, localClosed) streams
	vector      net.Buffers                         // used by writev in c.serve()
	fixedVector [2][]byte                           // used by writev in c.serve()
	http2Conn0                                      // all values must be zero by default in this struct!
}
type http2Conn0 struct { // for fast reset, entirely
	bufferEdge   uint32                            // incoming data ends at c.buffer.buf[c.bufferEdge]
	pBack        uint32                            // incoming frame part (header or payload) begins from c.buffer.buf[c.pBack]
	pFore        uint32                            // incoming frame part (header or payload) ends at c.buffer.buf[c.pFore]
	cBack        uint32                            // incoming continuation part (header or payload) begins from c.buffer.buf[c.cBack]
	cFore        uint32                            // incoming continuation part (header or payload) ends at c.buffer.buf[c.cFore]
	lastStreamID uint32                            // last served stream id
	streamIDs    [http2MaxActiveStreams + 1]uint32 // ids of c.streams. the extra 1 id is used for fast linear searching
	nInFrames    int64                             // num of incoming frames
	nStreams     uint8                             // num of active streams
	waitReceive  bool                              // ...
	acknowledged bool                              // server settings acknowledged by client?
	//unackedSettings?
	//queuedControlFrames?
}

func (c *http2Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c._webConn_.onGet()
	c.netConn = netConn
	c.rawConn = rawConn
	if c.buffer == nil {
		c.buffer = getHTTP2Buffer()
		c.buffer.incRef()
	}
	c.clientSettings = http2InitialSettings
	c.table.init()
	if c.incomingChan == nil {
		c.incomingChan = make(chan any)
	}
	c.inWindow = _2G1 - _64K1                        // as a receiver, we disable connection-level flow control
	c.outWindow = c.clientSettings.initialWindowSize // after we have received the client preface, this value will be changed to the real value of client.
	if c.outgoingChan == nil {
		c.outgoingChan = make(chan *http2OutFrame)
	}
}
func (c *http2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	// c.buffer is reserved
	// c.table is reserved
	// c.incoming is reserved
	// c.outgoing is reserved
	c.inFrame0.zero()
	c.inFrame1.zero()
	c.inFrame = nil
	c.streams = [http2MaxActiveStreams]*http2Stream{}
	c.vector = nil
	c.fixedVector = [2][]byte{}
	c.http2Conn0 = http2Conn0{}
	c._webConn_.onPut()
	c.ServerConn_.OnPut()
}

func (c *http2Conn) webServer() webServer { return c.Server().(webServer) }
func (c *http2Conn) makeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.Server().Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *http2Conn) serve() { // runner
	Printf("========================== conn=%d start =========================\n", c.id)
	defer func() {
		Printf("========================== conn=%d exit =========================\n", c.id)
		putHTTP2Conn(c)
	}()
	if err := c.handshake(); err != nil {
		c.closeConn()
		return
	}
	// Successfully handshake means we have acknowledged client settings and sent our settings. Need to receive a settings ACK from client.
	go c.receive()
serve:
	for { // each frame from c.receive() and streams
		select {
		case incoming := <-c.incomingChan: // from c.receive()
			if inFrame, ok := incoming.(*http2InFrame); ok { // data, headers, priority, rst_stream, settings, ping, windows_update, unknown
				if inFrame.isUnknown() {
					// Ignore unknown frames.
					continue
				}
				if err := http2FrameProcessors[inFrame.kind](c, inFrame); err == nil {
					// Successfully processed. Next one.
					continue
				} else if h2e, ok := err.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else { // processor i/o error
					c.goawayCloseConn(http2ErrorInternal)
				}
				// c.serve() is broken, but c.receive() is not. need wait
				c.waitReceive = true
			} else { // c.receive() is broken and quit.
				if h2e, ok := incoming.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else if netErr, ok := incoming.(net.Error); ok && netErr.Timeout() {
					c.goawayCloseConn(http2ErrorNoError)
				} else {
					c.closeConn()
				}
			}
			break serve
		case outFrame := <-c.outgoingChan: // from streams. only headers and data
			// TODO: collect as many frames as we can?
			Printf("%+v\n", outFrame)
			if outFrame.endStream { // a stream has ended
				c.quitStream(outFrame.streamID)
				c.nStreams--
			}
			if err := c.sendFrame(outFrame); err != nil {
				// send side is broken.
				c.closeConn()
				c.waitReceive = true
				break serve
			}
		}
	}
	Printf("conn=%d waiting for active streams to end\n", c.id)
	for c.nStreams > 0 {
		if outFrame := <-c.outgoingChan; outFrame.endStream {
			c.quitStream(outFrame.streamID)
			c.nStreams--
		}
	}
	if c.waitReceive {
		Printf("conn=%d waiting for c.receive() quits\n", c.id)
		for {
			incoming := <-c.incomingChan
			if _, ok := incoming.(*http2InFrame); !ok {
				// An error from c.receive() means it's quit
				break
			}
		}
	}
	Printf("conn=%d c.serve() quit\n", c.id)
}

func (c *http2Conn) handshake() error {
	// Set deadline for the first request headers
	if err := c.setReadDeadline(time.Now().Add(c.Server().ReadTimeout())); err != nil {
		return err
	}
	if err := c._growFrame(uint32(len(http2BytesPrism))); err != nil {
		return err
	}
	if !bytes.Equal(c.buffer.buf[0:len(http2BytesPrism)], http2BytesPrism) {
		return http2ErrorProtocol
	}
	firstFrame, err := c.recvFrame()
	if err != nil {
		return err
	}
	if firstFrame.kind != http2FrameSettings || firstFrame.ack {
		return http2ErrorProtocol
	}
	if err := c._updateClientSettings(firstFrame); err != nil {
		return err
	}
	// TODO: write deadline
	n, err := c.write(http2ServerPrefaceAndMore)
	Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
	Printf("conn=%d ---> %v\n", c.id, http2ServerPrefaceAndMore)
	if err != nil {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	return err
}
func (c *http2Conn) receive() { // runner
	if DbgLevel() >= 1 {
		defer Printf("conn=%d c.receive() quit\n", c.id)
	}
	for { // each incoming frame
		inFrame, err := c.recvFrame()
		if err != nil {
			c.incomingChan <- err
			return
		}
		if inFrame.kind == http2FrameGoaway {
			c.incomingChan <- http2ErrorNoError
			return
		}
		c.incomingChan <- inFrame
	}
}

var http2ServerPrefaceAndMore = []byte{
	// server preface settings
	0, 0, 30, // length=30
	4,          // kind=http2FrameSettings
	0,          // flags=
	0, 0, 0, 0, // streamID=0
	0, 1, 0, 0, 0x10, 0x00, // headerTableSize=4K
	0, 3, 0, 0, 0x00, 0x7f, // maxConcurrentStreams=127
	0, 4, 0, 0, 0xff, 0xff, // initialWindowSize=64K1
	0, 5, 0, 0, 0x40, 0x00, // maxFrameSize=16K
	0, 6, 0, 0, 0x40, 0x00, // maxHeaderListSize=16K

	// ack client settings
	0, 0, 0, // length=0
	4,          // kind=http2FrameSettings
	1,          // flags=ack
	0, 0, 0, 0, // streamID=0

	// window update for the entire connection
	0, 0, 4, // length=4
	8,          // kind=http2FrameWindowUpdate
	0,          // flags=
	0, 0, 0, 0, // streamID=0
	0x7f, 0xff, 0x00, 0x00, // windowSize=2G1-64K1
}

func (c *http2Conn) goawayCloseConn(h2e http2Error) {
	goaway := &c.outFrame
	goaway.length = 8
	goaway.streamID = 0
	goaway.kind = http2FrameGoaway
	payload := goaway.buffer[0:8]
	binary.BigEndian.PutUint32(payload[0:4], c.lastStreamID)
	binary.BigEndian.PutUint32(payload[4:8], uint32(h2e))
	goaway.payload = payload
	c.sendFrame(goaway) // ignore error
	goaway.zero()
	c.closeConn()
}

var http2FrameProcessors = [...]func(*http2Conn, *http2InFrame) error{
	(*http2Conn).processDataFrame,
	(*http2Conn).processHeadersFrame,
	(*http2Conn).processPriorityFrame,
	(*http2Conn).processRSTStreamFrame,
	(*http2Conn).processSettingsFrame,
	nil, // pushPromise frames are rejected in c.recvFrame()
	(*http2Conn).processPingFrame,
	nil, // goaway frames are hijacked by c.receive()
	(*http2Conn).processWindowUpdateFrame,
	nil, // discrete continuation frames are rejected in c.recvFrame()
}

func (c *http2Conn) processHeadersFrame(inFrame *http2InFrame) error {
	var (
		stream *http2Stream
		req    *http2Request
	)
	streamID := inFrame.streamID
	if streamID > c.lastStreamID { // new stream
		if c.nStreams == http2MaxActiveStreams {
			return http2ErrorProtocol
		}
		c.lastStreamID = streamID
		c.usedStreams.Add(1)
		stream = getHTTP2Stream(c, streamID, c.clientSettings.initialWindowSize)
		req = &stream.request
		if !c._decodeFields(inFrame.effective(), req.joinHeaders) {
			putHTTP2Stream(stream)
			return http2ErrorCompression
		}
		if inFrame.endStream {
			stream.state = http2StateRemoteClosed
		} else {
			stream.state = http2StateOpen
		}
		c.joinStream(stream)
		c.nStreams++
		go stream.execute()
	} else { // old stream
		stream = c.findStream(streamID)
		if stream == nil { // no specified active stream
			return http2ErrorProtocol
		}
		if stream.state != http2StateOpen {
			return http2ErrorProtocol
		}
		if !inFrame.endStream { // must be trailers
			return http2ErrorProtocol
		}
		req = &stream.request
		req.receiving = webSectionTrailers
		if !c._decodeFields(inFrame.effective(), req.joinTrailers) {
			return http2ErrorCompression
		}
	}
	return nil
}
func (c *http2Conn) _decodeFields(fields []byte, join func(p []byte) bool) bool {
	var (
		I  uint32
		j  int
		ok bool
		N  []byte // field name
		V  []byte // field value
	)
	i, l := 0, len(fields)
	for i < l { // TODO
		b := fields[i]
		if b >= 1<<7 { // Indexed Header Field Representation
			I, j, ok = http2DecodeInteger(fields[i:], 7, 128)
			if !ok {
				Println("decode error")
				return false
			}
			i += j
			if I == 0 {
				Println("index == 0")
				return false
			}
			field := http2StaticTable[I]
			Printf("name=%s value=%s\n", field.nameAt(http2BytesStatic), field.valueAt(http2BytesStatic))
		} else if b >= 1<<6 { // Literal Header Field with Incremental Indexing
			I, j, ok = http2DecodeInteger(fields[i:], 6, 128)
			if !ok {
				Println("decode error")
				return false
			}
			i += j
			if I != 0 { // Literal Header Field with Incremental Indexing — Indexed Name
				field := http2StaticTable[I]
				N = field.nameAt(http2BytesStatic)
			} else { // Literal Header Field with Incremental Indexing — New Name
				N, j, ok = http2DecodeString(fields[i:])
				if !ok {
					Println("decode error")
					return false
				}
				i += j
				if len(N) == 0 {
					Println("empty name")
					return false
				}
			}
			V, j, ok = http2DecodeString(fields[i:])
			if !ok {
				Println("decode error")
				return false
			}
			i += j
			Printf("name=%s value=%s\n", N, V)
		} else if b >= 1<<5 { // Dynamic Table Size Update
			I, j, ok = http2DecodeInteger(fields[i:], 5, http2MaxTableSize)
			if !ok {
				Println("decode error")
				return false
			}
			i += j
			Printf("update size=%d\n", I)
		} else if b >= 1<<4 { // Literal Header Field Never Indexed
			I, j, ok = http2DecodeInteger(fields[i:], 4, 128)
			if !ok {
				Println("decode error")
				return false
			}
			i += j
			if I != 0 { // Literal Header Field Never Indexed — Indexed Name
				field := http2StaticTable[I]
				N = field.nameAt(http2BytesStatic)
			} else { // Literal Header Field Never Indexed — New Name
				N, j, ok = http2DecodeString(fields[i:])
				if !ok {
					Println("decode error")
					return false
				}
				i += j
				if len(N) == 0 {
					Println("empty name")
					return false
				}
			}
			V, j, ok = http2DecodeString(fields[i:])
			if !ok {
				Println("decode error")
				return false
			}
			i += j
			Printf("name=%s value=%s\n", N, V)
		} else { // Literal Header Field without Indexing
			Println("2222222222222")
			return false
		}
	}
	return true
}
func (c *http2Conn) _decodeString(src []byte, req *http2Request) (int, bool) {
	I, j, ok := http2DecodeInteger(src, 7, _16K)
	if !ok {
		return 0, false
	}
	H := src[0]&0x80 == 0x80
	src = src[j:]
	if I > uint32(len(src)) {
		return j, false
	}
	src = src[0:I]
	j += int(I)
	if H {
		// TODO
		return j, true
	} else {
		return j, true
	}
}
func (c *http2Conn) processDataFrame(inFrame *http2InFrame) error {
	return nil
}
func (c *http2Conn) processWindowUpdateFrame(inFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(inFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		return http2ErrorProtocol
	}
	// TODO
	c.inWindow = int32(windowSize)
	Printf("conn=%d stream=%d windowUpdate=%d\n", c.id, inFrame.streamID, windowSize)
	return nil
}
func (c *http2Conn) processSettingsFrame(inFrame *http2InFrame) error {
	if inFrame.ack {
		c.acknowledged = true
		return nil
	}
	// TODO: client sent a new settings
	return nil
}
func (c *http2Conn) _updateClientSettings(inFrame *http2InFrame) error {
	settings := inFrame.effective()
	windowDelta, j := int32(0), uint32(0)
	for i, n := uint32(0), inFrame.length/6; i < n; i++ {
		ident := uint16(settings[j])<<8 | uint16(settings[j+1])
		value := uint32(settings[j+2])<<24 | uint32(settings[j+3])<<16 | uint32(settings[j+4])<<8 | uint32(settings[j+5])
		switch ident {
		case http2SettingHeaderTableSize:
			c.clientSettings.headerTableSize = value
			// TODO: Dynamic Table Size Update
		case http2SettingEnablePush:
			if value > 1 {
				return http2ErrorProtocol
			}
			c.clientSettings.enablePush = value == 1
		case http2SettingMaxConcurrentStreams:
			c.clientSettings.maxConcurrentStreams = value
			// TODO: notify shrink
		case http2SettingInitialWindowSize:
			if value > _2G1 {
				return http2ErrorFlowControl
			}
			windowDelta = int32(value) - c.clientSettings.initialWindowSize
		case http2SettingMaxFrameSize:
			if value < _16K || value > _16M-1 {
				return http2ErrorProtocol
			}
			c.clientSettings.maxFrameSize = value
		case http2SettingMaxHeaderListSize: // this is only an advisory.
			c.clientSettings.maxHeaderListSize = value
		}
		j += 6
	}
	if windowDelta != 0 {
		c.clientSettings.initialWindowSize += windowDelta
		c._adjustStreamWindows(windowDelta)
	}
	Printf("conn=%d clientSettings=%+v\n", c.id, c.clientSettings)
	return nil
}
func (c *http2Conn) _adjustStreamWindows(delta int32) {
}
func (c *http2Conn) processRSTStreamFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *http2Conn) processPriorityFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *http2Conn) processPingFrame(inFrame *http2InFrame) error {
	pong := &c.outFrame
	pong.length = 8
	pong.streamID = 0
	pong.kind = http2FramePing
	pong.ack = true
	pong.payload = inFrame.effective()
	err := c.sendFrame(pong)
	pong.zero()
	return err
}

func (c *http2Conn) findStream(streamID uint32) *http2Stream {
	c.streamIDs[http2MaxActiveStreams] = streamID
	index := uint8(0)
	for c.streamIDs[index] != streamID { // searching stream id
		index++
	}
	if index == http2MaxActiveStreams { // not found.
		return nil
	}
	if DbgLevel() >= 2 {
		Printf("conn=%d findStream=%d at %d\n", c.id, streamID, index)
	}
	return c.streams[index]
}
func (c *http2Conn) joinStream(stream *http2Stream) {
	c.streamIDs[http2MaxActiveStreams] = 0
	index := uint8(0)
	for c.streamIDs[index] != 0 { // searching a free slot
		index++
	}
	if index == http2MaxActiveStreams { // this should not happen
		BugExitln("joinStream cannot find an empty slot")
	}
	if DbgLevel() >= 2 {
		Printf("conn=%d joinStream=%d at %d\n", c.id, stream.id, index)
	}
	stream.index = index
	c.streams[index] = stream
	c.streamIDs[index] = stream.id
}
func (c *http2Conn) quitStream(streamID uint32) {
	stream := c.findStream(streamID)
	if stream == nil {
		BugExitln("quitStream cannot find the stream")
	}
	if DbgLevel() >= 2 {
		Printf("conn=%d quitStream=%d at %d\n", c.id, streamID, stream.index)
	}
	c.streams[stream.index] = nil
	c.streamIDs[stream.index] = 0
}

func (c *http2Conn) recvFrame() (*http2InFrame, error) {
	// Receive frame header, 9 bytes
	c.pBack = c.pFore
	if err := c._growFrame(9); err != nil {
		return nil, err
	}
	// Decode frame header
	if c.inFrame == nil || c.inFrame == &c.inFrame1 {
		c.inFrame = &c.inFrame0
	} else {
		c.inFrame = &c.inFrame1
	}
	inFrame := c.inFrame
	if err := inFrame.decodeHeader(c.buffer.buf[c.pBack:c.pFore]); err != nil {
		return nil, err
	}
	// Receive frame payload
	c.pBack = c.pFore
	if err := c._growFrame(inFrame.length); err != nil {
		return nil, err
	}
	// Mark frame payload
	inFrame.buffer = c.buffer
	inFrame.pFrom = c.pBack
	inFrame.pEdge = c.pFore
	if inFrame.kind == http2FramePushPromise || inFrame.kind == http2FrameContinuation {
		return nil, http2ErrorProtocol
	}
	// Check the frame
	if err := inFrame.check(); err != nil {
		return nil, err
	}
	c.nInFrames++
	if c.nInFrames == 20 && !c.acknowledged {
		return nil, http2ErrorSettingsTimeout
	}
	if inFrame.kind == http2FrameHeaders {
		if !inFrame.endHeaders { // continuations follow
			if err := c._joinContinuations(inFrame); err != nil {
				return nil, err
			}
		}
		// Got a new headers. Set deadline for next headers
		if err := c.setReadDeadline(time.Now().Add(c.Server().ReadTimeout())); err != nil {
			return nil, err
		}
	}
	if DbgLevel() >= 2 {
		Printf("conn=%d <--- %+v\n", c.id, inFrame)
	}
	return inFrame, nil
}
func (c *http2Conn) _growFrame(size uint32) error {
	c.pFore += size // size is limited, so won't overflow
	if c.pFore <= c.bufferEdge {
		return nil
	}
	// Needs grow.
	if c.pFore > c.buffer.size() { // needs slide
		if c.buffer.getRef() == 1 { // no streams are referring to c.buffer, just slide
			c.bufferEdge = uint32(copy(c.buffer.buf[:], c.buffer.buf[c.pBack:c.bufferEdge]))
		} else { // there are still streams referring to c.buffer. use a new buffer
			buffer := c.buffer
			c.buffer = getHTTP2Buffer()
			c.buffer.incRef()
			c.bufferEdge = uint32(copy(c.buffer.buf[:], buffer.buf[c.pBack:c.bufferEdge]))
			buffer.decRef()
		}
		c.pFore -= c.pBack
		c.pBack = 0
	}
	return c._fillBuffer(c.pFore - c.bufferEdge)
}
func (c *http2Conn) _fillBuffer(size uint32) error {
	n, err := c.readAtLeast(c.buffer.buf[c.bufferEdge:], int(size))
	if DbgLevel() >= 2 {
		Printf("--------------------- conn=%d CALL READ=%d -----------------------\n", c.id, n)
	}
	if err != nil && DbgLevel() >= 2 {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	c.bufferEdge += uint32(n)
	return err
}
func (c *http2Conn) _joinContinuations(headers *http2InFrame) error { // into a single headers frame
	headers.buffer = nil // will be restored at the end of continuations
	var continuation http2InFrame
	c.cBack, c.cFore = c.pFore, c.pFore
	for { // each continuation frame
		// Receive continuation header
		if err := c._growContinuation(9, headers); err != nil {
			return err
		}
		// Decode continuation header
		if err := continuation.decodeHeader(c.buffer.buf[c.cBack:c.cFore]); err != nil {
			return err
		}
		// Check continuation header
		if continuation.length == 0 || headers.length+continuation.length > http2MaxFrameSize {
			return http2ErrorFrameSize
		}
		if continuation.streamID != headers.streamID || continuation.kind != http2FrameContinuation {
			return http2ErrorProtocol
		}
		// Receive continuation payload
		c.cBack = c.cFore
		if err := c._growContinuation(continuation.length, headers); err != nil {
			return err
		}
		c.nInFrames++
		// Append to headers
		copy(c.buffer.buf[headers.pEdge:], c.buffer.buf[c.cBack:c.cFore]) // overwrite padding if exists
		headers.pEdge += continuation.length
		headers.length += continuation.length // we don't care that padding is overwrite. just accumulate
		c.pFore += continuation.length        // also accumulate headers payload, with padding included
		// End of headers?
		if continuation.endHeaders {
			headers.endHeaders = true
			headers.buffer = c.buffer
			c.pFore = c.cFore // for next frame.
			break
		} else {
			c.cBack = c.cFore
		}
	}
	return nil
}
func (c *http2Conn) _growContinuation(size uint32, headers *http2InFrame) error {
	c.cFore += size // won't overflow
	if c.cFore <= c.bufferEdge {
		return nil
	}
	// Needs grow. Cases are (A is payload of the headers frame):
	// c.buffer: [| .. ] | A | 9 | B | 9 | C | 9 | D |
	// c.buffer: [| .. ] | AB | oooo | 9 | C | 9 | D |
	// c.buffer: [| .. ] | ABC | ooooooooooo | 9 | D |
	// c.buffer: [| .. ] | ABCD | oooooooooooooooooo |
	if c.cFore > c.buffer.size() { // needs slide
		if c.pBack == 0 { // cannot slide again
			// This should only happens when looking for header, the 9 bytes
			return http2ErrorFrameSize
		}
		// Now slide. Skip holes (if any) when sliding
		buffer := c.buffer
		if c.buffer.getRef() != 1 { // there are still streams referring to c.buffer. use a new buffer
			c.buffer = getHTTP2Buffer()
			c.buffer.incRef()
		}
		c.pFore = uint32(copy(c.buffer.buf[:], buffer.buf[c.pBack:c.pFore]))
		c.bufferEdge = c.pFore + uint32(copy(c.buffer.buf[c.pFore:], buffer.buf[c.cBack:c.bufferEdge]))
		if buffer != c.buffer {
			buffer.decRef()
		}
		headers.pFrom -= c.pBack
		headers.pEdge -= c.pBack
		c.pBack = 0
		c.cBack = c.pFore
		c.cFore = c.cBack + size
	}
	return c._fillBuffer(c.cFore - c.bufferEdge)
}

func (c *http2Conn) sendFrame(outFrame *http2OutFrame) error {
	header := outFrame.encodeHeader()
	if len(outFrame.payload) > 0 {
		c.vector = c.fixedVector[0:2]
		c.vector[1] = outFrame.payload
	} else {
		c.vector = c.fixedVector[0:1]
	}
	c.vector[0] = header
	n, err := c.writev(&c.vector)
	if DbgLevel() >= 2 {
		Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Printf("conn=%d ---> %+v\n", c.id, outFrame)
	}
	return err
}

func (c *http2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *http2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *http2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}
func (c *http2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *http2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}

func (c *http2Conn) closeConn() {
	if DbgLevel() >= 2 {
		Printf("conn=%d connClosed by serve()\n", c.id)
	}
	c.netConn.Close()
	c.gate.OnConnClosed()
}

// poolHTTP2Stream is the server-side HTTP/2 stream pool.
var poolHTTP2Stream sync.Pool

func getHTTP2Stream(conn *http2Conn, id uint32, outWindow int32) *http2Stream {
	var stream *http2Stream
	if x := poolHTTP2Stream.Get(); x == nil {
		stream = new(http2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		stream = x.(*http2Stream)
	}
	stream.onUse(conn, id, outWindow)
	return stream
}
func putHTTP2Stream(stream *http2Stream) {
	stream.onEnd()
	poolHTTP2Stream.Put(stream)
}

// http2Stream is the server-side HTTP/2 stream.
type http2Stream struct {
	// Mixins
	_webStream_
	// Assocs
	request  http2Request  // the http/2 request.
	response http2Response // the http/2 response.
	socket   *http2Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn      *http2Conn
	id        uint32 // stream id
	inWindow  int32  // stream-level window size for incoming DATA frames
	outWindow int32  // stream-level window size for outgoing DATA frames
	// Stream states (zeros)
	http2Stream0 // all values must be zero by default in this struct!
}
type http2Stream0 struct { // for fast reset, entirely
	index uint8 // index in s.conn.streams
	state uint8 // http2StateOpen, http2StateRemoteClosed, ...
	reset bool  // received a RST_STREAM?
}

func (s *http2Stream) onUse(conn *http2Conn, id uint32, outWindow int32) { // for non-zeros
	s._webStream_.onUse()
	s.conn = conn
	s.id = id
	s.inWindow = _64K1 // max size of r.bodyWindow
	s.outWindow = outWindow
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *http2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s.http2Stream0 = http2Stream0{}
	s._webStream_.onEnd()
}

func (s *http2Stream) execute() { // runner
	defer putHTTP2Stream(s)
	// TODO ...
	if DbgLevel() >= 2 {
		Println("stream processing...")
	}
}

func (s *http2Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *http2Stream) executeExchan(webapp *Webapp, req *http2Request, resp *http2Response) { // request & response
	// TODO
	webapp.exchanDispatch(req, resp)
}
func (s *http2Stream) serveAbnormal(req *http2Request, resp *http2Response) { // 4xx & 5xx
	// TODO
}

func (s *http2Stream) executeSocket() { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
}

func (s *http2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *http2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *http2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *http2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

func (s *http2Stream) webAgent() webAgent   { return s.conn.webServer() }
func (s *http2Stream) webConn() webConn     { return s.conn }
func (s *http2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *http2Stream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *http2Stream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *http2Stream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *http2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

// http1Request is the server-side HTTP/1 request.
type http1Request struct { // incoming. needs parsing
	// Parent
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
	if !r._recvControl() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if DbgLevel() >= 2 {
		Printf("[http1Stream=%d]<------- [%s]\n", r.stream.webConn().ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *http1Request) _recvControl() bool { // method SP request-target SP HTTP-version CRLF
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

// http2Request is the server-side HTTP/2 request.
type http2Request struct { // incoming. needs parsing
	// Parent
	webServerRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http2Request) joinHeaders(p []byte) bool {
	if len(p) > 0 {
		if !r._growHeaders2(int32(len(p))) {
			return false
		}
		r.inputEdge += int32(copy(r.input[r.inputEdge:], p))
	}
	return true
}
func (r *http2Request) readContent() (p []byte, err error) { return r.readContent2() }
func (r *http2Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// http1Response is the server-side HTTP/1 response.
type http1Response struct { // outgoing. needs building
	// Parent
	webServerResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http1Response) control() []byte { // HTTP/1.1 xxx ?
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
func (r *http1Response) setConnectionClose() { r.stream.webConn().setPersistent(false) }

func (r *http1Response) AddCookie(cookie *Cookie) bool {
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

func (r *http1Response) proxyPass1xx(resp WebBackendResponse) bool {
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
		r.fieldsEdge += uint16(r.stream.webAgent().Stage().Clock().writeDate1(r.fields[r.fieldsEdge:]))
	}
	// expires: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.expires >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesExpires, r.unixTimes.expires))
	}
	// last-modified: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.lastModified >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesLastModified, r.unixTimes.lastModified))
	}
	conn := r.stream.webConn().(*http1Conn)
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
				conn.persistent = false // close conn anyway for HTTP/1.0
			}
		}
		// content-type: text/html; charset=utf-8\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeHTMLUTF8))
		}
	}
	if conn.persistent { // connection: keep-alive\r\n
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

// http2Response is the server-side HTTP/2 response.
type http2Response struct { // outgoing. needs building
	// Parent
	webServerResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http2Response) control() []byte { // :status xxx
	var start []byte
	if r.status >= int16(len(http2Controls)) || http2Controls[r.status] == nil {
		copy(r.start[:], http2Template[:])
		r.start[8] = byte(r.status/100 + '0')
		r.start[9] = byte(r.status/10%10 + '0')
		r.start[10] = byte(r.status%10 + '0')
		start = r.start[:len(http2Template)]
	} else {
		start = http2Controls[r.status]
	}
	return start
}

func (r *http2Response) addHeader(name []byte, value []byte) bool   { return r.addHeader2(name, value) }
func (r *http2Response) header(name []byte) (value []byte, ok bool) { return r.header2(name) }
func (r *http2Response) hasHeader(name []byte) bool                 { return r.hasHeader2(name) }
func (r *http2Response) delHeader(name []byte) (deleted bool)       { return r.delHeader2(name) }
func (r *http2Response) delHeaderAt(i uint8)                        { r.delHeaderAt2(i) }

func (r *http2Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *http2Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *http2Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}

func (r *http2Response) AddCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *http2Response) sendChain() error { return r.sendChain2() }

func (r *http2Response) echoHeaders() error { return r.writeHeaders2() }
func (r *http2Response) echoChain() error   { return r.echoChain2() }

func (r *http2Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}
func (r *http2Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer2(name)
}

func (r *http2Response) proxyPass1xx(resp WebBackendResponse) bool {
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
func (r *http2Response) passHeaders() error       { return r.writeHeaders2() }
func (r *http2Response) passBytes(p []byte) error { return r.passBytes2(p) }

func (r *http2Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			r.fieldsEdge += uint16(r.stream.webAgent().Stage().Clock().writeDate1(r.fields[r.fieldsEdge:]))
		}
	*/
}
func (r *http2Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *http2Response) addedHeaders() []byte { return nil } // TODO
func (r *http2Response) fixedHeaders() []byte { return nil } // TODO

// poolHTTP1Socket
var poolHTTP1Socket sync.Pool

func getHTTP1Socket(stream *http1Stream) *http1Socket {
	return nil
}
func putHTTP1Socket(socket *http1Socket) {
}

// http1Socket is the server-side HTTP/1 websocket.
type http1Socket struct {
	// Parent
	webServerSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *http1Socket) onUse() {
	s.webServerSocket_.onUse()
}
func (s *http1Socket) onEnd() {
	s.webServerSocket_.onEnd()
}

// poolHTTP2Socket
var poolHTTP2Socket sync.Pool

func getHTTP2Socket(stream *http2Stream) *http2Socket {
	return nil
}
func putHTTP2Socket(socket *http2Socket) {
}

// http2Socket is the server-side HTTP/2 websocket.
type http2Socket struct {
	// Parent
	webServerSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *http2Socket) onUse() {
	s.webServerSocket_.onUse()
}
func (s *http2Socket) onEnd() {
	s.webServerSocket_.onEnd()
}
