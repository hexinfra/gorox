// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 server implementation. See RFC 9113 and 7541.

// Server Push is not supported because it's rarely used.

package hemi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
)

func init() {
	RegisterServer("http2Server", func(name string, stage *Stage) Server {
		s := new(http2Server)
		s.onCreate(name, stage)
		return s
	})
}

// http2Server is the HTTP/2 server.
type http2Server struct {
	// Parent
	webServer_[*http2Gate]
	// States
	tlsEnableHTTP1 bool // enable switching to HTTP/1.1 for TLS?
}

func (s *http2Server) onCreate(name string, stage *Stage) {
	s.webServer_.onCreate(name, stage)
}

func (s *http2Server) OnConfigure() {
	s.webServer_.onConfigure()

	// tlsEnableHTTP1
	s.ConfigureBool("tlsEnableHTTP1", &s.tlsEnableHTTP1, true)
}
func (s *http2Server) OnPrepare() {
	s.webServer_.onPrepare()

	if s.IsTLS() {
		var nextProtos []string
		if s.tlsEnableHTTP1 {
			nextProtos = []string{"h2", "http/1.1"}
		} else {
			nextProtos = []string{"h2"}
		}
		s.tlsConfig.NextProtos = nextProtos
	}
}

func (s *http2Server) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(http2Gate)
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub()
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
		Printf("http2Server=%s done\n", s.Name())
	}
	s.stage.DecSub()
}

// http2Gate is a gate of http2Server.
type http2Gate struct {
	// Parent
	Gate_
	// Assocs
	// States
	listener net.Listener // the real gate. set after open
}

func (g *http2Gate) init(id int32, server *http2Server) {
	g.Gate_.Init(id, server)
}

func (g *http2Gate) Open() error {
	if g.IsUDS() {
		return g._openUnix()
	} else {
		return g._openInet()
	}
}
func (g *http2Gate) _openUnix() error {
	address := g.Address()
	// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
	// This affects graceful upgrading, maybe we can implement fd transfer in the future.
	os.Remove(address)
	listener, err := net.Listen("unix", address)
	if err == nil {
		g.listener = listener.(*net.UnixListener)
		if DebugLevel() >= 1 {
			Printf("http2Gate id=%d address=%s opened!\n", g.id, g.Address())
		}
	}
	return err
}
func (g *http2Gate) _openInet() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		if err := system.SetReusePort(rawConn); err != nil {
			return err
		}
		return system.SetDeferAccept(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.listener = listener.(*net.TCPListener)
		if DebugLevel() >= 1 {
			Printf("http2Gate id=%d address=%s opened!\n", g.id, g.Address())
		}
	}
	return err
}
func (g *http2Gate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *http2Gate) serveUDS() { // runner
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
	for {
		unixConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("http2Server[%s] http2Gate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				//g.stage.Logf("http2Server[%s] http2Gate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			serverConn := getServer2Conn(connID, g, unixConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http2Gate=%d TCP done\n", g.id)
	}
	g.server.DecSub()
}
func (g *http2Gate) serveTLS() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("http2Server[%s] http2Gate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
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
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http2Gate=%d TLS done\n", g.id)
	}
	g.server.DecSub()
}
func (g *http2Gate) serveTCP() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("http2Server[%s] http2Gate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				//g.stage.Logf("http2Server[%s] http2Gate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			serverConn := getServer2Conn(connID, g, tcpConn, rawConn)
			go serverConn.serve() // serverConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http2Gate=%d TCP done\n", g.id)
	}
	g.server.DecSub()
}

func (g *http2Gate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
}

// poolServer2Conn is the server-side HTTP/2 connection pool.
var poolServer2Conn sync.Pool

