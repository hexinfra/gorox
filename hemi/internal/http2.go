// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 protocol elements, incoming message and outgoing message implementation.

package internal

import (
	"sync"
	"sync/atomic"
)

// http2In_ is used by http2Request and H2Response.
type http2In_ = webIn_

func (r *http2In_) _growHeadersH2(size int32) bool {
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

func (r *http2In_) readContentH2() (p []byte, err error) {
	// TODO
	return
}

// http2Out_ is used by http2Response and H2Request.
type http2Out_ = webOut_

func (r *http2Out_) addHeaderH2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *http2Out_) headerH2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *http2Out_) hasHeaderH2(name []byte) bool {
	// TODO
	return false
}
func (r *http2Out_) delHeaderH2(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *http2Out_) delHeaderAtH2(o uint8) {
	// TODO
}

func (r *http2Out_) sendChainH2() error {
	// TODO
	return nil
}

func (r *http2Out_) echoHeadersH2() error {
	// TODO
	return nil
}
func (r *http2Out_) echoChainH2() error {
	// TODO
	return nil
}

func (r *http2Out_) trailerH2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *http2Out_) addTrailerH2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *http2Out_) trailersH2() []byte {
	// TODO
	return nil
}

func (r *http2Out_) passBytesH2(p []byte) error {
	// TODO
	return nil
}

func (r *http2Out_) finalizeUnsizedH2() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *http2Out_) writeHeadersH2() error { // used by echo and post
	// TODO
	return nil
}
func (r *http2Out_) writePieceH2(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *http2Out_) writeVectorH2() error {
	return nil
}

// poolHTTP2Frames
var poolHTTP2Frames sync.Pool

func getHTTP2Frames() *http2Frames {
	var frames *http2Frames
	if x := poolHTTP2Frames.Get(); x == nil {
		frames = new(http2Frames)
	} else {
		frames = x.(*http2Frames)
	}
	return frames
}
func putHTTP2Frames(frames *http2Frames) { poolHTTP2Frames.Put(frames) }

// http2Frames
type http2Frames struct {
	buf [9 + http2FrameMaxSize]byte // header + payload
	ref atomic.Int32
}

func (p *http2Frames) size() uint32  { return uint32(cap(p.buf)) }
func (p *http2Frames) getRef() int32 { return p.ref.Load() }
func (p *http2Frames) incRef()       { p.ref.Add(1) }
func (p *http2Frames) decRef() {
	if p.ref.Add(-1) == 0 {
		if IsDebug(1) {
			Debugf("putHTTP2Frames ref=%d\n", p.ref.Load())
		}
		putHTTP2Frames(p)
	}
}

//////////////////////////////////////// HTTP/2 protocol elements.

const ( // HTTP/2 sizes and limits
	http2FrameMaxSize     = _16K // for both of our HTTP/2 client and HTTP/2 server
	http2MaxTableSize     = _4K  // for both of our HTTP/2 client and HTTP/2 server
	http2MaxActiveStreams = 127  // for both of our HTTP/2 client and HTTP/2 server
)
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
// We treat stream errors as connection errors to simplify implementation.
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

var ( // HTTP/2 byteses
	http2BytesPrism  = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	http2BytesStatic = []byte(":authority:methodGETPOST:path//index.html:schemehttphttps:status200204206304400404500accept-charsetaccept-encodinggzip, deflateaccept-languageaccept-rangesacceptaccess-control-allow-originageallowauthorizationcache-controlcontent-dispositioncontent-encodingcontent-languagecontent-lengthcontent-locationcontent-rangecontent-typecookiedateetagexpectexpiresfromhostif-matchif-modified-sinceif-none-matchif-rangeif-unmodified-sincelast-modifiedlinklocationmax-forwardsproxy-authenticateproxy-authorizationrangerefererrefreshretry-afterserverset-cookiestrict-transport-securitytransfer-encodinguser-agentvaryviawww-authenticate") // DO NOT CHANGE THIS
)

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
	frames     *http2Frames // the frames holding payload
	pFrom      uint32       // (effective) payload from
	pEdge      uint32       // (effective) payload edge
}

func (f *http2InFrame) zero() { *f = http2InFrame{} }

func (f *http2InFrame) decodeHeader(header []byte) error {
	f.length = uint32(header[0])<<16 | uint32(header[1])<<8 | uint32(header[2])
	if f.length > http2FrameMaxSize {
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
func (f *http2InFrame) effective() []byte { return f.frames.buf[f.pFrom:f.pEdge] } // effective payload

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
		padLength = uint32(f.frames.buf[f.pFrom])
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
		padLength = uint32(f.frames.buf[f.pFrom])
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

var ( // HTTP/2 byteses, TODO
)
