// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 server implementation. See RFC 9114 and 9204.

// For simplicity, HTTP/3 Server Push is not supported.

package hemi

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
}

// http3Server is the HTTP/3 server.
type http3Server struct {
	// Mixins
	webServer_[*http3Gate]
	// States
}

func (s *http3Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
	s.tlsConfig = new(tls.Config) // tls mode is always enabled
}
func (s *http3Server) OnShutdown() {
	s.webServer_.onShutdown()
}

func (s *http3Server) OnConfigure() {
	s.webServer_.onConfigure(s)
}
func (s *http3Server) OnPrepare() {
	s.webServer_.onPrepare(s)
}

func (s *http3Server) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(http3Gate)
		gate.init(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub(1)
		if s.udsMode {
			go gate.serveUDS()
		} else { // tls mode is always enabled
			go gate.serveTLS()
		}
	}
	s.WaitSubs() // gates
	if Debug() >= 2 {
		Printf("http3Server=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// http3Gate is a gate of HTTP/3 server.
type http3Gate struct {
	// Mixins
	Gate_
	// Assocs
	server *http3Server
	// States
	gate *quix.Gate // the real gate. set after open
}

func (g *http3Gate) init(server *http3Server, id int32) {
	g.Gate_.Init(server.stage, id, server.udsMode, server.tlsMode, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *http3Gate) Open() error {
	gate := quix.NewGate(g.address)
	if err := gate.Open(); err != nil {
		return err
	}
	g.gate = gate
	return nil
}
func (g *http3Gate) Shut() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *http3Gate) serveTLS() { // runner
	connID := int64(0)
	for {
		quixConn, err := g.gate.Accept()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(quixConn)
		} else {
			http3Conn := getHTTP3Conn(connID, g.server, g, quixConn)
			go http3Conn.serve() // http3Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("http3Gate=%d done\n", g.id)
	}
	g.server.SubDone()
}
func (g *http3Gate) serveUDS() { // runner
	// TODO
}

func (g *http3Gate) justClose(quixConn *quix.Conn) {
	quixConn.Close()
	g.OnConnClosed()
}

// poolHTTP3Conn is the server-side HTTP/3 connection pool.
var poolHTTP3Conn sync.Pool

func getHTTP3Conn(id int64, server *http3Server, gate *http3Gate, quixConn *quix.Conn) *http3Conn {
	var httpConn *http3Conn
	if x := poolHTTP3Conn.Get(); x == nil {
		httpConn = new(http3Conn)
	} else {
		httpConn = x.(*http3Conn)
	}
	httpConn.onGet(id, server, gate, quixConn)
	return httpConn
}
func putHTTP3Conn(httpConn *http3Conn) {
	httpConn.onPut()
	poolHTTP3Conn.Put(httpConn)
}

// http3Conn is the server-side HTTP/3 connection.
type http3Conn struct {
	// Mixins
	webServerConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quixConn *quix.Conn        // the quic connection
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

func (c *http3Conn) onGet(id int64, server *http3Server, gate *http3Gate, quixConn *quix.Conn) {
	c.webServerConn_.onGet(id, server, gate)
	c.quixConn = quixConn
	if c.frames == nil {
		c.frames = getHTTP3Frames()
		c.frames.incRef()
	}
}
func (c *http3Conn) onPut() {
	c.webServerConn_.onPut()
	c.quixConn = nil
	// c.frames is reserved
	// c.table is reserved
	c.streams = [http3MaxActiveStreams]*http3Stream{}
	c.http3Conn0 = http3Conn0{}
}

func (c *http3Conn) serve() { // runner
	// TODO
	// use go c.receive()?
}
func (c *http3Conn) receive() { // runner
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
	c.quixConn.Close()
	c.gate.OnConnClosed()
}

// poolHTTP3Stream is the server-side HTTP/3 stream pool.
var poolHTTP3Stream sync.Pool

func getHTTP3Stream(conn *http3Conn, quixStream *quix.Stream) *http3Stream {
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
	stream.onUse(conn, quixStream)
	return stream
}
func putHTTP3Stream(stream *http3Stream) {
	stream.onEnd()
	poolHTTP3Stream.Put(stream)
}

// http3Stream is the server-side HTTP/3 stream.
type http3Stream struct {
	// Mixins
	webServerStream_
	// Assocs
	request  http3Request  // the http/3 request.
	response http3Response // the http/3 response.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *http3Conn   // ...
	quixStream *quix.Stream // the underlying quic stream
	// Stream states (zeros)
	http3Stream0 // all values must be zero by default in this struct!
}
type http3Stream0 struct { // for fast reset, entirely
	index uint8
	state uint8
	reset bool
}

func (s *http3Stream) onUse(conn *http3Conn, quixStream *quix.Stream) { // for non-zeros
	s.webServerStream_.onUse()
	s.conn = conn
	s.quixStream = quixStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *http3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.webServerStream_.onEnd()
	s.conn = nil
	s.quixStream = nil
	s.http3Stream0 = http3Stream0{}
}

func (s *http3Stream) execute() { // runner
	// TODO ...
	putHTTP3Stream(s)
}

func (s *http3Stream) webBroker() webBroker { return s.conn.webServer() }
func (s *http3Stream) webConn() webConn     { return s.conn }
func (s *http3Stream) remoteAddr() net.Addr { return nil } // TODO

func (s *http3Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *http3Stream) executeExchan(webapp *Webapp, req *http3Request, resp *http3Response) { // request & response
	// TODO
	webapp.exchanDispatch(req, resp)
}
func (s *http3Stream) serveAbnormal(req *http3Request, resp *http3Response) { // 4xx & 5xx
	// TODO
}
func (s *http3Stream) executeSocket() { // see RFC 9220
	// TODO
}

func (s *http3Stream) makeTempName(p []byte, unixTime int64) int {
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
	webServerRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Request) readContent() (p []byte, err error) { return r.readContent3() }

// http3Response is the server-side HTTP/3 response.
type http3Response struct { // outgoing. needs building
	// Mixins
	webServerResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Response) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *http3Response) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *http3Response) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *http3Response) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *http3Response) delHeaderAt(i uint8)                        { r.delHeaderAt3(i) }

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

func (r *http3Response) sendChain() error { return r.sendChain3() }

func (r *http3Response) echoHeaders() error { return r.writeHeaders3() }
func (r *http3Response) echoChain() error   { return r.echoChain3() }

func (r *http3Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}
func (r *http3Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer3(name)
}

func (r *http3Response) pass1xx(resp response) bool { // used by proxies
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
func (r *http3Response) passHeaders() error       { return r.writeHeaders3() }
func (r *http3Response) passBytes(p []byte) error { return r.passBytes3(p) }

func (r *http3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *http3Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *http3Response) addedHeaders() []byte { return nil }
func (r *http3Response) fixedHeaders() []byte { return nil }

// poolHTTP3Socket
var poolHTTP3Socket sync.Pool

// http3Socket is the server-side HTTP/3 websocket.
type http3Socket struct {
	// Mixins
	webServerSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}