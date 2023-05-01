// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/2 protocol elements, incoming message and outgoing message implementation.

// HWEB/2 is a simplified HTTP/2. Its design makes it easy to implement a server or client.

package internal

// hweb2In_ is used by hweb2Request and B2Response.
type hweb2In_ = webIn_

func (r *hweb2In_) readContentB2() (p []byte, err error) {
	return
}

// hweb2Out_ is used by hweb2Response and B2Request.
type hweb2Out_ = webOut_

func (r *hweb2Out_) addHeaderB2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *hweb2Out_) headerB2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *hweb2Out_) hasHeaderB2(name []byte) bool {
	// TODO
	return false
}
func (r *hweb2Out_) delHeaderB2(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *hweb2Out_) delHeaderAtB2(o uint8) {
	// TODO
}

func (r *hweb2Out_) sendChainB2() error {
	return nil
}

func (r *hweb2Out_) echoHeadersB2() error {
	// TODO
	return nil
}
func (r *hweb2Out_) echoChainB2() error {
	// TODO
	return nil
}

func (r *hweb2Out_) addTrailerB2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *hweb2Out_) trailerB2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *hweb2Out_) trailersB2() []byte {
	// TODO
	return nil
}

func (r *hweb2Out_) passBytesB2(p []byte) error { return r.writeBytesB2(p) }

func (r *hweb2Out_) finalizeUnsizedB2() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *hweb2Out_) writeHeadersB2() error { // used by echo and pass
	// TODO
	return nil
}
func (r *hweb2Out_) writePieceB2(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *hweb2Out_) writeBytesB2(p []byte) error {
	// TODO
	return nil
}
func (r *hweb2Out_) writeVectorB2() error {
	return nil
}

//////////////////////////////////////// HWEB/2 protocol elements ////////////////////////////////////////

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
//   sizeRecord = recordHead windowSize(3)

// On TCP connection established, client sends an init record, then server sends one, too.
// Whenever a client kicks a new request, it must use the least unused streamID starting from 1.
// A streamID is considered as unused after a response with this streamID was received entirely.
// If concurrent streams exceeds the limit set in init record, server can close the connection.

const ( // record types
	hweb2TypeINIT = 0 // contains connection settings
	hweb2TypeHEAD = 1 // contains name-value pairs for headers
	hweb2TypeDATA = 2 // contains content data
	hweb2TypeSIZE = 3 // available window size for receiving content
	hweb2TypeTAIL = 4 // contains name-value pairs for trailers
)

const ( // record flags
	hwebFlagEndStream = 0b00000001 // end of stream, used by HEAD, DATA, and TAIL
)

const ( // setting codes
	hweb2SettingMaxRecordBodySize    = 0
	hweb2SettingInitialWindowSize    = 1
	hweb2SettingMaxStreams           = 2
	hweb2SettingMaxConcurrentStreams = 3
)

var hweb2SettingDefaults = [...]int32{
	hweb2SettingMaxRecordBodySize:    16376, // allow: [16376-16777215]
	hweb2SettingInitialWindowSize:    16376, // allow: [16376-16777215]
	hweb2SettingMaxStreams:           1000,  // allow: [100-16777215]
	hweb2SettingMaxConcurrentStreams: 100,   // allow: [100-16777215]. cannot be larger than maxStreams
}

/*

on connection established:

    ==> type=INIT streamID=0 bodySize=8     body=[maxRecordBodySize=16376 initialWindowSize=16376]
    <== type=INIT streamID=0 bodySize=16    body=[maxRecordBodySize=16376 initialWindowSize=16376 maxStreams=1000 maxConcurrentStreams=10]

stream=1 (sized output):

    --> type=HEAD streamID=1 bodySize=?     body=[:method=GET :path=/hello host=example.com:8081] // endStream=1

    <-- type=HEAD streamID=1 bodySize=?     body=[:status=200 content-length=12]
    <-- type=DATA streamID=1 bodySize=6     body=[hello,]
    <-- type=DATA streamID=1 bodySize=6     body=[world!] // endStream=1

stream=2 (unsized output):

    --> type=HEAD streamID=2 bodySize=?     body=[:method=POST :path=/abc?d=e host=example.com:8081 content-length=90]
    --> type=DATA streamID=2 bodySize=90    body=[...90...] // endStream=1

    <-- type=HEAD streamID=2 bodySize=?     body=[:status=200 content-type=text/html;charset=utf-8]
    <-- type=DATA streamID=2 bodySize=16376 body=[...16376...]
    ==> type=SIZE streamID=2 bodySize=3     body=123
    <-- type=DATA streamID=2 bodySize=123   body=[...123...]
    ==> type=SIZE streamID=2 bodySize=3     body=16376
    <-- type=DATA streamID=2 bodySize=4567  body=[...4567...]
    <-- type=TAIL streamID=2 bodySize=?     body=[md5-digest=12345678901234567890123456789012] // endStream=1

*/
