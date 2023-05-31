// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 incoming message and outgoing message implementation.

package internal

// http3In_ is used by http3Request and H3Response.
type http3In_ = webIn_

func (r *http3In_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *http3In_) readContent3() (p []byte, err error) {
	// TODO
	return
}

// http3Out_ is used by http3Response and H3Request.
type http3Out_ = webOut_

func (r *http3Out_) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *http3Out_) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *http3Out_) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *http3Out_) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *http3Out_) delHeaderAt3(o uint8) {
	// TODO
}

func (r *http3Out_) sendChain3() error {
	// TODO
	return nil
}

func (r *http3Out_) echoHeaders3() error {
	// TODO
	return nil
}
func (r *http3Out_) echoChain3() error {
	// TODO
	return nil
}

func (r *http3Out_) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *http3Out_) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *http3Out_) trailers3() []byte {
	// TODO
	return nil
}

func (r *http3Out_) passBytes3(p []byte) error { return r.writeBytes3(p) }

func (r *http3Out_) finalizeUnsized3() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *http3Out_) writeHeaders3() error { // used by echo and pass
	// TODO
	return nil
}
func (r *http3Out_) writePiece3(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *http3Out_) writeBytes3(p []byte) error {
	// TODO
	return nil
}
func (r *http3Out_) writeVector3() error {
	return nil
}
