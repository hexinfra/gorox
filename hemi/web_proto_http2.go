// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 protocol elements. See RFC 9113 and RFC 7541.

// Server Push is not supported because it's rarely used. Chrome and Firefox even removed it.

package hemi

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
)

const ( // HTTP/2 sizes and limits for both of our HTTP/2 server and HTTP/2 backend
	http2MaxFrameSize         = _16K // currently hardcoded. must <= _64K1 - 9
	http2MaxTableSize         = _4K
	http2MaxConcurrentStreams = 127 // currently hardcoded
)

const ( // HTTP/2 frame kinds
	http2FrameData         = 0x0
	http2FrameFields       = 0x1 // a.k.a. headers
	http2FramePriority     = 0x2 // deprecated. ignored on receiving, and we won't send
	http2FrameResetStream  = 0x3
	http2FrameSettings     = 0x4
	http2FramePushPromise  = 0x5 // not supported
	http2FramePing         = 0x6
	http2FrameGoaway       = 0x7
	http2FrameWindowUpdate = 0x8
	http2FrameContinuation = 0x9
	http2NumFrameKinds     = 10
)
const ( // HTTP/2 error codes
	http2CodeNoError            = 0x0 // The associated condition is not a result of an error. For example, a GOAWAY might include this code to indicate graceful shutdown of a connection.
	http2CodeProtocol           = 0x1 // The endpoint detected an unspecific protocol error. This error is for use when a more specific error code is not available.
	http2CodeInternal           = 0x2 // The endpoint encountered an unexpected internal error.
	http2CodeFlowControl        = 0x3 // The endpoint detected that its peer violated the flow-control protocol.
	http2CodeSettingsTimeout    = 0x4 // The endpoint sent a SETTINGS frame but did not receive a response in a timely manner. See Section 6.5.3 ("Settings Synchronization").
	http2CodeStreamClosed       = 0x5 // The endpoint received a frame after a stream was half-closed.
	http2CodeFrameSize          = 0x6 // The endpoint received a frame with an invalid size.
	http2CodeRefusedStream      = 0x7 // The endpoint refused the stream prior to performing any application processing (see Section 8.7 for details).
	http2CodeCancel             = 0x8 // The endpoint uses this error code to indicate that the stream is no longer needed.
	http2CodeCompression        = 0x9 // The endpoint is unable to maintain the field section compression context for the connection.
	http2CodeConnect            = 0xa // The connection established in response to a CONNECT request (Section 8.5) was reset or abnormally closed.
	http2CodeEnhanceYourCalm    = 0xb // The endpoint detected that its peer is exhibiting a behavior that might be generating excessive load.
	http2CodeInadequateSecurity = 0xc // The underlying transport has properties that do not meet minimum security requirements (see Section 9.2).
	http2CodeHTTP11Required     = 0xd // The endpoint requires that HTTP/1.1 be used instead of HTTP/2.
	http2NumErrorCodes          = 14  // Unknown or unsupported error codes MUST NOT trigger any special behavior. These MAY be treated by an implementation as being equivalent to INTERNAL_ERROR.
)
const ( // HTTP/2 stream states
	http2StateIdle         = 0 // must be 0, default value
	http2StateOpen         = 1
	http2StateRemoteClosed = 2
	http2StateLocalClosed  = 3
	http2StateClosed       = 4
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
	enablePush           bool   // 0x2, always false as we don't support server push
	maxConcurrentStreams uint32 // 0x3
	initialWindowSize    int32  // 0x4
	maxFrameSize         uint32 // 0x5
	maxHeaderListSize    uint32 // 0x6
}

