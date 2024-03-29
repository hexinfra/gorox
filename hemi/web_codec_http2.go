// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 incoming message and outgoing message implementation. See RFC 9113 and 7541.

package hemi

import (
	"sync"
	"sync/atomic"
)

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

// HTTP/2 websocket

func (s *webSocket_) example2() {
}

// HTTP/2 protocol

const ( // HTTP/2 sizes and limits for both of our HTTP/2 server and HTTP/2 backend
	http2MaxFrameSize     = _16K
	http2MaxTableSize     = _4K
	http2MaxActiveStreams = 127
)

// poolHTTP2Buffer
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

// http2Buffer
type http2Buffer struct {
	buf [9 + http2MaxFrameSize]byte // header + payload
	ref atomic.Int32
}

func (b *http2Buffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http2Buffer) getRef() int32 { return b.ref.Load() }
func (b *http2Buffer) incRef()       { b.ref.Add(1) }
func (b *http2Buffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DbgLevel() >= 1 {
			Printf("putHTTP2Buffer ref=%d\n", b.ref.Load())
		}
		putHTTP2Buffer(b)
	}
}

const ( // HTTP/2 frame kinds
	http2FrameData         = 0x0
	http2FrameHeaders      = 0x1
	http2FramePriority     = 0x2
	http2FrameRSTStream    = 0x3
	http2FrameSettings     = 0x4
	http2FramePushPromise  = 0x5
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
	http2FramePriority:     "PRIORITY",
	http2FrameRSTStream:    "RST_STREAM",
	http2FrameSettings:     "SETTINGS",
	http2FramePushPromise:  "PUSH_PROMISE",
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

func (f *http2InFrame) check() error {
	if f.isUnknown() {
		return nil
	}
	return http2InFrameCheckers[f.kind](f)
}
func (f *http2InFrame) checkAsHeaders() error {
	var minLength uint32 = 1 // Field Block Fragment
	if f.padded {
		minLength += 1 // Pad Length
	}
	if f.priority {
		minLength += 5 // Stream Dependency + Weight
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
func (f *http2InFrame) checkAsData() error {
	var minLength uint32 = 1 // Data
	if f.padded {
		minLength += 1 // Pad Length
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
func (f *http2InFrame) checkAsRSTStream() error {
	if f.length != 4 {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
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
func (f *http2InFrame) checkAsPriority() error {
	if f.length != 5 {
		return http2ErrorFrameSize
	}
	if f.streamID == 0 {
		return http2ErrorProtocol
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
