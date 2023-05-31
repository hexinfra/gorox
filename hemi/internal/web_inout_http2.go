// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 incoming message and outgoing message implementation.

package internal

// http2In_ is used by http2Request and H2Response.
type http2In_ = webIn_

func (r *http2In_) _growHeaders2(size int32) bool {
	edge := r.inputEdge + size      // size is ensured to not overflow
	if edge < int32(cap(r.input)) { // fast path
		return true
	}
	if edge > _16K { // exceeds the max headers limit
		return false
	}
	input := GetNK(int64(edge)) // 4K/16K
	copy(input, r.input[0:r.inputEdge])
	if cap(r.input) != cap(r.stockInput) {
		PutNK(r.input)
	}
	r.input = input
	return true
}

func (r *http2In_) readContent2() (p []byte, err error) {
	// TODO
	return
}

// http2Out_ is used by http2Response and H2Request.
type http2Out_ = webOut_

func (r *http2Out_) addHeader2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *http2Out_) header2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *http2Out_) hasHeader2(name []byte) bool {
	// TODO
	return false
}
func (r *http2Out_) delHeader2(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *http2Out_) delHeaderAt2(o uint8) {
	// TODO
}

func (r *http2Out_) sendChain2() error {
	// TODO
	return nil
}

func (r *http2Out_) echoHeaders2() error {
	// TODO
	return nil
}
func (r *http2Out_) echoChain2() error {
	// TODO
	return nil
}

func (r *http2Out_) addTrailer2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *http2Out_) trailer2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *http2Out_) trailers2() []byte {
	// TODO
	return nil
}

func (r *http2Out_) passBytes2(p []byte) error { return r.writeBytes2(p) }

func (r *http2Out_) finalizeUnsized2() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *http2Out_) writeHeaders2() error { // used by echo and pass
	// TODO
	return nil
}
func (r *http2Out_) writePiece2(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *http2Out_) writeBytes2(p []byte) error {
	// TODO
	return nil
}
func (r *http2Out_) writeVector2() error {
	return nil
}
