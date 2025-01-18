// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 general stuff. See RFC 9114 and RFC 9204.

// Server Push is not supported because it's rarely used. Chrome and Firefox even removed it.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
)

//////////////////////////////////////// HTTP/3 general implementation ////////////////////////////////////////

// http3Conn collects shared methods between *server3Conn and *backend3Conn.
type http3Conn interface {
	// Imports
	httpConn
	// Methods
}

// http3Conn_ is the parent for server3Conn and backend3Conn.
type http3Conn_ struct {
	// Parent
	httpConn__
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *tcp2.Conn        // the quic connection
	inBuffer *http3InBuffer    // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	activeStreams [http3MaxConcurrentStreams]http3Stream // active (open, remoteClosed, localClosed) streams
	_http3Conn0                                          // all values in this struct must be zero by default!
}
type _http3Conn0 struct { // for fast reset, entirely
	inBufferEdge uint32 // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	partBack     uint32 // incoming frame part (header or payload) begins from c.inBuffer.buf[c.partBack]
	partFore     uint32 // incoming frame part (header or payload) ends at c.inBuffer.buf[c.partFore]
}

func (c *http3Conn_) onGet(id int64, stage *Stage, udsMode bool, tlsMode bool, quicConn *tcp2.Conn, readTimeout time.Duration, writeTimeout time.Duration) {
	c.httpConn__.onGet(id, stage, udsMode, tlsMode, readTimeout, writeTimeout)

	c.quicConn = quicConn
	if c.inBuffer == nil {
		c.inBuffer = getHTTP3InBuffer()
		c.inBuffer.incRef()
	}
}
func (c *http3Conn_) onPut() {
	// c.inBuffer is reserved
	// c.table is reserved
	c.activeStreams = [http3MaxConcurrentStreams]http3Stream{}
	c.quicConn = nil

	c.httpConn__.onPut()
}

func (c *http3Conn_) remoteAddr() net.Addr { return nil } // TODO

// http3Stream collects shared methods between *server3Stream and *backend3Stream.
type http3Stream interface {
	// Imports
	httpStream
	// Methods
}

// http3Stream_ is the parent for server3Stream and backend3Stream.
type http3Stream_[C http3Conn] struct {
	// Parent
	httpStream__
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       C            // the http/3 connection
	quicStream *tcp2.Stream // the quic stream
	// Stream states (zeros)
	_http3Stream0 // all values in this struct must be zero by default!
}
type _http3Stream0 struct { // for fast reset, entirely
}

func (s *http3Stream_[C]) onUse(conn C, quicStream *tcp2.Stream) {
	s.httpStream__.onUse()

	s.conn = conn
	s.quicStream = quicStream
}
func (s *http3Stream_[C]) onEnd() {
	s._http3Stream0 = _http3Stream0{}

	// s.conn will be set as nil by upper code
	s.quicStream = nil
	s.httpStream__.onEnd()
}

func (s *http3Stream_[C]) Conn() httpConn       { return s.conn }
func (s *http3Stream_[C]) remoteAddr() net.Addr { return s.conn.remoteAddr() }

func (s *http3Stream_[C]) markBroken()    {}               // TODO
func (s *http3Stream_[C]) isBroken() bool { return false } // TODO

func (s *http3Stream_[C]) setReadDeadline() error {
	// TODO
	return nil
}
func (s *http3Stream_[C]) setWriteDeadline() error {
	// TODO
	return nil
}

func (s *http3Stream_[C]) read(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) readFull(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) write(src []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) writev(srcVec *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

//////////////////////////////////////// HTTP/3 incoming implementation ////////////////////////////////////////

func (r *httpIn__) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *httpIn__) readContent3() (data []byte, err error) {
	// TODO
	return
}

// http3InFrame is the HTTP/3 incoming frame.
type http3InFrame struct {
	// TODO
}

func (f *http3InFrame) zero() { *f = http3InFrame{} }

// http3InBuffer
type http3InBuffer struct {
	buf [_16K]byte // header + payload
	ref atomic.Int32
}

var poolHTTP3InBuffer sync.Pool

