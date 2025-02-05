// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 server implementation. See RFC 9113 and RFC 7541.

package hemi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/library/system"
)

func init() {
	RegisterServer("httpxServer", func(compName string, stage *Stage) Server {
		s := new(httpxServer)
		s.onCreate(compName, stage)
		return s
	})
}

// httpxServer is the HTTP/1.x and HTTP/2 server. An httpxServer has many httpxGates.
type httpxServer struct {
	// Parent
	httpServer_[*httpxGate]
	// States
	httpMode int8 // 0: adaptive, 1: http/1.x, 2: http/2
}

func (s *httpxServer) onCreate(compName string, stage *Stage) {
	s.httpServer_.onCreate(compName, stage)

	s.httpMode = 1 // http/1.x by default. change to adaptive mode after http/2 server has been fully implemented
}

func (s *httpxServer) OnConfigure() {
	s.httpServer_.onConfigure()

	if DebugLevel() >= 2 { // remove this condition after http/2 server has been fully implemented
		// .httpMode
		var mode string
		s.ConfigureString("httpMode", &mode, func(value string) error {
			value = strings.ToLower(value)
			switch value {
			case "http1", "http/1", "http/1.x", "http2", "http/2", "adaptive":
				return nil
			default:
				return errors.New(".httpMode has an invalid value")
			}
		}, "adaptive")
		switch mode {
		case "http1", "http/1", "http/1.x":
			s.httpMode = 1
		case "http2", "http/2":
			s.httpMode = 2
		default:
			s.httpMode = 0
		}
	}
}
func (s *httpxServer) OnPrepare() {
	s.httpServer_.onPrepare()

	if s.TLSMode() {
		var nextProtos []string
		switch s.httpMode {
		case 2:
			nextProtos = []string{"h2"}
		case 1:
			nextProtos = []string{"http/1.1"}
		default: // adaptive mode
			nextProtos = []string{"h2", "http/1.1"}
		}
		s.tlsConfig.NextProtos = nextProtos
	}
}

func (s *httpxServer) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(httpxGate)
		gate.onNew(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSubGate()
		go gate.Serve()
	}
	s.WaitSubGates()
	if DebugLevel() >= 2 {
		Printf("httpxServer=%s done\n", s.CompName())
	}
	s.stage.DecSub() // server
}

// httpxGate is a gate of httpxServer.
type httpxGate struct {
	// Parent
	httpGate_[*httpxServer]
	// States
	listener net.Listener // the real gate. set after open
}

func (g *httpxGate) onNew(server *httpxServer, id int32) {
	g.httpGate_.onNew(server, id)
}

