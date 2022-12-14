// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 server implementation.

// For simplicity, HTTP/2 Server Push is not supported.

package internal

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

// poolHTTP2Conn is the server-side HTTP/2 connection pool.
var poolHTTP2Conn sync.Pool

func getHTTP2Conn(id int64, server *httpxServer, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) httpConn {
	var conn *http2Conn
	if x := poolHTTP2Conn.Get(); x == nil {
		conn = new(http2Conn)
	} else {
		conn = x.(*http2Conn)
	}
	conn.onGet(id, server, gate, netConn, rawConn)
	return conn
}
func putHTTP2Conn(conn *http2Conn) {
	conn.onPut()
	poolHTTP2Conn.Put(conn)
}

// http2Conn is the server-side HTTP/2 connection.
type http2Conn struct {
	// Mixins
	httpConn_
	// Conn states (buffers)
	// Conn states (controlled)
	outFrame http2OutFrame // used by c.serve() to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	netConn        net.Conn            // the connection (TCP/TLS)
	rawConn        syscall.RawConn     // for syscall. only usable when netConn is TCP
	inputs         *http2Inputs        // http2Inputs in use, for receiving incoming frames
	clientSettings http2Settings       // settings of remote client
	table          http2DynamicTable   // dynamic table
	incoming       chan any            // frames and errors generated by c.receive() and waiting for c.serve() to consume
	inWindow       int32               // connection-level window size for incoming DATA frames
	outWindow      int32               // connection-level window size for outgoing DATA frames
	outgoing       chan *http2OutFrame // frames generated by streams and waiting for c.serve() to send
	// Conn states (zeros)
	inFrame0    http2InFrame                        // incoming frame, http2Conn controlled
	inFrame1    http2InFrame                        // incoming frame, http2Conn controlled
	inFrame     *http2InFrame                       // current incoming frame, used by recvFrame(). refers to c.inFrame0 or c.inFrame1 in turn
	streams     [http2MaxActiveStreams]*http2Stream // active (open, remoteClosed, localClosed) streams
	vector      net.Buffers                         // used by writev in c.serve()
	fixedVector [2][]byte                           // used by writev in c.serve()
	http2Conn0                                      // all values must be zero by default in this struct!
}
type http2Conn0 struct { // for fast reset, entirely
	inputsEdge   uint32                            // incoming data ends at c.inputs.buf[c.inputsEdge]
	pBack        uint32                            // incoming frame part (header or payload) begins from c.inputs.buf[c.pBack]
	pFore        uint32                            // incoming frame part (header or payload) ends at c.inputs.buf[c.pFore]
	cBack        uint32                            // incoming continuation part (header or payload) begins from c.inputs.buf[c.cBack]
	cFore        uint32                            // incoming continuation part (header or payload) ends at c.inputs.buf[c.cFore]
	lastStreamID uint32                            // last served stream id
	streamIDs    [http2MaxActiveStreams + 1]uint32 // ids of c.streams. the extra 1 id is used for fast linear searching
	nInFrames    int64                             // num of incoming frames
	nStreams     uint8                             // num of active streams
	waitReceive  bool                              // ...
	acknowledged bool                              // server settings acknowledged by client?
	//unackedSettings?
	//queuedControlFrames?
}

func (c *http2Conn) onGet(id int64, server *httpxServer, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.httpConn_.onGet(id, server, gate)
	c.netConn = netConn
	c.rawConn = rawConn
	if c.inputs == nil {
		c.allocInputs()
	}
	c.clientSettings = http2InitialSettings
	c.table.init()
	if c.incoming == nil {
		c.incoming = make(chan any)
	}
	c.inWindow = _2G1 - _64K1                        // as a receiver, we disable connection-level flow control
	c.outWindow = c.clientSettings.initialWindowSize // after we have received the client preface, this value will be changed to the real value of client.
	if c.outgoing == nil {
		c.outgoing = make(chan *http2OutFrame)
	}
}
func (c *http2Conn) onPut() {
	c.httpConn_.onPut()
	c.netConn = nil
	c.rawConn = nil
	// c.inputs is reserved
	// c.table is reserved
	// c.incoming is reserved
	// c.outgoing is reserved
	c.inFrame0.zero()
	c.inFrame1.zero()
	c.inFrame = nil
	c.streams = [http2MaxActiveStreams]*http2Stream{}
	c.vector = nil
	c.fixedVector = [2][]byte{}
	c.http2Conn0 = http2Conn0{}
}

