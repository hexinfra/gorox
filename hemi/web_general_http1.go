// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1.x types. See RFC 9112.

package hemi

import (
	"bytes"
	"io"
	"net"
	"syscall"
	"time"
)

// http1Conn
type http1Conn interface { // for *backend1Conn and *server1Conn
	// Imports
	httpConn
	// Methods
	setReadDeadline() error
	setWriteDeadline() error
	read(dst []byte) (int, error)
	readFull(dst []byte) (int, error)
	write(src []byte) (int, error)
	writev(srcVec *net.Buffers) (int64, error)
}

// http1Conn_ is a parent.
type http1Conn_ struct { // for backend1Conn and server1Conn
	// Parent
	httpConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn    net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn    syscall.RawConn // for syscall, only usable when netConn is TCP/UDS
	persistent bool            // keep the connection after current stream? true by default, will be changed by "connection: close" header field received from the remote side
	// Conn states (zeros)
	streamID int64 // next stream id
}

func (c *http1Conn_) onGet(id int64, holder holder, netConn net.Conn, rawConn syscall.RawConn) {
	c.httpConn_.onGet(id, holder)

	c.netConn = netConn
	c.rawConn = rawConn
	c.persistent = true
}
func (c *http1Conn_) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.streamID = 0

	c.httpConn_.onPut()
}

func (c *http1Conn_) nextStreamID() int64 {
	c.streamID++
	return c.streamID
}

func (c *http1Conn_) remoteAddr() net.Addr { return c.netConn.RemoteAddr() }

