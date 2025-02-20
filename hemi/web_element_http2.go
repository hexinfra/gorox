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
	http2MaxTableSize         = _4K  // currently hardcoded
	http2MaxConcurrentStreams = 127  // currently hardcoded
)

const ( // HTTP/2 frame kinds
	http2FrameData         = 0x0
	http2FrameFields       = 0x1 // a.k.a. headers
	http2FramePriority     = 0x2 // deprecated. ignored on receiving, and we won't send
	http2FrameResetStream  = 0x3 // a.k.a. rst_stream
	http2FrameSettings     = 0x4
	http2FramePushPromise  = 0x5 // not supported
	http2FramePing         = 0x6
	http2FrameGoaway       = 0x7
	http2FrameWindowUpdate = 0x8
	http2FrameContinuation = 0x9
	http2NumFrameKinds     = 10
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

var http2InitialSettings = http2Settings{ // default settings for both backend2Conn and server2Conn
	headerTableSize:      _4K,   // the table size that we allow the remote peer to use
	enablePush:           false, // we don't support server push
	maxConcurrentStreams: 127,   // the number that we allow the remote peer to initiate
	initialWindowSize:    _64K1, // this requires the size of content buffer must up to 64K1
	maxFrameSize:         _16K,  // the size that we allow the remote peer to use
	maxHeaderListSize:    _16K,  // the size that we allow the remote peer to use
}

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

// http2Error denotes both connection error and stream error.
type http2Error uint32

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

func (e http2Error) Error() string {
	if e < http2NumErrorCodes {
		return http2CodeTexts[e]
	}
	return "UNKNOWN_ERROR"
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

func hpackDecodeVarint(fields []byte, N int, max uint32) (I uint32, j int, ok bool) { // ok = false if fields is malformed or result > max
	// Pseudocode to decode an integer I is as follows:
	//
	// decode I from the next N bits
	// if I < 2^N - 1, return I
	// else
	//     M = 0
	//     repeat
	//         B = next octet
	//         I = I + (B & 127) * 2^M
	//         M = M + 7
	//     while B & 128 == 128
	//     return I
	l := len(fields)
	if l == 0 {
		return 0, 0, false
	}
	K := uint32(1<<N) - 1
	I = uint32(fields[0]) & K
	if I < K {
		return I, 1, I <= max
	}
	j = 1
	for M := 0; j < l; M += 7 { // M -> 7,14,21,28
		B := fields[j]
		j++
		I += uint32(B&0x7F) << M // M = 0,7,14,21,28
		if I > max {
			break
		}
		if B < 0x80 {
			return I, j, true
		}
	}
	return I, j, false
}
func hpackDecodeString(input []byte, fields []byte, max uint32) (i int, j int, ok bool) { // ok = false if fields is malformed or length of result string > max
	I, j, ok := hpackDecodeVarint(fields, 7, max)
	if !ok {
		return 0, j, false
	}
	H := fields[0] >= 0x80
	fields = fields[j:]
	if I > uint32(len(fields)) {
		return 0, j, false
	}
	j += int(I)
	if H { // the string is huffman encoded, needs decoding
		i, ok := httpHuffmanDecode(input, fields[:I])
		return i, j, ok
	}
	copy(input, fields[:I])
	return j, j, true
}

func hpackEncodeVarint(fields []byte, I uint32, N int) (int, bool) { // ok = false if fields is not large enough
	// Pseudocode to encode an integer I is as follows:
	//
	// if I < 2^N - 1, encode I on N bits
	// else
	//     encode (2^N - 1) on N bits
	//     I = I - (2^N - 1)
	//     while I >= 128
	//          encode (I % 128 + 128) on 8 bits
	//          I = I / 128
	//     encode I on 8 bits
	l := len(fields)
	if l == 0 {
		return 0, false
	}
	K := uint32(1<<N) - 1
	if I < K {
		fields[0] = byte(I)
		return 1, true
	}
	fields[0] = byte(K)
	j := 1
	for I -= K; I >= 0x80 && j < l; I /= 0x80 {
		fields[j] = byte(I) | 0x80
		j++
	}
	if j == l {
		return j, false
	}
	fields[j] = byte(I)
	return j + 1, true
}
func hpackEncodeString(fields []byte, output []byte) (int, bool) { // ok = false if fields is not large enough
	I := len(output)
	if I == 0 {
		return 0, true
	}
	if I <= 8 { // the string is too short to use huffman encoding, so we use raw bytes
		j, ok := hpackEncodeVarint(fields, uint32(I), 7)
		if !ok {
			return j, false
		}
		fields = fields[j:]
		if len(fields) < I {
			return j, false
		}
		copy(fields, output)
		return j + I, true
	}
	// Use huffman encoding.
	if len(fields) < 2 {
		return 0, false
	}
	n := httpHuffmanEncode(fields[1:], output)
	if n < 127 { // this is the usual case
		fields[0] = byte(n) | 0x80
		return 1 + n, true
	}
	// n >= 127 means we have to use >= 2 bytes for the string length.
	h := make([]byte, 8) // enough, should not escape to heap
	j, _ := hpackEncodeVarint(h, uint32(I), 7)
	h[0] |= 0x80
	copy(fields[j:], fields[1:1+n]) // this is memmove
	copy(fields, h[:j])
	return j + n, true
}

// hpackTableEntry is an HPACK table entry.
type hpackTableEntry struct { // 8 bytes
	nameHash  uint16 // name hash
	nameFrom  uint16 // name edge at nameFrom+nameSize
	nameSize  uint8  // must <= 255
	isStatic  bool   // ...
	valueEdge uint16 // value: [nameFrom+nameSize:valueEdge]
}

var hpackStaticBytes = []byte(":authority:methodGET:methodPOST:path/:path/index.html:schemehttp:schemehttps:status200:status204:status206:status304:status400:status404:status500accept-charsetaccept-encodinggzip, deflateaccept-languageaccept-rangesacceptaccess-control-allow-originageallowauthorizationcache-controlcontent-dispositioncontent-encodingcontent-languagecontent-lengthcontent-locationcontent-rangecontent-typecookiedateetagexpectexpiresfromhostif-matchif-modified-sinceif-none-matchif-rangeif-unmodified-sincelast-modifiedlinklocationmax-forwardsproxy-authenticateproxy-authorizationrangerefererrefreshretry-afterserverset-cookiestrict-transport-securitytransfer-encodinguser-agentvaryviawww-authenticate")

// hpackStaticTable is the HPACK static table.
var hpackStaticTable = [...]hpackTableEntry{
	0:  {0, 0, 0, true, 0},         // empty, never used
	1:  {1059, 0, 10, true, 10},    // :authority=
	2:  {699, 10, 7, true, 20},     // :method=GET
	3:  {699, 20, 7, true, 31},     // :method=POST
	4:  {487, 31, 5, true, 37},     // :path=/
	5:  {487, 37, 5, true, 53},     // :path=/index.html
	6:  {687, 53, 7, true, 64},     // :scheme=http
	7:  {687, 64, 7, true, 76},     // :scheme=https
	8:  {734, 76, 7, true, 86},     // :status=200
	9:  {734, 86, 7, true, 96},     // :status=204
	10: {734, 96, 7, true, 106},    // :status=206
	11: {734, 106, 7, true, 116},   // :status=304
	12: {734, 116, 7, true, 126},   // :status=400
	13: {734, 126, 7, true, 136},   // :status=404
	14: {734, 136, 7, true, 146},   // :status=500
	15: {1415, 146, 14, true, 160}, // accept-charset=
	16: {1508, 160, 15, true, 188}, // accept-encoding=gzip, deflate
	17: {1505, 188, 15, true, 203}, // accept-language=
	18: {1309, 203, 13, true, 216}, // accept-ranges=
	19: {624, 216, 6, true, 222},   // accept=
	20: {2721, 222, 27, true, 249}, // access-control-allow-origin=
	21: {301, 249, 3, true, 252},   // age=
	22: {543, 252, 5, true, 257},   // allow=
	23: {1425, 257, 13, true, 270}, // authorization=
	24: {1314, 270, 13, true, 283}, // cache-control=
	25: {2013, 283, 19, true, 302}, // content-disposition=
	26: {1647, 302, 16, true, 318}, // content-encoding=
	27: {1644, 318, 16, true, 334}, // content-language=
	28: {1450, 334, 14, true, 348}, // content-length=
	29: {1665, 348, 16, true, 364}, // content-location=
	30: {1333, 364, 13, true, 377}, // content-range=
	31: {1258, 377, 12, true, 389}, // content-type=
	32: {634, 389, 6, true, 395},   // cookie=
	33: {414, 395, 4, true, 399},   // date=
	34: {417, 399, 4, true, 403},   // etag=
	35: {649, 403, 6, true, 409},   // expect=
	36: {768, 409, 7, true, 416},   // expires=
	37: {436, 416, 4, true, 420},   // from=
	38: {446, 420, 4, true, 424},   // host=
	39: {777, 424, 8, true, 432},   // if-match=
	40: {1660, 432, 17, true, 449}, // if-modified-since=
	41: {1254, 449, 13, true, 462}, // if-none-match=
	42: {777, 462, 8, true, 470},   // if-range=
	43: {1887, 470, 19, true, 489}, // if-unmodified-since=
	44: {1314, 489, 13, true, 502}, // last-modified=
	45: {430, 502, 4, true, 506},   // link=
	46: {857, 506, 8, true, 514},   // location=
	47: {1243, 514, 12, true, 526}, // max-forwards=
	48: {1902, 526, 18, true, 544}, // proxy-authenticate=
	49: {2048, 544, 19, true, 563}, // proxy-authorization=
	50: {525, 563, 5, true, 568},   // range=
	51: {747, 568, 7, true, 575},   // referer=
	52: {751, 575, 7, true, 582},   // refresh=
	53: {1141, 582, 11, true, 593}, // retry-after=
	54: {663, 593, 6, true, 599},   // server=
	55: {1011, 599, 10, true, 609}, // set-cookie=
	56: {2648, 609, 25, true, 634}, // strict-transport-security=
	57: {1753, 634, 17, true, 651}, // transfer-encoding=
	58: {1019, 651, 10, true, 661}, // user-agent=
	59: {450, 661, 4, true, 665},   // vary=
	60: {320, 665, 3, true, 668},   // via=
	61: {1681, 668, 16, true, 684}, // www-authenticate=
}

const hpackMaxTableIndex = 61 + 124 // static[1-61] + dynamic[62-185]

// hpackTable
type hpackTable struct { // <= 5KiB
	maxSize    uint32                       // <= http2MaxTableSize
	freeSize   uint32                       // <= maxSize
	maxEntries uint32                       // cap(entries)
	numEntries uint32                       // num of current entries. max num = floor(http2MaxTableSize/(1+32)) = 124, where "1" is the size of a shortest field
	iOldest    uint32                       // evict from t.entries[t.iOldest]
	iNewest    uint32                       // append to t.entries[t.iNewest]
	entries    [124]hpackTableEntry         // implemented as a circular buffer: https://en.wikipedia.org/wiki/Circular_buffer
	content    [http2MaxTableSize - 32]byte // the buffer. this size is the upper limit that remote manipulator can occupy
}

func (t *hpackTable) init() {
	t.maxSize = http2MaxTableSize
	t.freeSize = t.maxSize
	t.maxEntries = uint32(cap(t.entries))
	t.numEntries = 0
	t.iOldest = 0
	t.iNewest = 0
}

func (t *hpackTable) get(index uint32) (name []byte, value []byte, ok bool) {
	if index >= t.numEntries {
		return nil, nil, false
	}
	if t.iNewest <= t.iOldest && index > t.iNewest {
		index -= t.iNewest
		index = t.maxEntries - index
	} else {
		index = t.iNewest - index
	}
	entry := t.entries[index]
	nameEdge := entry.nameFrom + uint16(entry.nameSize)
	return t.content[entry.nameFrom:nameEdge], t.content[nameEdge:entry.valueEdge], true
}
func (t *hpackTable) add(name []byte, value []byte) bool { // name is not empty. sizes of name and value are limited
	if t.numEntries == t.maxEntries { // too many entries
		return false
	}
	nameSize, valueSize := uint32(len(name)), uint32(len(value))
	wantSize := nameSize + valueSize + 32 // won't overflow
	// Before a new entry is added to the dynamic table, entries are evicted
	// from the end of the dynamic table until the size of the dynamic table
	// is less than or equal to (maximum size - new entry size) or until the
	// table is empty.
	//
	// If the size of the new entry is less than or equal to the maximum
	// size, that entry is added to the table.  It is not an error to
	// attempt to add an entry that is larger than the maximum size; an
	// attempt to add an entry larger than the maximum size causes the table
	// to be emptied of all existing entries and results in an empty table.
	if wantSize > t.maxSize {
		t.freeSize = t.maxSize
		t.numEntries = 0
		t.iOldest = t.iNewest
		return true
	}
	for t.freeSize < wantSize {
		t._evictOne()
	}
	t.freeSize -= wantSize
	var entry hpackTableEntry
	if t.numEntries > 0 {
		entry.nameFrom = t.entries[t.iNewest].valueEdge
		if t.iNewest++; t.iNewest == t.maxEntries {
			t.iNewest = 0 // wrap around
		}
	} else { // empty table. starts from 0
		entry.nameFrom = 0
	}
	entry.nameSize = uint8(nameSize)
	nameEdge := entry.nameFrom + uint16(entry.nameSize)
	entry.valueEdge = nameEdge + uint16(valueSize)
	copy(t.content[entry.nameFrom:nameEdge], name)
	if valueSize > 0 {
		copy(t.content[nameEdge:entry.valueEdge], value)
	}
	t.numEntries++
	t.entries[t.iNewest] = entry
	return true
}
func (t *hpackTable) resize(newMaxSize uint32) { // newMaxSize must <= http2MaxTableSize
	if newMaxSize > http2MaxTableSize {
		BugExitln("newMaxSize out of range")
	}
	if newMaxSize >= t.maxSize {
		t.freeSize += newMaxSize - t.maxSize
	} else {
		for usedSize := t.maxSize - t.freeSize; usedSize > newMaxSize; usedSize = t.maxSize - t.freeSize {
			t._evictOne()
		}
		t.freeSize -= t.maxSize - newMaxSize
	}
	t.maxSize = newMaxSize
}
func (t *hpackTable) _evictOne() {
	if t.numEntries == 0 {
		BugExitln("no entries to evict!")
	}
	evictee := &t.entries[t.iOldest]
	t.freeSize += uint32(evictee.valueEdge - evictee.nameFrom + 32)
	if t.iOldest++; t.iOldest == t.maxEntries {
		t.iOldest = 0
	}
	if t.numEntries--; t.numEntries == 0 {
		t.iNewest = t.iOldest
	}
}

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

var http2InFrameCheckers = [http2NumFrameKinds]func(*http2InFrame) error{ // for known frames
	(*http2InFrame).checkAsData,
	(*http2InFrame).checkAsFields,
	(*http2InFrame).checkAsPriority,
	(*http2InFrame).checkAsResetStream,
	(*http2InFrame).checkAsSettings,
	(*http2InFrame).checkAsPushPromise,
	(*http2InFrame).checkAsPing,
	(*http2InFrame).checkAsGoaway,
	(*http2InFrame).checkAsWindowUpdate,
	(*http2InFrame).checkAsContinuation,
}

func (f *http2InFrame) checkAsData() error {
	var minLength uint16 = 0 // Data (..)
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
func (f *http2InFrame) checkAsPushPromise() error {
	return http2ErrorProtocol // we don't support server push
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
func (f *http2InFrame) checkAsContinuation() error {
	return http2ErrorProtocol // continuation frames cannot be alone. we coalesce continuation frames on receiving fields frame
}

// http2OutFrame is the HTTP/2 outgoing frame.
type http2OutFrame[S http2Stream] struct { // 64 bytes
	streamID  uint32   // id of stream, duplicate for convenience
	length    uint16   // length of payload. the real type is uint24, but we never use sizes out of range of uint16, so use uint16
	kind      uint8    // see http2FrameXXX. WARNING: http2FramePushPromise and http2FrameContinuation are NOT allowed! we don't use them.
	endFields bool     // is END_FIELDS flag set?
	endStream bool     // is END_STREAM flag set?
	ack       bool     // is ACK flag set?
	padded    bool     // is PADDED flag set?
	header    [9]byte  // frame header is encoded here
	outBuffer [12]byte // small payload of the frame is placed here temporarily
	payload   []byte   // refers to the payload
	stream    S        // the http/2 stream to which the frame belongs. nil if the frame belongs to conneciton
}

func (f *http2OutFrame[S]) zero() { *f = http2OutFrame[S]{} }

func (f *http2OutFrame[S]) encodeHeader() (outHeader []byte) { // caller must ensure the frame is legal.
	if f.streamID > 0x7fffffff {
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
	binary.BigEndian.PutUint32(outHeader[5:9], f.streamID)
	return
}

var http2FreeSeats = [http2MaxConcurrentStreams]uint8{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
	17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
	33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48,
	49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64,
	65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80,
	81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96,
	97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112,
	113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126,
}
