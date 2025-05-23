// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 types. See RFC 9113 and RFC 7541.

package hemi

import (
	"bytes"
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
type http2Conn_[S http2Stream] struct { // for backend2Conn and server2Conn
	// Parent
	httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	outFrame http2OutFrame[S] // used by c.manage() itself to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	netConn      net.Conn                         // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn      syscall.RawConn                  // for syscall. only usable when netConn is TCP/UDS
	peerSettings http2Settings                    // settings of the remote peer
	freeSeats    [http2MaxConcurrentStreams]uint8 // a stack holding free seats for streams of this connection in c.activeStreams and c.activeStreamIDs
	freeSeatsTop uint8                            // the stack top of c.freeSeats
	inBuffer     *http2Buffer                     // http2Buffer in use, for receiving incoming frames. may be replaced again and again during connection's lifetime
	decodeTable  hpackTable                       // <= 5 KiB. hpack table used for decoding field blocks that are received from remote
	incomingChan chan any                         // frames and errors generated by c.receive() and waiting for c.manage() to consume
	localWindow  int32                            // connection-level window (buffer) size for receiving incoming DATA frames
	remoteWindow int32                            // connection-level window (buffer) size for sending outgoing DATA frames
	outgoingChan chan *http2OutFrame[S]           // frames generated by streams of this connection and waiting for c.manage() to send
	encodeTable  hpackTable                       // <= 5 KiB. hpack table used for encoding field lists that are sent to remote
	// Conn states (zeros)
	activeStreams    [http2MaxConcurrentStreams]S // <= 1 KiB. active (http2StateOpen, http2StateLocalClosed, http2StateRemoteClosed) streams
	lastSettingsTime time.Time                    // last settings time
	lastPingTime     time.Time                    // last ping time
	inFrame0         http2InFrame                 // incoming frame 0
	inFrame1         http2InFrame                 // incoming frame 1
	inFrame          *http2InFrame                // current incoming frame. refers to inFrame0 or inFrame1 in turn
	vector           net.Buffers                  // used by writev in c.manage()
	fixedVector      [64][]byte                   // 1.5 KiB. used by writev in c.manage()
	_http2Conn0                                   // all values in this struct must be zero by default!
}
type _http2Conn0 struct { // for fast reset, entirely
	activeStreamIDs    [http2MaxConcurrentStreams + 1]uint32 // ids of c.activeStreams. the extra 1 id is used for fast linear searching
	readDeadlines      [http2MaxConcurrentStreams + 1]int64  // 1 KiB
	writeDeadlines     [http2MaxConcurrentStreams + 1]int64  // 1 KiB
	_                  uint32
	lastStreamID       uint32 // id of last stream sent to backend or received by server
	cumulativeInFrames int64  // num of incoming frames
	concurrentStreams  uint8  // num of active streams
	acknowledged       bool   // server settings acknowledged by client?
	pingSent           bool   // is there a ping frame sent and waiting for response?
	inBufferEdge       uint16 // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	sectBack           uint16 // incoming frame section (header or payload) begins from c.inBuffer.buf[c.sectBack]
	sectFore           uint16 // incoming frame section (header or payload) ends at c.inBuffer.buf[c.sectFore]
	contBack           uint16 // incoming continuation part (header or payload) begins from c.inBuffer.buf[c.contBack]
	contFore           uint16 // incoming continuation part (header or payload) ends at c.inBuffer.buf[c.contFore]
}

func (c *http2Conn_[S]) onGet(id int64, holder holder, netConn net.Conn, rawConn syscall.RawConn) {
	c.httpConn_.onGet(id, holder)

	c.netConn = netConn
	c.rawConn = rawConn
	c.peerSettings = http2InitialSettings
	c.freeSeats = http2FreeSeats
	c.freeSeatsTop = 255 // +1 becomes 0
	if c.inBuffer == nil {
		c.inBuffer = getHTTP2Buffer()
		c.inBuffer.incRef()
	}
	c.decodeTable.init()
	if c.incomingChan == nil {
		c.incomingChan = make(chan any)
	}
	c.localWindow = _2G1 - _64K1                      // as a receiver, we disable connection-level flow control
	c.remoteWindow = c.peerSettings.initialWindowSize // after we received the peer's preface, this value will be changed to the real value.
	if c.outgoingChan == nil {
		c.outgoingChan = make(chan *http2OutFrame[S])
	}
	c.encodeTable.init()
}
func (c *http2Conn_[S]) onPut() {
	c.netConn = nil
	c.rawConn = nil
	// c.inBuffer is reserved
	// c.decodeTable is reserved
	// c.incomingChan is reserved
	// c.outgoingChan is reserved
	// c.encodeTable is reserved
	c.activeStreams = [http2MaxConcurrentStreams]S{}
	c.lastSettingsTime = time.Time{}
	c.lastPingTime = time.Time{}
	c.inFrame0.zero()
	c.inFrame1.zero()
	c.inFrame = nil
	c.vector = nil
	c.fixedVector = [64][]byte{}
	c._http2Conn0 = _http2Conn0{}

	c.httpConn_.onPut()
}

func (c *http2Conn_[S]) receive(asServer bool) { // runner, employed by c.manage()
	if DebugLevel() >= 1 {
		defer Printf("conn=%d c.receive() quit\n", c.id)
	}
	if asServer { // We must read the HTTP/2 PRISM at the very begining
		if err := c.setReadDeadline(); err != nil {
			c.incomingChan <- err
			return
		}
		if err := c._growInFrame(24); err != nil { // HTTP/2 PRISM is exactly 24 bytes
			c.incomingChan <- err
			return
		}
		if !bytes.Equal(c.inBuffer.buf[:24], backend2PrefaceAndMore[:24]) {
			c.incomingChan <- http2ErrorProtocol
			return
		}
		c.incomingChan <- nil // notify c.manage() that we have successfully read the HTTP/2 PRISM
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

func (c *http2Conn_[S]) _getFreeSeat() uint8 {
	c.freeSeatsTop++
	return c.freeSeats[c.freeSeatsTop]
}
func (c *http2Conn_[S]) _putFreeSeat(seat uint8) {
	c.freeSeats[c.freeSeatsTop] = seat
	c.freeSeatsTop--
}

func (c *http2Conn_[S]) appendStream(stream S) { // O(1)
	seat := c._getFreeSeat()
	stream.setSeat(seat)
	c.activeStreams[seat] = stream
	c.activeStreamIDs[seat] = stream.nativeID()
	if DebugLevel() >= 2 {
		Printf("conn=%d appendStream=%d at %d\n", c.id, stream.nativeID(), seat)
	}
}
func (c *http2Conn_[S]) searchStream(streamID uint32) S { // O(http2MaxConcurrentStreams), but in practice this linear search algorithm should be fast enough
	c.activeStreamIDs[http2MaxConcurrentStreams] = streamID // the stream id to search for
	seat := uint8(0)
	for c.activeStreamIDs[seat] != streamID {
		seat++
	}
	if seat != http2MaxConcurrentStreams { // found
		if DebugLevel() >= 2 {
			Printf("conn=%d searchStream=%d at %d\n", c.id, streamID, seat)
		}
		return c.activeStreams[seat]
	} else { // not found
		var null S // nil
		return null
	}
}
func (c *http2Conn_[S]) retireStream(stream S) { // O(1)
	seat := stream.getSeat()
	if DebugLevel() >= 2 {
		Printf("conn=%d retireStream=%d at %d\n", c.id, stream.nativeID(), seat)
	}
	var null S // nil
	c.activeStreams[seat] = null
	c.activeStreamIDs[seat] = 0
	c._putFreeSeat(seat)
}

func (c *http2Conn_[S]) recvInFrame() (*http2InFrame, error) { // excluding pushPromise, which is not supported, and continuation, which cannot arrive alone
	// Receive frame header
	c.sectBack = c.sectFore
	if err := c.setReadDeadline(); err != nil {
		return nil, err
	}
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
	if !inFrame.isUnknown() {
		// Check the frame
		if err := http2InFrameCheckers[inFrame.kind](inFrame); err != nil {
			return nil, err
		}
	}
	c.cumulativeInFrames++
	if c.cumulativeInFrames == 10 && !c.acknowledged { // TODO: change this policy?
		return nil, http2ErrorSettingsTimeout
	}
	if inFrame.kind == http2FrameFields && !inFrame.endFields { // continuations follow, coalesce them into fields frame
		if err := c._coalesceContinuations(inFrame); err != nil {
			return nil, err
		}
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d <--- %+v\n", c.id, inFrame)
	}
	return inFrame, nil
}
func (c *http2Conn_[S]) _growInFrame(size uint16) error {
	c.sectFore += size // size is limited, so won't overflow
	if c.sectFore <= c.inBufferEdge {
		return nil
	}
	// c.sectFore > c.inBufferEdge, needs grow.
	if c.sectFore > c.inBuffer.size() { // needs slide
		if c.inBuffer.getRef() == 1 { // no streams are referring to c.inBuffer, so just slide
			c.inBufferEdge = uint16(copy(c.inBuffer.buf[:], c.inBuffer.buf[c.sectBack:c.inBufferEdge]))
		} else { // there are remaining streams referring to c.inBuffer. use a new inBuffer
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
func (c *http2Conn_[S]) _fillInBuffer(size uint16) error {
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

func (c *http2Conn_[S]) _coalesceContinuations(fieldsInFrame *http2InFrame) error { // into a single fields frame
	fieldsInFrame.inBuffer = nil // unset temporarily, will be restored at the end of continuations
	var continuationInFrame http2InFrame
	c.contBack, c.contFore = c.sectFore, c.sectFore
	for i := 0; i < 4; i++ { // max 4 continuation frames are allowed!
		// Receive continuation header
		if err := c._growContinuationFrame(9, fieldsInFrame); err != nil {
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
		if err := c._growContinuationFrame(continuationInFrame.length, fieldsInFrame); err != nil {
			return err
		}
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
	return http2ErrorEnhanceYourCalm
}
func (c *http2Conn_[S]) _growContinuationFrame(size uint16, fieldsInFrame *http2InFrame) error {
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

func (c *http2Conn_[S]) processDataInFrame(dataInFrame *http2InFrame) error {
	// TODO
	return nil
}
func (c *http2Conn_[S]) processPriorityInFrame(priorityInFrame *http2InFrame) error { return nil } // do nothing, priority frames are ignored
func (c *http2Conn_[S]) processResetStreamInFrame(resetStreamInFrame *http2InFrame) error {
	streamID := resetStreamInFrame.streamID
	if streamID > c.lastStreamID {
		// RST_STREAM frames MUST NOT be sent for a stream in the "idle" state. If a RST_STREAM frame identifying an idle stream is received,
		// the recipient MUST treat this as a connection error (Section 5.4.1) of type PROTOCOL_ERROR.
		return http2ErrorProtocol
	}
	// TODO
	return nil
}
func (c *http2Conn_[S]) processPushPromiseInFrame(pushPromiseInFrame *http2InFrame) error {
	panic("pushPromise frames should be rejected priorly")
}
func (c *http2Conn_[S]) processPingInFrame(pingInFrame *http2InFrame) error {
	if pingInFrame.ack { // pong
		if !c.pingSent { // TODO: confirm this
			return http2ErrorProtocol
		}
		return nil
	}
	if now := time.Now(); c.lastPingTime.IsZero() || now.Sub(c.lastPingTime) >= time.Second {
		c.lastPingTime = now
	} else {
		return http2ErrorEnhanceYourCalm
	}
	pongOutFrame := &c.outFrame
	pongOutFrame.streamID = 0
	pongOutFrame.length = 8
	pongOutFrame.kind = http2FramePing
	pongOutFrame.ack = true
	pongOutFrame.payload = pingInFrame.effective()
	err := c.sendOutFrame(pongOutFrame)
	pongOutFrame.zero()
	return err
}
func (c *http2Conn_[S]) processGoawayInFrame(goawayInFrame *http2InFrame) error {
	panic("goaway frames should be hijacked by c.receive()")
}
func (c *http2Conn_[S]) processWindowUpdateInFrame(windowUpdateInFrame *http2InFrame) error {
	windowSize := binary.BigEndian.Uint32(windowUpdateInFrame.effective())
	if windowSize == 0 || windowSize > _2G1 {
		// The legal range for the increment to the flow-control window is 1 to 2^31 - 1 (2,147,483,647) octets.
		return http2ErrorProtocol
	}
	// TODO
	c.localWindow = int32(windowSize)
	Printf("conn=%d stream=%d windowUpdate=%d\n", c.id, windowUpdateInFrame.streamID, windowSize)
	return nil
}
func (c *http2Conn_[S]) processContinuationInFrame(continuationInFrame *http2InFrame) error {
	panic("lonely continuation frames should be rejected priorly")
}

func (c *http2Conn_[S]) _updatePeerSettings(settingsInFrame *http2InFrame, asClient bool) error {
	settings := settingsInFrame.effective()
	windowDelta := int32(0)
	for i, j, n := uint16(0), uint16(0), settingsInFrame.length/6; i < n; i++ {
		ident := binary.BigEndian.Uint16(settings[j : j+2])
		value := binary.BigEndian.Uint32(settings[j+2 : j+6])
		switch ident {
		case http2SettingMaxHeaderTableSize:
			c.peerSettings.maxHeaderTableSize = value
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
		//c._adjustStreamWindows(windowDelta)
	}
	Printf("conn=%d peerSettings=%+v\n", c.id, c.peerSettings)
	return nil
}

func (c *http2Conn_[S]) decodeFields(fields []byte, input *[]byte) bool {
	// TODO
	return false
	/*
		var fieldName, lineValue []byte
		var (
			I  uint32 // the decoded integer
			j  int    // index of fields
			ok bool   // decode succeeds or not
		)
		i, n := 0, len(fields)
		for i < n {
			b := fields[i]
			if b >= 0b_1000_0000 { // 6.1. Indexed Header Field Representation
				I, j, ok = hpackDecodeVarint(fields[i:], 7, http2MaxTableIndex)
				if !ok || I == 0 { // The index value of 0 is not used.  It MUST be treated as a decoding error if found in an indexed header field representation.
					Println("decode error")
					return false
				}
				fieldName, lineValue, ok = c.decodeTable.get(I)
				if !ok { // Indices strictly greater than the sum of the lengths of both tables MUST be treated as a decoding error.
					return false
				}
				i += j
			} else if b >= 0b_0010_0000 && b < 0b_0100_0000 { // 6.3. Dynamic Table Size Update
				I, j, ok = hpackDecodeVarint(fields[i:], 5, http2MaxTableSize)
				if !ok {
					Println("decode error")
					return false
				}
				i += j
				Printf("update size=%d\n", I)
			} else { // 0b_01xx_xxxx, 0b_0000_xxxx, 0b_0001_xxxx
				var N int
				if b >= 0b_0100_0000 { // 6.2.1. Literal Header Field with Incremental Indexing
					N = 6
				} else { // 0b_0000_xxxx (6.2.2. Literal Header Field without Indexing), 0b_0001_xxxx (6.2.3. Literal Header Field Never Indexed)
					N = 4
				}
				I, j, ok = hpackDecodeVarint(fields[i:], N, http2MaxTableIndex)
				if !ok {
					println("decode error")
					return false
				}
				i += j
				if I != 0 { // Indexed Name
					fieldName, _, ok = c.decodeTable.get(I)
					if !ok {
						Println("decode error")
						return false
					}
				} else { // New Name
					fieldName, j, ok = hpackDecodeString(input, fields[i:], 255)
					if !ok || len(fieldName) == 0 {
						Println("decode error")
						return false
					}
					i += j
				}
				lineValue, j, ok = hpackDecodeString(input, fields[i:], _16K)
				if !ok {
					Println("decode error")
					return false
				}
				i += j
				if b >= 0b_0100_0000 {
					c.decodeTable.add(fieldName, lineValue)
				} else if b >= 0b_0001_0000 {
					// TODO: never indexed
				}
			}
			Printf("name=%s value=%s\n", fieldName, lineValue)
		}
		return i == n
	*/
}

func (c *http2Conn_[S]) sendOutFrame(outFrame *http2OutFrame[S]) error {
	outHeader := outFrame.encodeHeader()
	if len(outFrame.payload) > 0 {
		c.vector = c.fixedVector[0:2]
		c.vector[1] = outFrame.payload
	} else {
		c.vector = c.fixedVector[0:1]
	}
	c.vector[0] = outHeader
	if err := c.setWriteDeadline(); err != nil {
		return err
	}
	n, err := c.writev(&c.vector)
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Printf("conn=%d ---> %+v\n", c.id, outFrame)
	}
	return err
}

func (c *http2Conn_[S]) encodeFields(fields []byte, output *[]byte) { // TODO
	// uses c.encodeTable
}

func (c *http2Conn_[S]) remoteAddr() net.Addr { return c.netConn.RemoteAddr() }

func (c *http2Conn_[S]) setReadDeadline() error {
	if deadline := time.Now().Add(c.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *http2Conn_[S]) setWriteDeadline() error {
	if deadline := time.Now().Add(c.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *http2Conn_[S]) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(c.netConn, dst, min)
}
func (c *http2Conn_[S]) write(src []byte) (int, error) { return c.netConn.Write(src) }
func (c *http2Conn_[S]) writev(srcVec *net.Buffers) (int64, error) {
	return srcVec.WriteTo(c.netConn)
}

// http2Stream
type http2Stream interface {
	// Imports
	httpStream
	// Methods
	nativeID() uint32   // http/2 native stream id
	getSeat() uint8     // at activeStreamIDs and activeStreams
	setSeat(seat uint8) // at activeStreamIDs and activeStreams
}

// http2Stream_ is a parent.
type http2Stream_[C http2Conn] struct { // for backend2Stream and server2Stream
	// Parent
	httpStream_[C]
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	id           uint32 // the stream id. we use 4 byte instead of int64 (which is used in http/3) to save memory space! see activeStreamIDs
	localWindow  int32  // stream-level window (buffer) size for receiving incoming DATA frames
	remoteWindow int32  // stream-level window (buffer) size for sending outgoing DATA frames
	// Stream states (zeros)
	_http2Stream0 // all values in this struct must be zero by default!
}
type _http2Stream0 struct { // for fast reset, entirely
	seat  uint8 // the seat at s.conn.activeStreamIDs and s.conn.activeStreams
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

func (s *http2Stream_[C]) nativeID() uint32   { return s.id }
func (s *http2Stream_[C]) getSeat() uint8     { return s.seat }
func (s *http2Stream_[C]) setSeat(seat uint8) { s.seat = seat }

func (s *http2Stream_[C]) ID() int64 { return int64(s.id) } // implements httpStream interface

func (s *http2Stream_[C]) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?
func (s *http2Stream_[C]) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?

func (s *http2Stream_[C]) setReadDeadline() error { // for content and trailers i/o only
	// TODO
	return nil
}
func (s *http2Stream_[C]) setWriteDeadline() error {
	// TODO
	return nil
}

func (s *http2Stream_[C]) read(dst []byte) (int, error) { // for content and trailers i/o only
	// TODO
	return 0, nil
}
func (s *http2Stream_[C]) readFull(dst []byte) (int, error) { // for content and trailers i/o only
	// TODO
	return 0, nil
}
func (s *http2Stream_[C]) write(src []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http2Stream_[C]) writev(srcVec *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

// _http2In_ is a mixin.
type _http2In_ struct { // for backend2Response and server2Request
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
func (r *_http2In_) _readSizedContent() ([]byte, error) {
	// r.stream.setReadDeadline() // may be called multiple times during the reception of the sized content
	return nil, nil
}
func (r *_http2In_) _readVagueContent() ([]byte, error) {
	// r.stream.setReadDeadline() // may be called multiple times during the reception of the vague content
	return nil, nil
}

// _http2Out_ is a mixin.
type _http2Out_ struct { // for backend2Request and server2Response
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
	r.outputEdge = 0 // now that header fields are all sent, r.output will be used by trailer fields (if any), so reset it.
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
	// r.stream.setWriteDeadline() // for _writeFilePiece
	// r.stream.write() or r.stream.writev()
	return nil
}
func (r *_http2Out_) writeVector() error {
	// TODO
	// r.stream.setWriteDeadline() // for writeVector
	// r.stream.writev()
	return nil
}
func (r *_http2Out_) writeBytes(data []byte) error {
	// TODO
	// r.stream.setWriteDeadline() // for writeBytes
	// r.stream.write()
	return nil
}

// _http2Socket_ is a mixin.
type _http2Socket_ struct { // for backend2Socket and server2Socket
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