func (c *http1Conn_) setReadDeadline() error {
	if deadline := time.Now().Add(c.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *http1Conn_) setWriteDeadline() error {
	if deadline := time.Now().Add(c.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *http1Conn_) read(dst []byte) (int, error)              { return c.netConn.Read(dst) }
func (c *http1Conn_) readFull(dst []byte) (int, error)          { return io.ReadFull(c.netConn, dst) }
func (c *http1Conn_) write(src []byte) (int, error)             { return c.netConn.Write(src) }
func (c *http1Conn_) writev(srcVec *net.Buffers) (int64, error) { return srcVec.WriteTo(c.netConn) }

// http1Stream
type http1Stream interface {
	// Imports
	httpStream
	// Methods
}

// http1Stream_ is a parent.
type http1Stream_[C http1Conn] struct { // for backend1Stream and server1Stream
	// Parent
	httpStream_[C]
	// Assocs
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	id int64 // the stream id
	// Stream states (zeros)
}

func (s *http1Stream_[C]) onUse(conn C, id int64) {
	s.httpStream_.onUse(conn)
	s.id = id
}
func (s *http1Stream_[C]) onEnd() {
	s.httpStream_.onEnd()
}

func (s *http1Stream_[C]) ID() int64 { return s.id }

func (s *http1Stream_[C]) markBroken()    { s.conn.markBroken() }
func (s *http1Stream_[C]) isBroken() bool { return s.conn.isBroken() }

func (s *http1Stream_[C]) setReadDeadline() error  { return s.conn.setReadDeadline() }
func (s *http1Stream_[C]) setWriteDeadline() error { return s.conn.setWriteDeadline() }

func (s *http1Stream_[C]) read(dst []byte) (int, error)     { return s.conn.read(dst) }
func (s *http1Stream_[C]) readFull(dst []byte) (int, error) { return s.conn.readFull(dst) }
func (s *http1Stream_[C]) write(src []byte) (int, error)    { return s.conn.write(src) }
func (s *http1Stream_[C]) writev(srcVec *net.Buffers) (int64, error) {
	return s.conn.writev(srcVec)
}

// _http1In_ is a mixin.
type _http1In_ struct { // for backend1Response and server1Request
	// Parent
	*_httpIn_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *_http1In_) onUse(parent *_httpIn_) {
	r._httpIn_ = parent
}
func (r *_http1In_) onEnd() {
	r._httpIn_ = nil
}

func (r *_http1In_) growHead() bool { // HTTP/1.x is not a binary protocol, we don't know how many bytes to grow, so just grow.
	// Is r.input full?
	if inputSize := int32(cap(r.input)); r.inputEdge == inputSize { // r.inputEdge reached end, so r.input is full
		if inputSize == _16K { // max r.input size is 16K, we cannot use a larger input anymore
			if r.receiving == httpSectionControl {
				r.headResult = StatusURITooLong
			} else { // httpSectionHeaders
				r.headResult = StatusRequestHeaderFieldsTooLarge
			}
			return false
		}
		// r.input size < 16K. We switch to a larger input (stock -> 4K -> 16K)
		stockSize := int32(cap(r.stockInput))
		var input []byte
		if inputSize == stockSize {
			input = Get4K()
		} else { // 4K
			input = Get16K()
		}
		copy(input, r.input) // copy all
		if inputSize != stockSize {
			PutNK(r.input)
		}
		r.input = input // a larger input is now used
	}
	// r.input is not full.
	if n, err := r.stream.read(r.input[r.inputEdge:]); err == nil {
		r.inputEdge += int32(n) // we might have only read 1 byte.
		return true
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		r.headResult = StatusRequestTimeout
	} else { // i/o error or unexpected EOF
		r.headResult = -1
	}
	return false
}
func (r *_http1In_) recvHeaderLines() bool { // *( field-name ":" OWS field-value OWS CRLF ) CRLF
	r.headerLines.from = uint8(len(r.primes))
	r.headerLines.edge = r.headerLines.from
	headerLine := &r.mainPair
	headerLine.zero()
	headerLine.kind = pairHeader
	headerLine.place = placeInput // all received header lines are in r.input
	// r.elemFore is at header section (if any) or end of header section (if none).
	for { // each header line
		// End of header section?
		if b := r.input[r.elemFore]; b == '\r' {
			// Skip '\r'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
				return false
			}
			if r.input[r.elemFore] != '\n' {
				r.headResult, r.failReason = StatusBadRequest, "bad end of header section"
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		// field-line = field-name ":" OWS field-value OWS

		// field-name = token
		// token = 1*tchar

		r.elemBack = r.elemFore // now r.elemBack is at field-line
		for {
			b := r.input[r.elemFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.elemFore] = b
			} else if t == 3 { // '_'
				headerLine.setUnderscore()
			} else if b == ':' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			headerLine.nameHash += uint16(b)
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		if nameSize := r.elemFore - r.elemBack; nameSize > 0 && nameSize <= 255 {
			headerLine.nameFrom, headerLine.nameSize = r.elemBack, uint8(nameSize)
		} else {
			r.headResult, r.failReason = StatusBadRequest, "header name out of range"
			return false
		}
		// Skip ':'
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.input[r.elemFore] == ' ' || r.input[r.elemFore] == '\t' {
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		// field-value   = *field-content
		// field-content = field-vchar [ 1*( %x20 / %x09 / field-vchar) field-vchar ]
		// field-vchar   = %x21-7E / %x80-FF
		// In other words, a string of octets is a field-value if and only if:
		// - it is *( %x21-7E / %x80-FF / %x20 / %x09)
		// - if it is not empty, it starts and ends with field-vchar
		r.elemBack = r.elemFore // now r.elemBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.elemFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
					return false
				}
				if r.input[r.elemFore] != '\n' {
					r.headResult, r.failReason = StatusBadRequest, "header value contains bad eol"
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header value contains bad character"
				return false
			}
		}
		// r.elemFore is at '\n'
		fore := r.elemFore
		if r.input[fore-1] == '\r' {
			fore--
		}
		if fore > r.elemBack { // field-value is not empty. now trim OWS after field-value
			for r.input[fore-1] == ' ' || r.input[fore-1] == '\t' {
				fore--
			}
		}
		headerLine.value.set(r.elemBack, fore)

		// Header line is received in general algorithm. Now add it
		if !r.addHeaderLine(headerLine) {
			// r.headResult is set.
			return false
		}

		// Header line is successfully received. Skip '\n'
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
			return false
		}
		// r.elemFore is now at the next header line or end of header section.
		headerLine.nameHash, headerLine.flags = 0, 0 // reset for next header line
	}
	r.receiving = httpSectionContent
	// Skip end of header section
	r.elemFore++
	// Now the head is received, and r.elemFore is at the beginning of content (if exists) or next message (if exists and is pipelined).
	r.head.set(0, r.elemFore)

	return true
}

func (r *_http1In_) readContent() (data []byte, err error) {
	if r.contentSize >= 0 { // sized
		return r._readSizedContent()
	} else { // vague. must be -2. -1 (no content) is excluded priorly
		return r._readVagueContent()
	}
}
func (r *_http1In_) _readSizedContent() ([]byte, error) {
	if r.receivedSize == r.contentSize { // content is entirely received
		if r.bodyWindow == nil { // body window is not used. this means content is immediate
			return r.contentText[:r.receivedSize], io.EOF
		} else { // r.bodyWindow was used.
			PutNK(r.bodyWindow)
			r.bodyWindow = nil
			return nil, io.EOF
		}
	}
	// Need more content text.
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends. must be >= 16K so r.imme can fit
	}
	if r.imme.notEmpty() {
		immeSize := copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge]) // r.input is not larger than r.bodyWindow
		r.receivedSize = int64(immeSize)
		r.imme.zero()
		return r.bodyWindow[0:immeSize], nil
	}
	readSize := int64(cap(r.bodyWindow))
	if sizeLeft := r.contentSize - r.receivedSize; sizeLeft < readSize {
		readSize = sizeLeft
	}
	if r.bodyTime.IsZero() {
		r.bodyTime = time.Now()
	}
	if err := r.stream.setReadDeadline(); err != nil { // may be called multiple times during the reception of the sized content
		return nil, err
	}
	size, err := r.stream.readFull(r.bodyWindow[:readSize])
	if err == nil {
		if !r._isLongTime() {
			r.receivedSize += int64(size)
			return r.bodyWindow[:size], nil
		}
		err = httpInLongTime
	}
	return nil, err
}
func (r *_http1In_) _readVagueContent() ([]byte, error) {
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends. 16K is a tradeoff between performance and memory consumption, and can fit r.imme and trailer section
	}
	if r.imme.notEmpty() {
		r.chunkEdge = int32(copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge])) // r.input is not larger than r.bodyWindow
		r.imme.zero()
	}
	if r.chunkEdge == 0 && !r.growChunked() { // r.bodyWindow is empty. must fill
		goto badRead
	}
	switch r.chunkSize { // size left in receiving current chunk
	case -2: // got chunk-data. needs CRLF or LF
		if r.bodyWindow[r.chunkFore] == '\r' {
			if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked() {
				goto badRead
			}
		}
		fallthrough
	case -1: // got chunk-data CR. needs LF
		if r.bodyWindow[r.chunkFore] != '\n' {
			goto badRead
		}
		// Skip '\n'
		if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked() {
			goto badRead
		}
		fallthrough
	case 0: // start a new chunk = chunk-size [chunk-ext] CRLF chunk-data CRLF
		r.chunkBack = r.chunkFore // now r.bodyWindow is used for receiving: chunk-size [chunk-ext] CRLF
		chunkSize := int64(0)
		for { // chunk-size = 1*HEXDIG
			b := r.bodyWindow[r.chunkFore]
			if b >= '0' && b <= '9' {
				b = b - '0'
			} else if b >= 'a' && b <= 'f' {
				b = b - 'a' + 10
			} else if b >= 'A' && b <= 'F' {
				b = b - 'A' + 10
			} else {
				break
			}
			chunkSize <<= 4
			chunkSize += int64(b)
			if r.chunkFore++; r.chunkFore-r.chunkBack >= 16 || (r.chunkFore == r.chunkEdge && !r.growChunked()) {
				goto badRead
			}
		}
		if chunkSize < 0 { // bad chunk size.
			goto badRead
		}
		if b := r.bodyWindow[r.chunkFore]; b == ';' { // ignore chunk-ext = *( ";" chunk-ext-name [ "=" chunk-ext-val ] )
			for r.bodyWindow[r.chunkFore] != '\n' {
				if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked() {
					goto badRead
				}
			}
		} else if b == '\r' {
			// Skip '\r'
			if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked() {
				goto badRead
			}
		}
		// Must be LF
		if r.bodyWindow[r.chunkFore] != '\n' {
			goto badRead
		}
		// Check target size
		if targetSize := r.receivedSize + chunkSize; targetSize >= 0 && targetSize <= r.maxContentSize {
			r.chunkSize = chunkSize
		} else { // invalid target size.
			// TODO: log error?
			goto badRead
		}
		// Skip '\n' at the end of: chunk-size [chunk-ext] CRLF
		if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked() {
			goto badRead
		}
		// Last chunk?
		if r.chunkSize == 0 { // last-chunk = 1*("0") [chunk-ext] CRLF
			// last-chunk trailer-section CRLF
			if r.bodyWindow[r.chunkFore] == '\r' {
				// Skip '\r'
				if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked() {
					goto badRead
				}
				if r.bodyWindow[r.chunkFore] != '\n' {
					goto badRead
				}
			} else if r.bodyWindow[r.chunkFore] != '\n' { // must be trailer-section = *( field-line CRLF)
				r.receiving = httpSectionTrailers
				if !r.recvTrailerLines() || !r.in.examineTail() {
					goto badRead
				}
				// r.recvTrailerLines() must ends with r.chunkFore being at the last '\n' after trailer-section.
			}
			// Skip the last '\n'
			r.chunkFore++ // now the whole vague content is received and r.chunkFore is immediately after the vague content.
			// Now we have found the end of current message, so determine r.inputNext and r.inputEdge.
			if r.chunkFore < r.chunkEdge { // still has data, stream is pipelined
				r.overChunked = true                                // so r.bodyWindow will be used as r.input on stream ends
				r.inputNext, r.inputEdge = r.chunkFore, r.chunkEdge // mark the next message
			} else { // no data anymore, stream is not pipelined
				r.inputNext, r.inputEdge = 0, 0 // reset input
				PutNK(r.bodyWindow)
				r.bodyWindow = nil
			}
			return nil, io.EOF
		}
		// Not last chunk, now r.chunkFore is at the beginning of: chunk-data CRLF
		fallthrough
	default: // r.chunkSize > 0, we are receiving: chunk-data CRLF
		r.chunkBack = 0 // so growChunked() works correctly
		var data span   // the chunk data we are receiving
		data.from = r.chunkFore
		if haveSize := int64(r.chunkEdge - r.chunkFore); haveSize <= r.chunkSize { // 1 <= haveSize <= r.chunkSize. chunk-data can be taken entirely
			r.receivedSize += haveSize
			data.edge = r.chunkEdge
			if haveSize == r.chunkSize { // exact chunk-data
				r.chunkSize = -2 // got chunk-data, needs CRLF or LF
			} else { // haveSize < r.chunkSize, not enough data.
				r.chunkSize -= haveSize
			}
			r.chunkFore, r.chunkEdge = 0, 0 // all data taken
		} else { // haveSize > r.chunkSize, more than chunk-data
			r.receivedSize += r.chunkSize
			data.edge = r.chunkFore + int32(r.chunkSize)
			if sizeLeft := r.chunkEdge - data.edge; sizeLeft == 1 { // chunk-data ?
				if b := r.bodyWindow[data.edge]; b == '\r' { // exact chunk-data CR
					r.chunkSize = -1 // got chunk-data CR, needs LF
				} else if b == '\n' { // exact chunk-data LF
					r.chunkSize = 0
				} else { // chunk-data X
					goto badRead
				}
				r.chunkFore, r.chunkEdge = 0, 0 // all data taken
			} else if r.bodyWindow[data.edge] == '\r' && r.bodyWindow[data.edge+1] == '\n' { // chunk-data CRLF..
				r.chunkSize = 0
				if sizeLeft == 2 { // exact chunk-data CRLF
					r.chunkFore, r.chunkEdge = 0, 0 // all data taken
				} else { // > 2, chunk-data CRLF X
					r.chunkFore = data.edge + 2
				}
			} else if r.bodyWindow[data.edge] == '\n' { // >=2, chunk-data LF X
				r.chunkSize = 0
				r.chunkFore = data.edge + 1
			} else { // >=2, chunk-data XX
				goto badRead
			}
		}
		return r.bodyWindow[data.from:data.edge], nil
	}
