// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 mixins. See RFC 9113 and RFC 7541.

package hemi

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

//////////////////////////////////////// HTTP/2 general implementation ////////////////////////////////////////

// http2Conn collects shared methods between *server2Conn and *backend2Conn.
type http2Conn interface {
	// Imports
	httpConn
	// Methods
}

// _http2Conn_ is the parent for server2Conn and backend2Conn.
type _http2Conn_ struct {
	// Parent
	_httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	outFrame http2OutFrame // used by c.manager() to send special out frames. immediately reset after use
	// Conn states (non-zeros)
	netConn      net.Conn            // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn      syscall.RawConn     // for syscall. only usable when netConn is TCP/UDS
	peerSettings http2Settings       // settings of the remote peer
	inBuffer     *http2InBuffer      // http2InBuffer in use, for receiving incoming frames
	table        http2DynamicTable   // dynamic table
	incomingChan chan any            // frames and errors generated by c.receiver() and waiting for c.manager() to consume
	inWindow     int32               // connection-level window size for incoming DATA frames
	outWindow    int32               // connection-level window size for outgoing DATA frames
	outgoingChan chan *http2OutFrame // frames generated by streams and waiting for c.manager() to send
	// Conn states (zeros)
	inFrame0      http2InFrame                           // incoming frame 0
	inFrame1      http2InFrame                           // incoming frame 1
	inFrame       *http2InFrame                          // current incoming frame. refers to inFrame0 or inFrame1 in turn
	activeStreams [http2MaxConcurrentStreams]http2Stream // active (open, remoteClosed, localClosed) streams
	vector        net.Buffers                            // used by writev in c.manager()
	fixedVector   [2][]byte                              // used by writev in c.manager()
	_http2Conn0                                          // all values in this struct must be zero by default!
}
type _http2Conn0 struct { // for fast reset, entirely
	activeStreamIDs    [http2MaxConcurrentStreams + 1]uint32 // ids of c.activeStreams. the extra 1 id is used for fast linear searching
	cumulativeInFrames int64                                 // num of incoming frames
	concurrentStreams  uint8                                 // num of active streams
	acknowledged       bool                                  // server settings acknowledged by client?
	inBufferEdge       uint32                                // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	partBack           uint32                                // incoming frame part (header or payload) begins from c.inBuffer.buf[c.partBack]
	partFore           uint32                                // incoming frame part (header or payload) ends at c.inBuffer.buf[c.partFore]
	contBack           uint32                                // incoming continuation part (header or payload) begins from c.inBuffer.buf[c.contBack]
	contFore           uint32                                // incoming continuation part (header or payload) ends at c.inBuffer.buf[c.contFore]
}

func (c *_http2Conn_) onGet(id int64, stage *Stage, udsMode bool, tlsMode bool, netConn net.Conn, rawConn syscall.RawConn, readTimeout time.Duration, writeTimeout time.Duration) {
	c._httpConn_.onGet(id, stage, udsMode, tlsMode, readTimeout, writeTimeout)

	c.netConn = netConn
	c.rawConn = rawConn
	c.peerSettings = http2InitialSettings
	if c.inBuffer == nil {
		c.inBuffer = getHTTP2InBuffer()
		c.inBuffer.incRef()
	}
	c.table.init()
	if c.incomingChan == nil {
		c.incomingChan = make(chan any)
	}
	c.inWindow = _2G1 - _64K1                      // as a receiver, we disable connection-level flow control
	c.outWindow = c.peerSettings.initialWindowSize // after we have received the client preface, this value will be changed to the real value of client.
	if c.outgoingChan == nil {
		c.outgoingChan = make(chan *http2OutFrame)
	}
}
func (c *_http2Conn_) onPut() {
	// c.inBuffer is reserved
	// c.table is reserved
	// c.incomingChan is reserved
	// c.outgoingChan is reserved
	c.inFrame0.zero()
	c.inFrame1.zero()
	c.inFrame = nil
	c.activeStreams = [http2MaxConcurrentStreams]http2Stream{}
	c.vector = nil
	c.fixedVector = [2][]byte{}
	c._http2Conn0 = _http2Conn0{}
	c.netConn = nil
	c.rawConn = nil

	c._httpConn_.onPut()
}

