// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 server implementation.

// For simplicity, HTTP/3 Server Push is not supported.

package internal

import (
	"crypto/tls"
	"fmt"
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

func (s *http3Server) OnConfigure() {
	s.httpServer_.onConfigure()
}
func (s *http3Server) OnPrepare() {
	s.httpServer_.onPrepare()
}

func (s *http3Server) OnShutdown() {
	// We don't use s.Shutdown() here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
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
	if Debug(2) {
		fmt.Printf("http3Server=%s done\n", s.Name())
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
	gate *quix.Gate
}

func (g *http3Gate) init(server *http3Server, id int32) {
	g.Gate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
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
	g.Gate_.MarkShut()
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
	if Debug(2) {
		fmt.Printf("http3Gate=%d done\n", g.id)
	}
	g.server.SubDone()
}

func (g *http3Gate) justClose(quicConn *quix.Conn) {
	quicConn.Close()
	g.onConnectionClosed()
}
func (g *http3Gate) onConnectionClosed() {
	g.DecConns()
	g.SubDone()
}

// poolHTTP3Conn is the server-side HTTP/3 connection pool.
var poolHTTP3Conn sync.Pool

func getHTTP3Conn(id int64, server httpServer, gate *http3Gate, quicConn *quix.Conn) *http3Conn {
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
	gate     *http3Gate        // the gate to which the conn belongs
	quicConn *quix.Conn        // the quic conn
	inputs   *http3Inputs      // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	streams    [http3MaxActiveStreams]*http3Stream // active (open, remoteClosed, localClosed) streams
	http3Conn0                                     // all values must be zero by default in this struct!
}
type http3Conn0 struct { // for fast reset, entirely
	inputsEdge uint32 // incoming data ends at c.inputs.buf[c.inputsEdge]
	pBack      uint32 // incoming frame part (header or payload) begins from c.inputs.buf[c.pBack]
	pFore      uint32 // incoming frame part (header or payload) ends at c.inputs.buf[c.pFore]
}

func (c *http3Conn) onGet(id int64, server httpServer, gate *http3Gate, quicConn *quix.Conn) {
	c.httpConn_.onGet(id, server)
	c.gate = gate
	c.quicConn = quicConn
	if c.inputs == nil {
		c.allocInputs()
	}
}
func (c *http3Conn) onPut() {
	c.httpConn_.onPut()
	c.gate = nil
	c.quicConn = nil
	// c.inputs is reserved
	// c.table is reserved
	c.streams = [http3MaxActiveStreams]*http3Stream{}
	c.http3Conn0 = http3Conn0{}
}

func (c *http3Conn) allocInputs() {
	c.inputs = getHTTP3Inputs()
	c.inputs.incRef()
}

func (c *http3Conn) receive() {
	// TODO
}
func (c *http3Conn) serve() { // goroutine
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
	s.request.versionCode = Version3 // explicitly set
	s.request.onUse()
	s.response.onUse()
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
	// do
	putHTTP3Stream(s)
}

func (s *http3Stream) getHolder() holder {
	return s.conn.getServer()
}

func (s *http3Stream) peerAddr() net.Addr {
	// TODO
	return nil
}

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
	app.dispatchHandler(req, resp)
}
func (s *http3Stream) serveAbnormal(req *http3Request, resp *http3Response) { // 4xx & 5xx
	// TODO
}

func (s *http3Stream) makeTempName(p []byte, seconds int64) (from int, edge int) {
	return s.conn.makeTempName(p, seconds)
}

func (s *http3Stream) setReadDeadline(deadline time.Time) error { // for content only
	return nil
}
func (s *http3Stream) setWriteDeadline(deadline time.Time) error { // for content only
	return nil
}

func (s *http3Stream) read(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *http3Stream) readFull(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *http3Stream) write(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *http3Stream) writev(vector *net.Buffers) (int64, error) { // for content only
	return 0, nil
}

func (s *http3Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?
func (s *http3Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?

// http3Request is the server-side HTTP/3 request.
type http3Request struct {
	// Mixins
	httpRequest_
	// Assocs
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Request) joinHeaders(p []byte) bool {
	return false
}

func (r *http3Request) readContent() (p []byte, err error) {
	return r.readContent3()
}

func (r *http3Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// http3Response is the server-side HTTP/3 response.
type http3Response struct {
	// Mixins
	httpResponse_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http3Response) control() []byte {
	return nil
}

func (r *http3Response) header(name []byte) (value []byte, ok bool) {
	return r.header3(name)
}
func (r *http3Response) addHeader(name []byte, value []byte) bool {
	return r.addHeader3(name, value)
}
func (r *http3Response) delHeader(name []byte) bool {
	return r.delHeader3(name)
}
func (r *http3Response) addedHeaders() []byte {
	return nil
}
func (r *http3Response) fixedHeaders() []byte {
	return nil
}

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
func (r *http3Response) setConnectionClose() {
	BugExitln("not used in HTTP/3")
}

func (r *http3Response) AddCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *http3Response) sendChain(chain Chain) error {
	// TODO
	return nil
}

func (r *http3Response) pushHeaders() error {
	// TODO
	return nil
}
func (r *http3Response) pushChain(chain Chain) error {
	return r.pushChain3(chain)
}
func (r *http3Response) addTrailer(name []byte, value []byte) bool {
	// TODO
	return false
}

func (r *http3Response) pass1xx(resp response) bool { // used by proxies
	// TODO
	r.onEnd()
	r.onUse()
	return false
}
func (r *http3Response) passHeaders() error {
	return nil
}
func (r *http3Response) passBytes(p []byte) error {
	return nil
}

func (r *http3Response) finalizeHeaders() {
	// TODO
}
func (r *http3Response) finalizeChunked() error {
	// TODO
	return nil
}

// http3Socket is the server-side HTTP/3 websocket.
type http3Socket struct {
	// Mixins
	httpSocket_
	// Stream states (zeros)
}

func (s *http3Socket) onUse() {
	s.httpSocket_.onUse()
}
func (s *http3Socket) onEnd() {
	s.httpSocket_.onEnd()
}
