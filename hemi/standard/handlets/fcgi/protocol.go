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

const ( // request record types
	fcgiTypeBeginRequest = 1
	fcgiTypeParams       = 4
	fcgiTypeStdin        = 5
)

const ( // response record types
	fcgiTypeStdout     = 6
	fcgiTypeStderr     = 7
	fcgiTypeEndRequest = 3
)

var ( // begin request records
	fcgiBeginKeepConn = []byte{
		fcgiVersion,
		fcgiTypeBeginRequest,
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length = 8
		0, 0, // padding length = 0 & reserved

		0, fcgiResponder, // role
		1,             // flags=keepConn
		0, 0, 0, 0, 0, // reserved
	}
	fcgiBeginDontKeep = []byte{
		fcgiVersion,
		fcgiTypeBeginRequest,
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length = 8
		0, 0, // padding length = 0 & reserved

		0, fcgiResponder, // role
		0,             // flags=dontKeep
		0, 0, 0, 0, 0, // reserved
	}
)
