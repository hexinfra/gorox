// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI protocol.

package fcgi

const (
	fcgiVersion   = 1 // fcgi protocol version
	fcgiHeadLen   = 8 // length of fcgi record head
	fcgiNullID    = 0 // request id for management records
	fcgiResponder = 1 // traditional cgi role
	fcgiComplete  = 0 // protocol status ok
)

const ( // record types
	typeBeginRequest    = 1  // by request
	typeAbortRequest    = 2  // by request
	typeEndRequest      = 3  // by response
	typeParams          = 4  // by request
	typeStdin           = 5  // by request
	typeStdout          = 6  // by response
	typeStderr          = 7  // by response
	typeData            = 8  // by request, NOT supported
	typeGetValues       = 9  // by request
	typeGetValuesResult = 10 // by response
	typeUnknownKind     = 11 // by response
	typeMax             = typeUnknownKind
)

var (
	fcgiBeginKeepConn = []byte{
		fcgiVersion,
		typeBeginRequest,
		0, 1, // request id. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length
		0, 0, // padding length & reserved
		0, fcgiResponder, // role
		1,             // flags=keepConn
		0, 0, 0, 0, 0, // reserved
	}
	fcgiBeginDontKeep = []byte{
		fcgiVersion,
		typeBeginRequest,
		0, 1, // request id. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length
		0, 0, // padding length & reserved
		0, fcgiResponder, // role
		0,             // flags=dontKeep
		0, 0, 0, 0, 0, // reserved
	}
)
