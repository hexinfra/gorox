// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB protocol elements.

// HWEB is a simplified HTTP/2. Its design makes it easy to implement a server or client.

package internal

// hwebIn_ is used by hwebRequest and hResponse.
type hwebIn_ = webIn_

func (r *hwebIn_) readContentH() (p []byte, err error) {
	return
}

// hwebOut_ is used by hwebResponse and hRequest.
type hwebOut_ = webOut_

func (r *hwebOut_) addHeaderH(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *hwebOut_) headerH(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *hwebOut_) hasHeaderH(name []byte) bool {
	// TODO
	return false
}
func (r *hwebOut_) delHeaderH(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *hwebOut_) delHeaderAtH(o uint8) {
	// TODO
}

func (r *hwebOut_) sendChainH() error {
	return nil
}

func (r *hwebOut_) echoHeadersH() error {
	// TODO
	return nil
}
func (r *hwebOut_) echoChainH() error {
	// TODO
	return nil
}

func (r *hwebOut_) trailerH(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *hwebOut_) addTrailerH(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *hwebOut_) trailersH() []byte {
	// TODO
	return nil
}

func (r *hwebOut_) passBytesH(p []byte) error {
	// TODO
	return nil
}

func (r *hwebOut_) finalizeUnsizedH() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *hwebOut_) writeBlockH(block *Block, unsized bool) error {
	// TODO
	return nil
}
func (r *hwebOut_) writeVectorH() error {
	return nil
}

//////////////////////////////////////// HWEB protocol elements.

// recordHead(64) = type(8) streamID(24) flags(8) bodySize(24)

// initRecord = recordHead *setting
//   setting(32) = settingCode(8) settingValue(24)

// request  = headRecord *dataRecord [ tailRecord ]
// response = headRecord *dataRecord [ tailRecord ]

//   headRecord = recordHead 1*nameValue
//   tailRecord = recordHead 1*nameValue

//     nameValue = nameSize(8) valueSize(24) name value
//       name  = 1*OCTET
//       value = *OCTET

//   dataRecord = recordHead *OCTET
//   sizeRecord = recordHead bufferSize(3)

// On TCP connection established, client sends an init record, then server sends one, too.
// Whenever a client kicks a new request, it must use the least unused streamID starting from 1.
// A streamID is considered as unused after a response with this streamID was received entirely.
// If concurrent streams exceeds the limit set in init record, server can close the connection.

const ( // record types
	hwebTypeINIT = 0 // contains connection settings
	hwebTypeHEAD = 1 // contains name-value pairs for headers
	hwebTypeDATA = 2 // contains content data
	hwebTypeSIZE = 3 // available buffer size for receiving content
	hwebTypeTAIL = 4 // contains name-value pairs for trailers
)

const ( // record flags
	hwebFlagEndDATA = 0b00000001 // indicating end of DATA
)

const ( // setting codes
	hwebSettingMaxRecordBodySize    = 0
	hwebSettingInitialBufferSize    = 1
	hwebSettingMaxStreams           = 2
	hwebSettingMaxConcurrentStreams = 3
)

var hwebSettingDefaults = [...]int32{
	hwebSettingMaxRecordBodySize:    16376, // allow: [16376-16777215]
	hwebSettingInitialBufferSize:    16376, // allow: [16376-16777215]
	hwebSettingMaxStreams:           1000,  // allow: [100-16777215]
	hwebSettingMaxConcurrentStreams: 100,   // allow: [100-16777215]. cannot be larger than maxStreams
}

/*

on connection established:

    ==> type=INIT streamID=0 bodySize=8     body=[maxRecordBodySize=16376 initialBufferSize=16376]
    <== type=INIT streamID=0 bodySize=16    body=[maxRecordBodySize=16376 initialBufferSize=16376 maxStreams=1000 maxConcurrentStreams=10]

stream=1 (sized output):

    --> type=HEAD streamID=1 bodySize=?     body=[:method=GET :target=/hello host=example.com:8081]

    <-- type=HEAD streamID=1 bodySize=?     body=[:status=200 content-length=12]
    <-- type=DATA streamID=1 bodySize=6     body=[hello,]
    <-- type=DATA streamID=1 bodySize=6     body=[world!]

stream=2 (unsized output):

    --> type=HEAD streamID=2 bodySize=?     body=[:method=POST :target=/abc?d=e host=example.com:8081 content-length=90]
    --> type=DATA streamID=2 bodySize=90    body=[...90...]

    <-- type=HEAD streamID=2 bodySize=?     body=[:status=200 content-type=text/html;charset=utf-8]
    <-- type=DATA streamID=2 bodySize=16376 body=[...16376...]
    ==> type=SIZE streamID=2 bodySize=3     body=123
    <-- type=DATA streamID=2 bodySize=123   body=[...123...]
    ==> type=SIZE streamID=2 bodySize=3     body=16376
    <-- type=DATA streamID=2 bodySize=4567  body=[...4567...]
    <-- type=DATA streamID=2 bodySize=0     body=[] // flagEndDATA=1
    <-- type=TAIL streamID=2 bodySize=?     body=[md5-digest=12345678901234567890123456789012]

*/
