// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 protocol elements, incoming message and outgoing message implementation.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
)

// http3InMessage_ is used by http3Request and H3Response.

func (r *httpInMessage_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *httpInMessage_) readContent3() (p []byte, err error) {
	// TODO
	return
}

// http3OutMessage_ is used by http3Response and H3Request.

func (r *httpOutMessage_) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *httpOutMessage_) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *httpOutMessage_) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *httpOutMessage_) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *httpOutMessage_) delHeaderAt3(o uint8) {
	// TODO
}

func (r *httpOutMessage_) sendChain3(chain Chain, vector [][]byte) error {
	// TODO
	return nil
}

func (r *httpOutMessage_) pushHeaders3() error {
	// TODO
	return nil
}
func (r *httpOutMessage_) pushChain3(chain Chain) error {
	// TODO
	return nil
}

func (r *httpOutMessage_) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *httpOutMessage_) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *httpOutMessage_) trailers3() []byte {
	return nil
}

func (r *httpOutMessage_) passHeaders3() error {
	return nil
}
func (r *httpOutMessage_) passBytes3(p []byte) error {
	return nil
}

func (r *httpOutMessage_) finalizeChunked3() error {
	// TODO
	return nil
}

func (r *httpOutMessage_) writeBlock3(block *Block, chunked bool) error {
	// TODO
	return nil
}
func (r *httpOutMessage_) writeVector3(vector *net.Buffers) error {
	return nil
}

// poolHTTP3Inputs
var poolHTTP3Inputs sync.Pool

func getHTTP3Inputs() *http3Inputs {
	var inputs *http3Inputs
	if x := poolHTTP3Inputs.Get(); x == nil {
		inputs = new(http3Inputs)
	} else {
		inputs = x.(*http3Inputs)
	}
	return inputs
}
func putHTTP3Inputs(inputs *http3Inputs) { poolHTTP3Inputs.Put(inputs) }

// http3Inputs
type http3Inputs struct {
	buf [_16K]byte // header + payload
	ref atomic.Int32
}

func (a *http3Inputs) size() uint32  { return uint32(cap(a.buf)) }
func (a *http3Inputs) getRef() int32 { return a.ref.Load() }
func (a *http3Inputs) incRef()       { a.ref.Add(1) }
func (a *http3Inputs) decRef() {
	if a.ref.Add(-1) == 0 {
		if IsDebug(1) {
			Debugf("putHTTP3Inputs ref=%d\n", a.ref.Load())
		}
		putHTTP3Inputs(a)
	}
}

// HTTP/3 protocol elements.

const ( // HTTP/3 sizes and limits
	http3MaxActiveStreams = 127
	http3MaxTableSize     = _4K
)