var http2InitialSettings = http2Settings{ // default settings for both server and backend
	headerTableSize:      _4K,
	enablePush:           false, // we don't support server push
	maxConcurrentStreams: 127,
	initialWindowSize:    _64K1, // this requires the size of content buffer must up to 64K1
	maxFrameSize:         _16K,
	maxHeaderListSize:    _16K,
}
var http2FrameNames = [http2NumFrameKinds]string{
	http2FrameData:         "DATA",
	http2FrameFields:       "FIELDS",       // a.k.a. headers
	http2FramePriority:     "PRIORITY",     // deprecated. ignored on receiving, and we won't send
	http2FrameResetStream:  "RESET_STREAM", // a.k.a. rst_stream
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
	if e < http2NumErrorCodes {
		return http2CodeTexts[e]
	}
	return "UNKNOWN_ERROR"
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

var ( // HTTP/2 byteses
	http2BytesPrism  = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	http2BytesStatic = []byte(":authority:methodGETPOST:path//index.html:schemehttphttps:status200204206304400404500accept-charsetaccept-encodinggzip, deflateaccept-languageaccept-rangesacceptaccess-control-allow-originageallowauthorizationcache-controlcontent-dispositioncontent-encodingcontent-languagecontent-lengthcontent-locationcontent-rangecontent-typecookiedateetagexpectexpiresfromhostif-matchif-modified-sinceif-none-matchif-rangeif-unmodified-sincelast-modifiedlinklocationmax-forwardsproxy-authenticateproxy-authorizationrangerefererrefreshretry-afterserverset-cookiestrict-transport-securitytransfer-encodinguser-agentvaryviawww-authenticate") // DO NOT CHANGE THIS UNLESS YOU KNOW WHAT YOU ARE DOING
)

// http2Buffer is the HTTP/2 incoming buffer.
type http2Buffer struct {
	buf [9 + http2MaxFrameSize]byte // frame header + frame payload
	ref atomic.Int32
}

var poolHTTP2Buffer sync.Pool

func getHTTP2Buffer() *http2Buffer {
	var inBuffer *http2Buffer
	if x := poolHTTP2Buffer.Get(); x == nil {
		inBuffer = new(http2Buffer)
	} else {
		inBuffer = x.(*http2Buffer)
	}
	return inBuffer
}
func putHTTP2Buffer(inBuffer *http2Buffer) { poolHTTP2Buffer.Put(inBuffer) }

func (b *http2Buffer) size() uint16  { return uint16(cap(b.buf)) }
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

// http2InFrame is the HTTP/2 incoming frame.
type http2InFrame struct { // 24 bytes
	inBuffer  *http2Buffer // the inBuffer that holds payload
	streamID  uint32       // the real type is uint31
	length    uint16       // length of payload. the real type is uint24, but we never allow sizes out of range of uint16, so use uint16
	kind      uint8        // see http2FrameXXX
	endFields bool         // is END_FIELDS flag set?
	endStream bool         // is END_STREAM flag set?
	ack       bool         // is ACK flag set?
	padded    bool         // is PADDED flag set?
	priority  bool         // is PRIORITY flag set?
	efctFrom  uint16       // (effective) payload from
	efctEdge  uint16       // (effective) payload edge
}

func (f *http2InFrame) zero() { *f = http2InFrame{} }

func (f *http2InFrame) decodeHeader(inHeader []byte) error {
	inHeader[5] &= 0x7f // ignore the reserved bit
	f.streamID = binary.BigEndian.Uint32(inHeader[5:9])
	if f.streamID != 0 && f.streamID&1 == 0 { // we don't support server push, so only odd stream ids are allowed
		return http2ErrorProtocol
	}
	length := uint32(inHeader[0])<<16 | uint32(inHeader[1])<<8 | uint32(inHeader[2])
	if length > http2MaxFrameSize {
		// An endpoint MUST send an error code of FRAME_SIZE_ERROR if a frame exceeds the size defined in SETTINGS_MAX_FRAME_SIZE,
		// exceeds any limit defined for the frame type, or is too small to contain mandatory frame data.
		return http2ErrorFrameSize
	}
	f.length = uint16(length)
	f.kind = inHeader[3]
	flags := inHeader[4]
	f.endFields = flags&0x04 != 0 && (f.kind == http2FrameFields || f.kind == http2FrameContinuation)
	f.endStream = flags&0x01 != 0 && (f.kind == http2FrameData || f.kind == http2FrameFields)
	f.ack = flags&0x01 != 0 && (f.kind == http2FrameSettings || f.kind == http2FramePing)
	f.padded = flags&0x08 != 0 && (f.kind == http2FrameData || f.kind == http2FrameFields)
	f.priority = flags&0x20 != 0 && f.kind == http2FrameFields
	return nil
}

func (f *http2InFrame) isUnknown() bool   { return f.kind >= http2NumFrameKinds }
func (f *http2InFrame) effective() []byte { return f.inBuffer.buf[f.efctFrom:f.efctEdge] } // effective payload

var http2InFrameCheckers = [http2NumFrameKinds]func(*http2InFrame) error{
	(*http2InFrame).checkAsData,
	(*http2InFrame).checkAsFields,
	(*http2InFrame).checkAsPriority,
	(*http2InFrame).checkAsResetStream,
	(*http2InFrame).checkAsSettings,
	nil, // pushPromise frames are rejected priorly
	(*http2InFrame).checkAsPing,
	(*http2InFrame).checkAsGoaway,
	(*http2InFrame).checkAsWindowUpdate,
	nil, // continuation frames are rejected priorly
}

func (f *http2InFrame) checkAsData() error {
	var minLength uint16 = 1 // Data (..)
	if f.padded {
		minLength += 1 // Pad Length (8)
	}
	if f.length < minLength {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
		return http2ErrorProtocol
	}
	var padLength, othersLen uint16 = 0, 0
	if f.padded {
		padLength = uint16(f.inBuffer.buf[f.efctFrom])
		othersLen += 1
		f.efctFrom += 1
	}
	if padLength > 0 { // drop padding
		if othersLen+padLength >= f.length {
			return http2ErrorProtocol
		}
		f.efctEdge -= padLength
	}
	return nil
}
func (f *http2InFrame) checkAsFields() error {
	var minLength uint16 = 1 // Field Block Fragment
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
	var padLength, othersLen uint16 = 0, 0
	if f.padded { // skip pad length byte
		padLength = uint16(f.inBuffer.buf[f.efctFrom])
		othersLen += 1
		f.efctFrom += 1
	}
	if f.priority { // skip stream dependency and weight
		othersLen += 5
		f.efctFrom += 5
	}
	if padLength > 0 { // drop padding
		if othersLen+padLength >= f.length {
			return http2ErrorProtocol
		}
		f.efctEdge -= padLength
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
func (f *http2InFrame) checkAsResetStream() error {
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

// http2OutFrame is the HTTP/2 outgoing frame.
type http2OutFrame struct { // 64 bytes
	stream    http2Stream // the http/2 stream to which the frame belongs
	length    uint16      // length of payload. the real type is uint24, but we never use sizes out of range of uint16, so use uint16
	kind      uint8       // see http2FrameXXX. WARNING: http2FramePushPromise and http2FrameContinuation are NOT allowed! we don't use them.
	endFields bool        // is END_FIELDS flag set?
	endStream bool        // is END_STREAM flag set?
	ack       bool        // is ACK flag set?
	padded    bool        // is PADDED flag set?
	header    [9]byte     // frame header is encoded here
	outBuffer [8]byte     // small payload of the frame is placed here temporarily
	payload   []byte      // refers to the payload
}

func (f *http2OutFrame) zero() { *f = http2OutFrame{} }

func (f *http2OutFrame) encodeHeader() (outHeader []byte) { // caller must ensure the frame is legal.
	if f.stream.nativeID() > 0x7fffffff {
		BugExitln("stream id too large")
	}
	if f.length > http2MaxFrameSize {
		BugExitln("frame length too large")
	}
	if f.kind == http2FramePushPromise || f.kind == http2FrameContinuation {
		BugExitln("push promise and continuation are not allowed as out frame")
	}
	outHeader = f.header[:]
	outHeader[0], outHeader[1], outHeader[2] = byte(f.length>>16), byte(f.length>>8), byte(f.length)
	outHeader[3] = f.kind
	flags := uint8(0x00)
	if f.endFields && f.kind == http2FrameFields { // we never use http2FrameContinuation
		flags |= 0x04
	}
	if f.endStream && (f.kind == http2FrameData || f.kind == http2FrameFields) {
		flags |= 0x01
	}
	if f.ack && (f.kind == http2FrameSettings || f.kind == http2FramePing) {
		flags |= 0x01
	}
	if f.padded && (f.kind == http2FrameData || f.kind == http2FrameFields) {
		flags |= 0x08
	}
	outHeader[4] = flags
	binary.BigEndian.PutUint32(outHeader[5:9], f.stream.nativeID())
	return
}
