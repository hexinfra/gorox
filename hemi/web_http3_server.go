// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 server implementation. See RFC 9114 and 9204.

// Server Push is not supported because it's rarely used.

package hemi

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/library/quic"
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
	// Parent
	httpServer_[*http3Gate]
	// States
}

func (s *http3Server) onCreate(name string, stage *Stage) {
	s.httpServer_.onCreate(name, stage)
	s.tlsConfig = new(tls.Config) // tls mode is always enabled
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
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub()
		go gate.serve()
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("http3Server=%s done\n", s.Name())
	}
	s.stage.DecSub()
}

// http3Gate is a gate of http3Server.
type http3Gate struct {
	// Parent
	Gate_
	// Assocs
	server *http3Server
	// States
	listener *quic.Listener // the real gate. set after open
}

func (g *http3Gate) init(id int32, server *http3Server) {
	g.Gate_.Init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *http3Gate) Server() Server  { return g.server }
func (g *http3Gate) Address() string { return g.server.Address() }
func (g *http3Gate) IsTLS() bool     { return g.server.IsTLS() }
func (g *http3Gate) IsUDS() bool     { return g.server.IsUDS() }

func (g *http3Gate) Open() error {
	listener := quic.NewListener(g.Address())
	if err := listener.Open(); err != nil {
		return err
	}
	g.listener = listener
	return nil
}
func (g *http3Gate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *http3Gate) serve() { // runner
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
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(quicConn)
		} else {
			server3Conn := getServer3Conn(connID, g, quicConn)
			go server3Conn.serve() // server3Conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http3Gate=%d done\n", g.id)
	}
	g.server.DecSub()
}

func (g *http3Gate) justClose(quicConn *quic.Conn) {
	quicConn.Close()
	g.OnConnClosed()
}

// poolServer3Conn is the server-side HTTP/3 connection pool.
var poolServer3Conn sync.Pool

