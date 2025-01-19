// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// FCGI protocol elements. See: https://fastcgi-archives.github.io/FastCGI_Specification.html

package hemi

import (
	"sync"
)

// FCGI Record = FCGI Header(8) + payload[65535] + padding[255]
// FCGI Header = version(1) + type(1) + requestId(2) + payloadLen(2) + paddingLen(1) + reserved(1)

// Discrete records are standalone.
// Streamed records end with an empty record (payloadLen=0).

const ( // fcgi constants
	fcgiHeaderSize = 8
	fcgiMaxPayload = 65535
	fcgiMaxPadding = 255
	fcgiMaxRecords = fcgiHeaderSize + fcgiMaxPayload + fcgiMaxPadding
)

var poolFCGIRecords sync.Pool

func getFCGIRecords() []byte {
	if x := poolFCGIRecords.Get(); x == nil {
		return make([]byte, fcgiMaxRecords)
	} else {
		return x.([]byte)
	}
}
func putFCGIRecords(records []byte) {
	if cap(records) != fcgiMaxRecords {
		BugExitln("fcgi: bad records")
	}
	poolFCGIRecords.Put(records)
}

var ( // fcgi request records
	fcgiBeginKeepConn = []byte{ // 16 bytes
		1, 1, // version, FCGI_BEGIN_REQUEST
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1. same below
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 1, 0, 0, 0, 0, 0, // role=responder, flags=keepConn
	}
	fcgiBeginDontKeep = []byte{ // 16 bytes
		1, 1, // version, FCGI_BEGIN_REQUEST
		0, 1, // request id = 1
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 0, 0, 0, 0, 0, 0, // role=responder, flags=dontKeep
	}
	fcgiEmptyParams = []byte{ // end of params, 8 bytes
		1, 4, // version, FCGI_PARAMS
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	}
	fcgiEmptyStdin = []byte{ // end of stdins, 8 bytes
		1, 5, // version, FCGI_STDIN
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	}
)

var ( // fcgi request param names
	fcgiBytesAuthType         = []byte("AUTH_TYPE")
	fcgiBytesContentLength    = []byte("CONTENT_LENGTH")
	fcgiBytesContentType      = []byte("CONTENT_TYPE")
	fcgiBytesDocumentRoot     = []byte("DOCUMENT_ROOT")
	fcgiBytesDocumentURI      = []byte("DOCUMENT_URI")
	fcgiBytesGatewayInterface = []byte("GATEWAY_INTERFACE")
	fcgiBytesHTTP_            = []byte("HTTP_") // prefix
	fcgiBytesHTTPS            = []byte("HTTPS")
	fcgiBytesPathInfo         = []byte("PATH_INFO")
	fcgiBytesPathTranslated   = []byte("PATH_TRANSLATED")
	fcgiBytesQueryString      = []byte("QUERY_STRING")
	fcgiBytesRedirectStatus   = []byte("REDIRECT_STATUS")
	fcgiBytesRemoteAddr       = []byte("REMOTE_ADDR")
	fcgiBytesRemoteHost       = []byte("REMOTE_HOST")
	fcgiBytesRequestMethod    = []byte("REQUEST_METHOD")
	fcgiBytesRequestScheme    = []byte("REQUEST_SCHEME")
	fcgiBytesRequestURI       = []byte("REQUEST_URI")
	fcgiBytesScriptFilename   = []byte("SCRIPT_FILENAME")
	fcgiBytesScriptName       = []byte("SCRIPT_NAME")
	fcgiBytesServerAddr       = []byte("SERVER_ADDR")
	fcgiBytesServerName       = []byte("SERVER_NAME")
	fcgiBytesServerPort       = []byte("SERVER_PORT")
	fcgiBytesServerProtocol   = []byte("SERVER_PROTOCOL")
	fcgiBytesServerSoftware   = []byte("SERVER_SOFTWARE")
)

var ( // fcgi request param values
	fcgiBytesCGI1_1 = []byte("CGI/1.1")
	fcgiBytesON     = []byte("on")
	fcgiBytes200    = []byte("200")
)

const ( // fcgi response header hashes
	fcgiHashStatus = 676
)

var ( // fcgi response header names
	fcgiBytesStatus = []byte("status")
)
