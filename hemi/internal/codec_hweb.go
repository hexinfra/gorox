// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB protocol elements, incoming message and outgoing message implementation.

// HWEB is an HTTP gateway protocol without WebSocket, TCP Tunnel, and UDP Tunnel support.
// HWEB is under design, maybe we'll build it upon QUIC or Homa (https://homa-transport.atlassian.net/wiki/spaces/HOMA/overview).

package internal

// hwebIn_ is used by hwebRequest and HResponse.
type hwebIn_ = webIn_

func (r *hwebIn_) readContentH() (p []byte, err error) {
	return
}

// hwebOut_ is used by hwebResponse and HRequest.
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

func (r *hwebOut_) addTrailerH(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *hwebOut_) trailerH(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *hwebOut_) trailersH() []byte {
	// TODO
	return nil
}

func (r *hwebOut_) passBytesH(p []byte) error { return r.writeBytesH(p) }

func (r *hwebOut_) finalizeUnsizedH() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *hwebOut_) writeHeadersH() error { // used by echo and pass
	// TODO
	return nil
}
func (r *hwebOut_) writePieceH(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *hwebOut_) writeBytesH(p []byte) error {
	// TODO
	return nil
}
func (r *hwebOut_) writeVectorH() error {
	return nil
}

//////////////////////////////////////// HWEB protocol elements ////////////////////////////////////////

// WARNING: DRAFT DESIGN!

// recordHead(64) = type(8) exchanID(24) flags(8) bodySize(24)

// initRecord = recordHead *setting
//   setting(32) = settingCode(8) settingValue(24)

// request  = headRecord *dataRecord [ tailRecord ]
// response = headRecord *dataRecord [ tailRecord ]

//   headRecord = recordHead 1*field
//   tailRecord = recordHead 1*field

//     field = literalField | indexedField | indexedName | indexedValue
//       literalField = 00(2) valueSize(22) nameSize(8) name value
//       indexedField = 11(2) index(6)
//       indexedName  = 10(2) index(6) valueSize(24) value
//       indexedValue = 01(2) index(6) nameSize(8) name
//         name  = 1*OCTET
//         value = *OCTET

//   dataRecord = recordHead *OCTET
//   sizeRecord = recordHead windowSize(3)

// On connection established, client sends an init record, then server sends one, too.
// Whenever a client kicks a new request, it must use the least unused exchanID starting from 1.
// A exchanID is considered as unused after a response with this exchanID was received entirely.
// If concurrent exchans exceeds the limit set in init record, server can close the connection.

const ( // record types
	hwebTypeINIT = 0 // contains connection settings
	hwebTypeHEAD = 1 // contains name-value pairs for headers
	hwebTypeDATA = 2 // contains content data
	hwebTypeSIZE = 3 // available window size for receiving content
	hwebTypeTAIL = 4 // contains name-value pairs for trailers
)

const ( // record flags
	hwebFlagEndExchan = 0b00000001 // end of exchan, used by HEAD, DATA, and TAIL
)

const ( // setting codes
	hwebSettingMaxRecordBodySize    = 0
	hwebSettingInitialWindowSize    = 1
	hwebSettingMaxExchans           = 2
	hwebSettingMaxConcurrentExchans = 3
)

var hwebSettingDefaults = [...]int32{
	hwebSettingMaxRecordBodySize:    16376, // allow: [16376-16777215]
	hwebSettingInitialWindowSize:    16376, // allow: [16376-16777215]
	hwebSettingMaxExchans:           1000,  // allow: [100-16777215]
	hwebSettingMaxConcurrentExchans: 100,   // allow: [100-16777215]. cannot be larger than maxExchans
}

/*

on connection established:

    ==> type=INIT exchanID=0 bodySize=8     body=[maxRecordBodySize=16376 initialWindowSize=16376]
    <== type=INIT exchanID=0 bodySize=16    body=[maxRecordBodySize=16376 initialWindowSize=16376 maxExchans=1000 maxConcurrentExchans=10]

exchan=1 (sized output):

    --> type=HEAD exchanID=1 bodySize=?     body=[:method=GET :path=/hello host=example.com:8081] // endExchan=1

    <-- type=HEAD exchanID=1 bodySize=?     body=[:status=200 content-length=12]
    <-- type=DATA exchanID=1 bodySize=6     body=[hello,]
    <-- type=DATA exchanID=1 bodySize=6     body=[world!] // endExchan=1

exchan=2 (unsized output):

    --> type=HEAD exchanID=2 bodySize=?     body=[:method=POST :path=/abc?d=e host=example.com:8081 content-length=90]
    --> type=DATA exchanID=2 bodySize=90    body=[...90...] // endExchan=1

    <-- type=HEAD exchanID=2 bodySize=?     body=[:status=200 content-type=text/html;charset=utf-8]
    <-- type=DATA exchanID=2 bodySize=16376 body=[...16376...]
    ==> type=SIZE exchanID=2 bodySize=3     body=123
    <-- type=DATA exchanID=2 bodySize=123   body=[...123...]
    ==> type=SIZE exchanID=2 bodySize=3     body=16376
    <-- type=DATA exchanID=2 bodySize=4567  body=[...4567...]
    <-- type=TAIL exchanID=2 bodySize=?     body=[md5-digest=12345678901234567890123456789012] // endExchan=1

*/
