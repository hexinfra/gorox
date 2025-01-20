// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/3 mixins. See RFC 9114 and RFC 9204.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
)

//////////////////////////////////////// HTTP/3 general implementation ////////////////////////////////////////

// http3Conn collects shared methods between *server3Conn and *backend3Conn.
type http3Conn interface {
	// Imports
	httpConn
	// Methods
}

// _http3Conn_ is the parent for server3Conn and backend3Conn.
type _http3Conn_ struct {
	// Parent
	_httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *tcp2.Conn        // the quic connection
	inBuffer *http3InBuffer    // ...
	table    http3DynamicTable // ...
	// Conn states (zeros)
	activeStreams [http3MaxConcurrentStreams]http3Stream // active (open, remoteClosed, localClosed) streams
	_http3Conn0                                          // all values in this struct must be zero by default!
}
type _http3Conn0 struct { // for fast reset, entirely
	inBufferEdge uint32 // incoming data ends at c.inBuffer.buf[c.inBufferEdge]
	partBack     uint32 // incoming frame part (header or payload) begins from c.inBuffer.buf[c.partBack]
	partFore     uint32 // incoming frame part (header or payload) ends at c.inBuffer.buf[c.partFore]
}

func (c *_http3Conn_) onGet(id int64, stage *Stage, udsMode bool, tlsMode bool, quicConn *tcp2.Conn, readTimeout time.Duration, writeTimeout time.Duration) {
	c._httpConn_.onGet(id, stage, udsMode, tlsMode, readTimeout, writeTimeout)

	c.quicConn = quicConn
	if c.inBuffer == nil {
		c.inBuffer = getHTTP3InBuffer()
		c.inBuffer.incRef()
	}
}
func (c *_http3Conn_) onPut() {
	// c.inBuffer is reserved
	// c.table is reserved
	c.activeStreams = [http3MaxConcurrentStreams]http3Stream{}
	c.quicConn = nil

	c._httpConn_.onPut()
}

func (c *_http3Conn_) remoteAddr() net.Addr { return nil } // TODO

// http3Stream collects shared methods between *server3Stream and *backend3Stream.
type http3Stream interface {
	// Imports
	httpStream
	// Methods
	getID() int64
}

// _http3Stream_ is the parent for server3Stream and backend3Stream.
type _http3Stream_[C http3Conn] struct {
	// Parent
	_httpStream_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn       C            // the http/3 connection
	quicStream *tcp2.Stream // the quic stream
	// Stream states (zeros)
	_http3Stream0 // all values in this struct must be zero by default!
}
type _http3Stream0 struct { // for fast reset, entirely
}

func (s *_http3Stream_[C]) onUse(conn C, quicStream *tcp2.Stream) {
	s._httpStream_.onUse()

	s.conn = conn
	s.quicStream = quicStream
}
func (s *_http3Stream_[C]) onEnd() {
	s._http3Stream0 = _http3Stream0{}

	// s.conn will be set as nil by upper code
	s.quicStream = nil
	s._httpStream_.onEnd()
}

func (s *_http3Stream_[C]) getID() int64 { return s.quicStream.ID() }

func (s *_http3Stream_[C]) Conn() httpConn       { return s.conn }
func (s *_http3Stream_[C]) remoteAddr() net.Addr { return s.conn.remoteAddr() }

func (s *_http3Stream_[C]) markBroken()    {}               // TODO
func (s *_http3Stream_[C]) isBroken() bool { return false } // TODO

func (s *_http3Stream_[C]) setReadDeadline() error {
	// TODO
	return nil
}
func (s *_http3Stream_[C]) setWriteDeadline() error {
	// TODO
	return nil
}

func (s *_http3Stream_[C]) read(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *_http3Stream_[C]) readFull(dst []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *_http3Stream_[C]) write(src []byte) (int, error) {
	// TODO
	return 0, nil
}
func (s *_http3Stream_[C]) writev(srcVec *net.Buffers) (int64, error) {
	// TODO
	return 0, nil
}

