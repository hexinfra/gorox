// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 server implementation. See RFC 9114 and RFC 9204.

package hemi

import (
	"crypto/tls"
	"sync"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
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
		s.IncSub() // gate
		if s.UDSMode() {
			go gate.serveUDS()
		} else {
			go gate.serveTLS()
		}
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("http3Server=%s done\n", s.CompName())
	}
	s.stage.DecSub() // server
}

// http3Gate is a gate of http3Server.
type http3Gate struct {
	// Parent
	httpGate_[*http3Server]
	// States
	listener *tcp2.Listener // the real gate. set after open
}

func (g *http3Gate) onNew(server *http3Server, id int32) {
	g.httpGate_.onNew(server, id)
}

func (g *http3Gate) Open() error {
	listener := tcp2.NewListener(g.Address())
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

func (g *http3Gate) serveUDS() { // runner
	// TODO
}
func (g *http3Gate) serveTLS() { // runner
	connID := int64(0)
	for {
		quicConn, err := g.listener.Accept()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub() // conn
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			g.justClose(quicConn)
			continue
		}
		server3Conn := getServer3Conn(connID, g, quicConn)
		go server3Conn.manager() // server3Conn is put to pool in manager()
		connID++
	}
	g.WaitSubs() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http3Gate=%d done\n", g.id)
	}
	g.server.DecSub() // gate
}

func (g *http3Gate) justClose(quicConn *tcp2.Conn) {
	quicConn.Close()
	g.DecConcurrentConns()
	g.DecSub() // conn
}

// server3Conn is the server-side HTTP/3 connection.
type server3Conn struct {
	// Parent
	http3Conn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *http3Gate
	// Conn states (zeros)
	_server3Conn0 // all values in this struct must be zero by default!
}
type _server3Conn0 struct { // for fast reset, entirely
}

var poolServer3Conn sync.Pool

func getServer3Conn(id int64, gate *http3Gate, quicConn *tcp2.Conn) *server3Conn {
	var serverConn *server3Conn
	if x := poolServer3Conn.Get(); x == nil {
		serverConn = new(server3Conn)
	} else {
		serverConn = x.(*server3Conn)
	}
	serverConn.onGet(id, gate, quicConn)
	return serverConn
}
func putServer3Conn(serverConn *server3Conn) {
	serverConn.onPut()
	poolServer3Conn.Put(serverConn)
}

func (c *server3Conn) onGet(id int64, gate *http3Gate, quicConn *tcp2.Conn) {
	c.http3Conn_.onGet(id, gate.Stage(), gate.UDSMode(), gate.TLSMode(), quicConn, gate.ReadTimeout(), gate.WriteTimeout())

	c.gate = gate
}
func (c *server3Conn) onPut() {
	c._server3Conn0 = _server3Conn0{}
	c.gate = nil

	c.http3Conn_.onPut()
}

func (c *server3Conn) manager() { // runner
	// TODO
	// use go c.receiver()?
}

func (c *server3Conn) receiver() { // runner
	// TODO
}

func (c *server3Conn) closeConn() {
	c.quicConn.Close()
	c.gate.DecConcurrentConns()
	c.gate.DecSub()
}

// server3Stream is the server-side HTTP/3 stream.
type server3Stream struct {
	// Parent
	http3Stream_[*server3Conn]
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

func getServer3Stream(conn *server3Conn, quicStream *tcp2.Stream) *server3Stream {
	var serverStream *server3Stream
	if x := poolServer3Stream.Get(); x == nil {
		serverStream = new(server3Stream)
		req, resp := &serverStream.request, &serverStream.response
		req.stream = serverStream
		req.inMessage = req
		resp.stream = serverStream
		resp.outMessage = resp
		resp.request = req
	} else {
		serverStream = x.(*server3Stream)
	}
	serverStream.onUse(conn, quicStream)
	return serverStream
}
func putServer3Stream(serverStream *server3Stream) {
	serverStream.onEnd()
	poolServer3Stream.Put(serverStream)
}

func (s *server3Stream) onUse(conn *server3Conn, quicStream *tcp2.Stream) { // for non-zeros
	s.http3Stream_.onUse(conn, quicStream)

	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *server3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._server3Stream0 = _server3Stream0{}

	s.http3Stream_.onEnd()
	s.conn = nil // we can't do this in http3Stream_.onEnd() due to Go's limit, so put here
}

func (s *server3Stream) Holder() httpHolder { return s.conn.gate }

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
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Request) readContent() (data []byte, err error) { return r.readContent3() }

// server3Response is the server-side HTTP/3 response.
type server3Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Response) control() []byte { // :status NNN
	var start []byte
	if r.status >= int16(len(http3Controls)) || http3Controls[r.status] == nil {
		copy(r.start[:], http3Template[:])
		r.start[8] = byte(r.status/100 + '0')
		r.start[9] = byte(r.status/10%10 + '0')
		r.start[10] = byte(r.status%10 + '0')
		start = r.start[:len(http3Template)]
	} else {
		start = http3Controls[r.status]
	}
	return start
}

func (r *server3Response) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *server3Response) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *server3Response) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *server3Response) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *server3Response) delHeaderAt(i uint8)                        { r.delHeaderAt3(i) }

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

func (r *server3Response) sendChain() error { return r.sendChain3() }

func (r *server3Response) echoHeaders() error { return r.writeHeaders3() }
func (r *server3Response) echoChain() error   { return r.echoChain3() }

func (r *server3Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}
func (r *server3Response) trailer(name []byte) (value []byte, ok bool) { return r.trailer3(name) }

func (r *server3Response) proxyPass1xx(backResp backendResponse) bool {
	backResp.proxyDelHopHeaders()
	r.status = backResp.Status()
	if !backResp.proxyWalkHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.nameHash, name, value) // some headers are restricted
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse(Version3)
	return false
}
func (r *server3Response) proxyPassHeaders() error          { return r.writeHeaders3() }
func (r *server3Response) proxyPassBytes(data []byte) error { return r.proxyPassBytes3(data) }

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
}
func (s *server3Socket) onEnd() {
	s.serverSocket_.onEnd()
}