// http3StaticTable
var http3StaticTable = [...]pair{ // 99*16B=1584B. DO NOT CHANGE THIS
	0:  {1059, pairPlaceStatic3 | pairFlagPseudo, 10, 0, text{0, 0}},
	1:  {487, pairPlaceStatic3 | pairFlagPseudo, 5, 10, text{15, 16}},
	2:  {301, pairPlaceStatic3, 3, 16, text{19, 20}},
	3:  {2013, pairPlaceStatic3, 19, 20, text{0, 0}},
	4:  {1450, pairPlaceStatic3, 14, 39, text{53, 54}},
	5:  {634, pairPlaceStatic3, 6, 54, text{0, 0}},
	6:  {414, pairPlaceStatic3, 4, 60, text{0, 0}},
	7:  {417, pairPlaceStatic3, 4, 64, text{0, 0}},
	8:  {1660, pairPlaceStatic3, 17, 68, text{0, 0}},
	9:  {1254, pairPlaceStatic3, 13, 85, text{0, 0}},
	10: {1314, pairPlaceStatic3, 13, 98, text{0, 0}},
	11: {430, pairPlaceStatic3, 4, 111, text{0, 0}},
	12: {857, pairPlaceStatic3, 8, 115, text{0, 0}},
	13: {747, pairPlaceStatic3, 7, 123, text{0, 0}},
	14: {1011, pairPlaceStatic3, 10, 130, text{0, 0}},
	15: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{147, 154}},
	16: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{154, 160}},
	17: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{160, 163}},
	18: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{163, 167}},
	19: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{167, 174}},
	20: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{174, 178}},
	21: {699, pairPlaceStatic3 | pairFlagPseudo, 7, 140, text{178, 181}},
	22: {687, pairPlaceStatic3 | pairFlagPseudo, 7, 181, text{188, 192}},
	23: {687, pairPlaceStatic3 | pairFlagPseudo, 7, 181, text{192, 197}},
	24: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 197, text{204, 207}},
	25: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 197, text{207, 210}},
	26: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 197, text{210, 213}},
	27: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 197, text{213, 216}},
	28: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 197, text{216, 219}},
	29: {624, pairPlaceStatic3, 6, 219, text{225, 228}},
	30: {624, pairPlaceStatic3, 6, 219, text{228, 251}},
	31: {1508, pairPlaceStatic3, 15, 251, text{266, 283}},
	32: {1309, pairPlaceStatic3, 13, 283, text{296, 301}},
	33: {2805, pairPlaceStatic3, 28, 301, text{329, 342}},
	34: {2805, pairPlaceStatic3, 28, 301, text{342, 354}},
	35: {2721, pairPlaceStatic3, 27, 354, text{381, 382}},
	36: {1314, pairPlaceStatic3, 13, 382, text{395, 404}},
	37: {1314, pairPlaceStatic3, 13, 382, text{404, 419}},
	38: {1314, pairPlaceStatic3, 13, 382, text{419, 433}},
	39: {1314, pairPlaceStatic3, 13, 382, text{433, 441}},
	40: {1314, pairPlaceStatic3, 13, 382, text{441, 449}},
	41: {1314, pairPlaceStatic3, 13, 382, text{449, 473}},
	42: {1647, pairPlaceStatic3, 16, 473, text{489, 491}},
	43: {1647, pairPlaceStatic3, 16, 473, text{491, 495}},
	44: {1258, pairPlaceStatic3, 12, 495, text{507, 530}},
	45: {1258, pairPlaceStatic3, 12, 495, text{530, 552}},
	46: {1258, pairPlaceStatic3, 12, 495, text{552, 568}},
	47: {1258, pairPlaceStatic3, 12, 495, text{568, 601}},
	48: {1258, pairPlaceStatic3, 12, 495, text{601, 610}},
	49: {1258, pairPlaceStatic3, 12, 495, text{610, 620}},
	50: {1258, pairPlaceStatic3, 12, 495, text{620, 629}},
	51: {1258, pairPlaceStatic3, 12, 495, text{629, 637}},
	52: {1258, pairPlaceStatic3, 12, 495, text{637, 661}},
	53: {1258, pairPlaceStatic3, 12, 495, text{661, 671}},
	54: {1258, pairPlaceStatic3, 12, 495, text{671, 695}},
	55: {525, pairPlaceStatic3, 5, 695, text{700, 708}},
	56: {2648, pairPlaceStatic3, 25, 708, text{733, 749}},
	57: {2648, pairPlaceStatic3, 25, 708, text{749, 784}},
	58: {2648, pairPlaceStatic3, 25, 708, text{784, 828}},
	59: {450, pairPlaceStatic3, 4, 828, text{832, 847}},
	60: {450, pairPlaceStatic3, 4, 828, text{847, 853}},
	61: {2248, pairPlaceStatic3, 22, 853, text{875, 882}},
	62: {1655, pairPlaceStatic3, 16, 882, text{898, 911}},
	63: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{918, 921}},
	64: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{921, 924}},
	65: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{924, 927}},
	66: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{927, 930}},
	67: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{930, 933}},
	68: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{933, 936}},
	69: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{936, 939}},
	70: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{939, 942}},
	71: {734, pairPlaceStatic3 | pairFlagPseudo, 7, 911, text{942, 945}},
	72: {1505, pairPlaceStatic3, 15, 945, text{0, 0}},
	73: {3239, pairPlaceStatic3, 32, 960, text{992, 997}},
	74: {3239, pairPlaceStatic3, 32, 960, text{997, 1001}},
	75: {2805, pairPlaceStatic3, 28, 1001, text{1029, 1030}},
	76: {2829, pairPlaceStatic3, 28, 1030, text{1058, 1061}},
	77: {2829, pairPlaceStatic3, 28, 1030, text{1061, 1079}},
	78: {2829, pairPlaceStatic3, 28, 1030, text{1079, 1086}},
	79: {2922, pairPlaceStatic3, 29, 1086, text{1115, 1129}},
	80: {3039, pairPlaceStatic3, 30, 1129, text{1159, 1171}},
	81: {2948, pairPlaceStatic3, 29, 1171, text{1200, 1203}},
	82: {2948, pairPlaceStatic3, 29, 1171, text{1203, 1207}},
	83: {698, pairPlaceStatic3, 7, 1207, text{1214, 1219}},
	84: {1425, pairPlaceStatic3, 13, 1219, text{0, 0}},
	85: {2397, pairPlaceStatic3, 23, 1232, text{1255, 1308}},
	86: {996, pairPlaceStatic3, 10, 1308, text{1318, 1319}},
	87: {909, pairPlaceStatic3, 9, 1319, text{0, 0}},
	88: {958, pairPlaceStatic3, 9, 1328, text{0, 0}},
	89: {777, pairPlaceStatic3, 8, 1337, text{0, 0}},
	90: {648, pairPlaceStatic3, 6, 1345, text{0, 0}},
	91: {782, pairPlaceStatic3, 7, 1351, text{1358, 1366}},
	92: {663, pairPlaceStatic3, 6, 1366, text{0, 0}},
	93: {1929, pairPlaceStatic3, 19, 1372, text{1391, 1392}},
	94: {2588, pairPlaceStatic3, 25, 1392, text{1417, 1418}},
	95: {1019, pairPlaceStatic3, 10, 1418, text{0, 0}},
	96: {1495, pairPlaceStatic3, 15, 1428, text{0, 0}},
	97: {1513, pairPlaceStatic3, 15, 1443, text{1458, 1462}},
	98: {1513, pairPlaceStatic3, 15, 1443, text{1462, 1472}},
}