func (c *_http2Conn_) receiver() { // runner
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

func (c *_http2Conn_) recvInFrame() (*http2InFrame, error) {
	// Receive frame header
	c.partBack = c.partFore
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
	if err := inFrame.decodeHeader(c.inBuffer.buf[c.partBack:c.partFore]); err != nil {
		return nil, err
	}
	// Receive frame payload
	c.partBack = c.partFore
	if err := c._growInFrame(inFrame.length); err != nil {
		return nil, err
	}
	// Mark frame payload
	inFrame.inBuffer = c.inBuffer
	inFrame.realFrom = c.partBack
	inFrame.realEdge = c.partFore
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
	if inFrame.kind == http2FrameHeaders {
		if !inFrame.endHeaders { // continuations follow, join them into headers frame
			if err := c._joinContinuations(inFrame); err != nil {
				return nil, err
			}
		}
		// Got a new headers frame. Set deadline for next headers frame
		if err := c.setReadDeadline(); err != nil {
			return nil, err
		}
	}
	if DebugLevel() >= 2 {
		Printf("conn=%d <--- %+v\n", c.id, inFrame)
	}
	return inFrame, nil
}
func (c *_http2Conn_) _growInFrame(size uint32) error {
	c.partFore += size // size is limited, so won't overflow
	if c.partFore <= c.inBufferEdge {
		return nil
	}
	// c.partFore > c.inBufferEdge, needs grow.
	if c.partFore > c.inBuffer.size() { // needs slide
		if c.inBuffer.getRef() == 1 { // no streams are referring to c.inBuffer, so just slide
			c.inBufferEdge = uint32(copy(c.inBuffer.buf[:], c.inBuffer.buf[c.partBack:c.inBufferEdge]))
		} else { // there are still streams referring to c.inBuffer. use a new inBuffer
			oldBuffer := c.inBuffer
			c.inBuffer = getHTTP2InBuffer()
			c.inBuffer.incRef()
			c.inBufferEdge = uint32(copy(c.inBuffer.buf[:], oldBuffer.buf[c.partBack:c.inBufferEdge]))
			oldBuffer.decRef()
		}
		c.partFore -= c.partBack
		c.partBack = 0
	}
	return c._fillInBuffer(c.partFore - c.inBufferEdge)
}
func (c *_http2Conn_) _joinContinuations(headersInFrame *http2InFrame) error { // into a single headers frame
	headersInFrame.inBuffer = nil // will be restored at the end of continuations
	var continuationInFrame http2InFrame
	c.contBack, c.contFore = c.partFore, c.partFore
	for { // each continuation frame
		// Receive continuation header
		if err := c._growContinuation(9, headersInFrame); err != nil {
			return err
		}
		// Decode continuation header
		if err := continuationInFrame.decodeHeader(c.inBuffer.buf[c.contBack:c.contFore]); err != nil {
			return err
		}
		// Check continuation header
		if continuationInFrame.length == 0 || headersInFrame.length+continuationInFrame.length > http2MaxFrameSize {
			return http2ErrorFrameSize
		}
		if continuationInFrame.streamID != headersInFrame.streamID || continuationInFrame.kind != http2FrameContinuation {
			return http2ErrorProtocol
		}
		// Receive continuation payload
		c.contBack = c.contFore
		if err := c._growContinuation(continuationInFrame.length, headersInFrame); err != nil {
			return err
		}
		// TODO: limit the number of continuation frames to avoid DoS attack
		c.cumulativeInFrames++ // got the continuation frame.
		// Append continuation frame to headers frame
		copy(c.inBuffer.buf[headersInFrame.realEdge:], c.inBuffer.buf[c.contBack:c.contFore]) // may overwrite padding if exists
		headersInFrame.realEdge += continuationInFrame.length
		headersInFrame.length += continuationInFrame.length // we don't care if padding is overwritten. just accumulate
		c.partFore += continuationInFrame.length            // also accumulate headers payload, with padding included
		// End of headers?
		if continuationInFrame.endHeaders {
			headersInFrame.endHeaders = true
			headersInFrame.inBuffer = c.inBuffer // restore the inBuffer
			c.partFore = c.contFore              // for next frame.
			return nil
		}
		c.contBack = c.contFore
	}
}
func (c *_http2Conn_) _growContinuation(size uint32, headersInFrame *http2InFrame) error {
	c.contFore += size                // won't overflow
	if c.contFore <= c.inBufferEdge { // inBuffer is sufficient
		return nil
	}
	// Needs grow. Cases are (A is payload of the headers frame):
	// c.inBuffer: [| .. ] | A | 9 | B | 9 | C | 9 | D |
	// c.inBuffer: [| .. ] | AB | oooo | 9 | C | 9 | D |
	// c.inBuffer: [| .. ] | ABC | ooooooooooo | 9 | D |
	// c.inBuffer: [| .. ] | ABCD | oooooooooooooooooo |
	if c.contFore > c.inBuffer.size() { // needs slide
		if c.partBack == 0 { // cannot slide again
			// This should only happens when looking for header, the 9 bytes
			return http2ErrorFrameSize
		}
		// Now slide. Skip holes (if any) when sliding
		inBuffer := c.inBuffer
		if c.inBuffer.getRef() != 1 { // there are still streams referring to c.inBuffer. use a new inBuffer
			c.inBuffer = getHTTP2InBuffer()
			c.inBuffer.incRef()
		}
		c.partFore = uint32(copy(c.inBuffer.buf[:], inBuffer.buf[c.partBack:c.partFore]))
		c.inBufferEdge = c.partFore + uint32(copy(c.inBuffer.buf[c.partFore:], inBuffer.buf[c.contBack:c.inBufferEdge]))
		if inBuffer != c.inBuffer {
			inBuffer.decRef()
		}
		headersInFrame.realFrom -= c.partBack
		headersInFrame.realEdge -= c.partBack
		c.partBack = 0
		c.contBack = c.partFore
		c.contFore = c.contBack + size
	}
	return c._fillInBuffer(c.contFore - c.inBufferEdge)
}
func (c *_http2Conn_) _fillInBuffer(size uint32) error {
	n, err := c.readAtLeast(c.inBuffer.buf[c.inBufferEdge:], int(size))
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL READ=%d -----------------------\n", c.id, n)
	}
	if err != nil && DebugLevel() >= 2 {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
	c.inBufferEdge += uint32(n)
	return err
}

func (c *_http2Conn_) sendOutFrame(outFrame *http2OutFrame) error {
	frameHeader := outFrame.encodeHeader()
	if len(outFrame.payload) > 0 {
		c.vector = c.fixedVector[0:2]
		c.vector[1] = outFrame.payload
	} else {
		c.vector = c.fixedVector[0:1]
	}
	c.vector[0] = frameHeader
	n, err := c.writev(&c.vector)
	if DebugLevel() >= 2 {
		Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Printf("conn=%d ---> %+v\n", c.id, outFrame)
	}
	return err
}

func (c *_http2Conn_) _decodeFields(fields []byte, join func(p []byte) bool) bool {
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
func (c *_http2Conn_) _decodeString(src []byte, req *server2Request) (int, bool) {
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

func (c *_http2Conn_) findStream(streamID uint32) http2Stream {
	c.activeStreamIDs[http2MaxConcurrentStreams] = streamID // the stream id to search for
	index := uint8(0)
	for c.activeStreamIDs[index] != streamID { // searching for stream id
		index++
	}
	if index != http2MaxConcurrentStreams { // found
		if DebugLevel() >= 2 {
			Printf("conn=%d findStream=%d at %d\n", c.id, streamID, index)
		}
		return c.activeStreams[index]
	} else { // not found
		return nil
	}
}
func (c *_http2Conn_) joinStream(stream http2Stream) {
	c.activeStreamIDs[http2MaxConcurrentStreams] = 0
	index := uint8(0)
	for c.activeStreamIDs[index] != 0 { // searching a free slot
		index++
	}
	if index != http2MaxConcurrentStreams {
		if DebugLevel() >= 2 {
			Printf("conn=%d joinStream=%d at %d\n", c.id, stream.getID(), index)
		}
		stream.setIndex(index)
		c.activeStreams[index] = stream
		c.activeStreamIDs[index] = stream.getID()
	} else { // this MUST not happen
		BugExitln("joinStream cannot find an empty slot")
	}
}
func (c *_http2Conn_) quitStream(streamID uint32) {
	stream := c.findStream(streamID)
	if stream != nil {
		index := stream.getIndex()
		if DebugLevel() >= 2 {
			Printf("conn=%d quitStream=%d at %d\n", c.id, streamID, index)
		}
		c.activeStreams[index] = nil
		c.activeStreamIDs[index] = 0
	} else { // this MUST not happen
		BugExitln("quitStream cannot find the stream")
	}
}

func (c *_http2Conn_) remoteAddr() net.Addr { return c.netConn.RemoteAddr() }

func (c *_http2Conn_) setReadDeadline() error {
	if deadline := time.Now().Add(c.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *_http2Conn_) setWriteDeadline() error {
	if deadline := time.Now().Add(c.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *_http2Conn_) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(c.netConn, dst, min)
}
func (c *_http2Conn_) write(src []byte) (int, error) { return c.netConn.Write(src) }
func (c *_http2Conn_) writev(srcVec *net.Buffers) (int64, error) {
	return srcVec.WriteTo(c.netConn)
}

// http2Stream collects shared methods between *server2Stream and *backend2Stream.
type http2Stream interface {
	// Imports
	httpStream
	// Methods
	getID() uint32
	getIndex() uint8      // at activeStreams
	setIndex(index uint8) // at activeStreams
}

// _http2Stream_ is the parent for server2Stream and backend2Stream.
type _http2Stream_[C http2Conn] struct {
	// Parent
	_httpStream_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	id   uint32 // the stream id
	conn C      // the http/2 connection
	// Stream states (zeros)
	_http2Stream0 // all values in this struct must be zero by default!
}
type _http2Stream0 struct { // for fast reset, entirely
	index uint8 // the index at s.conn.activeStreams
	state uint8 // http2StateOpen, http2StateRemoteClosed, ...
}

func (s *_http2Stream_[C]) onUse(id uint32, conn C) {
	s._httpStream_.onUse()

	s.id = id
	s.conn = conn
}
func (s *_http2Stream_[C]) onEnd() {
	s._http2Stream0 = _http2Stream0{}

	// s.conn will be set as nil by upper code
	s._httpStream_.onEnd()
}

func (s *_http2Stream_[C]) getID() uint32 { return s.id }

func (s *_http2Stream_[C]) getIndex() uint8      { return s.index }
func (s *_http2Stream_[C]) setIndex(index uint8) { s.index = index }

func (s *_http2Stream_[C]) Conn() httpConn       { return s.conn }
func (s *_http2Stream_[C]) remoteAddr() net.Addr { return s.conn.remoteAddr() }

func (s *_http2Stream_[C]) markBroken()    { s.conn.markBroken() }      // TODO: limit the breakage in the stream?
func (s *_http2Stream_[C]) isBroken() bool { return s.conn.isBroken() } // TODO: limit the breakage in the stream?

func (s *_http2Stream_[C]) setReadDeadline() error { // for content i/o only
	// TODO
	return nil
}
func (s *_http2Stream_[C]) setWriteDeadline() error { // for content i/o only
	// TODO
	return nil
}

func (s *_http2Stream_[C]) read(dst []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *_http2Stream_[C]) readFull(dst []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *_http2Stream_[C]) write(src []byte) (int, error) { // for content i/o only
	// TODO
	return 0, nil
}
func (s *_http2Stream_[C]) writev(srcVec *net.Buffers) (int64, error) { // for content i/o only
	// TODO
	return 0, nil
}

//////////////////////////////////////// HTTP/2 incoming implementation ////////////////////////////////////////

// _http2In_
type _http2In_ struct {
}

func (r *_httpIn_) _growHeaders2(size int32) bool {
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

func (r *_httpIn_) readContent2() (data []byte, err error) {
	// TODO
	return
}

// http2InFrame is the HTTP/2 incoming frame.
type http2InFrame struct { // 32 bytes
	inBuffer   *http2InBuffer // the inBuffer that holds payload
	length     uint32         // length of payload. the real type is uint24
	streamID   uint32         // the real type is uint31
	kind       uint8          // see http2FrameXXX
	endHeaders bool           // is END_HEADERS flag set?
	endStream  bool           // is END_STREAM flag set?
	ack        bool           // is ACK flag set?
	padded     bool           // is PADDED flag set?
	priority   bool           // is PRIORITY flag set?
	_          [2]byte        // padding
	realFrom   uint32         // (effective) payload from
	realEdge   uint32         // (effective) payload edge
}

func (f *http2InFrame) zero() { *f = http2InFrame{} }

func (f *http2InFrame) decodeHeader(header []byte) error {
	f.length = uint32(header[0])<<16 | uint32(header[1])<<8 | uint32(header[2])
	if f.length > http2MaxFrameSize {
		return http2ErrorFrameSize
	}
	header[5] &= 0x7f // strip out the reserved bit
	f.streamID = binary.BigEndian.Uint32(header[5:9])
	if f.streamID != 0 && f.streamID&1 == 0 { // we don't support server push, so only odd stream ids are allowed
		return http2ErrorProtocol
	}
	f.kind = header[3]
	flags := header[4]
	f.endHeaders = flags&0x04 != 0 && (f.kind == http2FrameHeaders || f.kind == http2FrameContinuation)
	f.endStream = flags&0x01 != 0 && (f.kind == http2FrameData || f.kind == http2FrameHeaders)
	f.ack = flags&0x01 != 0 && (f.kind == http2FrameSettings || f.kind == http2FramePing)
	f.padded = flags&0x08 != 0 && (f.kind == http2FrameData || f.kind == http2FrameHeaders)
	f.priority = flags&0x20 != 0 && f.kind == http2FrameHeaders
	return nil
}

func (f *http2InFrame) isUnknown() bool   { return f.kind >= http2NumFrameKinds }
func (f *http2InFrame) effective() []byte { return f.inBuffer.buf[f.realFrom:f.realEdge] } // effective payload

var http2InFrameCheckers = [http2NumFrameKinds]func(*http2InFrame) error{
	(*http2InFrame).checkAsData,
	(*http2InFrame).checkAsHeaders,
	(*http2InFrame).checkAsPriority,
	(*http2InFrame).checkAsRSTStream,
	(*http2InFrame).checkAsSettings,
	nil, // pushPromise frames are rejected priorly
	(*http2InFrame).checkAsPing,
	(*http2InFrame).checkAsGoaway,
	(*http2InFrame).checkAsWindowUpdate,
	nil, // continuation frames are rejected priorly
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
		padLength = uint32(f.inBuffer.buf[f.realFrom])
		othersLen += 1
		f.realFrom += 1
	}
	if padLength > 0 { // drop padding
		if othersLen+padLength >= f.length {
			return http2ErrorProtocol
		}
		f.realEdge -= padLength
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
		padLength = uint32(f.inBuffer.buf[f.realFrom])
		othersLen += 1
		f.realFrom += 1
	}
	if f.priority { // skip stream dependency and weight
		othersLen += 5
		f.realFrom += 5
	}
	if padLength > 0 { // drop padding
		if othersLen+padLength >= f.length {
			return http2ErrorProtocol
		}
		f.realEdge -= padLength
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

// http2InBuffer
type http2InBuffer struct {
	buf [9 + http2MaxFrameSize]byte // header + payload
	ref atomic.Int32
}

var poolHTTP2InBuffer sync.Pool

func getHTTP2InBuffer() *http2InBuffer {
	var inBuffer *http2InBuffer
	if x := poolHTTP2InBuffer.Get(); x == nil {
		inBuffer = new(http2InBuffer)
	} else {
		inBuffer = x.(*http2InBuffer)
	}
	return inBuffer
}
func putHTTP2InBuffer(inBuffer *http2InBuffer) { poolHTTP2InBuffer.Put(inBuffer) }

func (b *http2InBuffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http2InBuffer) getRef() int32 { return b.ref.Load() }
func (b *http2InBuffer) incRef()       { b.ref.Add(1) }
func (b *http2InBuffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DebugLevel() >= 1 {
			Printf("putHTTP2InBuffer ref=%d\n", b.ref.Load())
		}
		putHTTP2InBuffer(b)
	}
}

//////////////////////////////////////// HTTP/2 outgoing implementation ////////////////////////////////////////

// _http2Out_
type _http2Out_ struct {
}

func (r *_httpOut_) addHeader2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_httpOut_) header2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_httpOut_) hasHeader2(name []byte) bool {
	// TODO
	return false
}
func (r *_httpOut_) delHeader2(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *_httpOut_) delHeaderAt2(i uint8) {
	// TODO
}

func (r *_httpOut_) sendChain2() error {
	// TODO
	return nil
}
func (r *_httpOut_) _sendEntireChain2() error {
	// TODO
	return nil
}
func (r *_httpOut_) _sendSingleRange2() error {
	// TODO
	return nil
}
func (r *_httpOut_) _sendMultiRanges2() error {
	// TODO
	return nil
}

func (r *_httpOut_) echoChain2() error {
	// TODO
	return nil
}

func (r *_httpOut_) addTrailer2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_httpOut_) trailer2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_httpOut_) trailers2() []byte {
	// TODO
	return nil
}

func (r *_httpOut_) proxyPassBytes2(data []byte) error { return r.writeBytes2(data) }

func (r *_httpOut_) finalizeVague2() error {
	// TODO
	if r.numTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *_httpOut_) writeHeaders2() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *_httpOut_) writePiece2(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *_httpOut_) _writeTextPiece2(piece *Piece) error {
	// TODO
	return nil
}
func (r *_httpOut_) _writeFilePiece2(piece *Piece) error {
	// TODO
	return nil
}
func (r *_httpOut_) writeVector2() error {
	// TODO
	return nil
}
func (r *_httpOut_) writeBytes2(data []byte) error {
	// TODO
	return nil
}

