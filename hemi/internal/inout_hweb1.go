// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/1 protocol elements, incoming message and outgoing message implementation.

// HWEB/1 is a binary HTTP/1.1. Its design makes it easy to implement a server or client.

package internal

// hweb1In_ is used by hweb1Request and B1Response.
type hweb1In_ = webIn_

func (r *hweb1In_) growHeadB1() bool {
	return false
}
func (r *hweb1In_) recvHeadersB1() bool {
	return true
}

func (r *hweb1In_) readContentB1() (p []byte, err error) {
	return nil, nil
}
func (r *hweb1In_) _readSizedContentB1() (p []byte, err error) {
	return nil, nil
}
func (r *hweb1In_) _readUnsizedContentB1() (p []byte, err error) {
	return nil, nil
}

func (r *hweb1In_) recvTrailersB1() bool {
	return true
}
func (r *hweb1In_) growChunkedB1() bool {
	return false
}

// hweb1Out_ is used by hweb1Response and B1Request.
type hweb1Out_ = webOut_

func (r *hweb1Out_) addHeaderB1(name []byte, value []byte) bool {
	return false
}
func (r *hweb1Out_) headerB1(name []byte) (value []byte, ok bool) {
	return nil, false
}
func (r *hweb1Out_) hasHeaderB1(name []byte) bool {
	return false
}
func (r *hweb1Out_) delHeaderB1(name []byte) (deleted bool) {
	return false
}
func (r *hweb1Out_) delHeaderAtB1(o uint8) {
}
func (r *hweb1Out_) _addFixedHeaderB1(name []byte, value []byte) { // used by finalizeHeaders
}

func (r *hweb1Out_) sendChainB1() error {
	return nil
}

func (r *hweb1Out_) echoChainB1() error { // TODO: coalesce?
	return nil
}

func (r *hweb1Out_) addTrailerB1(name []byte, value []byte) bool {
	return false
}
func (r *hweb1Out_) trailerB1(name []byte) (value []byte, ok bool) {
	return nil, false
}
func (r *hweb1Out_) trailersB1() []byte { return nil }

func (r *hweb1Out_) passBytesB1(p []byte) error { return r.writeBytesB1(p) }

func (r *hweb1Out_) finalizeUnsizedB1() error {
	return nil
}

func (r *hweb1Out_) writeHeadersB1() error { // used by echo and pass
	return nil
}
func (r *hweb1Out_) writePieceB1(piece *Piece) error {
	return nil
}
func (r *hweb1Out_) writeBytesB1(p []byte) error {
	return nil
}
func (r *hweb1Out_) writeVectorB1() error {
	return nil
}

//////////////////////////////////////// HWEB/1 protocol elements ////////////////////////////////////////

// nameValue = nameSize(8) valueSize(24) name value
// name      = 1*OCTET
// value     = *OCTET

// fields = zero(1) bodySize(63) 1*nameValue

// request  = head [ body ]
// response = head [ body ]

// head = fields
// body = sized | unsized

// sized   = zero(1) bodySize(63) *OCTET
// unsized = *sized last tail

// last = 0x0000000000000000
// tail = 0x0000000000000000 | fields

/*

stream=1 (sized output):

    --> bodySize=?     body=[:method=GET :target=/hello host=example.com:8081]

    <-- bodySize=?     body=[:status=200 content-length=12]
    <-- bodySize=12    body=[hello,world!]

stream=2 (unsized output):

    --> bodySize=?     body=[:method=POST :target=/abc?d=e host=example.com:8081 content-length=90]
    --> bodySize=90    body=[...90...]

    <-- bodySize=?     body=[:status=200 content-type=text/html;charset=utf-8 transfer-encoding=chunked]
    <-- bodySize=16376 body=[...16376...] // chunk
    <-- bodySize=123   body=[...123...] // chunk
    <-- bodySize=4567  body=[...4567...] // chunk
    <-- bodySize=0     body=[] // last chunk, MUST exist in chunked mode, MUST be empty
    <-- bodySize=?     body=[md5-digest=12345678901234567890123456789012] // trailers, MUST exist in chunked mode, MAY be empty

*/