//////////////////////////////////////// HTTP/3 incoming implementation ////////////////////////////////////////

func (r *_httpIn_) _growHeaders3(size int32) bool {
	// TODO
	// use r.input
	return false
}

func (r *_httpIn_) readContent3() (data []byte, err error) {
	// TODO
	return
}

// http3InFrame is the HTTP/3 incoming frame.
type http3InFrame struct {
	// TODO
}

func (f *http3InFrame) zero() { *f = http3InFrame{} }

// http3InBuffer
type http3InBuffer struct {
	buf [_16K]byte // header + payload
	ref atomic.Int32
}

var poolHTTP3InBuffer sync.Pool

func getHTTP3InBuffer() *http3InBuffer {
	var inBuffer *http3InBuffer
	if x := poolHTTP3InBuffer.Get(); x == nil {
		inBuffer = new(http3InBuffer)
	} else {
		inBuffer = x.(*http3InBuffer)
	}
	return inBuffer
}
func putHTTP3InBuffer(inBuffer *http3InBuffer) { poolHTTP3InBuffer.Put(inBuffer) }

func (b *http3InBuffer) size() uint32  { return uint32(cap(b.buf)) }
func (b *http3InBuffer) getRef() int32 { return b.ref.Load() }
func (b *http3InBuffer) incRef()       { b.ref.Add(1) }
func (b *http3InBuffer) decRef() {
	if b.ref.Add(-1) == 0 {
		if DebugLevel() >= 1 {
			Printf("putHTTP3InBuffer ref=%d\n", b.ref.Load())
		}
		putHTTP3InBuffer(b)
	}
}

//////////////////////////////////////// HTTP/3 outgoing implementation ////////////////////////////////////////

func (r *_httpOut_) addHeader3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_httpOut_) header3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_httpOut_) hasHeader3(name []byte) bool {
	// TODO
	return false
}
func (r *_httpOut_) delHeader3(name []byte) (deleted bool) {
	// TODO
	return false
}
func (r *_httpOut_) delHeaderAt3(i uint8) {
	// TODO
}

func (r *_httpOut_) sendChain3() error {
	// TODO
	return nil
}
func (r *_httpOut_) _sendEntireChain3() error {
	// TODO
	return nil
}
func (r *_httpOut_) _sendSingleRange3() error {
	// TODO
	return nil
}
func (r *_httpOut_) _sendMultiRanges3() error {
	// TODO
	return nil
}

func (r *_httpOut_) echoChain3() error {
	// TODO
	return nil
}

func (r *_httpOut_) addTrailer3(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *_httpOut_) trailer3(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *_httpOut_) trailers3() []byte {
	// TODO
	return nil
}

func (r *_httpOut_) proxyPassBytes3(data []byte) error { return r.writeBytes3(data) }

func (r *_httpOut_) finalizeVague3() error {
	// TODO
	if r.numTrailers == 1 { // no trailers
	} else { // with trailers
	}
	return nil
}

func (r *_httpOut_) writeHeaders3() error { // used by echo and pass
	// TODO
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *_httpOut_) writePiece3(piece *Piece, vague bool) error {
	// TODO
	return nil
}
func (r *_httpOut_) _writeTextPiece3(piece *Piece) error {
	// TODO
	return nil
}
func (r *_httpOut_) _writeFilePiece3(piece *Piece) error {
	// TODO
	return nil
}
func (r *_httpOut_) writeVector3() error {
	// TODO
	return nil
}
func (r *_httpOut_) writeBytes3(data []byte) error {
	// TODO
	return nil
}

// http3OutFrame is the HTTP/3 outgoing frame.
type http3OutFrame struct {
	// TODO
}

func (f *http3OutFrame) zero() { *f = http3OutFrame{} }

//////////////////////////////////////// HTTP/3 webSocket implementation ////////////////////////////////////////

func (s *_httpSocket_) todo3() {
}