badRead:
	return nil, httpInBadChunk
}

func (r *_http1In_) recvTrailerLines() bool { // trailer-section = *( field-line CRLF)
	copy(r.bodyWindow, r.bodyWindow[r.chunkFore:r.chunkEdge]) // slide to start, we need a clean r.bodyWindow
	r.chunkEdge -= r.chunkFore
	r.chunkBack, r.chunkFore = 0, 0 // setting r.chunkBack = 0 means r.bodyWindow will not slide, so the whole trailer section must fit in r.bodyWindow.
	r.elemBack, r.elemFore = 0, 0   // for parsing trailer fields

	r.trailerLines.from = uint8(len(r.primes))
	r.trailerLines.edge = r.trailerLines.from
	trailerLine := &r.mainPair
	trailerLine.zero()
	trailerLine.kind = pairTrailer
	trailerLine.place = placeArray // all received trailer lines are placed in r.array
	for {
		if b := r.bodyWindow[r.elemFore]; b == '\r' {
			// Skip '\r'
			if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
				return false
			}
			if r.bodyWindow[r.elemFore] != '\n' {
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		r.elemBack = r.elemFore // for field-name
		for {
			b := r.bodyWindow[r.elemFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.bodyWindow[r.elemFore] = b
			} else if t == 3 { // '_'
				trailerLine.setUnderscore()
			} else if b == ':' {
				break
			} else {
				return false
			}
			trailerLine.nameHash += uint16(b)
			if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
				return false
			}
		}
		if nameSize := r.elemFore - r.elemBack; nameSize > 0 && nameSize <= 255 {
			trailerLine.nameFrom, trailerLine.nameSize = r.elemBack, uint8(nameSize)
		} else {
			return false
		}
		// Skip ':'
		if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.bodyWindow[r.elemFore] == ' ' || r.bodyWindow[r.elemFore] == '\t' {
			if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
				return false
			}
		}
		r.elemBack = r.elemFore // for field-value or EOL
		for {
			if b := r.bodyWindow[r.elemFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
					return false
				}
				if r.bodyWindow[r.elemFore] != '\n' {
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				return false
			}
		}
		// r.elemFore is at '\n'
		fore := r.elemFore
		if r.bodyWindow[fore-1] == '\r' {
			fore--
		}
		if fore > r.elemBack { // field-value is not empty. now trim OWS after field-value
			for r.bodyWindow[fore-1] == ' ' || r.bodyWindow[fore-1] == '\t' {
				fore--
			}
		}
		trailerLine.value.set(r.elemBack, fore)

		// Copy trailer line data to r.array
		fore = r.arrayEdge
		if !r.arrayCopy(trailerLine.nameAt(r.bodyWindow)) {
			return false
		}
		trailerLine.nameFrom = fore
		fore = r.arrayEdge
		if !r.arrayCopy(trailerLine.valueAt(r.bodyWindow)) {
			return false
		}
		trailerLine.value.set(fore, r.arrayEdge)

		// Trailer line is received in general algorithm. Now add it
		if !r.addTrailerLine(trailerLine) {
			return false
		}

		// Trailer line is successfully received. Skip '\n'
		if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked() {
			return false
		}
		// r.elemFore is now at the next trailer line or end of trailer section.
		trailerLine.nameHash, trailerLine.flags = 0, 0 // reset for next trailer line
	}
	r.chunkFore = r.elemFore // r.chunkFore must ends at the last '\n'
	return true
}
func (r *_http1In_) growChunked() bool { // HTTP/1.x is not a binary protocol, we don't know how many bytes to grow, so just grow.
	if r.chunkEdge == int32(cap(r.bodyWindow)) && r.chunkBack == 0 { // r.bodyWindow is full and we can't slide
		return false // element is too large
	}
	if r.chunkBack > 0 { // has previously used data, but now useless. slide to start so we can read more
		copy(r.bodyWindow, r.bodyWindow[r.chunkBack:r.chunkEdge])
		r.chunkEdge -= r.chunkBack
		r.chunkFore -= r.chunkBack
		r.chunkBack = 0
	}
	if r.bodyTime.IsZero() {
		r.bodyTime = time.Now()
	}
	err := r.stream.setReadDeadline() // may be called multiple times during the reception of the vague content
	if err == nil {
		n, e := r.stream.read(r.bodyWindow[r.chunkEdge:])
		r.chunkEdge += int32(n)
		if e == nil {
			if !r._isLongTime() {
				return true
			}
			e = httpInLongTime
		}
		err = e // including io.EOF which is unexpected here
	}
	// err != nil. TODO: log err
	return false
}