func getHTTP3InBuffer() *http3InBuffer {
	var inBuffer *http3InBuffer
	if x := poolHTTP3InBuffer.Get(); x == nil {
		inBuffer = new(http3InBuffer)
	} else {
		inBuffer = x.(*http3InBuffer)
	}
	return inBuffer
}
func putHTTP3InBuffer(inBuffer *http3InBuffer) { poolHTTP3InBuffer.Put(inBuffer) }

func (b *http3InBuffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http3InBuffer) getRef() int32 { return b.ref.Load() }
func (b *http3InBuffer) incRef()       { b.ref.Add(1) }
func (b *http3InBuffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DebugLevel() >= 1 {
			Printf("putHTTP3InBuffer ref=%d\n", b.ref.Load())
		}
		putHTTP3InBuffer(b)
	}
}

//////////////////////////////////////// HTTP/3 outgoing implementation ////////////////////////////////////////

func (r *httpOut__) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *httpOut__) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *httpOut__) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *httpOut__) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *httpOut__) delHeaderAt3(i uint8) {
	// TODO
}

func (r *httpOut__) sendChain3() error {
	// TODO
	return nil
}
func (r *httpOut__) _sendEntireChain3() error {
	// TODO
	return nil
}
func (r *httpOut__) _sendSingleRange3() error {
	// TODO
	return nil
}
func (r *httpOut__) _sendMultiRanges3() error {
	// TODO
	return nil
}

func (r *httpOut__) echoChain3() error {
	// TODO
	return nil
}

func (r *httpOut__) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *httpOut__) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *httpOut__) trailers3() []byte {
	// TODO
	return nil
}

func (r *httpOut__) proxyPassBytes3(data []byte) error { return r.writeBytes3(data) }

func (r *httpOut__) finalizeVague3() error {
	// TODO
	if r.numTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *httpOut__) writeHeaders3() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *httpOut__) writePiece3(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *httpOut__) _writeTextPiece3(piece *Piece) error {
	// TODO
	return nil
}
func (r *httpOut__) _writeFilePiece3(piece *Piece) error {
	// TODO
	return nil
}
func (r *httpOut__) writeVector3() error {
	// TODO
	return nil
}
func (r *httpOut__) writeBytes3(data []byte) error {
	// TODO
	return nil
}

// http3OutFrame is the HTTP/3 outgoing frame.
type http3OutFrame struct {
	// TODO
}

func (f *http3OutFrame) zero() { *f = http3OutFrame{} }

//////////////////////////////////////// HTTP/3 webSocket implementation ////////////////////////////////////////

func (s *httpSocket__) todo3() {
}

//////////////////////////////////////// HTTP/3 protocol elements ////////////////////////////////////////

const ( // HTTP/3 sizes and limits for both of our HTTP/3 server and HTTP/3 backend
	http3MaxTableSize         = _4K
	http3MaxConcurrentStreams = 127 // currently hardcoded
)