// http2OutFrame is the HTTP/2 outgoing frame.
type http2OutFrame struct { // 64 bytes
	length     uint32   // length of payload. the real type is uint24
	streamID   uint32   // the real type is uint31
	kind       uint8    // see http2FrameXXX. WARNING: http2FramePushPromise and http2FrameContinuation are NOT allowed!
	endHeaders bool     // is END_HEADERS flag set?
	endStream  bool     // is END_STREAM flag set?
	ack        bool     // is ACK flag set?
	padded     bool     // is PADDED flag set?
	priority   bool     // is PRIORITY flag set?
	_          bool     // padding
	header     [9]byte  // header of the frame is encoded here
	outBuffer  [16]byte // small payload of the frame is placed here temporarily
	payload    []byte   // refers to the payload
}

func (f *http2OutFrame) zero() { *f = http2OutFrame{} }

func (f *http2OutFrame) encodeHeader() (frameHeader []byte) { // caller must ensure the frame is legal.
	if f.length > http2MaxFrameSize {
		BugExitln("frame length too large")
	}
	if f.streamID > 0x7fffffff {
		BugExitln("stream id too large")
	}
	if f.kind == http2FramePushPromise || f.kind == http2FrameContinuation {
		BugExitln("push promise and continuation are not allowed as out frame")
	}
	frameHeader = f.header[:]
	frameHeader[0], frameHeader[1], frameHeader[2] = byte(f.length>>16), byte(f.length>>8), byte(f.length)
	frameHeader[3] = f.kind
	flags := uint8(0x00)
	if f.endHeaders && f.kind == http2FrameHeaders {
		flags |= 0x04
	}
	if f.endStream && (f.kind == http2FrameData || f.kind == http2FrameHeaders) {
		flags |= 0x01
	}
	if f.ack && (f.kind == http2FrameSettings || f.kind == http2FramePing) {
		flags |= 0x01
	}
	if f.padded && (f.kind == http2FrameData || f.kind == http2FrameHeaders) {
		flags |= 0x08
	}
	if f.priority && f.kind == http2FrameHeaders {
		flags |= 0x20
	}
	frameHeader[4] = flags
	binary.BigEndian.PutUint32(frameHeader[5:9], f.streamID)
	return
}

//////////////////////////////////////// HTTP/2 webSocket implementation ////////////////////////////////////////

// _http2Socket_
type _http2Socket_ struct {
}

func (s *_httpSocket_) todo2() {
}