func getServer3Conn(id int64, gate *http3Gate, quicConn *quic.Conn) *server3Conn {
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

// server3Conn is the server-side HTTP/3 connection.
type server3Conn struct {
	// Parent
	ServerConn_
	// Mixins
	_httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	server   *http3Server
	gate     *http3Gate
	quicConn *quic.Conn        // the quic connection
	buffer   *http3Buffer      // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	streams      [http3MaxActiveStreams]*server3Stream // active (open, remoteClosed, localClosed) streams
	server3Conn0                                       // all values must be zero by default in this struct!
}
type server3Conn0 struct { // for fast reset, entirely
	bufferEdge uint32 // incoming data ends at c.buffer.buf[c.bufferEdge]
	pBack      uint32 // incoming frame part (header or payload) begins from c.buffer.buf[c.pBack]
	pFore      uint32 // incoming frame part (header or payload) ends at c.buffer.buf[c.pFore]
}

func (c *server3Conn) onGet(id int64, gate *http3Gate, quicConn *quic.Conn) {
	c.ServerConn_.OnGet(id)
	c._httpConn_.onGet()
	c.server = gate.server
	c.gate = gate
	c.quicConn = quicConn
	if c.buffer == nil {
		c.buffer = getHTTP3Buffer()
		c.buffer.incRef()
	}
}
func (c *server3Conn) onPut() {
	c.quicConn = nil
	// c.buffer is reserved
	// c.table is reserved
	c.streams = [http3MaxActiveStreams]*server3Stream{}
	c.server3Conn0 = server3Conn0{}
	c.server = nil
	c.gate = nil
	c._httpConn_.onPut()
	c.ServerConn_.OnPut()
}

func (c *server3Conn) IsTLS() bool { return c.server.IsTLS() }
func (c *server3Conn) IsUDS() bool { return c.server.IsUDS() }

func (c *server3Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *server3Conn) HTTPServer() HTTPServer { return c.server }

func (c *server3Conn) serve() { // runner
	// TODO
	// use go c.receive()?
}

func (c *server3Conn) receive() { // runner
	// TODO
}

func (c *server3Conn) setReadDeadline(deadline time.Time) error {
	// TODO
	return nil
}
func (c *server3Conn) setWriteDeadline(deadline time.Time) error {
	// TODO
	return nil
}

func (c *server3Conn) write(p []byte) (int, error) { return 0, nil } // TODO

func (c *server3Conn) closeConn() {
	c.quicConn.Close()
	c.gate.OnConnClosed()
}

// poolServer3Stream is the server-side HTTP/3 stream pool.
var poolServer3Stream sync.Pool

func getServer3Stream(conn *server3Conn, quicStream *quic.Stream) *server3Stream {
	var stream *server3Stream
	if x := poolServer3Stream.Get(); x == nil {
		stream = new(server3Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		stream = x.(*server3Stream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putServer3Stream(stream *server3Stream) {
	stream.onEnd()
	poolServer3Stream.Put(stream)
}

// server3Stream is the server-side HTTP/3 stream.
type server3Stream struct {
	// Mixins
	_httpStream_
	// Assocs
	request  server3Request  // the http/3 request.
	response server3Response // the http/3 response.
	socket   *server3Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       *server3Conn
	quicStream *quic.Stream // the underlying quic stream
	// Stream states (zeros)
	server3Stream0 // all values must be zero by default in this struct!
}
type server3Stream0 struct { // for fast reset, entirely
	index uint8
	state uint8
	reset bool
}

func (s *server3Stream) onUse(conn *server3Conn, quicStream *quic.Stream) { // for non-zeros
	s._httpStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
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
	s.conn = nil
	s.quicStream = nil
	s.server3Stream0 = server3Stream0{}
	s._httpStream_.onEnd()
}

func (s *server3Stream) execute() { // runner
	// TODO ...
	putServer3Stream(s)
}

func (s *server3Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *server3Stream) executeExchan(webapp *Webapp, req *server3Request, resp *server3Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server3Stream) serveAbnormal(req *server3Request, resp *server3Response) { // 4xx & 5xx
	// TODO
}

func (s *server3Stream) executeSocket() { // see RFC 9220
	// TODO
}

func (s *server3Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *server3Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *server3Stream) isBroken() bool { return false } // TODO
func (s *server3Stream) markBroken()    {}               // TODO

func (s *server3Stream) httpServend() httpServend { return s.conn.HTTPServer() }
func (s *server3Stream) httpConn() httpConn       { return s.conn }
func (s *server3Stream) remoteAddr() net.Addr     { return nil } // TODO

func (s *server3Stream) read(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *server3Stream) readFull(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *server3Stream) write(p []byte) (int, error) { // for content i/o only
	return 0, nil
}
func (s *server3Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	return 0, nil
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

func (r *server3Request) readContent() (p []byte, err error) { return r.readContent3() }

// server3Response is the server-side HTTP/3 response.
type server3Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server3Response) control() []byte { // :status xxx
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

func (r *server3Response) proxyPass1xx(resp backendResponse) bool {
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
func (r *server3Response) passHeaders() error       { return r.writeHeaders3() }
func (r *server3Response) passBytes(p []byte) error { return r.passBytes3(p) }

func (r *server3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			r.fieldsEdge += uint16(r.stream.webServend().Stage().Clock().writeDate1(r.fields[r.fieldsEdge:]))
		}
	*/
}
func (r *server3Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *server3Response) addedHeaders() []byte { return nil }
func (r *server3Response) fixedHeaders() []byte { return nil }

// poolServer3Socket
var poolServer3Socket sync.Pool

func getServer3Socket(stream *server3Stream) *server3Socket {
	return nil
}
func putServer3Socket(socket *server3Socket) {
}

// server3Socket is the server-side HTTP/3 websocket.
type server3Socket struct {
	// Parent
	serverSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server3Socket) onUse() {
	s.serverSocket_.onUse()
}
func (s *server3Socket) onEnd() {
	s.serverSocket_.onEnd()
}
