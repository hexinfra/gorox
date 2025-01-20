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
		s.IncSub() // gate
		if s.UDSMode() {
			go gate.serveUDS()
		} else if s.TLSMode() {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
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

func (g *httpxGate) serveUDS() { // runner
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
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
		g.IncSub() // conn
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
			serverConn := getServer2Conn(connID, g, udsConn, rawConn)
			go serverConn.manager() // serverConn is put to pool in manager()
		} else {
			serverConn := getServer1Conn(connID, g, udsConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
		}
		connID++
	}
	g.WaitSubs() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub() // gate
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
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.compName, g.id, err)
				continue
			}
		}
		g.IncSub() // conn
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
			serverConn := getServer2Conn(connID, g, tlsConn, nil)
			go serverConn.manager() // serverConn is put to pool in manager()
		} else {
			serverConn := getServer1Conn(connID, g, tlsConn, nil)
			go serverConn.serve() // serverConn is put to pool in serve()
		}
		connID++
	}
	g.WaitSubs() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TLS done\n", g.id)
	}
	g.server.DecSub() // gate
}
func (g *httpxGate) serveTCP() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
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
		g.IncSub() // conn
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
			serverConn := getServer2Conn(connID, g, tcpConn, rawConn)
			go serverConn.manager() // serverConn is put to pool in manager()
		} else {
			serverConn := getServer1Conn(connID, g, tcpConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
		}
		connID++
	}
	g.WaitSubs() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub() // gate
}

func (g *httpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecConcurrentConns()
	g.DecSub() // conn
}

// server2Conn is the server-side HTTP/2 connection.
type server2Conn struct {
	// Parent
	http2Conn_
	// Mixins
	_serverConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *httpxGate // the gate to which the connection belongs
	// Conn states (zeros)
	_server2Conn0 // all values in this struct must be zero by default!
}
type _server2Conn0 struct { // for fast reset, entirely
	lastStreamID uint32 // last received client stream id
	waitReceive  bool   // ...
	//unackedSettings?
	//queuedControlFrames?
}

var poolServer2Conn sync.Pool

func getServer2Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) *server2Conn {
	var serverConn *server2Conn
	if x := poolServer2Conn.Get(); x == nil {
		serverConn = new(server2Conn)
	} else {
		serverConn = x.(*server2Conn)
	}
	serverConn.onGet(id, gate, netConn, rawConn)
	return serverConn
}
func putServer2Conn(serverConn *server2Conn) {
	serverConn.onPut()
	poolServer2Conn.Put(serverConn)
}

