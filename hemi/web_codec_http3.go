// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 incoming message and outgoing message implementation. See RFC 9114 and 9204.

package hemi

import (
	"sync"
	"sync/atomic"
)

// HTTP/3 incoming

func (r *webIn_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *webIn_) readContent3() (p []byte, err error) {
	// TODO
	return
}

// HTTP/3 outgoing

func (r *webOut_) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *webOut_) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *webOut_) delHeaderAt3(i uint8) {
	// TODO
}

func (r *webOut_) sendChain3() error {
	// TODO
	return nil
}

func (r *webOut_) echoChain3() error {
	// TODO
	return nil
}

func (r *webOut_) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) trailers3() []byte {
	// TODO
	return nil
}

func (r *webOut_) passBytes3(p []byte) error { return r.writeBytes3(p) }

func (r *webOut_) finalizeVague3() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *webOut_) writeHeaders3() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *webOut_) writePiece3(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *webOut_) writeVector3() error {
	return nil
}
func (r *webOut_) writeBytes3(p []byte) error {
	// TODO
	return nil
}

// HTTP/3 websocket

func (s *webSocket_) example3() {
}

// HTTP/3 protocol

const ( // HTTP/2 sizes and limits for both of our HTTP/3 server and HTTP/3 backend
	http3MaxActiveStreams = 127
	http3MaxTableSize     = _4K
)

// poolHTTP3Buffer
var poolHTTP3Buffer sync.Pool

func getHTTP3Buffer() *http3Buffer {
	var buffer *http3Buffer
	if x := poolHTTP3Buffer.Get(); x == nil {
		buffer = new(http3Buffer)
	} else {
		buffer = x.(*http3Buffer)
	}
	return buffer
}
func putHTTP3Buffer(buffer *http3Buffer) { poolHTTP3Buffer.Put(buffer) }

// http3Buffer
type http3Buffer struct {
	buf [_16K]byte // header + payload
	ref atomic.Int32
}

func (b *http3Buffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http3Buffer) getRef() int32 { return b.ref.Load() }
func (b *http3Buffer) incRef()       { b.ref.Add(1) }
func (b *http3Buffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DbgLevel() >= 1 {
			Printf("putHTTP3Buffer ref=%d\n", b.ref.Load())
		}
		putHTTP3Buffer(b)
	}
}

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

// http3InFrame is the server-side HTTP/3 incoming frame.
type http3InFrame struct {
	// TODO
}

func (f *http3InFrame) zero() { *f = http3InFrame{} }

// http3OutFrame is the server-side HTTP/3 outgoing frame.
type http3OutFrame struct {
	// TODO
}

func (f *http3OutFrame) zero() { *f = http3OutFrame{} }

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
