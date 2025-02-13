// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 server implementation. See RFC 9114 and RFC 9204.

package hemi

import (
	"crypto/tls"
	"sync"

	"github.com/hexinfra/gorox/hemi/library/gotcp2"
)

func init() {
	RegisterServer("http3Server", func(compName string, stage *Stage) Server {
		s := new(http3Server)
		s.onCreate(compName, stage)
		return s
	})
}

// http3Server is the HTTP/3 server. An http3Server has many http3Gates.
type http3Server struct {
	// Parent
	httpServer_[*http3Gate]
	// States
}

func (s *http3Server) onCreate(compName string, stage *Stage) {
	s.httpServer_.onCreate(compName, stage)
	s.tlsConfig = new(tls.Config) // currently tls mode is always enabled in http/3
}

func (s *http3Server) OnConfigure() {
	s.httpServer_.onConfigure()
}
func (s *http3Server) OnPrepare() {
	s.httpServer_.onPrepare()
}

func (s *http3Server) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(http3Gate)
		gate.onNew(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		go gate.Serve()
	}
	s.WaitGates()
	if DebugLevel() >= 2 {
		Printf("http3Server=%s done\n", s.CompName())
	}
	s.stage.DecServer()
}

// http3Gate is a gate of http3Server.
type http3Gate struct {
	// Parent
	httpGate_[*http3Server]
	// States
	listener *gotcp2.Listener // the real gate. set after open
}

func (g *http3Gate) onNew(server *http3Server, id int32) {
	g.httpGate_.onNew(server, id)
}

func (g *http3Gate) Open() error {
	// TODO: udsMode or tlsMode?
	listener := gotcp2.NewListener(g.Address())
	if err := listener.Open(); err != nil {
		return err
	}
	g.listener = listener
	return nil
}
func (g *http3Gate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *http3Gate) Serve() { // runner
	if g.UDSMode() {
		g.serveUDS()
	} else {
		g.serveTLS()
	}
}
func (g *http3Gate) serveUDS() {
	// TODO
}
func (g *http3Gate) serveTLS() {
	connID := int64(1)
	for {
		quicConn, err := g.listener.Accept()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			g.justClose(quicConn)
			continue
		}
		server3Conn := getServer3Conn(connID, g, quicConn)
		go server3Conn.serve() // server3Conn is put to pool in serve()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http3Gate=%d done\n", g.id)
	}
	g.server.DecGate()
}

func (g *http3Gate) justClose(quicConn *gotcp2.Conn) {
	quicConn.Close()
	g.DecConcurrentConns()
	g.DecConn()
}

// server3Conn is the server-side HTTP/3 connection.
type server3Conn struct {
	// Parent
	http3Conn_
	// Mixins
	_serverConn_[*http3Gate]
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_server3Conn0 // all values in this struct must be zero by default!
}
type _server3Conn0 struct { // for fast reset, entirely
}

var poolServer3Conn sync.Pool

func getServer3Conn(id int64, gate *http3Gate, quicConn *gotcp2.Conn) *server3Conn {
	var servConn *server3Conn
	if x := poolServer3Conn.Get(); x == nil {
		servConn = new(server3Conn)
	} else {
		servConn = x.(*server3Conn)
	}
	servConn.onGet(id, gate, quicConn)
	return servConn
}
func putServer3Conn(servConn *server3Conn) {
	servConn.onPut()
	poolServer3Conn.Put(servConn)
}

func (c *server3Conn) onGet(id int64, gate *http3Gate, quicConn *gotcp2.Conn) {
	c.http3Conn_.onGet(id, gate, quicConn)
	c._serverConn_.onGet(gate)
}
func (c *server3Conn) onPut() {
	c._server3Conn0 = _server3Conn0{}

	c._serverConn_.onPut()
	c.gate = nil // put here due to Go's limitation
	c.http3Conn_.onPut()
}

func (c *server3Conn) serve() { // runner
	// TODO
	// use go c.receiver()?
}

func (c *server3Conn) closeConn() {
	c.quicConn.Close()
	c.gate.DecConcurrentConns()
	c.gate.DecConn()
}

// server3Stream is the server-side HTTP/3 stream.
type server3Stream struct {
	// Parent
	http3Stream_[*server3Conn]
	// Mixins
	_serverStream_
	// Assocs
	request  server3Request  // the http/3 request.
	response server3Response // the http/3 response.
	socket   *server3Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_server3Stream0 // all values in this struct must be zero by default!
}
type _server3Stream0 struct { // for fast reset, entirely
	index uint8
	state uint8
	reset bool
}

var poolServer3Stream sync.Pool

