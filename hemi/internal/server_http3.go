// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 server and its implementation.

// For simplicity, HTTP/3 Server Push is not supported.

package internal

import (
	"crypto/tls"
	"github.com/hexinfra/gorox/hemi/libraries/quix"
	"net"
	"sync"
	"time"
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
	httpServer_
	// States
}

func (s *http3Server) onCreate(name string, stage *Stage) {
	s.httpServer_.onCreate(name, stage)
	s.tlsConfig = new(tls.Config) // TLS mode is always enabled.
}
func (s *http3Server) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *http3Server) OnConfigure() {
	s.httpServer_.onConfigure(s)
}
func (s *http3Server) OnPrepare() {
	s.httpServer_.onPrepare(s)
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
	httpGate_
	// Assocs
	server *http3Server
	// States
	gate *quix.Gate
}

func (g *http3Gate) init(server *http3Server, id int32) {
	g.httpGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
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
	g.WaitSubs() // conns
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
	httpConn_
	// Conn states (buffers)
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
	c.httpConn_.onGet(id, server, gate)
	c.quicConn = quicConn
	if c.frames == nil {
		c.frames = getHTTP3Frames()
		c.frames.incRef()
	}
}
func (c *http3Conn) onPut() {
	c.httpConn_.onPut()
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
	httpStream_
	// Assocs
	request  http3Request  // the http3 request.
	response http3Response // the http3 response.
	// Stream states (buffers)
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
	s.httpStream_.onUse()
	s.conn = conn
	s.quicStream = quicStream
	s.request.onUse(Version3)
	s.response.onUse(Version3)
}
func (s *http3Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.httpStream_.onEnd()
	s.conn = nil
	s.quicStream = nil
	s.http3Stream0 = http3Stream0{}
}

func (s *http3Stream) execute() { // goroutine
	// TODO ...
	putHTTP3Stream(s)
}

func (s *http3Stream) keeper() keeper     { return s.conn.getServer() }
func (s *http3Stream) peerAddr() net.Addr { return nil } // TODO

func (s *http3Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *http3Stream) serveTCPTun() { // CONNECT method
	// TODO
}
func (s *http3Stream) serveUDPTun() { // see RFC 9298
	// TODO
}
func (s *http3Stream) serveSocket() { // see RFC 9220
	// TODO
}
func (s *http3Stream) serveNormal(app *App, req *http3Request, resp *http3Response) { // request & response
	// TODO
	app.dispatchHandlet(req, resp)
}
func (s *http3Stream) serveAbnormal(req *http3Request, resp *http3Response) { // 4xx & 5xx
	// TODO
}

func (s *http3Stream) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return s.conn.makeTempName(p, stamp)
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
	httpRequest_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Request) readContent() (p []byte, err error) {
	return r.readContent3()
}

// http3Response is the server-side HTTP/3 response.
type http3Response struct { // outgoing. needs building
	// Mixins
	httpResponse_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Response) header(name []byte) (value []byte, ok bool) { return r.header3(name) }
func (r *http3Response) hasHeader(name []byte) bool                 { return r.hasHeader3(name) }
func (r *http3Response) addHeader(name []byte, value []byte) bool   { return r.addHeader3(name, value) }
func (r *http3Response) delHeader(name []byte) (deleted bool)       { return r.delHeader3(name) }
func (r *http3Response) delHeaderAt(o uint8)                        { r.delHeaderAt3(o) }
func (r *http3Response) addedHeaders() []byte                       { return nil }
func (r *http3Response) fixedHeaders() []byte                       { return nil }

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

func (r *http3Response) sendChain(chain Chain) error {
	// TODO
	return r.sendChain2(chain, nil)
}

func (r *http3Response) pushHeaders() error { // headers are sent immediately upon pushing chunks.
	// TODO
	return nil
}
func (r *http3Response) pushChain(chain Chain) error {
	return r.pushChain3(chain)
}

func (r *http3Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer3(name)
}
func (r *http3Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer3(name, value)
}

func (r *http3Response) sync1xx(resp hResponse) bool { // used by proxies
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
func (r *http3Response) syncHeaders() error {
	// TODO
	return nil
}
func (r *http3Response) syncBytes(p []byte) error { return r.syncBytes3(p) }

func (r *http3Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *http3Response) finalizeUnsized() error {
	// TODO
	return nil
}

// poolHTTP3Socket
var poolHTTP3Socket sync.Pool

// http3Socket is the server-side HTTP/3 websocket.
type http3Socket struct {
	// Mixins
	httpSocket_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
