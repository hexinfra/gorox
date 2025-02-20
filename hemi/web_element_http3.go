// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 protocol elements. See RFC 9114 and RFC 9204.

// Server Push is not supported because it's rarely used. Chrome and Firefox even removed it.

package hemi

import (
	"sync"
	"sync/atomic"
)

const ( // HTTP/3 sizes and limits for both of our HTTP/3 server and HTTP/3 backend
	http3MaxTableSize         = _4K
	http3MaxConcurrentStreams = 127 // currently hardcoded
)

var qpackBytesStatic = []byte(":authority:path/age0content-dispositioncontent-length0cookiedateetagif-modified-sinceif-none-matchlast-modifiedlinklocationrefererset-cookie:methodCONNECTDELETEGETHEADOPTIONSPOSTPUT:schemehttphttps:status103200304404503accept*/*application/dns-messageaccept-encodinggzip, deflate, braccept-rangesbytesaccess-control-allow-headerscache-controlcontent-typeaccess-control-allow-origin*cache-controlmax-age=0max-age=2592000max-age=604800no-cacheno-storepublic, max-age=31536000content-encodingbrgzipcontent-typeapplication/dns-messageapplication/javascriptapplication/jsonapplication/x-www-form-urlencodedimage/gifimage/jpegimage/pngtext/csstext/html; charset=utf-8text/plaintext/plain;charset=utf-8rangebytes=0-strict-transport-securitymax-age=31536000max-age=31536000; includesubdomainsmax-age=31536000; includesubdomains; preloadvaryaccept-encodingoriginx-content-type-optionsnosniffx-xss-protection1; mode=block:status100204206302400403421425500accept-languageaccess-control-allow-credentialsFALSETRUEaccess-control-allow-headers*access-control-allow-methodsgetget, post, optionsoptionsaccess-control-expose-headerscontent-lengthaccess-control-request-headerscontent-typeaccess-control-request-methodgetpostalt-svcclearauthorizationcontent-security-policyscript-src 'none'; object-src 'none'; base-uri 'none'early-data1expect-ctforwardedif-rangeoriginpurposeprefetchservertiming-allow-origin*upgrade-insecure-requests1user-agentx-forwarded-forx-frame-optionsdenysameorigin") // DO NOT CHANGE THIS UNLESS YOU KNOW WHAT YOU ARE DOING

// qpackStaticTable
var qpackStaticTable = [...]qpackTableEntry{ // TODO
}

// qpackTableEntry is a QPACK table entry.
type qpackTableEntry struct { // 8 bytes
	nameHash  uint16 // name hash
	nameFrom  uint16 // name edge at nameFrom+nameSize
	nameSize  uint8  // must <= 255
	isStatic  bool   // ...
	valueEdge uint16 // value: [nameFrom+nameSize:valueEdge]
}

// qpackTable
type qpackTable struct {
	entries [1]qpackTableEntry // TODO: size
	content [_4K]byte
}

// http3Buffer
type http3Buffer struct {
	buf [_16K]byte // frame header + frame payload
	ref atomic.Int32
}

var poolHTTP3Buffer sync.Pool

func getHTTP3Buffer() *http3Buffer {
	var inBuffer *http3Buffer
	if x := poolHTTP3Buffer.Get(); x == nil {
		inBuffer = new(http3Buffer)
	} else {
		inBuffer = x.(*http3Buffer)
	}
	return inBuffer
}
func putHTTP3Buffer(inBuffer *http3Buffer) { poolHTTP3Buffer.Put(inBuffer) }

func (b *http3Buffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http3Buffer) getRef() int32 { return b.ref.Load() }
func (b *http3Buffer) incRef()       { b.ref.Add(1) }
func (b *http3Buffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DebugLevel() >= 1 {
			Printf("putHTTP3Buffer ref=%d\n", b.ref.Load())
		}
		putHTTP3Buffer(b)
	}
}

// http3InFrame is the HTTP/3 incoming frame.
type http3InFrame struct {
	// TODO
}

func (f *http3InFrame) zero() { *f = http3InFrame{} }

// http3OutFrame is the HTTP/3 outgoing frame.
type http3OutFrame struct {
	// TODO
}

func (f *http3OutFrame) zero() { *f = http3OutFrame{} }
