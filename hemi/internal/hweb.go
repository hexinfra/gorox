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

// head(64) = kind(8) streamID(24) bodySize(32)

// prefaceRecord = head prefaceBody
// prefaceBody = *setting
// setting(32) = code(8) value(24)

// request = message
// response = message
// message = headersRecord *contentRecord [ trailersRecord ]

// headersRecord = head headersBody
// headersBody = fieldsBody
// fieldsBody = *field
// field = flags(8) nameSize(8) valueSize(16) name value
// name = 1*OCTET
// value = *OCTET

// contentRecord = head contentBody
// contentBody = identityBody | chunkedBody
// identityBody = *OCTET
// chunkedBody = *chunk lastChunk
// chunk = chunkSize(32) chunkData
// chunkData = 1*OCTET
// lastChunk = 0x00000000

// trailersRecord = head trailersBody
// trailersBody = fieldsBody

const hwebVersion = 1

const ( // record kinds
	hwebKindPreface  = 0
	hwebKindHeaders  = 1
	hwebKindContent  = 2
	hwebKindTrailers = 3
)

const ( // setting codes
	hwebSettingMaxTotalStreams   = 0 // default value
	hwebSettingMaxActiveStreams  = 1 // default value
	hwebSettingMaxFieldsBodySize = 2 // default value
)

var hwebSettingDefaults = [...]int32{ // limit: 16777215
	hwebSettingMaxTotalStreams:   10000,
	hwebSettingMaxActiveStreams:  100,
	hwebSettingMaxFieldsBodySize: 16384,
}

const ( // field flags
	hwebFieldFlagPseudo = 0b00000001 // pseudo field?
)

/*

  prefaceRecord ->
    kind=0 streamID=0 bodySize=?? body=[??]

  <- prefaceRecord
    kind=0 streamID=0 bodySize=?? body=[??]


request=1

  headersRecord ->
    kind=1 streamID=1 bodySize=77 body=[..77..]

response=1

  <- headersRecord
    kind=1 streamID=1 bodySize=88 body=[..88..]
  <- contentRecord
    kind=2 streamID=1 bodySize=999 body=[...999...]


request=2

  headersRecord ->
    kind=1 streamID=2 bodySize=11 body=[..11..]
  contentRecord ->
    kind=2 streamID=2 bodySize=222 body=[..222..]

response=2

  <- headersRecord
    kind=1 streamID=2 bodySize=33 body=[..33..]
  <- contentRecord
    kind=2 streamID=2 bodySize=444 body=[...444...]
  <- contentRecord
    kind=2 streamID=2 bodySize=555 body=[...555...]
  <- trailersRecord
    kind=3 streamID=2 bodySize=66 body=[..66..]

*/