func (g *httpxGate) Open() error {
	var (
		listener net.Listener
		err      error
	)
	if g.UDSMode() {
		address := g.Address()
		// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
		// This affects graceful upgrading, maybe we can implement fd transfer in the future.
		os.Remove(address)
		if listener, err = net.Listen("unix", address); err == nil {
			g.listener = listener.(*net.UnixListener)
			if DebugLevel() >= 1 {
				Printf("httpxGate id=%d address=%s opened!\n", g.id, g.Address())
			}
		}
	} else {
		listenConfig := new(net.ListenConfig)
		listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
			if err := system.SetReusePort(rawConn); err != nil {
				return err
			}
			return system.SetDeferAccept(rawConn)
		}
		if listener, err = listenConfig.Listen(context.Background(), "tcp", g.Address()); err == nil {
			g.listener = listener.(*net.TCPListener)
			if DebugLevel() >= 1 {
				Printf("httpxGate id=%d address=%s opened!\n", g.id, g.Address())
			}
		}
	}
	return err
}
func (g *httpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *httpxGate) Serve() { // runner
	if g.UDSMode() {
		g.serveUDS()
	} else if g.TLSMode() {
		g.serveTLS()
	} else {
		g.serveTCP()
	}
}
func (g *httpxGate) serveUDS() {
	listener := g.listener.(*net.UnixListener)
	connID := int64(1)
	for {
		udsConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.compName, g.id, err)
				continue
			}
		}
		g.IncSubConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			g.justClose(udsConn)
			continue
		}
		rawConn, err := udsConn.SyscallConn()
		if err != nil {
			g.justClose(udsConn)
			//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.compName, g.id, err)
			continue
		}
		if g.server.httpMode == 2 {
			servConn := getServer2Conn(connID, g, udsConn, rawConn)
			go servConn.manager() // servConn is put to pool in manager()
		} else {
			servConn := getServer1Conn(connID, g, udsConn, rawConn)
			go servConn.manager() // servConn is put to pool in manager()
		}
		connID++
	}
	g.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSubGate()
}
func (g *httpxGate) serveTLS() {
	listener := g.listener.(*net.TCPListener)
	connID := int64(1)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.compName, g.id, err)
				continue
			}
		}
		g.IncSubConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			g.justClose(tcpConn)
			continue
		}
		tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
		// TODO: configure timeout
		if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
			g.justClose(tlsConn)
			continue
		}
		if connState := tlsConn.ConnectionState(); connState.NegotiatedProtocol == "h2" {
			servConn := getServer2Conn(connID, g, tlsConn, nil)
			go servConn.manager() // servConn is put to pool in manager()
		} else {
			servConn := getServer1Conn(connID, g, tlsConn, nil)
			go servConn.manager() // servConn is put to pool in manager()
		}
		connID++
	}
	g.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TLS done\n", g.id)
	}
	g.server.DecSubGate()
}
func (g *httpxGate) serveTCP() {
	listener := g.listener.(*net.TCPListener)
	connID := int64(1)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.compName, g.id, err)
				continue
			}
		}
		g.IncSubConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			g.justClose(tcpConn)
			continue
		}
		rawConn, err := tcpConn.SyscallConn()
		if err != nil {
			g.justClose(tcpConn)
			//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.compName, g.id, err)
			continue
		}
		if g.server.httpMode == 2 {
			servConn := getServer2Conn(connID, g, tcpConn, rawConn)
			go servConn.manager() // servConn is put to pool in manager()
		} else {
			servConn := getServer1Conn(connID, g, tcpConn, rawConn)
			go servConn.manager() // servConn is put to pool in manager()
		}
		connID++
	}
	g.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSubGate()
}

func (g *httpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecConcurrentConns()
	g.DecSubConn()
}

// server2Conn is the server-side HTTP/2 connection.
type server2Conn struct {
	// Parent
	http2Conn_
	// Mixins
	_serverConn_[*httpxGate]
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_server2Conn0 // all values in this struct must be zero by default!
}
type _server2Conn0 struct { // for fast reset, entirely
	lastStreamID uint32 // last received stream id from client
	waitReceive  bool   // ...
	//unackedSettings?
	//queuedControlFrames?
}

var poolServer2Conn sync.Pool

func getServer2Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) *server2Conn {
	var servConn *server2Conn
	if x := poolServer2Conn.Get(); x == nil {
		servConn = new(server2Conn)
	} else {
		servConn = x.(*server2Conn)
	}
	servConn.onGet(id, gate, netConn, rawConn)
	return servConn
}
func putServer2Conn(servConn *server2Conn) {
	servConn.onPut()
	poolServer2Conn.Put(servConn)
}

func (c *server2Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.http2Conn_.onGet(id, gate, netConn, rawConn)
	c._serverConn_.onGet(gate)
}
func (c *server2Conn) onPut() {
	c._server2Conn0 = _server2Conn0{}

	c._serverConn_.onPut()
	c.gate = nil // put here due to Go's limitation
	c.http2Conn_.onPut()
}