// http3TableEntry is a dynamic table entry.
type http3TableEntry struct { // 8 bytes
	nameFrom  uint16
	nameEdge  uint16 // nameEdge - nameFrom <= 255
	valueEdge uint16
	totalSize uint16 // nameSize + valueSize + 32
}

// http3DynamicTable
type http3DynamicTable struct {
	entries [124]http3TableEntry
	content [_4K]byte
}

var ( // HTTP/3 byteses
	http3BytesStatic = []byte(":authority:path/age0content-dispositioncontent-length0cookiedateetagif-modified-sinceif-none-matchlast-modifiedlinklocationrefererset-cookie:methodCONNECTDELETEGETHEADOPTIONSPOSTPUT:schemehttphttps:status103200304404503accept*/*application/dns-messageaccept-encodinggzip, deflate, braccept-rangesbytesaccess-control-allow-headerscache-controlcontent-typeaccess-control-allow-origin*cache-controlmax-age=0max-age=2592000max-age=604800no-cacheno-storepublic, max-age=31536000content-encodingbrgzipcontent-typeapplication/dns-messageapplication/javascriptapplication/jsonapplication/x-www-form-urlencodedimage/gifimage/jpegimage/pngtext/csstext/html; charset=utf-8text/plaintext/plain;charset=utf-8rangebytes=0-strict-transport-securitymax-age=31536000max-age=31536000; includesubdomainsmax-age=31536000; includesubdomains; preloadvaryaccept-encodingoriginx-content-type-optionsnosniffx-xss-protection1; mode=block:status100204206302400403421425500accept-languageaccess-control-allow-credentialsFALSETRUEaccess-control-allow-headers*access-control-allow-methodsgetget, post, optionsoptionsaccess-control-expose-headerscontent-lengthaccess-control-request-headerscontent-typeaccess-control-request-methodgetpostalt-svcclearauthorizationcontent-security-policyscript-src 'none'; object-src 'none'; base-uri 'none'early-data1expect-ctforwardedif-rangeoriginpurposeprefetchservertiming-allow-origin*upgrade-insecure-requests1user-agentx-forwarded-forx-frame-optionsdenysameorigin") // DO NOT CHANGE THIS
)

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

var ( // HTTP/3 byteses, TODO
	http3BytesFixedRequestHeaders  = []byte("user-agent gorox")
	http3BytesFixedResponseHeaders = []byte("server gorox")
)
