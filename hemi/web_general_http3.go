// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 types. See RFC 9114 and RFC 9204.

package hemi

import (
	"net"

	"github.com/hexinfra/gorox/hemi/library/gotcp2"
)

// http3Conn
type http3Conn interface {
	// Imports
	httpConn
	// Methods
}

// http3Conn_ is a parent.
type http3Conn_ struct { // for server3Conn and backend3Conn
	// Parent
	httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn    *gotcp2.Conn // the quic connection
	inBuffer    *http3Buffer // ...
	decodeTable http3Table   // ...
	encodeTable http3Table   // ...
	// Conn states (zeros)
	_http3Conn0 // all values in this struct must be zero by default!
}
type _http3Conn0 struct { // for fast reset, entirely
	inBufferEdge uint32 // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	sectBack     uint32 // incoming frame section (header or payload) begins from c.inBuffer.buf[c.sectBack]
	sectFore     uint32 // incoming frame section (header or payload) ends at c.inBuffer.buf[c.sectFore]
}

func (c *http3Conn_) onGet(id int64, holder holder, quicConn *gotcp2.Conn) {
	c.httpConn_.onGet(id, holder)

	c.quicConn = quicConn
	if c.inBuffer == nil {
		c.inBuffer = getHTTP3Buffer()
		c.inBuffer.incRef()
	}
}
func (c *http3Conn_) onPut() {
	// c.inBuffer is reserved
	// c.decodeTable is reserved
	// c.encodeTable is reserved
	c.quicConn = nil

	c.httpConn_.onPut()
}

func (c *http3Conn_) remoteAddr() net.Addr { return nil } // TODO

// http3Stream
type http3Stream interface {
	// Imports
	httpStream
	// Methods
}

// http3Stream_ is a parent.
type http3Stream_[C http3Conn] struct { // for server3Stream and backend3Stream
	// Parent
	httpStream_[C]
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	quicStream *gotcp2.Stream // the quic stream
	// Stream states (zeros)
	_http3Stream0 // all values in this struct must be zero by default!
}
type _http3Stream0 struct { // for fast reset, entirely
}

func (s *http3Stream_[C]) onUse(conn C, quicStream *gotcp2.Stream) {
	s.httpStream_.onUse(conn)

	s.quicStream = quicStream
}
func (s *http3Stream_[C]) onEnd() {
	s._http3Stream0 = _http3Stream0{}

	s.quicStream = nil
	s.httpStream_.onEnd()
}

func (s *http3Stream_[C]) ID() int64 { return s.quicStream.ID() }

func (s *http3Stream_[C]) markBroken()    {}               // TODO
func (s *http3Stream_[C]) isBroken() bool { return false } // TODO

func (s *http3Stream_[C]) setReadDeadline() error {
	// TODO
	return nil
}
func (s *http3Stream_[C]) setWriteDeadline() error {
	// TODO
	return nil
}

func (s *http3Stream_[C]) read(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) readFull(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) write(src []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *http3Stream_[C]) writev(srcVec *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

// _http3In_ is a mixin.
type _http3In_ struct { // for server3Request and backend3Response
	// Parent
	*_httpIn_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *_http3In_) onUse(parent *_httpIn_) {
	r._httpIn_ = parent
}
func (r *_http3In_) onEnd() {
	r._httpIn_ = nil
}

func (r *_http3In_) _growHeaders(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *_http3In_) readContent() (data []byte, err error) {
	// TODO
	return
}
func (r *_http3In_) _readSizedContent() ([]byte, error) {
	// r.stream.setReadDeadline() // may be called multiple times during the reception of the sized content
	return nil, nil
}
func (r *_http3In_) _readVagueContent() ([]byte, error) {
	// r.stream.setReadDeadline() // may be called multiple times during the reception of the vague content
	return nil, nil
}

// _http3Out_ is a mixin.
type _http3Out_ struct { // for server3Response and backend3Request
	// Parent
	*_httpOut_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *_http3Out_) onUse(parent *_httpOut_) {
	r._httpOut_ = parent
}
func (r *_http3Out_) onEnd() {
	r._httpOut_ = nil
}

func (r *_http3Out_) addHeader(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_http3Out_) header(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_http3Out_) hasHeader(name []byte) bool {
	// TODO
	return false
}
func (r *_http3Out_) delHeader(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *_http3Out_) delHeaderAt(i uint8) {
	// TODO
}

func (r *_http3Out_) sendChain() error {
	// TODO
	return nil
}
func (r *_http3Out_) _sendEntireChain() error {
	// TODO
	return nil
}
func (r *_http3Out_) _sendSingleRange() error {
	// TODO
	return nil
}
func (r *_http3Out_) _sendMultiRanges() error {
	// TODO
	return nil
}

func (r *_http3Out_) echoChain() error {
	// TODO
	return nil
}

func (r *_http3Out_) addTrailer(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_http3Out_) trailer(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_http3Out_) trailers() []byte {
	// TODO
	return nil
}

func (r *_http3Out_) proxyPassBytes(data []byte) error { return r.writeBytes(data) }

func (r *_http3Out_) finalizeVague() error {
	// TODO
	if r.numTrailerFields == 1 { // no trailer section
	} else { // with trailer section
	}
	return nil
}

func (r *_http3Out_) writeHeaders() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that header fields are all sent, r.fields will be used by trailer fields (if any), so reset it.
	return nil
}
func (r *_http3Out_) writePiece(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *_http3Out_) _writeTextPiece(piece *Piece) error {
	// TODO
	return nil
}
func (r *_http3Out_) _writeFilePiece(piece *Piece) error {
	// TODO
	// r.stream.setWriteDeadline() // for _writeFilePiece
	// r.stream.write() or r.stream.writev()
	return nil
}
func (r *_http3Out_) writeVector() error {
	// TODO
	// r.stream.setWriteDeadline() // for writeVector
	// r.stream.writev()
	return nil
}
func (r *_http3Out_) writeBytes(data []byte) error {
	// TODO
	// r.stream.setWriteDeadline() // for writeBytes
	// r.stream.write()
	return nil
}

// _http3Socket_ is a mixin.
type _http3Socket_ struct { // for server3Socket and backend3Socket
	// Parent
	*_httpSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *_http3Socket_) onUse(parent *_httpSocket_) {
	s._httpSocket_ = parent
}
func (s *_http3Socket_) onEnd() {
	s._httpSocket_ = nil
}

func (s *_http3Socket_) todo3() {
	s.todo()
}
