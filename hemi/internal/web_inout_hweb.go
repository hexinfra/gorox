// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB incoming message and outgoing message implementation.

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