// _http1Out_ is a mixin.
type _http1Out_ struct { // for backend1Request and server1Response
	// Parent
	*_httpOut_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *_http1Out_) onUse(parent *_httpOut_) {
	r._httpOut_ = parent
}
func (r *_http1Out_) onEnd() {
	r._httpOut_ = nil
}

func (r *_http1Out_) addHeader(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	headerSize := len(name) + len(bytesColonSpace) + len(value) + len(bytesCRLF) // name: value\r\n
	if from, _, ok := r.growHeaders(headerSize); ok {
		from += copy(r.output[from:], name)
		r.output[from] = ':'
		r.output[from+1] = ' '
		from += 2
		from += copy(r.output[from:], value)
		r._addCRLFHeader(from)
		return true
	} else {
		return false
	}
}
func (r *_http1Out_) header(name []byte) (value []byte, ok bool) {
	if r.numHeaderFields > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.numHeaderFields; i++ {
			edge := r.edges[i]
			header := r.output[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return header[p+len(bytesColonSpace) : len(header)-len(bytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *_http1Out_) hasHeader(name []byte) bool {
	if r.numHeaderFields > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.numHeaderFields; i++ {
			edge := r.edges[i]
			header := r.output[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return true
			}
			from = edge
		}
	}
	return false
}
func (r *_http1Out_) delHeader(name []byte) (deleted bool) {
	from := uint16(0)
	for i := uint8(1); i < r.numHeaderFields; {
		edge := r.edges[i]
		if p := bytes.IndexByte(r.output[from:edge], ':'); bytes.Equal(r.output[from:from+uint16(p)], name) {
			size := edge - from
			copy(r.output[from:], r.output[edge:])
			for j := i + 1; j < r.numHeaderFields; j++ {
				r.edges[j] -= size
			}
			r.outputEdge -= size
			r.numHeaderFields--
			deleted = true
		} else {
			from = edge
			i++
		}
	}
	return
}
func (r *_http1Out_) delHeaderAt(i uint8) {
	if i == 0 {
		BugExitln("delHeaderAt: i == 0 which must not happen")
	}
	from := r.edges[i-1]
	edge := r.edges[i]
	size := edge - from
	copy(r.output[from:], r.output[edge:])
	for j := i + 1; j < r.numHeaderFields; j++ {
		r.edges[j] -= size
	}
	r.outputEdge -= size
	r.numHeaderFields--
}
func (r *_http1Out_) _addCRLFHeader(from int) {
	r.output[from] = '\r'
	r.output[from+1] = '\n'
	r.edges[r.numHeaderFields] = uint16(from + 2)
	r.numHeaderFields++
}
func (r *_http1Out_) _addFixedHeader(name []byte, value []byte) { // used by finalizeHeaders
	r.outputEdge += uint16(copy(r.output[r.outputEdge:], name))
	r.output[r.outputEdge] = ':'
	r.output[r.outputEdge+1] = ' '
	r.outputEdge += 2
	r.outputEdge += uint16(copy(r.output[r.outputEdge:], value))
	r.output[r.outputEdge] = '\r'
	r.output[r.outputEdge+1] = '\n'
	r.outputEdge += 2
}

func (r *_http1Out_) sendChain() error { // TODO: if conn is TLS, don't use writev as it uses many Write() which might be slower than make+copy+write.
	return r._sendEntireChain()
	// TODO
	numRanges := len(r.contentRanges)
	if numRanges == 0 {
		return r._sendEntireChain()
	}
	// Partial content.
	if !r.asRequest { // as response
		r.out.(ServerResponse).SetStatus(StatusPartialContent)
	}
	if numRanges == 1 {
		return r._sendSingleRange()
	} else {
		return r._sendMultiRanges()
	}
}
func (r *_http1Out_) _sendEntireChain() error {
	r.out.finalizeHeaders()
	vector := r._prepareVector() // waiting to write
	if DebugLevel() >= 2 {
		if r.asRequest {
			Printf("[backend1Stream=%d]<=======[%s%s%s]\n", r.stream.ID(), vector[0], vector[1], vector[2])
		} else {
			Printf("[server1Stream=%d]------->[%s%s%s]\n", r.stream.ID(), vector[0], vector[1], vector[2])
		}
	}
	vectorFrom, vectorEdge := 0, 3
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if piece.size == 0 {
			continue
		}
		if piece.IsText() { // plain text
			vector[vectorEdge] = piece.Text()
			vectorEdge++
		} else if piece.size <= _16K { // small file, <= 16K
			buffer := GetNK(piece.size) // 4K/16K
			if err := piece.copyTo(buffer); err != nil {
				r.stream.markBroken()
				PutNK(buffer)
				return err
			}
			vector[vectorEdge] = buffer[0:piece.size]
			vectorEdge++
			r.vector = vector[vectorFrom:vectorEdge]
			if err := r.writeVector(); err != nil {
				PutNK(buffer)
				return err
			}
			PutNK(buffer)
			vectorFrom, vectorEdge = 0, 0
		} else { // large file, > 16K
			if vectorFrom < vectorEdge {
				r.vector = vector[vectorFrom:vectorEdge]
				if err := r.writeVector(); err != nil { // texts
					return err
				}
				vectorFrom, vectorEdge = 0, 0
			}
			if err := r.writePiece(piece, false); err != nil { // the file
				return err
			}
		}
	}
	if vectorFrom < vectorEdge {
		r.vector = vector[vectorFrom:vectorEdge]
		return r.writeVector()
	}
	return nil
}
func (r *_http1Out_) _sendSingleRange() error {
	r.AddContentType(r.rangeType)
	valueBuffer := r.stream.buffer256()
	n := copy(valueBuffer, "bytes ")
	contentRange := r.contentRanges[0]
	n += i64ToDec(contentRange.From, valueBuffer[n:])
	valueBuffer[n] = '-'
	n++
	n += i64ToDec(contentRange.Last-1, valueBuffer[n:])
	valueBuffer[n] = '/'
	n++
	n += i64ToDec(r.contentSize, valueBuffer[n:])
	r.AddHeaderBytes(bytesContentRange, valueBuffer[:n])
	//return r._sendEntireChain()
	return nil
}
func (r *_http1Out_) _sendMultiRanges() error {
	valueBuffer := r.stream.buffer256()
	n := copy(valueBuffer, "multipart/byteranges; boundary=")
	n += copy(valueBuffer[n:], "xsd3lxT9b5c")
	r.AddHeaderBytes(bytesContentType, valueBuffer[:n])
	// TODO
	return nil
}
func (r *_http1Out_) _prepareVector() [][]byte {
	var vector [][]byte // waiting for write
	if r.forbidContent {
		vector = r.fixedVector[0:3]
		r.chain.free()
	} else if numPieces := r.chain.Qnty(); numPieces == 1 { // content chain has exactly one piece
		vector = r.fixedVector[0:4]
	} else { // numPieces >= 2
		vector = make([][]byte, 3+numPieces) // TODO(diogin): get from pool? defer pool.put()
	}
	vector[0] = r.out.controlData()
	vector[1] = r.out.addedHeaders()
	vector[2] = r.out.fixedHeaders()
	return vector
}

func (r *_http1Out_) echoChain(inChunked bool) error { // TODO: coalesce text pieces?
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if err := r.writePiece(piece, inChunked); err != nil {
			return err
		}
	}
	return nil
}

func (r *_http1Out_) addTrailer(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	trailerSize := len(name) + len(bytesColonSpace) + len(value) + len(bytesCRLF) // name: value\r\n
	if from, _, ok := r.growTrailers(trailerSize); ok {
		from += copy(r.output[from:], name)
		r.output[from] = ':'
		r.output[from+1] = ' '
		from += 2
		from += copy(r.output[from:], value)
		r.output[from] = '\r'
		r.output[from+1] = '\n'
		r.edges[r.numTrailerFields] = uint16(from + 2)
		r.numTrailerFields++
		return true
	} else {
		return false
	}
}
func (r *_http1Out_) trailer(name []byte) (value []byte, ok bool) {
	if r.numTrailerFields > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.numTrailerFields; i++ {
			edge := r.edges[i]
			trailer := r.output[from:edge]
			if p := bytes.IndexByte(trailer, ':'); p != -1 && bytes.Equal(trailer[0:p], name) {
				return trailer[p+len(bytesColonSpace) : len(trailer)-len(bytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *_http1Out_) trailers() []byte { return r.output[0:r.outputEdge] } // Header fields and trailer fields are not manipulated at the same time, so after header fields is sent, r.output is used by trailer fields.

func (r *_http1Out_) proxyPassBytes(data []byte) error { return r.writeBytes(data) }

func (r *_http1Out_) finalizeVague() error {
	if r.numTrailerFields == 1 { // no trailer section
		return r.writeBytes(http1BytesZeroCRLFCRLF) // 0\r\n\r\n
	} else { // with trailer section
		r.vector = r.fixedVector[0:3]
		r.vector[0] = http1BytesZeroCRLF // 0\r\n
		r.vector[1] = r.trailers()       // field-name: field-value\r\n
		r.vector[2] = bytesCRLF          // \r\n
		return r.writeVector()
	}
}

func (r *_http1Out_) writeHeaders() error { // used by echo and pass
	r.out.finalizeHeaders()
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.out.controlData()
	r.vector[1] = r.out.addedHeaders()
	r.vector[2] = r.out.fixedHeaders()
	if DebugLevel() >= 2 {
		if r.asRequest {
			Printf("[backend1Stream=%d]<=======", r.stream.ID(), r.vector[0], r.vector[1], r.vector[2])
		} else {
			Printf("[server1Stream=%d]------->[%s%s%s]", r.stream.ID(), r.vector[0], r.vector[1], r.vector[2])
		}
	}
	if err := r.writeVector(); err != nil {
		return err
	}
	r.outputEdge = 0 // now that header fields are all sent, r.output will be used by trailer fields (if any), so reset it.
	return nil
}
func (r *_http1Out_) writePiece(piece *Piece, inChunked bool) error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if piece.IsText() { // text piece
		return r._writeTextPiece(piece, inChunked)
	} else {
		return r._writeFilePiece(piece, inChunked)
	}
}
func (r *_http1Out_) _writeTextPiece(piece *Piece, inChunked bool) error {
	if inChunked { // HTTP/1.1 chunked data
		sizeBuffer := r.stream.buffer256() // enough for chunk size
		n := i64ToHex(piece.size, sizeBuffer)
		sizeBuffer[n] = '\r'
		sizeBuffer[n+1] = '\n'
		n += 2
		r.vector = r.fixedVector[0:3] // we reuse r.vector and r.fixedVector
		r.vector[0] = sizeBuffer[:n]
		r.vector[1] = piece.Text()
		r.vector[2] = bytesCRLF
		return r.writeVector()
	} else { // HTTP/1.0, or raw data
		return r.writeBytes(piece.Text())
	}
}
func (r *_http1Out_) _writeFilePiece(piece *Piece, inChunked bool) error {
	// file piece. currently we don't use the sendfile(2) syscall, maybe in the future we'll find a way to use it for better performance.
	buffer := Get16K() // 16K is a tradeoff between performance and memory consumption.
	defer PutNK(buffer)
	sizeRead := int64(0)
	for {
		if sizeRead == piece.size {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := piece.size - sizeRead; sizeLeft < readSize {
			readSize = sizeLeft
		}
		n, err := piece.file.ReadAt(buffer[:readSize], sizeRead)
		sizeRead += int64(n)
		if err != nil && sizeRead != piece.size {
			r.stream.markBroken()
			return err
		}
		if r.sendTime.IsZero() {
			r.sendTime = time.Now()
		}
		if err = r.stream.setWriteDeadline(); err != nil { // for _writeFilePiece
			r.stream.markBroken()
			return err
		}
		if inChunked { // use HTTP/1.1 chunked mode
			sizeBuffer := r.stream.buffer256() // enough for chunk size
			k := i64ToHex(int64(n), sizeBuffer)
			sizeBuffer[k] = '\r'
			sizeBuffer[k+1] = '\n'
			k += 2
			r.vector = r.fixedVector[0:3]
			r.vector[0] = sizeBuffer[:k]
			r.vector[1] = buffer[:n]
			r.vector[2] = bytesCRLF
			_, err = r.stream.writev(&r.vector)
		} else { // HTTP/1.0, or identity content
			_, err = r.stream.write(buffer[0:n])
		}
		if err = r._longTimeCheck(err); err != nil {
			return err
		}
	}
}
func (r *_http1Out_) writeVector() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if len(r.vector) == 1 && len(r.vector[0]) == 0 { // empty data
		return nil
	}
	if r.sendTime.IsZero() {
		r.sendTime = time.Now()
	}
	if err := r.stream.setWriteDeadline(); err != nil { // for writeVector
		r.stream.markBroken()
		return err
	}
	_, err := r.stream.writev(&r.vector)
	return r._longTimeCheck(err)
}
func (r *_http1Out_) writeBytes(data []byte) error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if len(data) == 0 { // empty data
		return nil
	}
	if r.sendTime.IsZero() {
		r.sendTime = time.Now()
	}
	if err := r.stream.setWriteDeadline(); err != nil { // for writeBytes
		r.stream.markBroken()
		return err
	}
	_, err := r.stream.write(data)
	return r._longTimeCheck(err)
}

// _http1Socket_ is a mixin.
type _http1Socket_ struct { // for backend1Socket and server1Socket
	// Parent
	*_httpSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *_http1Socket_) onUse(parent *_httpSocket_) {
	s._httpSocket_ = parent
}
func (s *_http1Socket_) onEnd() {
	s._httpSocket_ = nil
}

func (s *_http1Socket_) todo1() {
	s.todo()
}
