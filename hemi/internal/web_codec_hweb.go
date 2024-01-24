// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB incoming message and outgoing message implementation.

package internal

// HWEB incoming

func (r *webIn_) readContentH() (p []byte, err error) {
	return
}

// HWEB outgoing

func (r *webOut_) addHeaderH(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) headerH(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) hasHeaderH(name []byte) bool {
	// TODO
	return false
}
func (r *webOut_) delHeaderH(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *webOut_) delHeaderAtH(i uint8) {
	// TODO
}

func (r *webOut_) sendChainH() error {
	// TODO
	return nil
}

func (r *webOut_) echoHeadersH() error {
	// TODO
	return nil
}
func (r *webOut_) echoChainH() error {
	// TODO
	return nil
}

func (r *webOut_) addTrailerH(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *webOut_) trailerH(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *webOut_) trailersH() []byte {
	// TODO
	return nil
}

func (r *webOut_) passBytesH(p []byte) error { return r.writeBytesH(p) }

func (r *webOut_) finalizeVagueH() error {
	// TODO
	if r.nTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *webOut_) writeHeadersH() error { // used by echo and pass
	// TODO
	return nil
}
func (r *webOut_) writePieceH(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *webOut_) writeBytesH(p []byte) error {
	// TODO
	return nil
}
func (r *webOut_) writeVectorH() error {
	return nil
}
