// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 incoming message and outgoing message implementation.

package internal

// HTTP/2 incoming

func (r *webIn_) _growHeaders2(size int32) bool {
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

func (r *webIn_) readContent2() (p []byte, err error) {
	// TODO
	return
}

// HTTP/2 outgoing

func (r *webOut_) addHeader2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) header2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) hasHeader2(name []byte) bool {
	// TODO
	return false
}
func (r *webOut_) delHeader2(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *webOut_) delHeaderAt2(o uint8) {
	// TODO
}

func (r *webOut_) sendChain2() error {
	// TODO
	return nil
}

func (r *webOut_) echoHeaders2() error {
	// TODO
	return nil
}
func (r *webOut_) echoChain2() error {
	// TODO
	return nil
}

func (r *webOut_) addTrailer2(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) trailer2(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) trailers2() []byte {
	// TODO
	return nil
}

func (r *webOut_) passBytes2(p []byte) error { return r.writeBytes2(p) }

func (r *webOut_) finalizeUnsized2() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *webOut_) writeHeaders2() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *webOut_) writePiece2(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *webOut_) writeBytes2(p []byte) error {
	// TODO
	return nil
}
func (r *webOut_) writeVector2() error {
	return nil
}