func (c *server2Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.http2Conn_.onGet(id, gate.Stage(), gate.UDSMode(), gate.TLSMode(), gate.ReadTimeout(), gate.WriteTimeout(), netConn, rawConn)
	c._serverConn_.onGet()

	c.gate = gate
}
func (c *server2Conn) onPut() {
	c._server2Conn0 = _server2Conn0{}
	c.gate = nil

	c._serverConn_.onPut()
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
			if inFrame, ok := incoming.(*http2InFrame); ok { // data, headers, priority, rst_stream, settings, ping, windows_update, unknown
				if inFrame.isUnknown() {
					// Ignore unknown frames.
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
		case outFrame := <-c.outgoingChan: // got an outgoing frame from streams. only headers frame and data frame!
			// TODO: collect as many outgoing frames as we can?
			Printf("%+v\n", outFrame)
			if outFrame.endStream { // a stream has ended
				c.quitStream(outFrame.streamID)
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
			c.quitStream(outFrame.streamID)
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
func (c *server2Conn) _handshake() error {
	// Set deadline for the first request headers frame
	if err := c.setReadDeadline(); err != nil {
		return err
	}
	if err := c._growInFrame(uint32(len(http2BytesPrism))); err != nil {
		return err
	}
	if !bytes.Equal(c.inBuffer.buf[0:len(http2BytesPrism)], http2BytesPrism) {
		return http2ErrorProtocol
	}
	prefaceInFrame, err := c.recvInFrame()
	if err != nil {
		return err
	}
	if prefaceInFrame.kind != http2FrameSettings || prefaceInFrame.ack {
		return http2ErrorProtocol
	}
	if err := c._updatePeerSettings(prefaceInFrame); err != nil {
		return err
	}
	// TODO: write deadline
	n, err := c.write(server2PrefaceAndMore)
	Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
	Printf("conn=%d ---> %v\n", c.id, server2PrefaceAndMore)
	if err != nil {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	return err
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

var server2InFrameProcessors = [http2NumFrameKinds]func(*server2Conn, *http2InFrame) error{
	(*server2Conn).onDataInFrame,
	(*server2Conn).onHeadersInFrame,
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
func (c *server2Conn) onHeadersInFrame(headersInFrame *http2InFrame) error {
	var (
		stream *server2Stream
		req    *server2Request
	)
	streamID := headersInFrame.streamID
	if streamID > c.lastStreamID { // new stream
		if c.concurrentStreams == http2MaxConcurrentStreams {
			return http2ErrorProtocol
		}
		c.lastStreamID = streamID
		c.cumulativeStreams.Add(1)
		stream = getServer2Stream(c, streamID, c.peerSettings.initialWindowSize)
		req = &stream.request
		if !c._decodeFields(headersInFrame.effective(), req.joinHeaders) {
			putServer2Stream(stream)
			return http2ErrorCompression
		}
		if headersInFrame.endStream {
			stream.state = http2StateRemoteClosed
		} else {
			stream.state = http2StateOpen
		}
		c.joinStream(stream)
		c.concurrentStreams++
		go stream.execute()
	} else { // old stream
		s := c.findStream(streamID)
		if s == nil { // no specified active stream
			return http2ErrorProtocol
		}
		stream = s.(*server2Stream)
		if stream.state != http2StateOpen {
			return http2ErrorProtocol
		}
		if !headersInFrame.endStream { // must be trailers
			return http2ErrorProtocol
		}
		req = &stream.request
		req.receiving = httpSectionTrailers
		if !c._decodeFields(headersInFrame.effective(), req.joinTrailers) {
			return http2ErrorCompression
		}
	}
	return nil
}
func (c *server2Conn) onPriorityInFrame(priorityInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *server2Conn) onRSTStreamInFrame(rstStreamInFrame *http2InFrame) error {
	streamID := rstStreamInFrame.streamID
	if streamID > c.lastStreamID {
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
	for i, j, n := uint32(0), uint32(0), settingsInFrame.length/6; i < n; i++ {
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
func (c *server2Conn) onPingInFrame(pingInFrame *http2InFrame) error {
	pongOutFrame := &c.outFrame
	pongOutFrame.length = 8
	pongOutFrame.streamID = 0
	pongOutFrame.kind = http2FramePing
	pongOutFrame.ack = true
	pongOutFrame.payload = pingInFrame.effective() // TODO: copy()?
	err := c.sendOutFrame(pongOutFrame)
	pongOutFrame.zero()
	return err
}
func (c *server2Conn) onWindowUpdateInFrame(windowUpdateInFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(windowUpdateInFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		return http2ErrorProtocol
	}
	// TODO
	c.inWindow = int32(windowSize)
	Printf("conn=%d stream=%d windowUpdate=%d\n", c.id, windowUpdateInFrame.streamID, windowSize)
	return nil
}

func (c *server2Conn) goawayCloseConn(h2e http2Error) {
	goawayOutFrame := &c.outFrame
	goawayOutFrame.length = 8
	goawayOutFrame.streamID = 0
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
	c.gate.DecSub() // conn
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
	var serverStream *server2Stream
	if x := poolServer2Stream.Get(); x == nil {
		serverStream = new(server2Stream)
		req, resp := &serverStream.request, &serverStream.response
		req.stream = serverStream
		req.inMessage = req
		resp.stream = serverStream
		resp.outMessage = resp
		resp.request = req
	} else {
		serverStream = x.(*server2Stream)
	}
	serverStream.onUse(id, conn, outWindow)
	return serverStream
}
func putServer2Stream(serverStream *server2Stream) {
	serverStream.onEnd()
	poolServer2Stream.Put(serverStream)
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

func (s *server2Stream) Holder() httpHolder { return s.conn.gate }

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
	// Embeds
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
		if !r.in2._growHeaders2(int32(len(p))) {
			return false
		}
		r.inputEdge += int32(copy(r.input[r.inputEdge:], p))
	}
	return true
}
func (r *server2Request) readContent() (data []byte, err error) { return r.in2.readContent2() }
func (r *server2Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// server2Response is the server-side HTTP/2 response.
type server2Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Embeds
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

func (r *server2Response) control() []byte { // :status NNN
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

func (r *server2Response) addHeader(name []byte, value []byte) bool {
	return r.out2.addHeader2(name, value)
}
func (r *server2Response) header(name []byte) (value []byte, ok bool) { return r.out2.header2(name) }
func (r *server2Response) hasHeader(name []byte) bool                 { return r.out2.hasHeader2(name) }
func (r *server2Response) delHeader(name []byte) (deleted bool)       { return r.out2.delHeader2(name) }
func (r *server2Response) delHeaderAt(i uint8)                        { r.out2.delHeaderAt2(i) }

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

func (r *server2Response) sendChain() error { return r.out2.sendChain2() }

func (r *server2Response) echoHeaders() error { return r.out2.writeHeaders2() }
func (r *server2Response) echoChain() error   { return r.out2.echoChain2() }

func (r *server2Response) addTrailer(name []byte, value []byte) bool {
	return r.out2.addTrailer2(name, value)
}
func (r *server2Response) trailer(name []byte) (value []byte, ok bool) { return r.out2.trailer2(name) }

func (r *server2Response) proxyPass1xx(backResp backendResponse) bool {
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
	r.onUse()
	return false
}
func (r *server2Response) proxyPassHeaders() error          { return r.out2.writeHeaders2() }
func (r *server2Response) proxyPassBytes(data []byte) error { return r.out2.proxyPassBytes2(data) }

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
	// Embeds
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