func (c *server2Conn) manager() { // runner
	Printf("========================== conn=%d start =========================\n", c.id)
	defer func() {
		Printf("========================== conn=%d exit =========================\n", c.id)
		putServer2Conn(c)
	}()
	if err := c._handshake(); err != nil {
		c.closeConn()
		return
	}
	// Successfully handshake means we have acknowledged client settings and sent our settings. Still need to receive a settings ACK from client.
	go c.receiver()
serve:
	for { // each frame from c.receiver() and server streams
		select {
		case incoming := <-c.incomingChan: // got an incoming frame from c.receiver()
			if inFrame, ok := incoming.(*http2InFrame); ok { // data, fields, priority, rst_stream, settings, ping, windows_update, unknown
				if inFrame.isUnknown() {
					// Implementations MUST ignore and discard frames of unknown types.
					continue
				}
				if err := server2InFrameProcessors[inFrame.kind](c, inFrame); err == nil {
					// Successfully processed. Next one.
					continue
				} else if h2e, ok := err.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else { // processor i/o error
					c.goawayCloseConn(http2ErrorInternal)
				}
				// c.manager() was broken, but c.receiver() was not. need wait
				c.waitReceive = true
			} else { // c.receiver() was broken and quit.
				if h2e, ok := incoming.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else if netErr, ok := incoming.(net.Error); ok && netErr.Timeout() {
					c.goawayCloseConn(http2ErrorNoError)
				} else {
					c.closeConn()
				}
			}
			break serve
		case outFrame := <-c.outgoingChan: // got an outgoing frame from streams. MUST be fields frame or data frame!
			// TODO: collect as many outgoing frames as we can?
			Printf("%+v\n", outFrame)
			if outFrame.endStream { // a stream has ended
				c.quitStream(outFrame.stream)
				c.concurrentStreams--
			}
			if err := c.sendOutFrame(outFrame); err != nil {
				// send side is broken.
				c.closeConn()
				c.waitReceive = true
				break serve
			}
		}
	}
	Printf("conn=%d waiting for active streams to end\n", c.id)
	for c.concurrentStreams > 0 {
		if outFrame := <-c.outgoingChan; outFrame.endStream {
			c.quitStream(outFrame.stream)
			c.concurrentStreams--
		}
	}
	if c.waitReceive {
		Printf("conn=%d waiting for c.receiver() to quit\n", c.id)
		for {
			incoming := <-c.incomingChan
			if _, ok := incoming.(*http2InFrame); !ok {
				// An error from c.receiver() means it's quit
				break
			}
		}
	}
	Printf("conn=%d c.manager() quit\n", c.id)
}