// http3StaticTable
var http3StaticTable = [99]pair{ // TODO
	/*
		0:  {1059, placeStatic3, 10, 0, span{0, 0}},
		1:  {487, placeStatic3, 5, 10, span{15, 16}},
		2:  {301, placeStatic3, 3, 16, span{19, 20}},
		3:  {2013, placeStatic3, 19, 20, span{0, 0}},
		4:  {1450, placeStatic3, 14, 39, span{53, 54}},
		5:  {634, placeStatic3, 6, 54, span{0, 0}},
		6:  {414, placeStatic3, 4, 60, span{0, 0}},
		7:  {417, placeStatic3, 4, 64, span{0, 0}},
		8:  {1660, placeStatic3, 17, 68, span{0, 0}},
		9:  {1254, placeStatic3, 13, 85, span{0, 0}},
		10: {1314, placeStatic3, 13, 98, span{0, 0}},
		11: {430, placeStatic3, 4, 111, span{0, 0}},
		12: {857, placeStatic3, 8, 115, span{0, 0}},
		13: {747, placeStatic3, 7, 123, span{0, 0}},
		14: {1011, placeStatic3, 10, 130, span{0, 0}},
		15: {699, placeStatic3, 7, 140, span{147, 154}},
		16: {699, placeStatic3, 7, 140, span{154, 160}},
		17: {699, placeStatic3, 7, 140, span{160, 163}},
		18: {699, placeStatic3, 7, 140, span{163, 167}},
		19: {699, placeStatic3, 7, 140, span{167, 174}},
		20: {699, placeStatic3, 7, 140, span{174, 178}},
		21: {699, placeStatic3, 7, 140, span{178, 181}},
		22: {687, placeStatic3, 7, 181, span{188, 192}},
		23: {687, placeStatic3, 7, 181, span{192, 197}},
		24: {734, placeStatic3, 7, 197, span{204, 207}},
		25: {734, placeStatic3, 7, 197, span{207, 210}},
		26: {734, placeStatic3, 7, 197, span{210, 213}},
		27: {734, placeStatic3, 7, 197, span{213, 216}},
		28: {734, placeStatic3, 7, 197, span{216, 219}},
		29: {624, placeStatic3, 6, 219, span{225, 228}},
		30: {624, placeStatic3, 6, 219, span{228, 251}},
		31: {1508, placeStatic3, 15, 251, span{266, 283}},
		32: {1309, placeStatic3, 13, 283, span{296, 301}},
		33: {2805, placeStatic3, 28, 301, span{329, 342}},
		34: {2805, placeStatic3, 28, 301, span{342, 354}},
		35: {2721, placeStatic3, 27, 354, span{381, 382}},
		36: {1314, placeStatic3, 13, 382, span{395, 404}},
		37: {1314, placeStatic3, 13, 382, span{404, 419}},
		38: {1314, placeStatic3, 13, 382, span{419, 433}},
		39: {1314, placeStatic3, 13, 382, span{433, 441}},
		40: {1314, placeStatic3, 13, 382, span{441, 449}},
		41: {1314, placeStatic3, 13, 382, span{449, 473}},
		42: {1647, placeStatic3, 16, 473, span{489, 491}},
		43: {1647, placeStatic3, 16, 473, span{491, 495}},
		44: {1258, placeStatic3, 12, 495, span{507, 530}},
		45: {1258, placeStatic3, 12, 495, span{530, 552}},
		46: {1258, placeStatic3, 12, 495, span{552, 568}},
		47: {1258, placeStatic3, 12, 495, span{568, 601}},
		48: {1258, placeStatic3, 12, 495, span{601, 610}},
		49: {1258, placeStatic3, 12, 495, span{610, 620}},
		50: {1258, placeStatic3, 12, 495, span{620, 629}},
		51: {1258, placeStatic3, 12, 495, span{629, 637}},
		52: {1258, placeStatic3, 12, 495, span{637, 661}},
		53: {1258, placeStatic3, 12, 495, span{661, 671}},
		54: {1258, placeStatic3, 12, 495, span{671, 695}},
		55: {525, placeStatic3, 5, 695, span{700, 708}},
		56: {2648, placeStatic3, 25, 708, span{733, 749}},
		57: {2648, placeStatic3, 25, 708, span{749, 784}},
		58: {2648, placeStatic3, 25, 708, span{784, 828}},
		59: {450, placeStatic3, 4, 828, span{832, 847}},
		60: {450, placeStatic3, 4, 828, span{847, 853}},
		61: {2248, placeStatic3, 22, 853, span{875, 882}},
		62: {1655, placeStatic3, 16, 882, span{898, 911}},
		63: {734, placeStatic3, 7, 911, span{918, 921}},
		64: {734, placeStatic3, 7, 911, span{921, 924}},
		65: {734, placeStatic3, 7, 911, span{924, 927}},
		66: {734, placeStatic3, 7, 911, span{927, 930}},
		67: {734, placeStatic3, 7, 911, span{930, 933}},
		68: {734, placeStatic3, 7, 911, span{933, 936}},
		69: {734, placeStatic3, 7, 911, span{936, 939}},
		70: {734, placeStatic3, 7, 911, span{939, 942}},
		71: {734, placeStatic3, 7, 911, span{942, 945}},
		72: {1505, placeStatic3, 15, 945, span{0, 0}},
		73: {3239, placeStatic3, 32, 960, span{992, 997}},
		74: {3239, placeStatic3, 32, 960, span{997, 1001}},
		75: {2805, placeStatic3, 28, 1001, span{1029, 1030}},
		76: {2829, placeStatic3, 28, 1030, span{1058, 1061}},
		77: {2829, placeStatic3, 28, 1030, span{1061, 1079}},
		78: {2829, placeStatic3, 28, 1030, span{1079, 1086}},
		79: {2922, placeStatic3, 29, 1086, span{1115, 1129}},
		80: {3039, placeStatic3, 30, 1129, span{1159, 1171}},
		81: {2948, placeStatic3, 29, 1171, span{1200, 1203}},
		82: {2948, placeStatic3, 29, 1171, span{1203, 1207}},
		83: {698, placeStatic3, 7, 1207, span{1214, 1219}},
		84: {1425, placeStatic3, 13, 1219, span{0, 0}},
		85: {2397, placeStatic3, 23, 1232, span{1255, 1308}},
		86: {996, placeStatic3, 10, 1308, span{1318, 1319}},
		87: {909, placeStatic3, 9, 1319, span{0, 0}},
		88: {958, placeStatic3, 9, 1328, span{0, 0}},
		89: {777, placeStatic3, 8, 1337, span{0, 0}},
		90: {648, placeStatic3, 6, 1345, span{0, 0}},
		91: {782, placeStatic3, 7, 1351, span{1358, 1366}},
		92: {663, placeStatic3, 6, 1366, span{0, 0}},
		93: {1929, placeStatic3, 19, 1372, span{1391, 1392}},
		94: {2588, placeStatic3, 25, 1392, span{1417, 1418}},
		95: {1019, placeStatic3, 10, 1418, span{0, 0}},
		96: {1495, placeStatic3, 15, 1428, span{0, 0}},
		97: {1513, placeStatic3, 15, 1443, span{1458, 1462}},
		98: {1513, placeStatic3, 15, 1443, span{1462, 1472}},
	*/
}

