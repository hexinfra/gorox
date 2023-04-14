// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB protocol elements.

// HWEB is a binary HTTP gateway protocol like FCGI, but has a lot of improvements over FCGI.
// HWEB also borrows some ideas from HTTP/2.

package internal

// hwebIn_ is used by hwebRequest and hResponse.
type hwebIn_ = webIn_

func (r *hwebIn_) readContentH() (p []byte, err error) {
	return
}

// hwebOut_ is used by hwebResponse and hRequest.
type hwebOut_ = webOut_

func (r *hwebOut_) sendChainH() error {
	return nil
}

//////////////////////////////////////// HWEB protocol elements.

// head(64) = kind(8) streamID(24) flags(8) bodySize(24)

// prefaceRecord = head *setting
// setting(32) = code(8) value(24)

// message = headersRecord *fragmentRecord [ trailersRecord ]

// headersRecord  = head *field
// trailersRecord = head *field

// field(32+) = flags(8) nameSize(8) valueSize(16) name value
// name = 1*OCTET
// value = *OCTET

// fragmentRecord = head *OCTET

const ( // record kinds
	hwebKindPreface  = 0
	hwebKindHeaders  = 1
	hwebKindFragment = 2
	hwebKindTrailers = 3
)

const ( // setting codes
	hwebSettingMaxRecordBodySize    = 0
	hwebSettingMaxConcurrentStreams = 1
	hwebSettingTotalStreams         = 2
)

var hwebSettingDefaults = [...]int32{
	hwebSettingMaxRecordBodySize:    16376, // allow: [16376-16777215]
	hwebSettingMaxConcurrentStreams: 100,   // allow: [1-16777215]
	hwebSettingTotalStreams:         1000,  // allow: [10-16777215]
}

const ( // field flags
	hwebFieldFlagPseudo = 0b00000001 // pseudo field or not
)

/*

on connection established:

    -> kind=preface streamID=0 bodySize=4 body=[maxRecordBodySize=16376]
    <- kind=preface streamID=0 bodySize=12 body=[maxRecordBodySize=16376 maxConcurrentStreams=10 totalStreams=1000]

stream=1 (identity output):

    -> kind=headers streamID=1 bodySize=?? body=[:method=GET :uri=/hello host=example.com:8081]

    <- kind=headers streamID=1 bodySize=?? body=[:status=200 content-length=12]
    <- kind=fragment streamID=1 bodySize=6 body=[hello,]
    <- kind=fragment streamID=1 bodySize=6 body=[world!]

stream=2 (chunked output):

    -> kind=headers streamID=2 bodySize=?? body=[:method=POST :uri=/abc?d=e host=example.com:8081 content-length=90]
    -> kind=fragment streamID=2 bodySize=90 body=[...90...]

    <- kind=headers streamID=2 bodySize=?? body=[:status=200 content-type=text/html]
    <- kind=fragment streamID=2 bodySize=77 body=[...77...]
    <- kind=fragment streamID=2 bodySize=88 body=[...88...]
    <- kind=fragment streamID=2 bodySize=0 body=[]
    <- kind=trailers streamID=2 bodySize=?? body=[md5-digest=12345678901234567890123456789012]

*/
