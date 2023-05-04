// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 server implementation.

// For simplicity, HTTP/3 Server Push is not supported.

package internal

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

func init() {
	RegisterServer("http3Server", func(name string, stage *Stage) Server {
		s := new(http3Server)
		s.onCreate(name, stage)
		return s
	})
	RegisterHandlet("http3Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(http3Proxy)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterSocklet("sock3Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock3Proxy)
		s.onCreate(name, stage, app)
		return s
	})
}

// http3Server is the HTTP/3 server.
type http3Server struct {
	// Mixins
	webServer_
	// States
}

func (s *http3Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
	s.tlsConfig = new(tls.Config) // TLS mode is always enabled.
}
func (s *http3Server) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *http3Server) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *http3Server) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *http3Server) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(http3Gate)
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
		Debugf("http3Server=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// http3Gate is a gate of HTTP/3 server.
type http3Gate struct {
	// Mixins
	webGate_
	// Assocs
	server *http3Server
	// States
	gate *quix.Gate // the real gate. set after open
}

func (g *http3Gate) init(server *http3Server, id int32) {
	g.webGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *http3Gate) open() error {
	gate := quix.NewGate(g.address)
	if err := gate.Open(); err != nil {
		return err
	}
	g.gate = gate
	return nil
}
func (g *http3Gate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *http3Gate) serve() { // goroutine
	connID := int64(0)
	for {
		quicConn, err := g.gate.Accept()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(quicConn)
		} else {
			http3Conn := getHTTP3Conn(connID, g.server, g, quicConn)
			go http3Conn.serve() // http3Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("http3Gate=%d done\n", g.id)
	}
	g.server.SubDone()
}

func (g *http3Gate) justClose(quicConn *quix.Conn) {
	quicConn.Close()
	g.onConnectionClosed()
}

// poolHTTP3Conn is the server-side HTTP/3 connection pool.
var poolHTTP3Conn sync.Pool

func getHTTP3Conn(id int64, server *http3Server, gate *http3Gate, quicConn *quix.Conn) *http3Conn {
	var conn *http3Conn
	if x := poolHTTP3Conn.Get(); x == nil {
		conn = new(http3Conn)
	} else {
		conn = x.(*http3Conn)
	}
	conn.onGet(id, server, gate, quicConn)
	return conn
}
func putHTTP3Conn(conn *http3Conn) {
	conn.onPut()
	poolHTTP3Conn.Put(conn)
}

// http3Conn is the server-side HTTP/3 connection.
type http3Conn struct {
	// Mixins
	serverConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *quix.Conn        // the quic conn
	frames   *http3Frames      // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	streams    [http3MaxActiveStreams]*http3Stream // active (open, remoteClosed, localClosed) streams
	http3Conn0                                     // all values must be zero by default in this struct!
}
type http3Conn0 struct { // for fast reset, entirely
	framesEdge uint32 // incoming data ends at c.frames.buf[c.framesEdge]
	pBack      uint32 // incoming frame part (header or payload) begins from c.frames.buf[c.pBack]
	pFore      uint32 // incoming frame part (header or payload) ends at c.frames.buf[c.pFore]
}

func (c *http3Conn) onGet(id int64, server *http3Server, gate *http3Gate, quicConn *quix.Conn) {
	c.serverConn_.onGet(id, server, gate)
	c.quicConn = quicConn
	if c.frames == nil {
		c.frames = getHTTP3Frames()
		c.frames.incRef()
	}
}
func (c *http3Conn) onPut() {
	c.serverConn_.onPut()
	c.quicConn = nil
	// c.frames is reserved
	// c.table is reserved
	c.streams = [http3MaxActiveStreams]*http3Stream{}
	c.http3Conn0 = http3Conn0{}
}

func (c *http3Conn) serve() { // goroutine
	// TODO
	// use go c.receive()?
}
func (c *http3Conn) receive() { // goroutine
	// TODO
}

func (c *http3Conn) setReadDeadline(deadline time.Time) error {
	// TODO
	return nil
}
func (c *http3Conn) setWriteDeadline(deadline time.Time) error {
	// TODO
	return nil
}

func (c *http3Conn) closeConn() {
	c.quicConn.Close()
	c.gate.onConnectionClosed()
}

// poolHTTP3Stream is the server-side HTTP/3 stream pool.
var poolHTTP3Stream sync.Pool

func getHTTP3Stream(conn *http3Conn, quicStream *quix.Stream) *http3Stream {
	var stream *http3Stream
	if x := poolHTTP3Stream.Get(); x == nil {
		stream = new(http3Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		stream = x.(*http3Stream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putHTTP3Stream(stream *http3Stream) {
	stream.onEnd()
	poolHTTP3Stream.Put(stream)
}

// http3Stream is the server-side HTTP/3 stream.
type http3Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  http3Request  // the http/3 request.
	response http3Response // the http/3 response.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *http3Conn   // ...
	quicStream *quix.Stream // the underlying quic stream
	// Stream states (zeros)
	http3Stream0 // all values must be zero by default in this struct!
}
type http3Stream0 struct { // for fast reset, entirely
	index uint8
	state uint8
	reset bool
}

func (s *http3Stream) onUse(conn *http3Conn, quicStream *quix.Stream) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *http3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webStream_.onEnd()
	s.conn = nil
	s.quicStream = nil
	s.http3Stream0 = http3Stream0{}
}

func (s *http3Stream) execute() { // goroutine
	// TODO ...
	putHTTP3Stream(s)
}

func (s *http3Stream) webAgent() webAgent { return s.conn.getServer() }
func (s *http3Stream) peerAddr() net.Addr { return nil } // TODO

func (s *http3Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *http3Stream) executeWebApp(app *App, req *http3Request, resp *http3Response) { // request & response
	// TODO
	app.dispatchHandlet(req, resp)
}
func (s *http3Stream) executeRPCSvc(svc *Svc, req *http3Request, resp *http3Response) { // request & response
	// TODO
	svc.dispatchHRPC(req, resp)
}
func (s *http3Stream) serveAbnormal(req *http3Request, resp *http3Response) { // 4xx & 5xx
	// TODO
}
func (s *http3Stream) executeSocket() { // see RFC 9220
	// TODO, use s.socketServer()
}
func (s *http3Stream) executeTCPTun() { // CONNECT method
	// TODO, use s.tcpTunServer()
}
func (s *http3Stream) executeUDPTun() { // see RFC 9298
	// TODO, use s.udpTunServer()
}

func (s *http3Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *http3Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *http3Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *http3Stream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *http3Stream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *http3Stream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *http3Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
}

func (s *http3Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *http3Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

// http3Request is the server-side HTTP/3 request.
type http3Request struct { // incoming. needs parsing
	// Mixins
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Request) readContent() (p []byte, err error) { return r.readContentH3() }

// http3Response is the server-side HTTP/3 response.
type http3Response struct { // outgoing. needs building
	// Mixins
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Response) addHeader(name []byte, value []byte) bool   { return r.addHeaderH3(name, value) }
func (r *http3Response) header(name []byte) (value []byte, ok bool) { return r.headerH3(name) }
func (r *http3Response) hasHeader(name []byte) bool                 { return r.hasHeaderH3(name) }
func (r *http3Response) delHeader(name []byte) (deleted bool)       { return r.delHeaderH3(name) }
func (r *http3Response) delHeaderAt(o uint8)                        { r.delHeaderAtH3(o) }

func (r *http3Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *http3Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *http3Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}
func (r *http3Response) setConnectionClose() { BugExitln("not used in HTTP/3") }

func (r *http3Response) SetCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *http3Response) sendChain() error { return r.sendChainH3() }

func (r *http3Response) echoHeaders() error { return r.writeHeadersH3() }
func (r *http3Response) echoChain() error   { return r.echoChainH3() }

func (r *http3Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailerH3(name, value)
}
func (r *http3Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailerH3(name)
}

func (r *http3Response) pass1xx(resp clientResponse) bool { // used by proxies
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
	r.onUse(Version3)
	return false
}
func (r *http3Response) passHeaders() error       { return r.writeHeadersH3() }
func (r *http3Response) passBytes(p []byte) error { return r.passBytesH3(p) }

func (r *http3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *http3Response) finalizeUnsized() error {
	// TODO
	return nil
}

func (r *http3Response) addedHeaders() []byte { return nil }
func (r *http3Response) fixedHeaders() []byte { return nil }

// http3Proxy handlet passes requests to another/backend HTTP/3 servers and cache responses.
type http3Proxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *http3Proxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *http3Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http3Proxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *http3Proxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *http3Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	var (
		content  any
		conn3    *H3Conn
		err3     error
		content3 any
	)

	hasContent := req.HasContent()
	if hasContent && h.bufferClientContent { // including size 0
		content = req.takeContent()
		if content == nil { // take failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.isForward {
		outgate3 := h.stage.http3Outgate
		conn3, err3 = outgate3.FetchConn(req.Authority(), req.IsHTTPS()) // TODO
		if err3 != nil {
			if IsDebug(1) {
				Debugln(err3.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer conn3.closeConn() // TODO
	} else { // reverse
		backend3 := h.backend.(*HTTP3Backend)
		conn3, err3 = backend3.FetchConn()
		if err3 != nil {
			if IsDebug(1) {
				Debugln(err3.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer backend3.StoreConn(conn3)
	}

	stream3 := conn3.FetchStream()
	defer conn3.StoreStream(stream3)

	// TODO: use stream3.ForwardProxy() or stream3.ReverseProxy()

	req3 := stream3.Request()
	if !req3.copyHeadFrom(req, h.hostname, h.colonPort, h.viaName) {
		stream3.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if !hasContent || h.bufferClientContent {
		hasTrailers := req.HasTrailers()
		err3 = req3.post(content, hasTrailers) // nil (no content), []byte, tempFile
		if err3 == nil && hasTrailers {
			if !req3.copyTailFrom(req) {
				stream3.markBroken()
				err3 = webOutTrailerFailed
			} else if err3 = req3.endUnsized(); err3 != nil {
				stream3.markBroken()
			}
		} else if hasTrailers {
			stream3.markBroken()
		}
	} else if err3 = req3.pass(req); err3 != nil {
		stream3.markBroken()
	} else if req3.isUnsized() { // write last chunk and trailers (if exist)
		if err3 = req3.endUnsized(); err3 != nil {
			stream3.markBroken()
		}
	}
	if err3 != nil {
		resp.SendBadGateway(nil)
		return
	}

	resp3 := stream3.Response()
	for { // until we found a non-1xx status (>= 200)
		//resp3.recvHead()
		if resp3.headResult != StatusOK || resp3.Status() == StatusSwitchingProtocols { // websocket is not served in handlets.
			stream3.markBroken()
			if resp3.headResult == StatusRequestTimeout {
				resp.SendGatewayTimeout(nil)
			} else {
				resp.SendBadGateway(nil)
			}
			return
		}
		if resp3.Status() >= StatusOK {
			break
		}
		// We got 1xx
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.pass1xx(resp3) {
			stream3.markBroken()
			return
		}
		resp3.onEnd()
		resp3.onUse(Version3)
	}

	hasContent3 := false
	if req.MethodCode() != MethodHEAD {
		hasContent3 = resp3.HasContent()
	}
	if hasContent3 && h.bufferServerContent { // including size 0
		content3 = resp3.takeContent()
		if content3 == nil { // take failed
			// stream3 is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHeadFrom(resp3, nil) {
		stream3.markBroken()
		return
	}
	if !hasContent3 || h.bufferServerContent {
		hasTrailers3 := resp3.HasTrailers()
		if resp.post(content3, hasTrailers3) != nil { // nil (no content), []byte, tempFile
			if hasTrailers3 {
				stream3.markBroken()
			}
			return
		} else if hasTrailers3 && !resp.copyTailFrom(resp3) {
			return
		}
	} else if err := resp.pass(resp3); err != nil {
		stream3.markBroken()
		return
	}
	return
}

// poolHTTP3Socket
var poolHTTP3Socket sync.Pool

// http3Socket is the server-side HTTP/3 websocket.
type http3Socket struct {
	// Mixins
	serverSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

// sock3Proxy socklet passes websockets to another/backend WebSocket/3 servers.
type sock3Proxy struct {
	// Mixins
	socketProxy_
	// States
}

func (s *sock3Proxy) onCreate(name string, stage *Stage, app *App) {
	s.socketProxy_.onCreate(name, stage, app)
}
func (s *sock3Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock3Proxy) OnConfigure() {
	s.socketProxy_.onConfigure()
}
func (s *sock3Proxy) OnPrepare() {
	s.socketProxy_.onPrepare()
}

func (s *sock3Proxy) Serve(req Request, sock Socket) { // forward or reverse
	// TODO(diogin): Implementation
	if s.isForward {
	} else {
	}
	sock.Close()
}
