// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 incoming message and outgoing message implementation. See RFC 9114 and 9204.

package internal

// HTTP/3 incoming

func (r *webIn_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *webIn_) readContent3() (p []byte, err error) {
	// TODO
	return
}

// HTTP/3 outgoing

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
func (r *webOut_) delHeaderAt3(i uint8) {
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

func (r *webOut_) finalizeVague3() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *webOut_) writeHeaders3() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *webOut_) writePiece3(piece *Piece, vague bool) error {
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