func getServer2Conn(id int64, gate Gate, netConn net.Conn, rawConn syscall.RawConn) *server2Conn {
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

// server2Conn is the server-side HTTP/2 connection.
type server2Conn struct {
	// Parent
	ServerConn_
	// Mixins
	_webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	outFrame http2OutFrame // used by c.serve() to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	netConn        net.Conn            // the connection (TCP/TLS)
	rawConn        syscall.RawConn     // for syscall. only usable when netConn is TCP
	buffer         *http2Buffer        // http2Buffer in use, for receiving incoming frames
	clientSettings http2Settings       // settings of remote client
	table          http2DynamicTable   // dynamic table
	incomingChan   chan any            // frames and errors generated by c.receive() and waiting for c.serve() to consume
	inWindow       int32               // connection-level window size for incoming DATA frames
	outWindow      int32               // connection-level window size for outgoing DATA frames
	outgoingChan   chan *http2OutFrame // frames generated by streams and waiting for c.serve() to send
	// Conn states (zeros)
	inFrame0     http2InFrame                          // incoming frame, server2Conn controlled
	inFrame1     http2InFrame                          // incoming frame, server2Conn controlled
	inFrame      *http2InFrame                         // current incoming frame, used by recvFrame(). refers to c.inFrame0 or c.inFrame1 in turn
	streams      [http2MaxActiveStreams]*server2Stream // active (open, remoteClosed, localClosed) streams
	vector       net.Buffers                           // used by writev in c.serve()
	fixedVector  [2][]byte                             // used by writev in c.serve()
	server2Conn0                                       // all values must be zero by default in this struct!
}
type server2Conn0 struct { // for fast reset, entirely
	bufferEdge   uint32                            // incoming data ends at c.buffer.buf[c.bufferEdge]
	pBack        uint32                            // incoming frame part (header or payload) begins from c.buffer.buf[c.pBack]
	pFore        uint32                            // incoming frame part (header or payload) ends at c.buffer.buf[c.pFore]
	cBack        uint32                            // incoming continuation part (header or payload) begins from c.buffer.buf[c.cBack]
	cFore        uint32                            // incoming continuation part (header or payload) ends at c.buffer.buf[c.cFore]
	lastStreamID uint32                            // last served stream id
	streamIDs    [http2MaxActiveStreams + 1]uint32 // ids of c.streams. the extra 1 id is used for fast linear searching
	nInFrames    int64                             // num of incoming frames
	nStreams     uint8                             // num of active streams
	waitReceive  bool                              // ...
	acknowledged bool                              // server settings acknowledged by client?
	//unackedSettings?
	//queuedControlFrames?
}

func (c *server2Conn) onGet(id int64, gate Gate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c._webConn_.onGet()
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
	c.server2Conn0 = server2Conn0{}
	c._webConn_.onPut()
	c.ServerConn_.OnPut()
}

func (c *server2Conn) WebServer() WebServer { return c.Server().(WebServer) }

func (c *server2Conn) serve() { // runner
	Printf("========================== conn=%d start =========================\n", c.id)
	defer func() {
		Printf("========================== conn=%d exit =========================\n", c.id)
		putServer2Conn(c)
	}()
	if err := c.handshake(); err != nil {
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
				// c.serve() is broken, but c.receive() is not. need wait
				c.waitReceive = true
			} else { // c.receive() is broken and quit.
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

func (c *server2Conn) handshake() error {
	// Set deadline for the first request headers
	if err := c.setReadDeadline(time.Now().Add(c.Server().ReadTimeout())); err != nil {
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
		req.receiving = webSectionTrailers
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
func (c *server2Conn) processDataFrame(inFrame *http2InFrame) error {
	return nil
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
func (c *server2Conn) processRSTStreamFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *server2Conn) processPriorityFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
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

func (c *server2Conn) findStream(streamID uint32) *server2Stream {
	c.streamIDs[http2MaxActiveStreams] = streamID
	index := uint8(0)
	for c.streamIDs[index] != streamID { // searching stream id
		index++
	}
	if index == http2MaxActiveStreams { // not found.
		return nil
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d findStream=%d at %d\n", c.id, streamID, index)
	}
	return c.streams[index]
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
		if !inFrame.endHeaders { // continuations follow
			if err := c._joinContinuations(inFrame); err != nil {
				return nil, err
			}
		}
		// Got a new headers. Set deadline for next headers
		if err := c.setReadDeadline(time.Now().Add(c.Server().ReadTimeout())); err != nil {
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
		c.nInFrames++
		// Append to headers
		copy(c.buffer.buf[headers.pEdge:], c.buffer.buf[c.cBack:c.cFore]) // overwrite padding if exists
		headers.pEdge += continuation.length
		headers.length += continuation.length // we don't care that padding is overwrite. just accumulate
		c.pFore += continuation.length        // also accumulate headers payload, with padding included
		// End of headers?
		if continuation.endHeaders {
			headers.endHeaders = true
			headers.buffer = c.buffer
			c.pFore = c.cFore // for next frame.
			break
		} else {
			c.cBack = c.cFore
		}
	}
	return nil
}
func (c *server2Conn) _growContinuation(size uint32, headers *http2InFrame) error {
	c.cFore += size // won't overflow
	if c.cFore <= c.bufferEdge {
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

func (c *server2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *server2Conn) setWriteDeadline(deadline time.Time) error {
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
	c.gate.OnConnClosed()
}

// poolServer2Stream is the server-side HTTP/2 stream pool.
var poolServer2Stream sync.Pool

func getServer2Stream(conn *server2Conn, id uint32, outWindow int32) *server2Stream {
	var stream *server2Stream
	if x := poolServer2Stream.Get(); x == nil {
		stream = new(server2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
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

// server2Stream is the server-side HTTP/2 stream.
type server2Stream struct {
	// Mixins
	_webStream_
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
	server2Stream0 // all values must be zero by default in this struct!
}
type server2Stream0 struct { // for fast reset, entirely
	index uint8 // index in s.conn.streams
	state uint8 // http2StateOpen, http2StateRemoteClosed, ...
	reset bool  // received a RST_STREAM?
}

func (s *server2Stream) onUse(conn *server2Conn, id uint32, outWindow int32) { // for non-zeros
	s._webStream_.onUse()
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
	s.server2Stream0 = server2Stream0{}
	s._webStream_.onEnd()
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

func (s *server2Stream) setReadDeadline(deadline time.Time) error { // for content i/o only
	return nil
}
func (s *server2Stream) setWriteDeadline(deadline time.Time) error { // for content i/o only
	return nil
}

func (s *server2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream
func (s *server2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream

func (s *server2Stream) webKeeper() webKeeper { return s.conn.WebServer() }
func (s *server2Stream) webConn() webConn     { return s.conn }
func (s *server2Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

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

func (r *server2Response) control() []byte { // :status xxx
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

func (r *server2Response) proxyPass1xx(resp backendResponse) bool {
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
			r.fieldsEdge += uint16(r.stream.webKeeper().Stage().Clock().writeDate1(r.fields[r.fieldsEdge:]))
		}
	*/
}
func (r *server2Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *server2Response) addedHeaders() []byte { return nil } // TODO
func (r *server2Response) fixedHeaders() []byte { return nil } // TODO

// poolServer2Socket
var poolServer2Socket sync.Pool

func getServer2Socket(stream *server2Stream) *server2Socket {
	return nil
}
func putServer2Socket(socket *server2Socket) {
}

// server2Socket is the server-side HTTP/2 websocket.
type server2Socket struct {
	// Parent
	serverSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server2Socket) onUse() {
	s.serverSocket_.onUse()
}
func (s *server2Socket) onEnd() {
	s.serverSocket_.onEnd()
}
