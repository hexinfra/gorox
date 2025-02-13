// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 types. See RFC 9113 and RFC 7541.

package hemi

import (
	"encoding/binary"
	"io"
	"net"
	"syscall"
	"time"
)

// http2Conn
type http2Conn interface {
	// Imports
	httpConn
	// Methods
}

// http2Conn_ is a parent.
type http2Conn_ struct { // for server2Conn and backend2Conn
	// Parent
	httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	outFrame http2OutFrame // used by c.manager() to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	netConn      net.Conn            // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn      syscall.RawConn     // for syscall. only usable when netConn is TCP/UDS
	peerSettings http2Settings       // settings of the remote peer
	inBuffer     *http2Buffer        // http2Buffer in use, for receiving incoming frames
	dynamicTable http2DynamicTable   // hpack dynamic table
	incomingChan chan any            // frames and errors generated by c.receiver() and waiting for c.manager() to consume
	inWindow     int32               // connection-level window (buffer) size for incoming DATA frames
	outWindow    int32               // connection-level window (buffer) size for outgoing DATA frames
	outgoingChan chan *http2OutFrame // frames generated by streams and waiting for c.manager() to send
	// Conn states (zeros)
	inFrame0      http2InFrame                           // incoming frame 0
	inFrame1      http2InFrame                           // incoming frame 1
	inFrame       *http2InFrame                          // current incoming frame. refers to inFrame0 or inFrame1 in turn
	activeStreams [http2MaxConcurrentStreams]http2Stream // active (open, remoteClosed, localClosed) streams
	vector        net.Buffers                            // used by writev in c.manager()
	fixedVector   [2][]byte                              // used by writev in c.manager()
	lastPingTime  time.Time                              // last ping time
	_http2Conn0                                          // all values in this struct must be zero by default!
}
type _http2Conn0 struct { // for fast reset, entirely
	activeStreamIDs    [http2MaxConcurrentStreams + 1]uint32 // ids of c.activeStreams. the extra 1 id is used for fast linear searching
	lastStreamID       uint32                                // id of last stream sent to backend or received by server
	cumulativeInFrames int64                                 // num of incoming frames
	concurrentStreams  uint8                                 // num of active streams
	acknowledged       bool                                  // server settings acknowledged by client?
	pingSent           bool                                  // is there a ping frame sent and waiting for response?
	inBufferEdge       uint16                                // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	sectBack           uint16                                // incoming frame section (header or payload) begins from c.inBuffer.buf[c.sectBack]
	sectFore           uint16                                // incoming frame section (header or payload) ends at c.inBuffer.buf[c.sectFore]
	contBack           uint16                                // incoming continuation part (header or payload) begins from c.inBuffer.buf[c.contBack]
	contFore           uint16                                // incoming continuation part (header or payload) ends at c.inBuffer.buf[c.contFore]
}

func (c *http2Conn_) onGet(id int64, holder holder, netConn net.Conn, rawConn syscall.RawConn) {
	c.httpConn_.onGet(id, holder)

	c.netConn = netConn
	c.rawConn = rawConn
	c.peerSettings = http2InitialSettings
	if c.inBuffer == nil {
		c.inBuffer = getHTTP2Buffer()
		c.inBuffer.incRef()
	}
	c.dynamicTable.init()
	if c.incomingChan == nil {
		c.incomingChan = make(chan any)
	}
	c.inWindow = _2G1 - _64K1                      // as a receiver, we disable connection-level flow control
	c.outWindow = c.peerSettings.initialWindowSize // after we received the peer's preface, this value will be changed to the real value.
	if c.outgoingChan == nil {
		c.outgoingChan = make(chan *http2OutFrame)
	}
}
func (c *http2Conn_) onPut() {
	c.netConn = nil
	c.rawConn = nil
	// c.inBuffer is reserved
	// c.dynamicTable is reserved
	// c.incomingChan is reserved
	// c.outgoingChan is reserved
	c.inFrame0.zero()
	c.inFrame1.zero()
	c.inFrame = nil
	c.activeStreams = [http2MaxConcurrentStreams]http2Stream{}
	c.vector = nil
	c.fixedVector = [2][]byte{}
	c.lastPingTime = time.Time{}
	c._http2Conn0 = _http2Conn0{}

	c.httpConn_.onPut()
}

