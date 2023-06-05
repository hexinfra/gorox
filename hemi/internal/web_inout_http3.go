// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 incoming message and outgoing message implementation.

package internal

func (r *webIn_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *webIn_) readContent3() (p []byte, err error) {
	// TODO
	return
}

func (r *webOut_) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *webOut_) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *webOut_) delHeaderAt3(o uint8) {
	// TODO
}

func (r *webOut_) sendChain3() error {
	// TODO
	return nil
}

func (r *webOut_) echoHeaders3() error {
	// TODO
	return nil
}
func (r *webOut_) echoChain3() error {
	// TODO
	return nil
}

func (r *webOut_) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) trailers3() []byte {
	// TODO
	return nil
}

func (r *webOut_) passBytes3(p []byte) error { return r.writeBytes3(p) }

func (r *webOut_) finalizeUnsized3() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *webOut_) writeHeaders3() error { // used by echo and pass
	// TODO
	return nil
}
func (r *webOut_) writePiece3(piece *Piece, unsized bool) error {
	// TODO
	return nil
}
func (r *webOut_) writeBytes3(p []byte) error {
	// TODO
	return nil
}
func (r *webOut_) writeVector3() error {
	return nil
}