func (c *http2Conn) allocInputs() {
	c.inputs = getHTTP2Inputs()
	c.inputs.incRef()
}

func (c *http2Conn) receive() { // goroutine
	if IsDebug(1) {
		defer Debugf("conn=%d c.receive() quit\n", c.id)
	}
	for { // each incoming frame
		inFrame, err := c.recvFrame()
		if err != nil {
			c.incoming <- err
			return
		}
		if inFrame.kind == http2FrameGoaway {
			c.incoming <- http2ErrorNoError
			return
		}
		c.incoming <- inFrame
	}
}
func (c *http2Conn) serve() { // goroutine
	Debugf("========================== conn=%d start =========================\n", c.id)
	defer func() {
		Debugf("========================== conn=%d exit =========================\n", c.id)
		putHTTP2Conn(c)
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
		case incoming := <-c.incoming: // from c.receive()
			if inFrame, ok := incoming.(*http2InFrame); ok { // data, headers, priority, rst_stream, settings, ping, windows_update, unknown
				if inFrame.isUnknown() {
					// Ignore unknown frames.
					continue
				}
				if err := http2FrameProcessors[inFrame.kind](c, inFrame); err == nil {
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
		case outFrame := <-c.outgoing: // from streams. only headers and data
			// TODO: collect as many frames as we can?
			Debugf("%+v\n", outFrame)
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
	Debugf("conn=%d waiting for active streams to end\n", c.id)
	for c.nStreams > 0 {
		if outFrame := <-c.outgoing; outFrame.endStream {
			c.quitStream(outFrame.streamID)
			c.nStreams--
		}
	}
	if c.waitReceive {
		Debugf("conn=%d waiting for c.receive() quits\n", c.id)
		for {
			incoming := <-c.incoming
			if _, ok := incoming.(*http2InFrame); !ok {
				// An error from c.receive() means it's quit
				break
			}
		}
	}
	Debugf("conn=%d c.serve() quit\n", c.id)
}
func (c *http2Conn) handshake() error {
	// Set deadline for the first request headers
	if err := c.setReadDeadline(time.Now().Add(c.server.ReadTimeout())); err != nil {
		return err
	}
	if err := c.growFrame(uint32(len(http2BytesPrism))); err != nil {
		return err
	}
	if !bytes.Equal(c.inputs.buf[0:len(http2BytesPrism)], http2BytesPrism) {
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
	n, err := c.write(http2ServerPrefaceAndMore)
	Debugf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
	Debugf("conn=%d ---> %v\n", c.id, http2ServerPrefaceAndMore)
	if err != nil {
		Debugf("conn=%d error=%s\n", c.id, err.Error())
	}
	return err
}

var http2ServerPrefaceAndMore = []byte{
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

func (c *http2Conn) goawayCloseConn(h2e http2Error) {
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

var http2FrameProcessors = [...]func(*http2Conn, *http2InFrame) error{
	(*http2Conn).processDataFrame,
	(*http2Conn).processHeadersFrame,
	(*http2Conn).processPriorityFrame,
	(*http2Conn).processRSTStreamFrame,
	(*http2Conn).processSettingsFrame,
	nil, // pushPromise frames are rejected in c.recvFrame()
	(*http2Conn).processPingFrame,
	nil, // goaway frames are hijacked by c.receive()
	(*http2Conn).processWindowUpdateFrame,
	nil, // discrete continuation frames are rejected in c.recvFrame()
}

func (c *http2Conn) processHeadersFrame(inFrame *http2InFrame) error {
	var (
		stream *http2Stream
		req    *http2Request
	)
	streamID := inFrame.streamID
	if streamID > c.lastStreamID { // new stream
		if c.nStreams == http2MaxActiveStreams {
			return http2ErrorProtocol
		}
		c.lastStreamID = streamID
		c.usedStreams.Add(1)
		stream = getHTTP2Stream(c, streamID, c.clientSettings.initialWindowSize)
		req = &stream.request
		if !c._decodeFields(inFrame.effective(), req.joinHeaders) {
			putHTTP2Stream(stream)
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
func (c *http2Conn) _decodeFields(fields []byte, join func(p []byte) bool) bool {
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
				Debugln("decode error")
				return false
			}
			i += j
			if I == 0 {
				Debugln("index == 0")
				return false
			}
			field := http2StaticTable[I]
			Debugf("name=%s value=%s\n", field.nameAt(http2BytesStatic), field.valueAt(http2BytesStatic))
		} else if b >= 1<<6 { // Literal Header Field with Incremental Indexing
			I, j, ok = http2DecodeInteger(fields[i:], 6, 128)
			if !ok {
				Debugln("decode error")
				return false
			}
			i += j
			if I != 0 { // Literal Header Field with Incremental Indexing ??? Indexed Name
				field := http2StaticTable[I]
				N = field.nameAt(http2BytesStatic)
			} else { // Literal Header Field with Incremental Indexing ??? New Name
				N, j, ok = http2DecodeString(fields[i:])
				if !ok {
					Debugln("decode error")
					return false
				}
				i += j
				if len(N) == 0 {
					Debugln("empty name")
					return false
				}
			}
			V, j, ok = http2DecodeString(fields[i:])
			if !ok {
				Debugln("decode error")
				return false
			}
			i += j
			Debugf("name=%s value=%s\n", N, V)
		} else if b >= 1<<5 { // Dynamic Table Size Update
			I, j, ok = http2DecodeInteger(fields[i:], 5, http2MaxTableSize)
			if !ok {
				Debugln("decode error")
				return false
			}
			i += j
			Debugf("update size=%d\n", I)
		} else if b >= 1<<4 { // Literal Header Field Never Indexed
			I, j, ok = http2DecodeInteger(fields[i:], 4, 128)
			if !ok {
				Debugln("decode error")
				return false
			}
			i += j
			if I != 0 { // Literal Header Field Never Indexed ??? Indexed Name
				field := http2StaticTable[I]
				N = field.nameAt(http2BytesStatic)
			} else { // Literal Header Field Never Indexed ??? New Name
				N, j, ok = http2DecodeString(fields[i:])
				if !ok {
					Debugln("decode error")
					return false
				}
				i += j
				if len(N) == 0 {
					Debugln("empty name")
					return false
				}
			}
			V, j, ok = http2DecodeString(fields[i:])
			if !ok {
				Debugln("decode error")
				return false
			}
			i += j
			Debugf("name=%s value=%s\n", N, V)
		} else { // Literal Header Field without Indexing
			Debugln("2222222222222")
			return false
		}
	}
	return true
}
func (c *http2Conn) _decodeString(src []byte, req *http2Request) (int, bool) {
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
func (c *http2Conn) processDataFrame(inFrame *http2InFrame) error {
	return nil
}
func (c *http2Conn) processWindowUpdateFrame(inFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(inFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		return http2ErrorProtocol
	}
	// TODO
	c.inWindow = int32(windowSize)
	Debugf("conn=%d stream=%d windowUpdate=%d\n", c.id, inFrame.streamID, windowSize)
	return nil
}
func (c *http2Conn) processSettingsFrame(inFrame *http2InFrame) error {
	if inFrame.ack {
		c.acknowledged = true
		return nil
	}
	// TODO: client sent a new settings
	return nil
}
func (c *http2Conn) _updateClientSettings(inFrame *http2InFrame) error {
	settings := inFrame.effective()
	windowDelta, j := int32(0), uint32(0)
	for i, n := uint32(0), inFrame.length/6; i < n; i++ {
		identifier := uint16(settings[j])<<8 | uint16(settings[j+1])
		value := uint32(settings[j+2])<<24 | uint32(settings[j+3])<<16 | uint32(settings[j+4])<<8 | uint32(settings[j+5])
		switch identifier {
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
	Debugf("conn=%d clientSettings=%+v\n", c.id, c.clientSettings)
	return nil
}
func (c *http2Conn) _adjustStreamWindows(delta int32) {
}
func (c *http2Conn) processRSTStreamFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *http2Conn) processPriorityFrame(inFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *http2Conn) processPingFrame(inFrame *http2InFrame) error {
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

func (c *http2Conn) findStream(streamID uint32) *http2Stream {
	c.streamIDs[http2MaxActiveStreams] = streamID
	index := uint8(0)
	for c.streamIDs[index] != streamID { // searching stream id
		index++
	}
	if index == http2MaxActiveStreams { // not found.
		return nil
	}
	if IsDebug(2) {
		Debugf("conn=%d findStream=%d at %d\n", c.id, streamID, index)
	}
	return c.streams[index]
}
func (c *http2Conn) joinStream(stream *http2Stream) {
	c.streamIDs[http2MaxActiveStreams] = 0
	index := uint8(0)
	for c.streamIDs[index] != 0 { // searching a free slot
		index++
	}
	if index == http2MaxActiveStreams { // this should not happen
		BugExitln("joinStream cannot find an empty slot")
	}
	if IsDebug(2) {
		Debugf("conn=%d joinStream=%d at %d\n", c.id, stream.id, index)
	}
	stream.index = index
	c.streams[index] = stream
	c.streamIDs[index] = stream.id
}
func (c *http2Conn) quitStream(streamID uint32) {
	stream := c.findStream(streamID)
	if stream == nil {
		BugExitln("quitStream cannot find the stream")
	}
	if IsDebug(2) {
		Debugf("conn=%d quitStream=%d at %d\n", c.id, streamID, stream.index)
	}
	c.streams[stream.index] = nil
	c.streamIDs[stream.index] = 0
}

func (c *http2Conn) recvFrame() (*http2InFrame, error) {
	// Receive frame header
	c.pBack = c.pFore
	if err := c.growFrame(9); err != nil {
		return nil, err
	}
	// Decode frame header
	if c.inFrame == nil || c.inFrame == &c.inFrame1 {
		c.inFrame = &c.inFrame0
	} else {
		c.inFrame = &c.inFrame1
	}
	inFrame := c.inFrame
	if err := inFrame.decodeHeader(c.inputs.buf[c.pBack:c.pFore]); err != nil {
		return nil, err
	}
	// Receive frame payload
	c.pBack = c.pFore
	if err := c.growFrame(inFrame.length); err != nil {
		return nil, err
	}
	// Mark frame payload
	inFrame.inputs = c.inputs
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
			if err := c.joinContinuations(inFrame); err != nil {
				return nil, err
			}
		}
		// Got a new headers. Set deadline for next headers
		if err := c.setReadDeadline(time.Now().Add(c.server.ReadTimeout())); err != nil {
			return nil, err
		}
	}
	if IsDebug(2) {
		Debugf("conn=%d <--- %+v\n", c.id, inFrame)
	}
	return inFrame, nil
}
func (c *http2Conn) growFrame(size uint32) error {
	c.pFore += size // size is limited, so won't overflow
	if c.pFore <= c.inputsEdge {
		return nil
	}
	// Needs grow.
	if c.pFore > c.inputs.size() { // needs slide
		if c.inputs.getRef() == 1 { // no streams are referring to c.inputs, just slide
			c.inputsEdge = uint32(copy(c.inputs.buf[:], c.inputs.buf[c.pBack:c.inputsEdge]))
		} else { // there are still streams referring to c.inputs. use a new inputs
			inputs := c.inputs
			c.allocInputs()
			c.inputsEdge = uint32(copy(c.inputs.buf[:], inputs.buf[c.pBack:c.inputsEdge]))
			inputs.decRef()
		}
		c.pFore -= c.pBack
		c.pBack = 0
	}
	return c.fillInputs(c.pFore - c.inputsEdge)
}
func (c *http2Conn) fillInputs(size uint32) error {
	n, err := c.readAtLeast(c.inputs.buf[c.inputsEdge:], int(size))
	if IsDebug(2) {
		Debugf("--------------------- conn=%d CALL READ=%d -----------------------\n", c.id, n)
	}
	if err != nil && IsDebug(2) {
		Debugf("conn=%d error=%s\n", c.id, err.Error())
	}
	c.inputsEdge += uint32(n)
	return err
}
func (c *http2Conn) joinContinuations(headers *http2InFrame) error { // into a single headers frame
	headers.inputs = nil // will be restored at the end of continuations
	var continuation http2InFrame
	c.cBack, c.cFore = c.pFore, c.pFore
	for { // each continuation frame
		// Receive continuation header
		if err := c.growContinuation(9, headers); err != nil {
			return err
		}
		// Decode continuation header
		if err := continuation.decodeHeader(c.inputs.buf[c.cBack:c.cFore]); err != nil {
			return err
		}
		// Check continuation header
		if continuation.length == 0 || headers.length+continuation.length > http2FrameMaxSize {
			return http2ErrorFrameSize
		}
		if continuation.streamID != headers.streamID || continuation.kind != http2FrameContinuation {
			return http2ErrorProtocol
		}
		// Receive continuation payload
		c.cBack = c.cFore
		if err := c.growContinuation(continuation.length, headers); err != nil {
			return err
		}
		c.nInFrames++
		// Append to headers
		copy(c.inputs.buf[headers.pEdge:], c.inputs.buf[c.cBack:c.cFore]) // overwrite padding if exists
		headers.pEdge += continuation.length
		headers.length += continuation.length // we don't care that padding is overwrite. just accumulate
		c.pFore += continuation.length        // also accumulate headers payload, with padding included
		// End of headers?
		if continuation.endHeaders {
			headers.endHeaders = true
			headers.inputs = c.inputs
			c.pFore = c.cFore // for next frame.
			break
		} else {
			c.cBack = c.cFore
		}
	}
	return nil
}
func (c *http2Conn) growContinuation(size uint32, headers *http2InFrame) error {
	c.cFore += size // won't overflow
	if c.cFore <= c.inputsEdge {
		return nil
	}
	// Needs grow. Cases are (A is payload of the headers frame):
	// c.inputs: [| .. ] | A | 9 | B | 9 | C | 9 | D |
	// c.inputs: [| .. ] | AB | oooo | 9 | C | 9 | D |
	// c.inputs: [| .. ] | ABC | ooooooooooo | 9 | D |
	// c.inputs: [| .. ] | ABCD | oooooooooooooooooo |
	if c.cFore > c.inputs.size() { // needs slide
		if c.pBack == 0 { // cannot slide again
			// This should only happens when looking for header, the 9 bytes
			return http2ErrorFrameSize
		}
		// Now slide. Skip holes (if any) when sliding
		inputs := c.inputs
		if c.inputs.getRef() != 1 { // there are still streams referring to c.inputs. use a new inputs
			c.allocInputs()
		}
		c.pFore = uint32(copy(c.inputs.buf[:], inputs.buf[c.pBack:c.pFore]))
		c.inputsEdge = c.pFore + uint32(copy(c.inputs.buf[c.pFore:], inputs.buf[c.cBack:c.inputsEdge]))
		if inputs != c.inputs {
			inputs.decRef()
		}
		headers.pFrom -= c.pBack
		headers.pEdge -= c.pBack
		c.pBack = 0
		c.cBack = c.pFore
		c.cFore = c.cBack + size
	}
	return c.fillInputs(c.cFore - c.inputsEdge)
}

func (c *http2Conn) sendFrame(outFrame *http2OutFrame) error {
	header := outFrame.encodeHeader()
	if len(outFrame.payload) > 0 {
		c.vector = c.fixedVector[0:2]
		c.vector[1] = outFrame.payload
	} else {
		c.vector = c.fixedVector[0:1]
	}
	c.vector[0] = header
	n, err := c.writev(&c.vector)
	if IsDebug(2) {
		Debugf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Debugf("conn=%d ---> %+v\n", c.id, outFrame)
	}
	return err
}

func (c *http2Conn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= c.server.ReadTimeout()/4 {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *http2Conn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= c.server.WriteTimeout()/4 {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *http2Conn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.netConn, p, n)
}
func (c *http2Conn) write(p []byte) (int, error) { return c.netConn.Write(p) }
func (c *http2Conn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.netConn)
}

func (c *http2Conn) closeConn() {
	if IsDebug(2) {
		Debugf("conn=%d connClosed by serve()\n", c.id)
	}
	c.netConn.Close()
	c.gate.onConnectionClosed()
}

// poolHTTP2Stream is the server-side HTTP/2 stream pool.
var poolHTTP2Stream sync.Pool

func getHTTP2Stream(conn *http2Conn, id uint32, outWindow int32) *http2Stream {
	var stream *http2Stream
	if x := poolHTTP2Stream.Get(); x == nil {
		stream = new(http2Stream)
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
		resp.request = req
	} else {
		stream = x.(*http2Stream)
	}
	stream.onUse(conn, id, outWindow)
	return stream
}
func putHTTP2Stream(stream *http2Stream) {
	stream.onEnd()
	poolHTTP2Stream.Put(stream)
}

// http2Stream is the server-side HTTP/2 stream.
type http2Stream struct {
	// Mixins
	httpStream_
	// Assocs
	request  http2Request  // the http/2 request.
	response http2Response // the http/2 response.
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn      *http2Conn // ...
	id        uint32     // stream id
	inWindow  int32      // stream-level window size for incoming DATA frames
	outWindow int32      // stream-level window size for outgoing DATA frames
	// Stream states (zeros)
	http2Stream0 // all values must be zero by default in this struct!
}
type http2Stream0 struct { // for fast reset, entirely
	index uint8 // index in s.conn.streams
	state uint8 // http2StateOpen, http2StateRemoteClosed, ...
	reset bool  // received a RST_STREAM?
}

func (s *http2Stream) onUse(conn *http2Conn, id uint32, outWindow int32) { // for non-zeros
	s.httpStream_.onUse()
	s.conn = conn
	s.id = id
	s.inWindow = _64K1 // max size of r.bodyWindow
	s.outWindow = outWindow
	s.request.versionCode = Version2 // explicitly set
	s.request.onUse()
	s.response.onUse()
}
func (s *http2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.httpStream_.onEnd()
	s.conn = nil
	s.http2Stream0 = http2Stream0{}
}

func (s *http2Stream) execute() { // goroutine
	// do
	if IsDebug(2) {
		Debugln("stream processing...")
	}
	putHTTP2Stream(s)
}

func (s *http2Stream) holder() holder {
	return s.conn.getServer()
}
func (s *http2Stream) peerAddr() net.Addr {
	return s.conn.netConn.RemoteAddr()
}

func (s *http2Stream) writeContinue() bool { // 100 continue
	// TODO
	return false
}
func (s *http2Stream) serveTCPTun() { // CONNECT method
	// TODO
}
func (s *http2Stream) serveUDPTun() { // see RFC 9298
	// TODO
}
func (s *http2Stream) serveSocket() { // see RFC 8441
	// TODO
}
func (s *http2Stream) serveNormal(app *App, req *http2Request, resp *http2Response) { // request & response
	// TODO
	app.dispatchHandlet(req, resp)
}
func (s *http2Stream) serveAbnormal(req *http2Request, resp *http2Response) { // 4xx & 5xx
	// TODO
}

func (s *http2Stream) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return s.conn.makeTempName(p, stamp)
}

func (s *http2Stream) setReadDeadline(deadline time.Time) error { // for content only
	return nil
}
func (s *http2Stream) setWriteDeadline(deadline time.Time) error { // for content only
	return nil
}

func (s *http2Stream) read(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *http2Stream) readFull(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *http2Stream) write(p []byte) (int, error) { // for content only
	return 0, nil
}
func (s *http2Stream) writev(vector *net.Buffers) (int64, error) { // for content only
	return 0, nil
}

func (s *http2Stream) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?
func (s *http2Stream) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?

// http2Request is the server-side HTTP/2 request.
type http2Request struct {
	// Mixins
	httpRequest_
	// Assocs
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http2Request) joinHeaders(p []byte) bool {
	if len(p) > 0 {
		if !r._growHeaders2(int32(len(p))) {
			return false
		}
		r.inputEdge += int32(copy(r.input[r.inputEdge:], p))
	}
	return true
}

func (r *http2Request) readContent() (p []byte, err error) {
	return r.readContent2()
}

func (r *http2Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// http2Response is the server-side HTTP/2 response.
type http2Response struct {
	// Mixins
	httpResponse_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *http2Response) control() []byte {
	return nil
}

func (r *http2Response) header(name []byte) (value []byte, ok bool) {
	return r.header2(name)
}
func (r *http2Response) hasHeader(name []byte) bool {
	return r.hasHeader2(name)
}
func (r *http2Response) addHeader(name []byte, value []byte) bool {
	return r.addHeader2(name, value)
}
func (r *http2Response) delHeader(name []byte) (deleted bool) {
	return r.delHeader2(name)
}
func (r *http2Response) delHeaderAt(o uint8) {
	r.delHeaderAt2(o)
}
func (r *http2Response) addedHeaders() []byte {
	return nil
}
func (r *http2Response) fixedHeaders() []byte {
	return nil
}

func (r *http2Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *http2Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *http2Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}
func (r *http2Response) setConnectionClose() {
	BugExitln("not used in HTTP/2")
}

func (r *http2Response) SetCookie(setCookie *SetCookie) bool {
	// TODO
	return false
}

func (r *http2Response) sendChain(chain Chain) error {
	// TODO
	return r.sendChain2(chain, nil)
}

func (r *http2Response) pushHeaders() error {
	// TODO
	return nil
}
func (r *http2Response) pushChain(chain Chain) error {
	return r.pushChain2(chain)
}

func (r *http2Response) trailer(name []byte) (value []byte, ok bool) {
	return r.trailer2(name)
}
func (r *http2Response) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer2(name, value)
}

func (r *http2Response) pass1xx(resp response) bool { // used by proxies
	// TODO
	r.onEnd()
	r.onUse()
	return false
}
func (r *http2Response) passHeaders() error {
	return nil
}
func (r *http2Response) passBytes(p []byte) error {
	return nil
}

func (r *http2Response) finalizeHeaders() {
	// TODO
}
func (r *http2Response) finalizeChunked() error {
	// TODO
	return nil
}

// http2Socket is the server-side HTTP/2 websocket.
type http2Socket struct {
	// Mixins
	httpSocket_
	// Stream states (zeros)
}

func (s *http2Socket) onUse() {
	s.httpSocket_.onUse()
}
func (s *http2Socket) onEnd() {
	s.httpSocket_.onEnd()
}