var server2PrefaceAndMore = []byte{
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

func (c *server2Conn) _handshake() error {
	// Set deadline for the first request fields frame
	if err := c.setReadDeadline(); err != nil {
		return err
	}
	if err := c._growInFrame(uint16(len(http2BytesPrism))); err != nil {
		return err
	}
	// Check client connection preface = PRISM + SETTINGS
	if !bytes.Equal(c.inBuffer.buf[0:len(http2BytesPrism)], http2BytesPrism) {
		return http2ErrorProtocol
	}
	settingsInFrame, err := c.recvInFrame()
	if err != nil {
		return err
	}
	if settingsInFrame.kind != http2FrameSettings || settingsInFrame.ack {
		return http2ErrorProtocol
	}
	if err := c._updatePeerSettings(settingsInFrame); err != nil {
		return err
	}
	// Send server connection preface
	if err := c.setWriteDeadline(); err != nil {
		return err
	}
	n, err := c.write(server2PrefaceAndMore)
	Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
	Printf("conn=%d ---> %v\n", c.id, server2PrefaceAndMore)
	if err != nil {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	return err
}

var server2InFrameProcessors = [http2NumFrameKinds]func(*server2Conn, *http2InFrame) error{
	(*server2Conn).onDataInFrame,
	(*server2Conn).onFieldsInFrame,
	(*server2Conn).onPriorityInFrame,
	(*server2Conn).onRSTStreamInFrame,
	(*server2Conn).onSettingsInFrame,
	nil, // pushPromise frames are rejected priorly
	(*server2Conn).onPingInFrame,
	nil, // goaway frames are hijacked by c.receiver()
	(*server2Conn).onWindowUpdateInFrame,
	nil, // discrete continuation frames are rejected priorly
}

func (c *server2Conn) onDataInFrame(dataInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *server2Conn) onFieldsInFrame(fieldsInFrame *http2InFrame) error {
	var (
		servStream *server2Stream
		servReq    *server2Request
	)
	streamID := fieldsInFrame.streamID
	if streamID > c.lastStreamID { // new stream
		if c.concurrentStreams == http2MaxConcurrentStreams {
			return http2ErrorProtocol
		}
		c.lastStreamID = streamID
		c.cumulativeStreams.Add(1)
		servStream = getServer2Stream(c, streamID, c.peerSettings.initialWindowSize)
		servReq = &servStream.request
		if !c._decodeFields(fieldsInFrame.effective(), servReq.joinHeaders) {
			putServer2Stream(servStream)
			return http2ErrorCompression
		}
		if fieldsInFrame.endStream {
			servStream.state = http2StateRemoteClosed
		} else {
			servStream.state = http2StateOpen
		}
		c.joinStream(servStream)
		c.concurrentStreams++
		go servStream.execute()
	} else { // old stream
		stream := c.findStream(streamID)
		if stream == nil { // no specified active stream
			return http2ErrorProtocol
		}
		servStream = stream.(*server2Stream)
		if servStream.state != http2StateOpen {
			return http2ErrorProtocol
		}
		if !fieldsInFrame.endStream { // here must be trailer fields that end the stream
			return http2ErrorProtocol
		}
		servReq = &servStream.request
		servReq.receiving = httpSectionTrailers
		if !c._decodeFields(fieldsInFrame.effective(), servReq.joinTrailers) {
			return http2ErrorCompression
		}
	}
	return nil
}
func (c *server2Conn) onRSTStreamInFrame(rstStreamInFrame *http2InFrame) error {
	streamID := rstStreamInFrame.streamID
	if streamID > c.lastStreamID {
		// RST_STREAM frames MUST NOT be sent for a stream in the "idle" state. If a RST_STREAM frame identifying an idle stream is received,
		// the recipient MUST treat this as a connection error (Section 5.4.1) of type PROTOCOL_ERROR.
		return http2ErrorProtocol
	}
	// TODO
	return nil
}
func (c *server2Conn) onSettingsInFrame(settingsInFrame *http2InFrame) error {
	if settingsInFrame.ack {
		c.acknowledged = true
		return nil
	}
	// TODO: client sent a new settings
	return nil
}
func (c *server2Conn) _updatePeerSettings(settingsInFrame *http2InFrame) error {
	settings := settingsInFrame.effective()
	windowDelta := int32(0)
	for i, j, n := uint16(0), uint16(0), settingsInFrame.length/6; i < n; i++ {
		ident := binary.BigEndian.Uint16(settings[j : j+2])
		value := binary.BigEndian.Uint32(settings[j+2 : j+6])
		switch ident {
		case http2SettingHeaderTableSize:
			c.peerSettings.headerTableSize = value
			// TODO: Dynamic Table Size Update
		case http2SettingEnablePush:
			if value > 1 {
				return http2ErrorProtocol
			}
			c.peerSettings.enablePush = false // we don't support server push
		case http2SettingMaxConcurrentStreams:
			c.peerSettings.maxConcurrentStreams = value
			// TODO: notify shrink
		case http2SettingInitialWindowSize:
			if value > _2G1 {
				return http2ErrorFlowControl
			}
			windowDelta = int32(value) - c.peerSettings.initialWindowSize
		case http2SettingMaxFrameSize:
			if value < _16K || value > _16M-1 {
				return http2ErrorProtocol
			}
			c.peerSettings.maxFrameSize = value
		case http2SettingMaxHeaderListSize: // this is only an advisory.
			c.peerSettings.maxHeaderListSize = value
		default:
			// RFC 9113 (section 6.5.2): An endpoint that receives a SETTINGS frame with any unknown or unsupported identifier MUST ignore that setting.
		}
		j += 6
	}
	if windowDelta != 0 {
		c.peerSettings.initialWindowSize += windowDelta
		c._adjustStreamWindows(windowDelta)
	}
	Printf("conn=%d peerSettings=%+v\n", c.id, c.peerSettings)
	return nil
}
func (c *server2Conn) _adjustStreamWindows(delta int32) {
	// TODO
}

func (c *server2Conn) goawayCloseConn(h2e http2Error) {
	goawayOutFrame := &c.outFrame
	goawayOutFrame.stream = nil
	goawayOutFrame.length = 8
	goawayOutFrame.kind = http2FrameGoaway
	binary.BigEndian.PutUint32(goawayOutFrame.outBuffer[0:4], c.lastStreamID)
	binary.BigEndian.PutUint32(goawayOutFrame.outBuffer[4:8], uint32(h2e))
	goawayOutFrame.payload = goawayOutFrame.outBuffer[0:8]
	c.sendOutFrame(goawayOutFrame) // ignore error
	goawayOutFrame.zero()
	c.closeConn()
}

func (c *server2Conn) closeConn() {
	if DebugLevel() >= 2 {
		Printf("conn=%d connClosed by manager()\n", c.id)
	}
	c.netConn.Close()
	c.gate.DecConcurrentConns()
	c.gate.DecSubConn()
}

// server2Stream is the server-side HTTP/2 stream.
type server2Stream struct {
	// Parent
	http2Stream_[*server2Conn]
	// Mixins
	_serverStream_
	// Assocs
	request  server2Request  // the http/2 request.
	response server2Response // the http/2 response.
	socket   *server2Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	inWindow  int32 // stream-level window size for incoming DATA frames
	outWindow int32 // stream-level window size for outgoing DATA frames
	// Stream states (zeros)
	_server2Stream0 // all values in this struct must be zero by default!
}
type _server2Stream0 struct { // for fast reset, entirely
	reset bool // received a RST_STREAM?
}

var poolServer2Stream sync.Pool

func getServer2Stream(conn *server2Conn, id uint32, outWindow int32) *server2Stream {
	var servStream *server2Stream
	if x := poolServer2Stream.Get(); x == nil {
		servStream = new(server2Stream)
		servReq, servResp := &servStream.request, &servStream.response
		servReq.stream = servStream
		servReq.in = servReq
		servResp.stream = servStream
		servResp.out = servResp
		servResp.request = servReq
	} else {
		servStream = x.(*server2Stream)
	}
	servStream.onUse(id, conn, outWindow)
	return servStream
}
func putServer2Stream(servStream *server2Stream) {
	servStream.onEnd()
	poolServer2Stream.Put(servStream)
}

func (s *server2Stream) onUse(id uint32, conn *server2Conn, outWindow int32) { // for non-zeros
	s.http2Stream_.onUse(id, conn)
	s._serverStream_.onUse()

	s.inWindow = _64K1 // max size of r.bodyWindow
	s.outWindow = outWindow
	s.request.onUse()
	s.response.onUse()
}
func (s *server2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._server2Stream0 = _server2Stream0{}

	s._serverStream_.onEnd()
	s.http2Stream_.onEnd()
	s.conn = nil // we can't do this in http2Stream_.onEnd() due to Go's limit, so put here
}

func (s *server2Stream) execute() { // runner
	defer putServer2Stream(s)
	// TODO ...
	if DebugLevel() >= 2 {
		Println("stream processing...")
	}
}
func (s *server2Stream) _serveAbnormal(req *server2Request, resp *server2Response) { // 4xx & 5xx
	// TODO
}
func (s *server2Stream) _writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *server2Stream) executeExchan(webapp *Webapp, req *server2Request, resp *server2Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server2Stream) executeSocket() { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
}

// server2Request is the server-side HTTP/2 request.
type server2Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Assocs
	in2 _http2In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server2Request) onUse() {
	r.serverRequest_.onUse(Version2)
	r.in2.onUse(&r._httpIn_)
}
func (r *server2Request) onEnd() {
	r.serverRequest_.onEnd()
	r.in2.onEnd()
}