func (c *http2Conn_) receiver() { // runner
	if DebugLevel() >= 1 {
		defer Printf("conn=%d c.receiver() quit\n", c.id)
	}
	for { // each incoming frame
		inFrame, err := c.recvInFrame()
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

func (c *http2Conn_) recvInFrame() (*http2InFrame, error) {
	// Receive frame header
	c.sectBack = c.sectFore
	if err := c._growInFrame(9); err != nil {
		return nil, err
	}
	// Decode frame header
	if c.inFrame == nil || c.inFrame == &c.inFrame1 {
		c.inFrame = &c.inFrame0
	} else {
		c.inFrame = &c.inFrame1
	}
	inFrame := c.inFrame
	if err := inFrame.decodeHeader(c.inBuffer.buf[c.sectBack:c.sectFore]); err != nil {
		return nil, err
	}
	// Receive frame payload
	c.sectBack = c.sectFore
	if err := c._growInFrame(inFrame.length); err != nil {
		return nil, err
	}
	// Mark frame payload
	inFrame.inBuffer = c.inBuffer
	inFrame.efctFrom = c.sectBack
	inFrame.efctEdge = c.sectFore
	// Reject unexpected frames - pushPromise is NOT supported and continuation CANNOT be alone
	if inFrame.kind == http2FramePushPromise || inFrame.kind == http2FrameContinuation {
		return nil, http2ErrorProtocol
	}
	if !inFrame.isUnknown() {
		// Check the frame
		if err := http2InFrameCheckers[inFrame.kind](inFrame); err != nil {
			return nil, err
		}
	}
	c.cumulativeInFrames++
	if c.cumulativeInFrames == 20 && !c.acknowledged {
		return nil, http2ErrorSettingsTimeout
	}
	if inFrame.kind == http2FrameFields {
		if !inFrame.endFields { // continuations follow, join them into fields frame
			if err := c._appendContinuations(inFrame); err != nil {
				return nil, err
			}
		}
		// Got a new fields frame. Set deadline for next fields frame
		if err := c.setReadDeadline(); err != nil {
			return nil, err
		}
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d <--- %+v\n", c.id, inFrame)
	}
	return inFrame, nil
}
func (c *http2Conn_) _growInFrame(size uint16) error {
	c.sectFore += size // size is limited, so won't overflow
	if c.sectFore <= c.inBufferEdge {
		return nil
	}
	// c.sectFore > c.inBufferEdge, needs grow.
	if c.sectFore > c.inBuffer.size() { // needs slide
		if c.inBuffer.getRef() == 1 { // no streams are referring to c.inBuffer, so just slide
			c.inBufferEdge = uint16(copy(c.inBuffer.buf[:], c.inBuffer.buf[c.sectBack:c.inBufferEdge]))
		} else { // there are still streams referring to c.inBuffer. use a new inBuffer
			oldBuffer := c.inBuffer
			c.inBuffer = getHTTP2Buffer()
			c.inBuffer.incRef()
			c.inBufferEdge = uint16(copy(c.inBuffer.buf[:], oldBuffer.buf[c.sectBack:c.inBufferEdge]))
			oldBuffer.decRef()
		}
		c.sectFore -= c.sectBack
		c.sectBack = 0
	}
	return c._fillInBuffer(c.sectFore - c.inBufferEdge)
}
func (c *http2Conn_) _appendContinuations(fieldsInFrame *http2InFrame) error { // into a single fields frame
	fieldsInFrame.inBuffer = nil // will be restored at the end of continuations
	var continuationInFrame http2InFrame
	c.contBack, c.contFore = c.sectFore, c.sectFore
	for { // each continuation frame
		// Receive continuation header
		if err := c._growContinuation(9, fieldsInFrame); err != nil {
			return err
		}
		// Decode continuation header
		if err := continuationInFrame.decodeHeader(c.inBuffer.buf[c.contBack:c.contFore]); err != nil {
			return err
		}
		// Check continuation header
		if continuationInFrame.length == 0 || fieldsInFrame.length+continuationInFrame.length > http2MaxFrameSize {
			return http2ErrorFrameSize
		}
		if continuationInFrame.streamID != fieldsInFrame.streamID || continuationInFrame.kind != http2FrameContinuation {
			return http2ErrorProtocol
		}
		// Receive continuation payload
		c.contBack = c.contFore
		if err := c._growContinuation(continuationInFrame.length, fieldsInFrame); err != nil {
			return err
		}
		// TODO: limit the number of continuation frames to avoid DoS attack
		c.cumulativeInFrames++ // got the continuation frame.
		// Append continuation frame to fields frame
		copy(c.inBuffer.buf[fieldsInFrame.efctEdge:], c.inBuffer.buf[c.contBack:c.contFore]) // may overwrite padding if exists
		fieldsInFrame.efctEdge += continuationInFrame.length
		fieldsInFrame.length += continuationInFrame.length // we don't care if padding is overwritten. just accumulate
		c.sectFore += continuationInFrame.length           // also accumulate fields payload, with padding included
		// End of fields frame?
		if continuationInFrame.endFields {
			fieldsInFrame.endFields = true
			fieldsInFrame.inBuffer = c.inBuffer // restore the inBuffer
			c.sectFore = c.contFore             // for next frame.
			return nil
		}
		c.contBack = c.contFore
	}
}
func (c *http2Conn_) _growContinuation(size uint16, fieldsInFrame *http2InFrame) error {
	c.contFore += size                // won't overflow
	if c.contFore <= c.inBufferEdge { // inBuffer is sufficient
		return nil
	}
	// Needs grow. Cases are (A is payload of the fields frame):
	// c.inBuffer: [| .. ] | A | 9 | B | 9 | C | 9 | D |
	// c.inBuffer: [| .. ] | AB | oooo | 9 | C | 9 | D |
	// c.inBuffer: [| .. ] | ABC | ooooooooooo | 9 | D |
	// c.inBuffer: [| .. ] | ABCD | oooooooooooooooooo |
	if c.contFore > c.inBuffer.size() { // needs slide
		if c.sectBack == 0 { // cannot slide again
			// This should only happens when looking for frame header, the 9 bytes
			return http2ErrorFrameSize
		}
		// Now slide. Skip holes (if any) when sliding
		inBuffer := c.inBuffer
		if c.inBuffer.getRef() != 1 { // there are still streams referring to c.inBuffer. use a new inBuffer
			c.inBuffer = getHTTP2Buffer()
			c.inBuffer.incRef()
		}
		c.sectFore = uint16(copy(c.inBuffer.buf[:], inBuffer.buf[c.sectBack:c.sectFore]))
		c.inBufferEdge = c.sectFore + uint16(copy(c.inBuffer.buf[c.sectFore:], inBuffer.buf[c.contBack:c.inBufferEdge]))
		if inBuffer != c.inBuffer {
			inBuffer.decRef()
		}
		fieldsInFrame.efctFrom -= c.sectBack
		fieldsInFrame.efctEdge -= c.sectBack
		c.sectBack = 0
		c.contBack = c.sectFore
		c.contFore = c.contBack + size
	}
	return c._fillInBuffer(c.contFore - c.inBufferEdge)
}
func (c *http2Conn_) _fillInBuffer(size uint16) error {
	n, err := c.readAtLeast(c.inBuffer.buf[c.inBufferEdge:], int(size))
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL READ=%d -----------------------\n", c.id, n)
	}
	if err != nil && DebugLevel() >= 2 {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	c.inBufferEdge += uint16(n)
	return err
}

func (c *http2Conn_) onPriorityInFrame(priorityInFrame *http2InFrame) error { return nil } // do nothing, priority frames are ignored
func (c *http2Conn_) onPingInFrame(pingInFrame *http2InFrame) error {
	if pingInFrame.ack { // pong
		if c.pingSent {
			return nil
		} else { // TODO: confirm this
			return http2ErrorProtocol
		}
	}
	if now := time.Now(); c.lastPingTime.IsZero() || now.Sub(c.lastPingTime) >= time.Second {
		c.lastPingTime = now
	} else {
		return http2ErrorEnhanceYourCalm
	}
	pongOutFrame := &c.outFrame
	pongOutFrame.stream = nil
	pongOutFrame.length = 8
	pongOutFrame.kind = http2FramePing
	pongOutFrame.ack = true
	pongOutFrame.payload = pingInFrame.effective()
	err := c.sendOutFrame(pongOutFrame)
	pongOutFrame.zero()
	return err
}
func (c *http2Conn_) onWindowUpdateInFrame(windowUpdateInFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(windowUpdateInFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		// The legal range for the increment to the flow-control window is 1 to 2^31 - 1 (2,147,483,647) octets.
		return http2ErrorProtocol
	}
	// TODO
	c.inWindow = int32(windowSize)
	Printf("conn=%d stream=%d windowUpdate=%d\n", c.id, windowUpdateInFrame.streamID, windowSize)
	return nil
}

func (c *http2Conn_) _updatePeerSettings(settingsInFrame *http2InFrame, asClient bool) error {
	settings := settingsInFrame.effective()
	windowDelta := int32(0)
	for i, j, n := uint16(0), uint16(0), settingsInFrame.length/6; i < n; i++ {
		ident := binary.BigEndian.Uint16(settings[j : j+2])
		value := binary.BigEndian.Uint32(settings[j+2 : j+6])
		switch ident {
		case http2SettingHeaderTableSize:
			c.peerSettings.headerTableSize = value
			// TODO: immediate Dynamic Table Size Update
		case http2SettingEnablePush:
			if value > 1 || (value == 1 && asClient) {
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
			c.peerSettings.maxFrameSize = value // we don't use this. we only send frames of the minimal size
		case http2SettingMaxHeaderListSize:
			c.peerSettings.maxHeaderListSize = value // this is only an advisory.
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
func (c *http2Conn_) _adjustStreamWindows(delta int32) {
	// TODO
}

func (c *http2Conn_) sendOutFrame(outFrame *http2OutFrame) error {
	outHeader := outFrame.encodeHeader()
	if len(outFrame.payload) > 0 {
		c.vector = c.fixedVector[0:2]
		c.vector[1] = outFrame.payload
	} else {
		c.vector = c.fixedVector[0:1]
	}
	c.vector[0] = outHeader
	n, err := c.writev(&c.vector)
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Printf("conn=%d ---> %+v\n", c.id, outFrame)
	}
	return err
}

func (c *http2Conn_) _decodeFields(fields []byte, join func(p []byte) bool) bool { // TODO: method value escapes to heap?
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

/*
func (c *http2Conn_) _decodeString(src []byte, req *server2Request) (int, bool) {
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
*/

func (c *http2Conn_) findStream(streamID uint32) http2Stream {
	if index := c._findStreamID(streamID); index != http2MaxConcurrentStreams { // found
		if DebugLevel() >= 2 {
			Printf("conn=%d findStream=%d at %d\n", c.id, streamID, index)
		}
		return c.activeStreams[index]
	} else { // not found
		return nil
	}
}
func (c *http2Conn_) joinStream(stream http2Stream) {
	if index := c._findStreamID(0); index != http2MaxConcurrentStreams { // found
		if DebugLevel() >= 2 {
			Printf("conn=%d joinStream=%d at %d\n", c.id, stream.nativeID(), index)
		}
		stream.setIndex(index)
		c.activeStreams[index] = stream
		c.activeStreamIDs[index] = stream.nativeID()
	} else { // this MUST not happen
		BugExitln("joinStream cannot find an empty slot")
	}
}
func (c *http2Conn_) _findStreamID(streamID uint32) uint8 {
	c.activeStreamIDs[http2MaxConcurrentStreams] = streamID // the stream id to search for
	index := uint8(0)
	for c.activeStreamIDs[index] != streamID {
		index++
	}
	return index
}
func (c *http2Conn_) quitStream(stream http2Stream) {
	index := stream.getIndex()
	if DebugLevel() >= 2 {
		Printf("conn=%d quitStream=%d at %d\n", c.id, stream.nativeID(), index)
	}
	c.activeStreams[index] = nil
	c.activeStreamIDs[index] = 0
}

func (c *http2Conn_) remoteAddr() net.Addr { return c.netConn.RemoteAddr() }

func (c *http2Conn_) setReadDeadline() error {
	if deadline := time.Now().Add(c.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *http2Conn_) setWriteDeadline() error {
	if deadline := time.Now().Add(c.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *http2Conn_) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(c.netConn, dst, min)
}
func (c *http2Conn_) write(src []byte) (int, error) { return c.netConn.Write(src) }
func (c *http2Conn_) writev(srcVec *net.Buffers) (int64, error) {
	return srcVec.WriteTo(c.netConn)
}

// http2Stream
type http2Stream interface {
	// Imports
	httpStream
	// Methods
	nativeID() uint32     // http/2 native stream id
	getIndex() uint8      // at activeStreams
	setIndex(index uint8) // at activeStreams
}

// http2Stream_ is a parent.
type http2Stream_[C http2Conn] struct { // for server2Stream and backend2Stream
	// Parent
	httpStream_[C]
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	id        uint32 // the stream id. we use 4 byte instead of int64 to save memory space! see activeStreamIDs
	inWindow  int32  // stream-level window (buffer) size for incoming DATA frames
	outWindow int32  // stream-level window (buffer) size for outgoing DATA frames
	// Stream states (zeros)
	_http2Stream0 // all values in this struct must be zero by default!
}
type _http2Stream0 struct { // for fast reset, entirely
	index uint8 // the index at s.conn.activeStreams
	state uint8 // http2StateOpen, http2StateLocalClosed, http2StateRemoteClosed
}

func (s *http2Stream_[C]) onUse(conn C, id uint32) {
	s.httpStream_.onUse(conn)

	s.id = id
}
func (s *http2Stream_[C]) onEnd() {
	s._http2Stream0 = _http2Stream0{}

	s.httpStream_.onEnd()
}

func (s *http2Stream_[C]) nativeID() uint32     { return s.id }
func (s *http2Stream_[C]) getIndex() uint8      { return s.index }
func (s *http2Stream_[C]) setIndex(index uint8) { s.index = index }

func (s *http2Stream_[C]) ID() int64 { return int64(s.id) } // implements httpStream interface

func (s *http2Stream_[C]) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?
func (s *http2Stream_[C]) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?

func (s *http2Stream_[C]) setReadDeadline() error { // for content i/o only
	// TODO
	return nil
}
func (s *http2Stream_[C]) setWriteDeadline() error { // for content i/o only
	// TODO
	return nil
}

func (s *http2Stream_[C]) read(dst []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *http2Stream_[C]) readFull(dst []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *http2Stream_[C]) write(src []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *http2Stream_[C]) writev(srcVec *net.Buffers) (int64, error) { // for content i/o only
	// TODO
	return 0, nil
}

// _http2In_ is a mixin.
type _http2In_ struct { // for server2Request and backend2Response
	// Parent
	*_httpIn_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *_http2In_) onUse(parent *_httpIn_) {
	r._httpIn_ = parent
}
func (r *_http2In_) onEnd() {
	r._httpIn_ = nil
}

func (r *_http2In_) _growHeaders(size int32) bool {
	edge := r.inputEdge + size      // size is ensured to not overflow
	if edge < int32(cap(r.input)) { // fast path
		return true
	}
	if edge > _16K { // exceeds the max header section limit
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

func (r *_http2In_) readContent() (data []byte, err error) {
	// TODO
	return
}

// _http2Out_ is a mixin.
type _http2Out_ struct { // for server2Response and backend2Request
	// Parent
	*_httpOut_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *_http2Out_) onUse(parent *_httpOut_) {
	r._httpOut_ = parent
}
func (r *_http2Out_) onEnd() {
	r._httpOut_ = nil
}

func (r *_http2Out_) addHeader(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_http2Out_) header(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_http2Out_) hasHeader(name []byte) bool {
	// TODO
	return false
}
func (r *_http2Out_) delHeader(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *_http2Out_) delHeaderAt(i uint8) {
	// TODO
}

func (r *_http2Out_) sendChain() error {
	// TODO
	return nil
}
func (r *_http2Out_) _sendEntireChain() error {
	// TODO
	return nil
}
func (r *_http2Out_) _sendSingleRange() error {
	// TODO
	return nil
}
func (r *_http2Out_) _sendMultiRanges() error {
	// TODO
	return nil
}

func (r *_http2Out_) echoChain() error {
	// TODO
	return nil
}

func (r *_http2Out_) addTrailer(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_http2Out_) trailer(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_http2Out_) trailers() []byte {
	// TODO
	return nil
}

func (r *_http2Out_) proxyPassBytes(data []byte) error { return r.writeBytes(data) }

func (r *_http2Out_) finalizeVague2() error {
	// TODO
	if r.numTrailerFields == 1 { // no trailer section
	} else { // with trailer section
	}
	return nil
}

func (r *_http2Out_) writeHeaders() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that header fields are all sent, r.fields will be used by trailer fields (if any), so reset it.
	return nil
}
func (r *_http2Out_) writePiece(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *_http2Out_) _writeTextPiece(piece *Piece) error {
	// TODO
	return nil
}
func (r *_http2Out_) _writeFilePiece(piece *Piece) error {
	// TODO
	return nil
}
func (r *_http2Out_) writeVector() error {
	// TODO
	return nil
}
func (r *_http2Out_) writeBytes(data []byte) error {
	// TODO
	return nil
}

// _http2Socket_ is a mixin.
type _http2Socket_ struct { // for server2Socket and backend2Socket
	// Parent
	*_httpSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *_http2Socket_) onUse(parent *_httpSocket_) {
	s._httpSocket_ = parent
}
func (s *_http2Socket_) onEnd() {
	s._httpSocket_ = nil
}

func (s *_http2Socket_) todo2() {
	s.todo()
}