// http3TableEntry is a dynamic table entry.
type http3TableEntry struct { // 8 bytes
	nameFrom  uint16
	nameEdge  uint16 // nameEdge - nameFrom <= 255?
	valueEdge uint16
	totalSize uint16 // nameSize + valueSize + 32
}

// http3DynamicTable
type http3DynamicTable struct {
	entries [124]http3TableEntry
	content [_4K]byte
}

var http3Template = [11]byte{':', 's', 't', 'a', 't', 'u', 's', ' ', 'x', 'x', 'x'}
var http3Controls = [...][]byte{ // size: 512*24B=12K. keep sync with http1Control and http2Control!
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

var ( // HTTP/3 byteses
	http3BytesStatic = []byte(":authority:path/age0content-dispositioncontent-length0cookiedateetagif-modified-sinceif-none-matchlast-modifiedlinklocationrefererset-cookie:methodCONNECTDELETEGETHEADOPTIONSPOSTPUT:schemehttphttps:status103200304404503accept*/*application/dns-messageaccept-encodinggzip, deflate, braccept-rangesbytesaccess-control-allow-headerscache-controlcontent-typeaccess-control-allow-origin*cache-controlmax-age=0max-age=2592000max-age=604800no-cacheno-storepublic, max-age=31536000content-encodingbrgzipcontent-typeapplication/dns-messageapplication/javascriptapplication/jsonapplication/x-www-form-urlencodedimage/gifimage/jpegimage/pngtext/csstext/html; charset=utf-8text/plaintext/plain;charset=utf-8rangebytes=0-strict-transport-securitymax-age=31536000max-age=31536000; includesubdomainsmax-age=31536000; includesubdomains; preloadvaryaccept-encodingoriginx-content-type-optionsnosniffx-xss-protection1; mode=block:status100204206302400403421425500accept-languageaccess-control-allow-credentialsFALSETRUEaccess-control-allow-headers*access-control-allow-methodsgetget, post, optionsoptionsaccess-control-expose-headerscontent-lengthaccess-control-request-headerscontent-typeaccess-control-request-methodgetpostalt-svcclearauthorizationcontent-security-policyscript-src 'none'; object-src 'none'; base-uri 'none'early-data1expect-ctforwardedif-rangeoriginpurposeprefetchservertiming-allow-origin*upgrade-insecure-requests1user-agentx-forwarded-forx-frame-optionsdenysameorigin") // DO NOT CHANGE THIS UNLESS YOU KNOW WHAT YOU ARE DOING
)