func getServer3Stream(conn *server3Conn, quicStream *gotcp2.Stream) *server3Stream {
	var servStream *server3Stream
	if x := poolServer3Stream.Get(); x == nil {
		servStream = new(server3Stream)
		servReq, servResp := &servStream.request, &servStream.response
		servReq.stream = servStream
		servReq.in = servReq
		servResp.stream = servStream
		servResp.out = servResp
		servResp.request = servReq
	} else {
		servStream = x.(*server3Stream)
	}
	servStream.onUse(conn, quicStream)
	return servStream
}
func putServer3Stream(servStream *server3Stream) {
	servStream.onEnd()
	poolServer3Stream.Put(servStream)
}

func (s *server3Stream) onUse(conn *server3Conn, quicStream *gotcp2.Stream) { // for non-zeros
	s.http3Stream_.onUse(conn, quicStream)
	s._serverStream_.onUse()

	s.request.onUse()
	s.response.onUse()
}
func (s *server3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._server3Stream0 = _server3Stream0{}

	s._serverStream_.onEnd()
	s.http3Stream_.onEnd()
	s.conn = nil // we can't do this in http3Stream_.onEnd() due to Go's limit, so put here
}

func (s *server3Stream) execute() { // runner
	// TODO ...
	putServer3Stream(s)
}
func (s *server3Stream) _serveAbnormal(req *server3Request, resp *server3Response) { // 4xx & 5xx
	// TODO
}
func (s *server3Stream) _writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *server3Stream) executeExchan(webapp *Webapp, req *server3Request, resp *server3Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server3Stream) executeSocket() { // see RFC 9220
	// TODO
}

// server3Request is the server-side HTTP/3 request.
type server3Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Assocs
	in3 _http3In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Request) onUse() {
	r.serverRequest_.onUse(Version3)
	r.in3.onUse(&r._httpIn_)
}
func (r *server3Request) onEnd() {
	r.serverRequest_.onEnd()
	r.in3.onEnd()
}

func (r *server3Request) readContent() (data []byte, err error) { return r.in3.readContent() }

// server3Response is the server-side HTTP/3 response.
type server3Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Assocs
	out3 _http3Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Response) onUse() {
	r.serverResponse_.onUse(Version3)
	r.out3.onUse(&r._httpOut_)
}
func (r *server3Response) onEnd() {
	r.serverResponse_.onEnd()
	r.out3.onEnd()
}

func (r *server3Response) addHeader(name []byte, value []byte) bool {
	return r.out3.addHeader(name, value)
}
func (r *server3Response) header(name []byte) (value []byte, ok bool) { return r.out3.header(name) }
func (r *server3Response) hasHeader(name []byte) bool                 { return r.out3.hasHeader(name) }
func (r *server3Response) delHeader(name []byte) (deleted bool)       { return r.out3.delHeader(name) }
func (r *server3Response) delHeaderAt(i uint8)                        { r.out3.delHeaderAt(i) }

func (r *server3Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *server3Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *server3Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}

func (r *server3Response) AddCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *server3Response) sendChain() error { return r.out3.sendChain() }

func (r *server3Response) echoHeaders() error { return r.out3.writeHeaders() }
func (r *server3Response) echoChain() error   { return r.out3.echoChain() }

func (r *server3Response) addTrailer(name []byte, value []byte) bool {
	return r.out3.addTrailer(name, value)
}
func (r *server3Response) trailer(name []byte) (value []byte, ok bool) { return r.out3.trailer(name) }

func (r *server3Response) proxyPass1xx(backResp BackendResponse) bool {
	backResp.proxyDelHopHeaderFields()
	r.status = backResp.Status()
	if !backResp.proxyWalkHeaderLines(r, func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool {
		return out.insertHeader(headerLine.nameHash, headerName, lineValue) // some header fields (e.g. "connection") are restricted
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse()
	return false
}
func (r *server3Response) proxyPassHeaders() error          { return r.out3.writeHeaders() }
func (r *server3Response) proxyPassBytes(data []byte) error { return r.out3.proxyPassBytes(data) }

func (r *server3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			clock := r.stream.(*server3Stream).conn.gate.stage.clock
			r.fieldsEdge += uint16(clock.writeDate3(r.fields[r.fieldsEdge:]))
		}
	*/
}
func (r *server3Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *server3Response) addedHeaders() []byte { return nil }
func (r *server3Response) fixedHeaders() []byte { return nil }

// server3Socket is the server-side HTTP/3 webSocket.
type server3Socket struct { // incoming and outgoing
	// Parent
	serverSocket_
	// Assocs
	so3 _http3Socket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolServer3Socket sync.Pool

func getServer3Socket(stream *server3Stream) *server3Socket {
	// TODO
	return nil
}
func putServer3Socket(socket *server3Socket) {
	// TODO
}

func (s *server3Socket) onUse() {
	s.serverSocket_.onUse()
	s.so3.onUse(&s._httpSocket_)
}
func (s *server3Socket) onEnd() {
	s.serverSocket_.onEnd()
	s.so3.onEnd()
}

func (s *server3Socket) serverTodo3() {
	s.serverTodo()
	s.so3.todo3()
}
