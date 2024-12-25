// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 server and backend implementation. See RFC 9113, RFC 7541, and RFC 9218.
// NOTE: httpxServer and httpxGate are used by both HTTP/2 and HTTP/1.x.

// Server Push is not supported because it's rarely used. Chrome and Firefox even removed it.

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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/library/system"
)

func init() {
	RegisterServer("httpxServer", func(name string, stage *Stage) Server {
		s := new(httpxServer)
		s.onCreate(name, stage)
		return s
	})
	RegisterBackend("http2Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP2Backend)
		b.onCreate(name, stage)
		return b
	})
}

//////////////////////////////////////// HTTP/2 server implementation ////////////////////////////////////////

// httpxServer is the HTTP/1.x and HTTP/2 server. An httpxServer has many httpxGates.
type httpxServer struct {
	// Parent
	webServer_[*httpxGate]
	// States
	httpMode int8 // 0: adaptive, 1: http/1.x, 2: http/2
}

func (s *httpxServer) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)

	s.httpMode = 1 // http/1.x by default. change to adaptive mode after http/2 server has been fully implemented
}

func (s *httpxServer) OnConfigure() {
	s.webServer_.onConfigure()

	if DebugLevel() >= 2 { // remove this condition after http/2 server has been fully implemented
		// httpMode
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
	s.webServer_.onPrepare()

	if s.IsTLS() {
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
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub() // gate
		if s.IsUDS() {
			go gate.serveUDS()
		} else if s.IsTLS() {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("httpxServer=%s done\n", s.Name())
	}
	s.stage.DecSub() // server
}

// httpxGate is a gate of httpxServer.
type httpxGate struct {
	// Parent
	webGate_
	// Assocs
	server *httpxServer
	// States
	listener net.Listener // the real gate. set after open
}

func (g *httpxGate) init(id int32, server *httpxServer) {
	g.webGate_.init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *httpxGate) Server() Server  { return g.server }
func (g *httpxGate) Address() string { return g.server.Address() }
func (g *httpxGate) IsUDS() bool     { return g.server.IsUDS() }
func (g *httpxGate) IsTLS() bool     { return g.server.IsTLS() }

func (g *httpxGate) Open() error {
	var (
		listener net.Listener
		err      error
	)
	if g.IsUDS() {
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
	return g.listener.Close() // breaks serve()
}

func (g *httpxGate) serveUDS() { // runner
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
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.justClose(unixConn)
			continue
		}
		rawConn, err := unixConn.SyscallConn()
		if err != nil {
			g.justClose(unixConn)
			//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
			continue
		}
		if g.server.httpMode == 2 {
			serverConn := getServer2Conn(connID, g, unixConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
		} else {
			serverConn := getServer1Conn(connID, g, unixConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
		}
		connID++
	}
	g.WaitConns() // TODO: max timeout?
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
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.justClose(tcpConn)
			continue
		}
		tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
		if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
			g.justClose(tlsConn)
			continue
		}
		if connState := tlsConn.ConnectionState(); connState.NegotiatedProtocol == "h2" {
			serverConn := getServer2Conn(connID, g, tlsConn, nil)
			go serverConn.serve() // serverConn is put to pool in serve()
		} else {
			serverConn := getServer1Conn(connID, g, tlsConn, nil)
			go serverConn.serve() // serverConn is put to pool in serve()
		}
		connID++
	}
	g.WaitConns() // TODO: max timeout?
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
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.justClose(tcpConn)
			continue
		}
		rawConn, err := tcpConn.SyscallConn()
		if err != nil {
			g.justClose(tcpConn)
			//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
			continue
		}
		if g.server.httpMode == 2 {
			serverConn := getServer2Conn(connID, g, tcpConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
		} else {
			serverConn := getServer1Conn(connID, g, tcpConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
		}
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub() // gate
}

func (g *httpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecActives()
	g.DecConn()
}

// server2Conn is the server-side HTTP/2 connection.
type server2Conn struct {
	// Parent
	webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	outFrame http2OutFrame // used by c.serve() to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	gate           *httpxGate          // the gate to which the connection belongs
	netConn        net.Conn            // the connection (TCP/TLS/UDS)
	rawConn        syscall.RawConn     // for syscall. only usable when netConn is TCP
	buffer         *http2Buffer        // http2Buffer in use, for receiving incoming frames
	clientSettings http2Settings       // settings of remote client
	table          http2DynamicTable   // dynamic table
	incomingChan   chan any            // frames and errors generated by c.receive() and waiting for c.serve() to consume
	inWindow       int32               // connection-level window size for incoming DATA frames
	outWindow      int32               // connection-level window size for outgoing DATA frames
	outgoingChan   chan *http2OutFrame // frames generated by streams and waiting for c.serve() to send
	// Conn states (zeros)
	inFrame0      http2InFrame                          // incoming frame, server2Conn controlled
	inFrame1      http2InFrame                          // incoming frame, server2Conn controlled
	inFrame       *http2InFrame                         // current incoming frame, used by recvFrame(). refers to inFrame0 or inFrame1 in turn
	streams       [http2MaxActiveStreams]*server2Stream // active (open, remoteClosed, localClosed) streams
	vector        net.Buffers                           // used by writev in c.serve()
	fixedVector   [2][]byte                             // used by writev in c.serve()
	_server2Conn0                                       // all values in this struct must be zero by default!
}
type _server2Conn0 struct { // for fast reset, entirely
	bufferEdge   uint32                            // incoming data ends at c.buffer.buf[c.bufferEdge]
	pBack        uint32                            // incoming frame part (header or payload) begins from c.buffer.buf[c.pBack]
	pFore        uint32                            // incoming frame part (header or payload) ends at c.buffer.buf[c.pFore]
	cBack        uint32                            // incoming continuation part (header or payload) begins from c.buffer.buf[c.cBack]
	cFore        uint32                            // incoming continuation part (header or payload) ends at c.buffer.buf[c.cFore]
	lastStreamID uint32                            // last received client stream id
	streamIDs    [http2MaxActiveStreams + 1]uint32 // ids of c.streams. the extra 1 id is used for fast linear searching
	nInFrames    int64                             // num of incoming frames
	nStreams     uint8                             // num of active streams
	waitReceive  bool                              // ...
	acknowledged bool                              // server settings acknowledged by client?
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
	c.webConn_.onGet(id)

	c.gate = gate
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
func (c *server2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	// c.buffer is reserved
	// c.table is reserved
	// c.incoming is reserved
	// c.outgoing is reserved
	c.inFrame0.zero()
	c.inFrame1.zero()
	c.inFrame = nil
	c.streams = [http2MaxActiveStreams]*server2Stream{}
	c.vector = nil
	c.fixedVector = [2][]byte{}
	c._server2Conn0 = _server2Conn0{}
	c.gate = nil

	c.webConn_.onPut()
}

func (c *server2Conn) IsUDS() bool { return c.gate.IsUDS() }
func (c *server2Conn) IsTLS() bool { return c.gate.IsTLS() }

func (c *server2Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.gate.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *server2Conn) serve() { // runner
	Printf("========================== conn=%d start =========================\n", c.id)
	defer func() {
		Printf("========================== conn=%d exit =========================\n", c.id)
		putServer2Conn(c)
	}()
	if err := c._handshake(); err != nil {
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
				if err := server2FrameProcessors[inFrame.kind](c, inFrame); err == nil {
					// Successfully processed. Next one.
					continue
				} else if h2e, ok := err.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else { // processor i/o error
					c.goawayCloseConn(http2ErrorInternal)
				}
				// c.serve() was broken, but c.receive() was not. need wait
				c.waitReceive = true
			} else { // c.receive() was broken and quit.
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
func (c *server2Conn) _handshake() error {
	// Set deadline for the first request headers
	if err := c.setReadDeadline(); err != nil {
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

func (c *server2Conn) receive() { // runner
	if DebugLevel() >= 1 {
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

func (c *server2Conn) goawayCloseConn(h2e http2Error) {
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

var server2FrameProcessors = [...]func(*server2Conn, *http2InFrame) error{
	(*server2Conn).processDataFrame,
	(*server2Conn).processHeadersFrame,
	(*server2Conn).processPriorityFrame,
	(*server2Conn).processRSTStreamFrame,
	(*server2Conn).processSettingsFrame,
	nil, // pushPromise frames are rejected in c.recvFrame()
	(*server2Conn).processPingFrame,
	nil, // goaway frames are hijacked by c.receive()
	(*server2Conn).processWindowUpdateFrame,
	nil, // discrete continuation frames are rejected in c.recvFrame()
}

func (c *server2Conn) processDataFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *server2Conn) processHeadersFrame(inFrame *http2InFrame) error {
	var (
		stream *server2Stream
		req    *server2Request
	)
	streamID := inFrame.streamID
	if streamID > c.lastStreamID { // new stream
		if c.nStreams == http2MaxActiveStreams {
			return http2ErrorProtocol
		}
		c.lastStreamID = streamID
		c.usedStreams.Add(1)
		stream = getServer2Stream(c, streamID, c.clientSettings.initialWindowSize)
		req = &stream.request
		if !c._decodeFields(inFrame.effective(), req.joinHeaders) {
			putServer2Stream(stream)
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
		req.receiving = httpSectionTrailers
		if !c._decodeFields(inFrame.effective(), req.joinTrailers) {
			return http2ErrorCompression
		}
	}
	return nil
}
func (c *server2Conn) _decodeFields(fields []byte, join func(p []byte) bool) bool {
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
func (c *server2Conn) _decodeString(src []byte, req *server2Request) (int, bool) {
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
func (c *server2Conn) processPriorityFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *server2Conn) processRSTStreamFrame(inFrame *http2InFrame) error {
	streamID := inFrame.streamID
	if streamID > c.lastStreamID {
		return http2ErrorProtocol
	}
	// TODO
	return nil
}
func (c *server2Conn) processSettingsFrame(inFrame *http2InFrame) error {
	if inFrame.ack {
		c.acknowledged = true
		return nil
	}
	// TODO: client sent a new settings
	return nil
}
func (c *server2Conn) _updateClientSettings(inFrame *http2InFrame) error {
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
func (c *server2Conn) _adjustStreamWindows(delta int32) {
}
func (c *server2Conn) processPingFrame(inFrame *http2InFrame) error {
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
func (c *server2Conn) processWindowUpdateFrame(inFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(inFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		return http2ErrorProtocol
	}
	// TODO
	c.inWindow = int32(windowSize)
	Printf("conn=%d stream=%d windowUpdate=%d\n", c.id, inFrame.streamID, windowSize)
	return nil
}

func (c *server2Conn) findStream(streamID uint32) *server2Stream {
	c.streamIDs[http2MaxActiveStreams] = streamID // the stream id to search for
	index := uint8(0)
	for c.streamIDs[index] != streamID { // searching for stream id
		index++
	}
	if index != http2MaxActiveStreams { // found
		if DebugLevel() >= 2 {
			Printf("conn=%d findStream=%d at %d\n", c.id, streamID, index)
		}
		return c.streams[index]
	}
	return nil // not found
}
func (c *server2Conn) joinStream(stream *server2Stream) {
	c.streamIDs[http2MaxActiveStreams] = 0
	index := uint8(0)
	for c.streamIDs[index] != 0 { // searching a free slot
		index++
	}
	if index == http2MaxActiveStreams { // this should not happen
		BugExitln("joinStream cannot find an empty slot")
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d joinStream=%d at %d\n", c.id, stream.id, index)
	}
	stream.index = index
	c.streams[index] = stream
	c.streamIDs[index] = stream.id
}
func (c *server2Conn) quitStream(streamID uint32) {
	stream := c.findStream(streamID)
	if stream == nil {
		BugExitln("quitStream cannot find the stream")
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d quitStream=%d at %d\n", c.id, streamID, stream.index)
	}
	c.streams[stream.index] = nil
	c.streamIDs[stream.index] = 0
}

func (c *server2Conn) recvFrame() (*http2InFrame, error) {
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
	// Reject unexpected frames
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
		if !inFrame.endHeaders { // continuations follow, join them into headers
			if err := c._joinContinuations(inFrame); err != nil {
				return nil, err
			}
		}
		// Got a new headers. Set deadline for next headers
		if err := c.setReadDeadline(); err != nil {
			return nil, err
		}
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d <--- %+v\n", c.id, inFrame)
	}
	return inFrame, nil
}
func (c *server2Conn) _growFrame(size uint32) error {
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
func (c *server2Conn) _fillBuffer(size uint32) error {
	n, err := c.readAtLeast(c.buffer.buf[c.bufferEdge:], int(size))
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL READ=%d -----------------------\n", c.id, n)
	}
	if err != nil && DebugLevel() >= 2 {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	c.bufferEdge += uint32(n)
	return err
}
func (c *server2Conn) _joinContinuations(headers *http2InFrame) error { // into a single headers frame
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
		// TODO: limit the number of continuation frames to avoid DoS attack
		c.nInFrames++ // got the continuation frame.
		// Append continuation to headers
		copy(c.buffer.buf[headers.pEdge:], c.buffer.buf[c.cBack:c.cFore]) // may overwrite padding if exists
		headers.pEdge += continuation.length
		headers.length += continuation.length // we don't care if padding is overwritten. just accumulate
		c.pFore += continuation.length        // also accumulate headers payload, with padding included
		// End of headers?
		if continuation.endHeaders {
			headers.endHeaders = true
			headers.buffer = c.buffer // restore the buffer
			c.pFore = c.cFore         // for next frame.
			return nil
		}
		c.cBack = c.cFore
	}
}
func (c *server2Conn) _growContinuation(size uint32, headers *http2InFrame) error {
	c.cFore += size              // won't overflow
	if c.cFore <= c.bufferEdge { // buffer is sufficient
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

func (c *server2Conn) sendFrame(outFrame *http2OutFrame) error {
	header := outFrame.encodeHeader()
	if len(outFrame.payload) > 0 {
		c.vector = c.fixedVector[0:2]
		c.vector[1] = outFrame.payload
	} else {
		c.vector = c.fixedVector[0:1]
	}
	c.vector[0] = header
	n, err := c.writev(&c.vector)
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Printf("conn=%d ---> %+v\n", c.id, outFrame)
	}
	return err
}

func (c *server2Conn) setReadDeadline() error {
	deadline := time.Now().Add(c.gate.server.ReadTimeout())
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *server2Conn) setWriteDeadline() error {
	deadline := time.Now().Add(c.gate.server.WriteTimeout())
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *server2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}
func (c *server2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *server2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}

func (c *server2Conn) closeConn() {
	if DebugLevel() >= 2 {
		Printf("conn=%d connClosed by serve()\n", c.id)
	}
	c.netConn.Close()
	c.gate.DecActives()
	c.gate.DecConn()
}

// server2Stream is the server-side HTTP/2 stream.
type server2Stream struct {
	// Parent
	webStream_
	// Assocs
	request  server2Request  // the http/2 request.
	response server2Response // the http/2 response.
	socket   *server2Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn      *server2Conn
	id        uint32 // stream id
	inWindow  int32  // stream-level window size for incoming DATA frames
	outWindow int32  // stream-level window size for outgoing DATA frames
	// Stream states (zeros)
	_server2Stream0 // all values in this struct must be zero by default!
}
type _server2Stream0 struct { // for fast reset, entirely
	index uint8 // index in s.conn.streams
	state uint8 // http2StateOpen, http2StateRemoteClosed, ...
	reset bool  // received a RST_STREAM?
}

var poolServer2Stream sync.Pool

func getServer2Stream(conn *server2Conn, id uint32, outWindow int32) *server2Stream {
	var stream *server2Stream
	if x := poolServer2Stream.Get(); x == nil {
		stream = new(server2Stream)
		req, resp := &stream.request, &stream.response
		req.stream = stream
		req.inMessage = req
		resp.stream = stream
		resp.outMessage = resp
		resp.request = req
	} else {
		stream = x.(*server2Stream)
	}
	stream.onUse(conn, id, outWindow)
	return stream
}
func putServer2Stream(stream *server2Stream) {
	stream.onEnd()
	poolServer2Stream.Put(stream)
}

func (s *server2Stream) onUse(conn *server2Conn, id uint32, outWindow int32) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.id = id
	s.inWindow = _64K1 // max size of r.bodyWindow
	s.outWindow = outWindow
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *server2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s._server2Stream0 = _server2Stream0{}
	s.webStream_.onEnd()
}

func (s *server2Stream) execute() { // runner
	defer putServer2Stream(s)
	// TODO ...
	if DebugLevel() >= 2 {
		Println("stream processing...")
	}
}

func (s *server2Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}

func (s *server2Stream) executeExchan(webapp *Webapp, req *server2Request, resp *server2Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server2Stream) serveAbnormal(req *server2Request, resp *server2Response) { // 4xx & 5xx
	// TODO
}

func (s *server2Stream) executeSocket() { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
}

func (s *server2Stream) Holder() webHolder    { return s.conn.gate.server }
func (s *server2Stream) Conn() webConn        { return s.conn }
func (s *server2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *server2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream
func (s *server2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream

func (s *server2Stream) setReadDeadline() error { // for content i/o only
	// TODO
	return nil
}
func (s *server2Stream) setWriteDeadline() error { // for content i/o only
	// TODO
	return nil
}

func (s *server2Stream) read(p []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *server2Stream) readFull(p []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *server2Stream) write(p []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *server2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only
	// TODO
	return 0, nil
}

func (s *server2Stream) buffer256() []byte          { return s.stockBuffer[:] }
func (s *server2Stream) unsafeMake(size int) []byte { return s.region.Make(size) }

// server2Request is the server-side HTTP/2 request.
type server2Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server2Request) joinHeaders(p []byte) bool {
	if len(p) > 0 {
		if !r._growHeaders2(int32(len(p))) {
			return false
		}
		r.inputEdge += int32(copy(r.input[r.inputEdge:], p))
	}
	return true
}
func (r *server2Request) readContent() (p []byte, err error) { return r.readContent2() }
func (r *server2Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// server2Response is the server-side HTTP/2 response.
type server2Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
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

func (r *server2Response) addHeader(name []byte, value []byte) bool   { return r.addHeader2(name, value) }
func (r *server2Response) header(name []byte) (value []byte, ok bool) { return r.header2(name) }
func (r *server2Response) hasHeader(name []byte) bool                 { return r.hasHeader2(name) }
func (r *server2Response) delHeader(name []byte) (deleted bool)       { return r.delHeader2(name) }
func (r *server2Response) delHeaderAt(i uint8)                        { r.delHeaderAt2(i) }

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

func (r *server2Response) sendChain() error { return r.sendChain2() }

func (r *server2Response) echoHeaders() error { return r.writeHeaders2() }
func (r *server2Response) echoChain() error   { return r.echoChain2() }

func (r *server2Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}
func (r *server2Response) trailer(name []byte) (value []byte, ok bool) { return r.trailer2(name) }

func (r *server2Response) proxyPass1xx(backResp response) bool {
	backResp.proxyDelHopHeaders()
	r.status = backResp.Status()
	if !backResp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.nameHash, name, value)
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse(Version2)
	return false
}
func (r *server2Response) passHeaders() error       { return r.writeHeaders2() }
func (r *server2Response) passBytes(p []byte) error { return r.passBytes2(p) }

func (r *server2Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			clock := r.stream.(*server2Stream).conn.gate.server.stage.clock
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
}
func (s *server2Socket) onEnd() {
	s.serverSocket_.onEnd()
}

//////////////////////////////////////// HTTP/2 backend implementation ////////////////////////////////////////

// HTTP2Backend
type HTTP2Backend struct {
	// Parent
	webBackend_[*http2Node]
	// States
}

func (b *HTTP2Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.OnCreate(name, stage)
}

func (b *HTTP2Backend) OnConfigure() {
	b.webBackend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *HTTP2Backend) OnPrepare() {
	b.webBackend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *HTTP2Backend) CreateNode(name string) Node {
	node := new(http2Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP2Backend) FetchStream() (stream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP2Backend) StoreStream(stream stream) {
	stream2 := stream.(*backend2Stream)
	stream2.conn.node.storeStream(stream2)
}

// http2Node
type http2Node struct {
	// Parent
	webNode_
	// Assocs
	backend *HTTP2Backend
	// States
}

func (n *http2Node) onCreate(name string, backend *HTTP2Backend) {
	n.webNode_.OnCreate(name)
	n.backend = backend
}

func (n *http2Node) OnConfigure() {
	n.webNode_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"h2"}
	}
}
func (n *http2Node) OnPrepare() {
	n.webNode_.OnPrepare()
}

func (n *http2Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("http2Node=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *http2Node) fetchStream() (*backend2Stream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *http2Node) storeStream(stream *backend2Stream) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
}

// backend2Conn
type backend2Conn struct {
	// Parent
	webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *http2Node      // the node to which the connection belongs
	netConn net.Conn        // the connection (TCP/TLS/UDS)
	rawConn syscall.RawConn // for syscall. only usable when netConn is TCP
	expire  time.Time       // when the conn is considered expired
	// Conn states (zeros)
	nStreams       atomic.Int32                           // concurrent streams
	streams        [http2MaxActiveStreams]*backend2Stream // active (open, remoteClosed, localClosed) streams
	_backend2Conn0                                        // all values in this struct must be zero by default!
}
type _backend2Conn0 struct { // for fast reset, entirely
}

var poolBackend2Conn sync.Pool

func getBackend2Conn(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) *backend2Conn {
	var backendConn *backend2Conn
	if x := poolBackend2Conn.Get(); x == nil {
		backendConn = new(backend2Conn)
	} else {
		backendConn = x.(*backend2Conn)
	}
	backendConn.onGet(id, node, netConn, rawConn)
	return backendConn
}
func putBackend2Conn(backendConn *backend2Conn) {
	backendConn.onPut()
	poolBackend2Conn.Put(backendConn)
}

func (c *backend2Conn) onGet(id int64, node *http2Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.webConn_.onGet(id)

	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
	c.expire = time.Now().Add(node.backend.aliveTimeout)
}
func (c *backend2Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.nStreams.Store(0)
	c.streams = [http2MaxActiveStreams]*backend2Stream{}
	c._backend2Conn0 = _backend2Conn0{}
	c.node = nil
	c.expire = time.Time{}

	c.webConn_.onPut()
}

func (c *backend2Conn) IsUDS() bool { return c.node.IsUDS() }
func (c *backend2Conn) IsTLS() bool { return c.node.IsTLS() }

func (c *backend2Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.node.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *backend2Conn) runOut() bool {
	return c.usedStreams.Add(1) > c.node.backend.MaxStreamsPerConn()
}

func (c *backend2Conn) fetchStream() (*backend2Stream, error) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO: incRef, stream.onUse()
	return nil, nil
}
func (c *backend2Conn) storeStream(stream *backend2Stream) {
	// Note: A backend2Conn can be used concurrently, limited by maxStreams.
	// TODO
	//stream.onEnd()
}

func (c *backend2Conn) setWriteDeadline() error {
	deadline := time.Now().Add(c.node.backend.WriteTimeout())
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *backend2Conn) setReadDeadline() error {
	deadline := time.Now().Add(c.node.backend.ReadTimeout())
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *backend2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *backend2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}
func (c *backend2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}

func (c *backend2Conn) Close() error {
	netConn := c.netConn
	putBackend2Conn(c)
	return netConn.Close()
}

// backend2Stream
type backend2Stream struct {
	// Parent
	webStream_
	// Assocs
	request  backend2Request
	response backend2Response
	socket   *backend2Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *backend2Conn
	id   uint32
	// Stream states (zeros)
	_backend2Stream0 // all values in this struct must be zero by default!
}
type _backend2Stream0 struct { // for fast reset, entirely
}

var poolBackend2Stream sync.Pool

func getBackend2Stream(conn *backend2Conn, id uint32) *backend2Stream {
	var stream *backend2Stream
	if x := poolBackend2Stream.Get(); x == nil {
		stream = new(backend2Stream)
		req, resp := &stream.request, &stream.response
		req.stream = stream
		req.outMessage = req
		req.response = resp
		resp.stream = stream
		resp.inMessage = resp
	} else {
		stream = x.(*backend2Stream)
	}
	stream.onUse(conn, id)
	return stream
}
func putBackend2Stream(stream *backend2Stream) {
	stream.onEnd()
	poolBackend2Stream.Put(stream)
}

func (s *backend2Stream) onUse(conn *backend2Conn, id uint32) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.id = id
	s.request.onUse(Version2)
	s.response.onUse(Version2)
}
func (s *backend2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s.conn = nil
	s._backend2Stream0 = _backend2Stream0{}
	s.webStream_.onEnd()
}

func (s *backend2Stream) Request() request   { return &s.request }
func (s *backend2Stream) Response() response { return &s.response }

func (s *backend2Stream) Socket() socket { return nil } // TODO. See RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441

func (s *backend2Stream) Holder() webHolder    { return s.conn.node.backend }
func (s *backend2Stream) Conn() webConn        { return s.conn }
func (s *backend2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *backend2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream
func (s *backend2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream

func (s *backend2Stream) setWriteDeadline() error { // for content i/o only?
	// TODO
	return nil
}
func (s *backend2Stream) setReadDeadline() error { // for content i/o only?
	// TODO
	return nil
}

func (s *backend2Stream) write(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *backend2Stream) writev(vector *net.Buffers) (int64, error) { // for content i/o only?
	return 0, nil
}
func (s *backend2Stream) read(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}
func (s *backend2Stream) readFull(p []byte) (int, error) { // for content i/o only?
	return 0, nil
}

func (s *backend2Stream) buffer256() []byte          { return s.stockBuffer[:] }
func (s *backend2Stream) unsafeMake(size int) []byte { return s.region.Make(size) }

// backend2Request is the backend-side HTTP/2 request.
type backend2Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend2Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :path = uri
	// TODO: set :method and :path
	return false
}
func (r *backend2Request) proxySetAuthority(hostname []byte, colonPort []byte) bool {
	// TODO: set :authority
	return false
}

func (r *backend2Request) addHeader(name []byte, value []byte) bool   { return r.addHeader2(name, value) }
func (r *backend2Request) header(name []byte) (value []byte, ok bool) { return r.header2(name) }
func (r *backend2Request) hasHeader(name []byte) bool                 { return r.hasHeader2(name) }
func (r *backend2Request) delHeader(name []byte) (deleted bool)       { return r.delHeader2(name) }
func (r *backend2Request) delHeaderAt(i uint8)                        { r.delHeaderAt2(i) }

func (r *backend2Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *backend2Request) proxyCopyCookies(foreReq Request) bool { // DO NOT merge into one "cookie" header!
	// TODO: one by one?
	return true
}

func (r *backend2Request) sendChain() error { return r.sendChain2() }

func (r *backend2Request) echoHeaders() error { return r.writeHeaders2() }
func (r *backend2Request) echoChain() error   { return r.echoChain2() }

func (r *backend2Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}
func (r *backend2Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer2(name) }

func (r *backend2Request) passHeaders() error       { return r.writeHeaders2() }
func (r *backend2Request) passBytes(p []byte) error { return r.passBytes2(p) }

func (r *backend2Request) finalizeHeaders() { // add at most 256 bytes
	// TODO
}
func (r *backend2Request) finalizeVague() error {
	// TODO
	return nil
}

func (r *backend2Request) addedHeaders() []byte { return nil } // TODO
func (r *backend2Request) fixedHeaders() []byte { return nil } // TODO

// backend2Response is the backend-side HTTP/2 response.
type backend2Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend2Response) recvHead() {
	// TODO
}

func (r *backend2Response) readContent() (p []byte, err error) { return r.readContent2() }

// backend2Socket is the backend-side HTTP/2 webSocket.
type backend2Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolBackend2Socket sync.Pool

func getBackend2Socket(stream *backend2Stream) *backend2Socket {
	// TODO
	return nil
}
func putBackend2Socket(socket *backend2Socket) {
	// TODO
}

func (s *backend2Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *backend2Socket) onEnd() {
	s.backendSocket_.onEnd()
}

//////////////////////////////////////// HTTP/2 i/o implementation ////////////////////////////////////////

// HTTP/2 incoming

func (r *webIn_) _growHeaders2(size int32) bool {
	edge := r.inputEdge + size      // size is ensured to not overflow
	if edge < int32(cap(r.input)) { // fast path
		return true
	}
	if edge > _16K { // exceeds the max headers limit
		return false
	}
	input := GetNK(int64(edge)) // 4K/16K
	copy(input, r.input[0:r.inputEdge])
	if cap(r.input) != cap(r.stockInput) {
		PutNK(r.input)
	}
	r.input = input
	return true
}

func (r *webIn_) readContent2() (p []byte, err error) {
	// TODO
	return
}

// HTTP/2 outgoing

func (r *webOut_) addHeader2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) header2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) hasHeader2(name []byte) bool {
	// TODO
	return false
}
func (r *webOut_) delHeader2(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *webOut_) delHeaderAt2(i uint8) {
	// TODO
}

func (r *webOut_) sendChain2() error {
	// TODO
	return nil
}

func (r *webOut_) echoChain2() error {
	// TODO
	return nil
}

func (r *webOut_) addTrailer2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) trailer2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) trailers2() []byte {
	// TODO
	return nil
}

func (r *webOut_) passBytes2(p []byte) error { return r.writeBytes2(p) }

func (r *webOut_) finalizeVague2() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *webOut_) writeHeaders2() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *webOut_) writePiece2(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *webOut_) writeVector2() error {
	return nil
}
func (r *webOut_) writeBytes2(p []byte) error {
	// TODO
	return nil
}

// HTTP/2 webSocket

func (s *webSocket_) todo2() {
}

//////////////////////////////////////// HTTP/2 protocol elements ////////////////////////////////////////

const ( // HTTP/2 sizes and limits for both of our HTTP/2 server and HTTP/2 backend
	http2MaxFrameSize     = _16K
	http2MaxTableSize     = _4K
	http2MaxActiveStreams = 127
)

// http2Buffer
type http2Buffer struct {
	buf [9 + http2MaxFrameSize]byte // header + payload
	ref atomic.Int32
}

var poolHTTP2Buffer sync.Pool

func getHTTP2Buffer() *http2Buffer {
	var buffer *http2Buffer
	if x := poolHTTP2Buffer.Get(); x == nil {
		buffer = new(http2Buffer)
	} else {
		buffer = x.(*http2Buffer)
	}
	return buffer
}
func putHTTP2Buffer(buffer *http2Buffer) { poolHTTP2Buffer.Put(buffer) }

func (b *http2Buffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http2Buffer) getRef() int32 { return b.ref.Load() }
func (b *http2Buffer) incRef()       { b.ref.Add(1) }
func (b *http2Buffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DebugLevel() >= 1 {
			Printf("putHTTP2Buffer ref=%d\n", b.ref.Load())
		}
		putHTTP2Buffer(b)
	}
}

const ( // HTTP/2 frame kinds
	http2FrameData         = 0x0
	http2FrameHeaders      = 0x1
	http2FramePriority     = 0x2 // deprecated
	http2FrameRSTStream    = 0x3
	http2FrameSettings     = 0x4
	http2FramePushPromise  = 0x5 // not supported
	http2FramePing         = 0x6
	http2FrameGoaway       = 0x7
	http2FrameWindowUpdate = 0x8
	http2FrameContinuation = 0x9
	http2FrameMax          = http2FrameContinuation
)
const ( // HTTP/2 error codes
	http2CodeNoError            = 0x0
	http2CodeProtocol           = 0x1
	http2CodeInternal           = 0x2
	http2CodeFlowControl        = 0x3
	http2CodeSettingsTimeout    = 0x4
	http2CodeStreamClosed       = 0x5
	http2CodeFrameSize          = 0x6
	http2CodeRefusedStream      = 0x7
	http2CodeCancel             = 0x8
	http2CodeCompression        = 0x9
	http2CodeConnect            = 0xa
	http2CodeEnhanceYourCalm    = 0xb
	http2CodeInadequateSecurity = 0xc
	http2CodeHTTP11Required     = 0xd
	http2CodeMax                = http2CodeHTTP11Required
)
const ( // HTTP/2 stream states
	http2StateClosed       = 0 // must be 0
	http2StateOpen         = 1
	http2StateRemoteClosed = 2
	http2StateLocalClosed  = 3
)
const ( // HTTP/2 settings
	http2SettingHeaderTableSize      = 0x1
	http2SettingEnablePush           = 0x2
	http2SettingMaxConcurrentStreams = 0x3
	http2SettingInitialWindowSize    = 0x4
	http2SettingMaxFrameSize         = 0x5
	http2SettingMaxHeaderListSize    = 0x6
)

// http2Settings
type http2Settings struct {
	headerTableSize      uint32 // 0x1
	enablePush           bool   // 0x2
	maxConcurrentStreams uint32 // 0x3
	initialWindowSize    int32  // 0x4
	maxFrameSize         uint32 // 0x5
	maxHeaderListSize    uint32 // 0x6
}

var http2InitialSettings = http2Settings{ // default settings of remote peer
	headerTableSize:      _4K,
	enablePush:           true,
	maxConcurrentStreams: 100,
	initialWindowSize:    _64K1, // this requires the size of content buffer must up to 64K1
	maxFrameSize:         _16K,
	maxHeaderListSize:    _16K,
}
var http2FrameNames = [...]string{
	http2FrameData:         "DATA",
	http2FrameHeaders:      "HEADERS",
	http2FramePriority:     "PRIORITY", // deprecated
	http2FrameRSTStream:    "RST_STREAM",
	http2FrameSettings:     "SETTINGS",
	http2FramePushPromise:  "PUSH_PROMISE", // not supported
	http2FramePing:         "PING",
	http2FrameGoaway:       "GOAWAY",
	http2FrameWindowUpdate: "WINDOW_UPDATE",
	http2FrameContinuation: "CONTINUATION",
}
var http2CodeTexts = [...]string{
	http2CodeNoError:            "NO_ERROR",
	http2CodeProtocol:           "PROTOCOL_ERROR",
	http2CodeInternal:           "INTERNAL_ERROR",
	http2CodeFlowControl:        "FLOW_CONTROL_ERROR",
	http2CodeSettingsTimeout:    "SETTINGS_TIMEOUT",
	http2CodeStreamClosed:       "STREAM_CLOSED",
	http2CodeFrameSize:          "FRAME_SIZE_ERROR",
	http2CodeRefusedStream:      "REFUSED_STREAM",
	http2CodeCancel:             "CANCEL",
	http2CodeCompression:        "COMPRESSION_ERROR",
	http2CodeConnect:            "CONNECT_ERROR",
	http2CodeEnhanceYourCalm:    "ENHANCE_YOUR_CALM",
	http2CodeInadequateSecurity: "INADEQUATE_SECURITY",
	http2CodeHTTP11Required:     "HTTP_1_1_REQUIRED",
}
var ( // HTTP/2 errors
	http2ErrorNoError            http2Error = http2CodeNoError
	http2ErrorProtocol           http2Error = http2CodeProtocol
	http2ErrorInternal           http2Error = http2CodeInternal
	http2ErrorFlowControl        http2Error = http2CodeFlowControl
	http2ErrorSettingsTimeout    http2Error = http2CodeSettingsTimeout
	http2ErrorStreamClosed       http2Error = http2CodeStreamClosed
	http2ErrorFrameSize          http2Error = http2CodeFrameSize
	http2ErrorRefusedStream      http2Error = http2CodeRefusedStream
	http2ErrorCancel             http2Error = http2CodeCancel
	http2ErrorCompression        http2Error = http2CodeCompression
	http2ErrorConnect            http2Error = http2CodeConnect
	http2ErrorEnhanceYourCalm    http2Error = http2CodeEnhanceYourCalm
	http2ErrorInadequateSecurity http2Error = http2CodeInadequateSecurity
	http2ErrorHTTP11Required     http2Error = http2CodeHTTP11Required
)

// http2Error denotes both connection error and stream error.
type http2Error uint32

func (e http2Error) Error() string {
	if e > http2CodeMax {
		return "UNKNOWN_ERROR"
	}
	return http2CodeTexts[e]
}

// http2StaticTable is used by HPACK decoder.
var http2StaticTable = [62]pair{ // TODO
	/*
		0:  {0, placeStatic2, 0, 0, span{0, 0}},
		1:  {1059, placeStatic2, 10, 0, span{0, 0}},
		2:  {699, placeStatic2, 7, 10, span{17, 20}},
		3:  {699, placeStatic2, 7, 10, span{20, 24}},
		4:  {487, placeStatic2, 5, 24, span{29, 30}},
		5:  {487, placeStatic2, 5, 24, span{30, 41}},
		6:  {687, placeStatic2, 7, 41, span{48, 52}},
		7:  {687, placeStatic2, 7, 41, span{52, 57}},
		8:  {734, placeStatic2, 7, 57, span{64, 67}},
		9:  {734, placeStatic2, 7, 57, span{67, 70}},
		10: {734, placeStatic2, 7, 57, span{70, 73}},
		11: {734, placeStatic2, 7, 57, span{73, 76}},
		12: {734, placeStatic2, 7, 57, span{76, 79}},
		13: {734, placeStatic2, 7, 57, span{79, 82}},
		14: {734, placeStatic2, 7, 57, span{82, 85}},
		15: {1415, placeStatic2, 14, 85, span{0, 0}},
		16: {1508, placeStatic2, 15, 99, span{114, 127}},
		17: {1505, placeStatic2, 15, 127, span{0, 0}},
		18: {1309, placeStatic2, 13, 142, span{0, 0}},
		19: {624, placeStatic2, 6, 155, span{0, 0}},
		20: {2721, placeStatic2, 27, 161, span{0, 0}},
		21: {301, placeStatic2, 3, 188, span{0, 0}},
		22: {543, placeStatic2, 5, 191, span{0, 0}},
		23: {1425, placeStatic2, 13, 196, span{0, 0}},
		24: {1314, placeStatic2, 13, 209, span{0, 0}},
		25: {2013, placeStatic2, 19, 222, span{0, 0}},
		26: {1647, placeStatic2, 16, 241, span{0, 0}},
		27: {1644, placeStatic2, 16, 257, span{0, 0}},
		28: {1450, placeStatic2, 14, 273, span{0, 0}},
		29: {1665, placeStatic2, 16, 287, span{0, 0}},
		30: {1333, placeStatic2, 13, 303, span{0, 0}},
		31: {1258, placeStatic2, 12, 316, span{0, 0}},
		32: {634, placeStatic2, 6, 328, span{0, 0}},
		33: {414, placeStatic2, 4, 334, span{0, 0}},
		34: {417, placeStatic2, 4, 338, span{0, 0}},
		35: {649, placeStatic2, 6, 342, span{0, 0}},
		36: {768, placeStatic2, 7, 348, span{0, 0}},
		37: {436, placeStatic2, 4, 355, span{0, 0}},
		38: {446, placeStatic2, 4, 359, span{0, 0}},
		39: {777, placeStatic2, 8, 363, span{0, 0}},
		40: {1660, placeStatic2, 17, 371, span{0, 0}},
		41: {1254, placeStatic2, 13, 388, span{0, 0}},
		42: {777, placeStatic2, 8, 401, span{0, 0}},
		43: {1887, placeStatic2, 19, 409, span{0, 0}},
		44: {1314, placeStatic2, 13, 428, span{0, 0}},
		45: {430, placeStatic2, 4, 441, span{0, 0}},
		46: {857, placeStatic2, 8, 445, span{0, 0}},
		47: {1243, placeStatic2, 12, 453, span{0, 0}},
		48: {1902, placeStatic2, 18, 465, span{0, 0}},
		49: {2048, placeStatic2, 19, 483, span{0, 0}},
		50: {525, placeStatic2, 5, 502, span{0, 0}},
		51: {747, placeStatic2, 7, 507, span{0, 0}},
		52: {751, placeStatic2, 7, 514, span{0, 0}},
		53: {1141, placeStatic2, 11, 521, span{0, 0}},
		54: {663, placeStatic2, 6, 532, span{0, 0}},
		55: {1011, placeStatic2, 10, 538, span{0, 0}},
		56: {2648, placeStatic2, 25, 548, span{0, 0}},
		57: {1753, placeStatic2, 17, 573, span{0, 0}},
		58: {1019, placeStatic2, 10, 590, span{0, 0}},
		59: {450, placeStatic2, 4, 600, span{0, 0}},
		60: {320, placeStatic2, 3, 604, span{0, 0}},
		61: {1681, placeStatic2, 16, 607, span{0, 0}},
	*/
}

func http2IsStaticIndex(index uint32) bool  { return index <= 61 }
func http2GetStaticPair(index uint32) *pair { return &http2StaticTable[index] }
func http2DynamicIndex(index uint32) uint32 { return index - 62 }

// http2TableEntry is a dynamic table entry.
type http2TableEntry struct { // 8 bytes
	nameFrom  uint16
	nameEdge  uint16 // nameEdge - nameFrom <= 255?
	valueEdge uint16
	totalSize uint16 // nameSize + valueSize + 32
}

// http2DynamicTable
type http2DynamicTable struct {
	maxSize  uint32 // <= http2MaxTableSize
	freeSize uint32 // <= maxSize
	eEntries uint32 // len(entries)
	nEntries uint32 // num of current entries. max num = floor(http2MaxTableSize/(1+32)) = 124
	oldest   uint32 // evict from oldest
	newest   uint32 // append to newest
	entries  [124]http2TableEntry
	content  [http2MaxTableSize - 32]byte
}

func (t *http2DynamicTable) init() {
	t.maxSize = http2MaxTableSize
	t.freeSize = t.maxSize
	t.eEntries = uint32(cap(t.entries))
	t.nEntries = 0
	t.oldest = 0
	t.newest = 0
}

func (t *http2DynamicTable) get(index uint32) (name []byte, value []byte, ok bool) {
	if index >= t.nEntries {
		return nil, nil, false
	}
	if t.newest > t.oldest || index <= t.newest {
		index = t.newest - index
	} else {
		index -= t.newest
		index = t.eEntries - index
	}
	entry := t.entries[index]
	return t.content[entry.nameFrom:entry.nameEdge], t.content[entry.nameEdge:entry.valueEdge], true
}
func (t *http2DynamicTable) add(name []byte, value []byte) bool { // name is not empty. sizes of name and value are limited
	if t.nEntries == t.eEntries { // too many entries
		return false
	}
	nameSize, valueSize := uint32(len(name)), uint32(len(value))
	wantSize := nameSize + valueSize + 32 // won't overflow
	if wantSize > t.maxSize {
		t.freeSize = t.maxSize
		t.nEntries = 0
		t.oldest = t.newest
		return true
	}
	for t.freeSize < wantSize {
		t._evictOne()
	}
	t.freeSize -= wantSize
	var entry http2TableEntry
	if t.nEntries > 0 {
		entry.nameFrom = t.entries[t.newest].valueEdge
		if t.newest++; t.newest == t.eEntries {
			t.newest = 0
		}
	} else { // empty table. starts from 0
		entry.nameFrom = 0
	}
	entry.nameEdge = entry.nameFrom + uint16(nameSize)
	entry.valueEdge = entry.nameEdge + uint16(valueSize)
	entry.totalSize = uint16(wantSize)
	copy(t.content[entry.nameFrom:entry.nameEdge], name)
	if valueSize > 0 {
		copy(t.content[entry.nameEdge:entry.valueEdge], value)
	}
	t.nEntries++
	t.entries[t.newest] = entry
	return true
}
func (t *http2DynamicTable) resize(maxSize uint32) { // maxSize must <= http2MaxTableSize
	if maxSize > http2MaxTableSize {
		BugExitln("maxSize out of range")
	}
	if maxSize >= t.maxSize {
		t.freeSize += maxSize - t.maxSize
	} else {
		for usedSize := t.maxSize - t.freeSize; usedSize > maxSize; usedSize = t.maxSize - t.freeSize {
			t._evictOne()
		}
		t.freeSize -= t.maxSize - maxSize
	}
	t.maxSize = maxSize
}
func (t *http2DynamicTable) _evictOne() {
	if t.nEntries == 0 {
		BugExitln("no entries to evict!")
	}
	t.freeSize += uint32(t.entries[t.oldest].totalSize)
	if t.oldest++; t.oldest == t.eEntries {
		t.oldest = 0
	}
	if t.nEntries--; t.nEntries == 0 {
		t.newest = t.oldest
	}
}

func http2DecodeInteger(src []byte, N byte, max uint32) (uint32, int, bool) {
	l := len(src)
	if l == 0 {
		return 0, 0, false
	}
	K := uint32(1<<N - 1)
	I := uint32(src[0])
	if N < 8 {
		I &= K
	}
	if I < K {
		return I, 1, I <= max
	}
	j := 1
	M := 0
	for j < l {
		B := src[j]
		j++
		I += uint32(B&0x7F) << M // 0,7,14,21,28
		if I > max {
			break
		}
		if B&0x80 != 0x80 {
			return I, j, true
		}
		M += 7 // 7,14,21,28
	}
	return I, j, false
}
func http2DecodeString(src []byte) ([]byte, int, bool) {
	I, j, ok := http2DecodeInteger(src, 7, _16K)
	if !ok {
		return nil, 0, false
	}
	H := src[0]&0x80 == 0x80
	src = src[j:]
	if I > uint32(len(src)) {
		return nil, j, false
	}
	src = src[0:I]
	j += int(I)
	if H {
		return []byte("huffman"), j, true
	} else {
		return src, j, true
	}
}

func http2EncodeInteger(I uint32, N byte, dst []byte) (int, bool) {
	l := len(dst)
	if l == 0 {
		return 0, false
	}
	K := uint32(1<<N - 1)
	if I < K {
		dst[0] = byte(I)
		return 1, true
	}
	dst[0] = byte(K)
	j := 1
	for I -= K; I >= 0x80; I >>= 7 {
		if j == l {
			return j, false
		}
		dst[j] = byte(I | 0x80)
		j++
	}
	if j < l {
		dst[j] = byte(I)
		return j + 1, true
	}
	return j, false
}
func http2EncodeString(S string, literal bool, dst []byte) (int, bool) {
	// TODO
	return 0, false
}

// http2InFrame is the server-side HTTP/2 incoming frame.
type http2InFrame struct { // 32 bytes
	length     uint32       // length of payload. the real type is uint24
	streamID   uint32       // the real type is uint31
	kind       uint8        // see http2FrameXXX
	endHeaders bool         // is END_HEADERS flag set?
	endStream  bool         // is END_STREAM flag set?
	ack        bool         // is ACK flag set?
	padded     bool         // is PADDED flag set?
	priority   bool         // is PRIORITY flag set?
	buffer     *http2Buffer // the buffer holding payload
	pFrom      uint32       // (effective) payload from
	pEdge      uint32       // (effective) payload edge
}

func (f *http2InFrame) zero() { *f = http2InFrame{} }

func (f *http2InFrame) decodeHeader(header []byte) error {
	f.length = uint32(header[0])<<16 | uint32(header[1])<<8 | uint32(header[2])
	if f.length > http2MaxFrameSize {
		return http2ErrorFrameSize
	}
	f.streamID = uint32(header[5]&0x7f)<<24 | uint32(header[6])<<16 | uint32(header[7])<<8 | uint32(header[8])
	if f.streamID > 0 && f.streamID&0x1 == 0 { // only for server side
		return http2ErrorProtocol
	}
	f.kind = header[3]
	flags := header[4]
	f.endHeaders = flags&0x4 > 0 && (f.kind == http2FrameHeaders || f.kind == http2FrameContinuation)
	f.endStream = flags&0x1 > 0 && (f.kind == http2FrameData || f.kind == http2FrameHeaders)
	f.ack = flags&0x1 > 0 && (f.kind == http2FrameSettings || f.kind == http2FramePing)
	f.padded = flags&0x8 > 0 && (f.kind == http2FrameData || f.kind == http2FrameHeaders)
	f.priority = flags&0x20 > 0 && f.kind == http2FrameHeaders
	return nil
}
func (f *http2InFrame) isUnknown() bool   { return f.kind > http2FrameMax }
func (f *http2InFrame) effective() []byte { return f.buffer.buf[f.pFrom:f.pEdge] } // effective payload

func (f *http2InFrame) check() error {
	if f.isUnknown() {
		return nil
	}
	return http2InFrameCheckers[f.kind](f)
}

var http2InFrameCheckers = [...]func(*http2InFrame) error{
	(*http2InFrame).checkAsData,
	(*http2InFrame).checkAsHeaders,
	(*http2InFrame).checkAsPriority,
	(*http2InFrame).checkAsRSTStream,
	(*http2InFrame).checkAsSettings,
	nil, // pushPromise frames are rejected before check() in recvFrame()
	(*http2InFrame).checkAsPing,
	(*http2InFrame).checkAsGoaway,
	(*http2InFrame).checkAsWindowUpdate,
	nil, // continuation frames are rejected before check() in recvFrame()
}

func (f *http2InFrame) checkAsData() error {
	var minLength uint32 = 1 // Data (..)
	if f.padded {
		minLength += 1 // Pad Length (8)
	}
	if f.length < minLength {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
		return http2ErrorProtocol
	}
	var padLength, othersLen uint32 = 0, 0
	if f.padded {
		padLength = uint32(f.buffer.buf[f.pFrom])
		othersLen += 1
		f.pFrom += 1
	}
	if padLength > 0 { // drop padding
		if othersLen+padLength >= f.length {
			return http2ErrorProtocol
		}
		f.pEdge -= padLength
	}
	return nil
}
func (f *http2InFrame) checkAsHeaders() error {
	var minLength uint32 = 1 // Field Block Fragment
	if f.padded {
		minLength += 1 // Pad Length (8)
	}
	if f.priority {
		minLength += 5 // Exclusive (1) + Stream Dependency (31) + Weight (8)
	}
	if f.length < minLength {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
		return http2ErrorProtocol
	}
	var padLength, othersLen uint32 = 0, 0
	if f.padded { // skip pad length byte
		padLength = uint32(f.buffer.buf[f.pFrom])
		othersLen += 1
		f.pFrom += 1
	}
	if f.priority { // skip stream dependency and weight
		othersLen += 5
		f.pFrom += 5
	}
	if padLength > 0 { // drop padding
		if othersLen+padLength >= f.length {
			return http2ErrorProtocol
		}
		f.pEdge -= padLength
	}
	return nil
}
func (f *http2InFrame) checkAsPriority() error {
	if f.length != 5 {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
		return http2ErrorProtocol
	}
	return nil
}
func (f *http2InFrame) checkAsRSTStream() error {
	if f.length != 4 {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
		return http2ErrorProtocol
	}
	return nil
}
func (f *http2InFrame) checkAsSettings() error {
	if f.length%6 != 0 || f.length > 48 { // we allow 8 defined settings.
		return http2ErrorFrameSize
	}
	if f.streamID != 0 {
		return http2ErrorProtocol
	}
	if f.ack && f.length != 0 {
		return http2ErrorFrameSize
	}
	return nil
}
func (f *http2InFrame) checkAsPing() error {
	if f.length != 8 {
		return http2ErrorFrameSize
	}
	if f.streamID != 0 {
		return http2ErrorProtocol
	}
	return nil
}
func (f *http2InFrame) checkAsGoaway() error {
	if f.length < 8 {
		return http2ErrorFrameSize
	}
	if f.streamID != 0 {
		return http2ErrorProtocol
	}
	return nil
}
func (f *http2InFrame) checkAsWindowUpdate() error {
	if f.length != 4 {
		return http2ErrorFrameSize
	}
	return nil
}

// http2OutFrame is the server-side HTTP/2 outgoing frame.
type http2OutFrame struct { // 64 bytes
	length     uint32   // length of payload. the real type is uint24
	streamID   uint32   // the real type is uint31
	kind       uint8    // see http2FrameXXX. WARNING: http2FramePushPromise and http2FrameContinuation are NOT allowed!
	endHeaders bool     // is END_HEADERS flag set?
	endStream  bool     // is END_STREAM flag set?
	ack        bool     // is ACK flag set?
	padded     bool     // is PADDED flag set?
	priority   bool     // is PRIORITY flag set?
	header     [9]byte  // header of the frame is encoded here
	buffer     [16]byte // small payload of the frame is placed here temporarily
	payload    []byte   // refers to the payload
}

func (f *http2OutFrame) zero() { *f = http2OutFrame{} }

func (f *http2OutFrame) encodeHeader() (header []byte) { // caller must ensure the frame is legal.
	if f.kind == http2FramePushPromise || f.kind == http2FrameContinuation {
		BugExitln("push promise and continuation are not allowed as out frame")
	}
	header = f.header[:]
	header[0], header[1], header[2] = byte(f.length>>16), byte(f.length>>8), byte(f.length)
	header[5], header[6], header[7], header[8] = byte(f.streamID>>24), byte(f.streamID>>16), byte(f.streamID>>8), byte(f.streamID)
	header[3] = f.kind
	flags := uint8(0)
	if f.endHeaders && f.kind == http2FrameHeaders {
		flags |= 0x4
	}
	if f.endStream && (f.kind == http2FrameData || f.kind == http2FrameHeaders) {
		flags |= 0x1
	}
	if f.ack && (f.kind == http2FrameSettings || f.kind == http2FramePing) {
		flags |= 0x1
	}
	if f.padded && (f.kind == http2FrameData || f.kind == http2FrameHeaders) {
		flags |= 0x8
	}
	if f.priority && f.kind == http2FrameHeaders {
		flags |= 0x20
	}
	header[4] = flags
	return
}

var http2Template = [11]byte{':', 's', 't', 'a', 't', 'u', 's', ' ', 'x', 'x', 'x'}
var http2Controls = [...][]byte{ // size: 512*24B=12K. keep sync with http1Control and http3Control!
	// 1XX
	StatusContinue:           []byte(":status 100"),
	StatusSwitchingProtocols: []byte(":status 101"),
	StatusProcessing:         []byte(":status 102"),
	StatusEarlyHints:         []byte(":status 103"),
	// 2XX
	StatusOK:                         []byte(":status 200"),
	StatusCreated:                    []byte(":status 201"),
	StatusAccepted:                   []byte(":status 202"),
	StatusNonAuthoritativeInfomation: []byte(":status 203"),
	StatusNoContent:                  []byte(":status 204"),
	StatusResetContent:               []byte(":status 205"),
	StatusPartialContent:             []byte(":status 206"),
	StatusMultiStatus:                []byte(":status 207"),
	StatusAlreadyReported:            []byte(":status 208"),
	StatusIMUsed:                     []byte(":status 226"),
	// 3XX
	StatusMultipleChoices:   []byte(":status 300"),
	StatusMovedPermanently:  []byte(":status 301"),
	StatusFound:             []byte(":status 302"),
	StatusSeeOther:          []byte(":status 303"),
	StatusNotModified:       []byte(":status 304"),
	StatusUseProxy:          []byte(":status 305"),
	StatusTemporaryRedirect: []byte(":status 307"),
	StatusPermanentRedirect: []byte(":status 308"),
	// 4XX
	StatusBadRequest:                  []byte(":status 400"),
	StatusUnauthorized:                []byte(":status 401"),
	StatusPaymentRequired:             []byte(":status 402"),
	StatusForbidden:                   []byte(":status 403"),
	StatusNotFound:                    []byte(":status 404"),
	StatusMethodNotAllowed:            []byte(":status 405"),
	StatusNotAcceptable:               []byte(":status 406"),
	StatusProxyAuthenticationRequired: []byte(":status 407"),
	StatusRequestTimeout:              []byte(":status 408"),
	StatusConflict:                    []byte(":status 409"),
	StatusGone:                        []byte(":status 410"),
	StatusLengthRequired:              []byte(":status 411"),
	StatusPreconditionFailed:          []byte(":status 412"),
	StatusContentTooLarge:             []byte(":status 413"),
	StatusURITooLong:                  []byte(":status 414"),
	StatusUnsupportedMediaType:        []byte(":status 415"),
	StatusRangeNotSatisfiable:         []byte(":status 416"),
	StatusExpectationFailed:           []byte(":status 417"),
	StatusMisdirectedRequest:          []byte(":status 421"),
	StatusUnprocessableEntity:         []byte(":status 422"),
	StatusLocked:                      []byte(":status 423"),
	StatusFailedDependency:            []byte(":status 424"),
	StatusTooEarly:                    []byte(":status 425"),
	StatusUpgradeRequired:             []byte(":status 426"),
	StatusPreconditionRequired:        []byte(":status 428"),
	StatusTooManyRequests:             []byte(":status 429"),
	StatusRequestHeaderFieldsTooLarge: []byte(":status 431"),
	StatusUnavailableForLegalReasons:  []byte(":status 451"),
	// 5XX
	StatusInternalServerError:           []byte(":status 500"),
	StatusNotImplemented:                []byte(":status 501"),
	StatusBadGateway:                    []byte(":status 502"),
	StatusServiceUnavailable:            []byte(":status 503"),
	StatusGatewayTimeout:                []byte(":status 504"),
	StatusHTTPVersionNotSupported:       []byte(":status 505"),
	StatusVariantAlsoNegotiates:         []byte(":status 506"),
	StatusInsufficientStorage:           []byte(":status 507"),
	StatusLoopDetected:                  []byte(":status 508"),
	StatusNotExtended:                   []byte(":status 510"),
	StatusNetworkAuthenticationRequired: []byte(":status 511"),
}

var ( // HTTP/2 byteses
	http2BytesPrism  = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	http2BytesStatic = []byte(":authority:methodGETPOST:path//index.html:schemehttphttps:status200204206304400404500accept-charsetaccept-encodinggzip, deflateaccept-languageaccept-rangesacceptaccess-control-allow-originageallowauthorizationcache-controlcontent-dispositioncontent-encodingcontent-languagecontent-lengthcontent-locationcontent-rangecontent-typecookiedateetagexpectexpiresfromhostif-matchif-modified-sinceif-none-matchif-rangeif-unmodified-sincelast-modifiedlinklocationmax-forwardsproxy-authenticateproxy-authorizationrangerefererrefreshretry-afterserverset-cookiestrict-transport-securitytransfer-encodinguser-agentvaryviawww-authenticate") // DO NOT CHANGE THIS UNLESS YOU KNOW WHAT YOU ARE DOING
)