func (r *server2Request) joinHeaders(p []byte) bool {
	if len(p) > 0 {
		if !r.in2._growHeaders(int32(len(p))) {
			return false
		}
		r.inputEdge += int32(copy(r.input[r.inputEdge:], p))
	}
	return true
}
func (r *server2Request) readContent() (data []byte, err error) { return r.in2.readContent() }
func (r *server2Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// server2Response is the server-side HTTP/2 response.
type server2Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Assocs
	out2 _http2Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server2Response) onUse() {
	r.serverResponse_.onUse(Version2)
	r.out2.onUse(&r._httpOut_)
}
func (r *server2Response) onEnd() {
	r.serverResponse_.onEnd()
	r.out2.onEnd()
}

func (r *server2Response) addHeader(name []byte, value []byte) bool {
	return r.out2.addHeader(name, value)
}
func (r *server2Response) header(name []byte) (value []byte, ok bool) { return r.out2.header(name) }
func (r *server2Response) hasHeader(name []byte) bool                 { return r.out2.hasHeader(name) }
func (r *server2Response) delHeader(name []byte) (deleted bool)       { return r.out2.delHeader(name) }
func (r *server2Response) delHeaderAt(i uint8)                        { r.out2.delHeaderAt(i) }

func (r *server2Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *server2Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *server2Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}

func (r *server2Response) AddCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *server2Response) sendChain() error { return r.out2.sendChain() }

func (r *server2Response) echoHeaders() error { return r.out2.writeHeaders() }
func (r *server2Response) echoChain() error   { return r.out2.echoChain() }

func (r *server2Response) addTrailer(name []byte, value []byte) bool {
	return r.out2.addTrailer(name, value)
}
func (r *server2Response) trailer(name []byte) (value []byte, ok bool) { return r.out2.trailer(name) }

func (r *server2Response) proxyPass1xx(backResp BackendResponse) bool {
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
func (r *server2Response) proxyPassHeaders() error          { return r.out2.writeHeaders() }
func (r *server2Response) proxyPassBytes(data []byte) error { return r.out2.proxyPassBytes(data) }

func (r *server2Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			clock := r.stream.(*server2Stream).conn.gate.stage.clock
			r.fieldsEdge += uint16(clock.writeDate2(r.fields[r.fieldsEdge:]))
		}
	*/
}
func (r *server2Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *server2Response) addedHeaders() []byte { return nil } // TODO
func (r *server2Response) fixedHeaders() []byte { return nil } // TODO

// server2Socket is the server-side HTTP/2 webSocket.
type server2Socket struct { // incoming and outgoing
	// Parent
	serverSocket_
	// Assocs
	so2 _http2Socket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolServer2Socket sync.Pool

func getServer2Socket(stream *server2Stream) *server2Socket {
	// TODO
	return nil
}
func putServer2Socket(socket *server2Socket) {
	// TODO
}

func (s *server2Socket) onUse() {
	s.serverSocket_.onUse()
	s.so2.onUse(&s._httpSocket_)
}
func (s *server2Socket) onEnd() {
	s.serverSocket_.onEnd()
	s.so2.onEnd()
}

func (s *server2Socket) serverTodo2() {
	s.serverTodo()
	s.so2.todo2()
}
