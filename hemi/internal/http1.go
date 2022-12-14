// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 protocol elements, incoming message and outgoing message implementation.

package internal

import (
	"bytes"
	"errors"
	"io"
	"net"
	"time"
)

// http1InMessage_ is used by http1Request and H1Response.

func (r *httpInMessage_) growHead1() bool { // HTTP/1 is not a binary protocol, we don't know how many bytes to grow, so just grow.
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
		var input []byte
		stockSize := int32(cap(r.stockInput))
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
	} else { // i/o error
		r.headResult = -1
	}
	return false
}
func (r *httpInMessage_) recvHeaders1() bool { // *( field-name ":" OWS field-value OWS CRLF ) CRLF
	r.headers.from = uint8(len(r.primes))
	r.headers.edge = r.headers.from
	header := &r.field
	header.zero()
	header.setPlace(pairPlaceInput) // all received headers are in r.input
	// r.pFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.pFore]; b == '\r' {
			// Skip '\r'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '\n' {
				r.headResult, r.headReason = StatusBadRequest, "bad end of headers"
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		// header-field = field-name ":" OWS field-value OWS

		// field-name = token
		// token = 1*tchar
		header.hash = 0
		r.pBack = r.pFore // now r.pBack is at header-field
		for {
			b := r.input[r.pFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.pFore] = b
			} else if b == ':' {
				break
			} else {
				r.headResult, r.headReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			header.hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		size := r.pFore - r.pBack
		if size == 0 || size > 255 {
			r.headResult, r.headReason = StatusBadRequest, "header name out of range"
			return false
		}
		header.nameFrom, header.nameSize = r.pBack, uint8(size)
		// Skip ':'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}

		// Skip OWS before field-value (and OWS after field-value, if field-value is empty)
		for r.input[r.pFore] == ' ' || r.input[r.pFore] == '\t' {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}

		// field-value   = *field-content
		// field-content = field-vchar [ 1*( %x20 / %x09 / field-vchar) field-vchar ]
		// field-vchar   = %x21-7E / %x80-FF
		// In other words, a string of octets is a field-value if and only if:
		// - it is *( %x21-7E / %x80-FF / %x20 / %x09)
		// - if it is not empty, it starts and ends with field-vchar
		r.pBack = r.pFore // now r.pBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.pFore]; httpVchar[b] == 1 {
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
				if r.input[r.pFore] != '\n' {
					r.headResult, r.headReason = StatusBadRequest, "header value contains bad eol"
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				r.headResult, r.headReason = StatusBadRequest, "header value contains bad character"
				return false
			}
		}
		// r.pFore is at '\n'
		fore := r.pFore
		if r.input[fore-1] == '\r' {
			fore--
		}
		if fore > r.pBack { // field-value is not empty. now trim OWS after field-value
			for r.input[fore-1] == ' ' || r.input[fore-1] == '\t' {
				fore--
			}
			header.value.set(r.pBack, fore)
		} else {
			header.value.zero()
		}

		// Header is received in general algorithm. Now apply it
		if !r.shell.applyHeader(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
		// r.pFore is now at the next header or end of headers.
	}
	r.receiving = httpSectionContent
	// Skip end of headers
	r.pFore++
	// Now the head is received, and r.pFore is at the beginning of content (if exists) or next message (if exists and is pipelined).
	r.head.set(0, r.pFore)

	return true
}

func (r *httpInMessage_) readContent1() (p []byte, err error) {
	if r.contentSize >= 0 { // counted
		return r._readCountedContent1()
	} else { // must be -2 (chunked). -1 is excluded priorly
		return r._readChunkedContent1()
	}
}
func (r *httpInMessage_) _readCountedContent1() (p []byte, err error) {
	if r.sizeReceived == r.contentSize {
		if r.bodyWindow == nil { // body window is not used. this means content is immediate
			return r.contentBlob[:r.sizeReceived], io.EOF
		}
		PutNK(r.bodyWindow)
		r.bodyWindow = nil
		return nil, io.EOF
	}
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends
	}
	if r.imme.notEmpty() {
		size := copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge]) // r.input is not larger than r.bodyWindow
		r.sizeReceived = int64(size)
		r.imme.zero()
		return r.bodyWindow[0:size], nil
	}
	if err = r._beforeRead(&r.bodyTime); err != nil {
		return nil, err
	}
	recvSize := int64(cap(r.bodyWindow))
	if sizeLeft := r.contentSize - r.sizeReceived; sizeLeft < recvSize {
		recvSize = sizeLeft
	}
	size, err := r.stream.readFull(r.bodyWindow[:recvSize])
	r.sizeReceived += int64(size)
	if err == nil && (r.maxRecvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.maxRecvTimeout) {
		err = httpReadTooSlow
	}
	return r.bodyWindow[0:size], err
}
func (r *httpInMessage_) _readChunkedContent1() (p []byte, err error) {
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends
	}
	if r.imme.notEmpty() {
		r.chunkEdge = int32(copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge])) // r.input is not larger than r.bodyWindow
		r.imme.zero()
	}
	if r.chunkEdge == 0 && !r.growChunked1() { // r.bodyWindow is empty. must fill
		goto badRecv
	}
	switch r.chunkSize { // size left in receiving current chunk
	case -2: // got chunk-data. needs CRLF or LF
		if r.bodyWindow[r.cFore] == '\r' {
			if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
				goto badRecv
			}
		}
		fallthrough
	case -1: // got chunk-data CR. needs LF
		if r.bodyWindow[r.cFore] != '\n' {
			goto badRecv
		}
		// Skip '\n'
		if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
			goto badRecv
		}
		fallthrough
	case 0: // start a new chunk = chunk-size [chunk-ext] CRLF chunk-data CRLF
		r.cBack = r.cFore // now r.bodyWindow is used for receiving chunk-size [chunk-ext] CRLF
		chunkSize := int64(0)
		for { // chunk-size = 1*HEXDIG
			b := r.bodyWindow[r.cFore]
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
			if r.cFore++; r.cFore-r.cBack >= 16 || (r.cFore == r.chunkEdge && !r.growChunked1()) {
				goto badRecv
			}
		}
		if chunkSize < 0 { // bad chunk size.
			goto badRecv
		}
		if b := r.bodyWindow[r.cFore]; b == ';' { // ignore chunk-ext = *( ";" chunk-ext-name [ "=" chunk-ext-val ] )
			for r.bodyWindow[r.cFore] != '\n' {
				if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
					goto badRecv
				}
			}
		} else if b == '\r' {
			// Skip '\r'
			if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
				goto badRecv
			}
		}
		if r.bodyWindow[r.cFore] != '\n' {
			goto badRecv
		}
		// Check target size
		if targetSize := r.sizeReceived + chunkSize; targetSize >= 0 && targetSize <= r.maxContentSize {
			r.chunkSize = chunkSize
		} else { // invalid target size.
			// TODO: log error?
			goto badRecv
		}
		// Skip '\n' at the end of chunk-size [chunk-ext] CRLF
		if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
			goto badRecv
		}
		// Last chunk?
		if r.chunkSize == 0 { // last-chunk = 1*("0") [chunk-ext] CRLF
			// last-chunk trailer-section CRLF
			if r.bodyWindow[r.cFore] == '\r' {
				// Skip '\r'
				if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
					goto badRecv
				}
				if r.bodyWindow[r.cFore] != '\n' {
					goto badRecv
				}
			} else if r.bodyWindow[r.cFore] != '\n' { // must be trailer-section = *( field-line CRLF)
				r.receiving = httpSectionTrailers
				if !r.recvTrailers1() {
					goto badRecv
				}
				// r.recvTrailers1() must ends with r.cFore being at the last '\n' after trailer-section.
			}
			// Skip the last '\n'
			r.cFore++ // now the whole chunked content is received and r.cFore is immediately after chunked content.
			// Now we have found the end of current message, so determine r.inputNext and r.inputEdge.
			if r.cFore < r.chunkEdge { // still has data, stream is pipelined
				r.overChunked = true                            // so r.bodyWindow will be used as r.input on stream ends
				r.inputNext, r.inputEdge = r.cFore, r.chunkEdge // mark the next message
			} else { // no data anymore, stream is not pipelined
				r.inputNext, r.inputEdge = 0, 0 // reset input
				PutNK(r.bodyWindow)
				r.bodyWindow = nil
			}
			return nil, io.EOF
		}
		// Not last chunk, now r.cFore is at the beginning of chunk-data CRLF
		fallthrough
	default: // r.chunkSize > 0, receiving chunk-data CRLF
		r.cBack = 0 // so growChunked1() works correctly
		from := int(r.cFore)
		var dataEdge int32
		if haveSize := int64(r.chunkEdge - r.cFore); haveSize <= r.chunkSize { // 1 <= haveSize <= r.chunkSize. chunk-data can be taken entirely
			r.sizeReceived += haveSize
			dataEdge = r.chunkEdge
			if haveSize == r.chunkSize { // exact chunk-data
				r.chunkSize = -2 // got chunk-data, needs CRLF or LF
			} else { // haveSize < r.chunkSize, not enough data.
				r.chunkSize -= haveSize
			}
			r.cFore, r.chunkEdge = 0, 0 // all data taken
		} else { // haveSize > r.chunkSize, more than chunk-data
			r.sizeReceived += r.chunkSize
			dataEdge = r.cFore + int32(r.chunkSize)
			if sizeLeft := r.chunkEdge - dataEdge; sizeLeft == 1 { // chunk-data ?
				if b := r.bodyWindow[dataEdge]; b == '\r' { // exact chunk-data CR
					r.chunkSize = -1 // got chunk-data CR, needs LF
				} else if b == '\n' { // exact chunk-data LF
					r.chunkSize = 0
				} else { // chunk-data X
					goto badRecv
				}
				r.cFore, r.chunkEdge = 0, 0 // all data taken
			} else if r.bodyWindow[dataEdge] == '\r' && r.bodyWindow[dataEdge+1] == '\n' { // chunk-data CRLF..
				r.chunkSize = 0
				if sizeLeft == 2 { // exact chunk-data CRLF
					r.cFore, r.chunkEdge = 0, 0 // all data taken
				} else { // > 2, chunk-data CRLF X
					r.cFore = dataEdge + 2
				}
			} else if r.bodyWindow[dataEdge] == '\n' { // >= 2, chunk-data LF X
				r.chunkSize = 0
				r.cFore = dataEdge + 1
			} else { // >= 2, chunk-data XX
				goto badRecv
			}
		}
		return r.bodyWindow[from:int(dataEdge)], nil
	}
badRecv:
	return nil, http1ReadBadChunk
}

func (r *httpInMessage_) recvTrailers1() bool { // trailer-section = *( field-line CRLF)
	copy(r.bodyWindow, r.bodyWindow[r.cFore:r.chunkEdge]) // slide to start
	r.chunkEdge -= r.cFore
	r.cBack, r.cFore = 0, 0 // setting r.cBack = 0 means r.bodyWindow will not slide, so the whole trailers must fit in r.bodyWindow.
	r.pBack, r.pFore = 0, 0 // for parsing trailer fields
	r.trailers.from = uint8(len(r.primes))
	r.trailers.edge = r.trailers.from
	trailer := &r.field
	trailer.zero()
	trailer.setPlace(pairPlaceArray) // all received trailers are placed in r.array
	for {
		if b := r.bodyWindow[r.pFore]; b == '\r' {
			// Skip '\r'
			if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
			if r.bodyWindow[r.pFore] != '\n' {
				return false
			}
			break
		} else if b == '\n' {
			break
		}
		trailer.hash = 0
		r.pBack = r.pFore // for field-name
		for {
			b := r.bodyWindow[r.pFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.bodyWindow[r.pFore] = b
			} else if b == ':' {
				break
			} else {
				return false
			}
			trailer.hash += uint16(b)
			if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
		}
		size := r.pFore - r.pBack
		if size == 0 || size > 255 {
			return false
		}
		trailer.nameFrom, trailer.nameSize = r.pBack, uint8(size)
		// Skip ':'
		if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
			return false
		}
		for r.bodyWindow[r.pFore] == ' ' || r.bodyWindow[r.pFore] == '\t' {
			if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
		}
		r.pBack = r.pFore // for field-value or EOL
		for {
			if b := r.bodyWindow[r.pFore]; httpVchar[b] == 1 {
				if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
					return false
				}
				if r.bodyWindow[r.pFore] != '\n' {
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				return false
			}
		}
		// r.pFore is at '\n'
		fore := r.pFore
		if r.bodyWindow[fore-1] == '\r' {
			fore--
		}
		if fore > r.pBack {
			for r.bodyWindow[fore-1] == ' ' || r.bodyWindow[fore-1] == '\t' {
				fore--
			}
			trailer.value.set(r.pBack, fore)
		} else {
			trailer.value.zero()
		}

		// Copy trailer to r.array
		fore = r.arrayEdge
		if !r.shell.arrayCopy(trailer.nameAt(r.bodyWindow)) {
			return false
		}
		trailer.nameFrom = fore // adjust name from
		fore = r.arrayEdge
		if !r.shell.arrayCopy(trailer.valueAt(r.bodyWindow)) {
			return false
		}
		trailer.value.set(fore, r.arrayEdge)

		// Trailer is received in general algorithm. Now apply it
		if !r.shell.applyTrailer(trailer) {
			return false
		}
		// Trailer is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
			return false
		}
		// r.pFore is now at the next trailer or end of trailers.
	}
	r.cFore = r.pFore // r.cFore must ends at the last '\n'
	return true
}
func (r *httpInMessage_) growChunked1() bool { // HTTP/1 is not a binary protocol, we don't know how many bytes to grow, so just grow.
	if r.chunkEdge == int32(cap(r.bodyWindow)) && r.cBack == 0 { // r.bodyWindow is full and we can't slide
		return false
	}
	if r.cBack > 0 { // has previously used data. slide to start so we can read more data
		copy(r.bodyWindow, r.bodyWindow[r.cBack:r.chunkEdge])
		r.chunkEdge -= r.cBack
		r.cFore -= r.cBack
		r.cBack = 0
	}
	err := r._beforeRead(&r.bodyTime)
	if err == nil {
		n, e := r.stream.read(r.bodyWindow[r.chunkEdge:])
		if e == nil {
			if r.maxRecvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.maxRecvTimeout {
				e = httpReadTooSlow
			} else {
				r.chunkEdge += int32(n)
				return true
			}
		}
		err = e
	}
	// err != nil. TODO: log err
	return false
}

var ( // http/1 message reading errors
	http1ReadBadChunk = errors.New("bad chunk")
)

// http1OutMessage_ is used by http1Response and H1Request.

func (r *httpOutMessage_) header1(name []byte) (value []byte, ok bool) {
	if r.nHeaders > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nHeaders; i++ {
			edge := r.edges[i]
			header := r.fields[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return header[p+len(httpBytesColonSpace) : len(header)-len(httpBytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *httpOutMessage_) hasHeader1(name []byte) bool {
	if r.nHeaders > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nHeaders; i++ {
			edge := r.edges[i]
			header := r.fields[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return true
			}
			from = edge
		}
	}
	return false
}
func (r *httpOutMessage_) addHeader1(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	size := len(name) + len(httpBytesColonSpace) + len(value) + len(httpBytesCRLF) // name: value\r\n
	if from, _, ok := r.growHeader(size); ok {
		from += copy(r.fields[from:], name)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += copy(r.fields[from:], value)
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}
func (r *httpOutMessage_) delHeader1(name []byte) (deleted bool) {
	from := uint16(0)
	for i := uint8(1); i < r.nHeaders; {
		edge := r.edges[i]
		if p := bytes.IndexByte(r.fields[from:edge], ':'); bytes.Equal(r.fields[from:from+uint16(p)], name) {
			size := edge - from
			copy(r.fields[from:], r.fields[edge:])
			for j := i + 1; j < r.nHeaders; j++ {
				r.edges[j] -= size
			}
			r.fieldsEdge -= size
			r.nHeaders--
			deleted = true
		} else {
			from = edge
			i++
		}
	}
	return
}
func (r *httpOutMessage_) delHeaderAt1(o uint8) {
	if o == 0 {
		BugExitln("delHeaderAt1: o == 0 must not happen!")
	}
	from := r.edges[o-1]
	edge := r.edges[o]
	size := edge - from
	copy(r.fields[from:], r.fields[edge:])
	for j := o + 1; j < r.nHeaders; j++ {
		r.edges[j] -= size
	}
	r.fieldsEdge -= size
	r.nHeaders--
}
func (r *httpOutMessage_) _addCRLFHeader1(from int) {
	r.fields[from] = '\r'
	r.fields[from+1] = '\n'
	r.edges[r.nHeaders] = uint16(from + 2)
	r.nHeaders++
}
func (r *httpOutMessage_) _addFixedHeader1(name []byte, value []byte) { // used by finalizeHeaders
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], name))
	r.fields[r.fieldsEdge] = ':'
	r.fields[r.fieldsEdge+1] = ' '
	r.fieldsEdge += 2
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], value))
	r.fields[r.fieldsEdge] = '\r'
	r.fields[r.fieldsEdge+1] = '\n'
	r.fieldsEdge += 2
}

func (r *httpOutMessage_) sendChain1(chain Chain) error {
	r.shell.finalizeHeaders()
	var vector [][]byte // waiting for write
	if r.forbidContent {
		vector = r.fixedVector[0:3]
		chain.free()
	} else if nBlocks := chain.Size(); nBlocks == 1 { // content chain has exactly one block
		vector = r.fixedVector[0:4]
	} else { // nBlocks >= 2
		vector = make([][]byte, 3+nBlocks) // TODO(diogin): get from pool? defer pool.put()
	}
	vector[0] = r.shell.control()
	vector[1] = r.shell.addedHeaders()
	vector[2] = r.shell.fixedHeaders()
	if IsDebug(2) {
		if r.asRequest {
			Debugf("[H1Stream=%d]=======> ", r.stream.(*H1Stream).conn.id)
		} else {
			Debugf("[http1Stream=%d]-------> ", r.stream.(*http1Stream).conn.id)
		}
		Debugf("[%s%s%s]\n", vector[0], vector[1], vector[2])
	}
	vFrom, vEdge := 0, 3
	// TODO: ranged content support
	for block := chain.head; block != nil; block = block.next {
		if block.size == 0 {
			continue
		}
		if block.IsBlob() {
			vector[vEdge] = block.Blob()
			vEdge++
		} else if block.size <= _16K { // small file
			buffer := GetNK(block.size)
			if err := block.copyTo(buffer); err != nil {
				r.stream.markBroken()
				PutNK(buffer)
				return err
			}
			vector[vEdge] = buffer[0:block.size]
			vEdge++
			r.vector = vector[vFrom:vEdge]
			if err := r.writeVector1(&r.vector); err != nil {
				PutNK(buffer)
				return err
			}
			PutNK(buffer)
			vFrom, vEdge = 0, 0
		} else { // large file
			if vFrom < vEdge {
				r.vector = vector[vFrom:vEdge]
				if err := r.writeVector1(&r.vector); err != nil {
					return err
				}
				vFrom, vEdge = 0, 0
			}
			if err := r.writeBlock1(block, false); err != nil {
				return err
			}
		}
	}
	if vFrom < vEdge {
		r.vector = vector[vFrom:vEdge]
		return r.writeVector1(&r.vector)
	}
	return nil
}

func (r *httpOutMessage_) pushChain1(chain Chain, chunked bool) error {
	for block := chain.head; block != nil; block = block.next {
		if err := r.writeBlock1(block, chunked); err != nil {
			return err
		}
	}
	return nil
}

func (r *httpOutMessage_) trailer1(name []byte) (value []byte, ok bool) {
	if r.nTrailers > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nTrailers; i++ {
			edge := r.edges[i]
			trailer := r.fields[from:edge]
			if p := bytes.IndexByte(trailer, ':'); p != -1 && bytes.Equal(trailer[0:p], name) {
				return trailer[p+len(httpBytesColonSpace) : len(trailer)-len(httpBytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *httpOutMessage_) addTrailer1(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	size := len(name) + len(httpBytesColonSpace) + len(value) + len(httpBytesCRLF) // name: value\r\n
	if from, _, ok := r.growTrailer(size); ok {
		from += copy(r.fields[from:], name)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += copy(r.fields[from:], value)
		r.fields[from] = '\r'
		r.fields[from+1] = '\n'
		r.edges[r.nTrailers] = uint16(from + 2)
		r.nTrailers++
		return true
	} else {
		return false
	}
}
func (r *httpOutMessage_) trailers1() []byte {
	// Headers and trailers are not present at the same time, so after headers is sent, r.fields is used by trailers.
	return r.fields[0:r.fieldsEdge]
}

func (r *httpOutMessage_) passBytes1(p []byte) error {
	r.vector = r.fixedVector[0:1]
	r.vector[0] = p
	return r.writeVector1(&r.vector)
}

func (r *httpOutMessage_) finalizeChunked1() error {
	if r.nTrailers == 1 { // no trailers
		r.vector = r.fixedVector[0:1]
		r.vector[0] = http1BytesZeroCRLFCRLF // 0\r\n\r\n
	} else { // with trailers
		r.vector = r.fixedVector[0:3]
		r.vector[0] = http1BytesZeroCRLF // 0\r\n
		r.vector[1] = r.trailers1()      // field-name: field-value\r\n
		r.vector[2] = httpBytesCRLF      // \r\n
	}
	return r.writeVector1(&r.vector)
}

func (r *httpOutMessage_) writeHeaders1() error { // used by push and pass
	r.shell.finalizeHeaders()
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.shell.control()
	r.vector[1] = r.shell.addedHeaders()
	r.vector[2] = r.shell.fixedHeaders()
	if IsDebug(2) {
		if r.asRequest {
			Debugf("[H1Stream=%d]", r.stream.(*H1Stream).conn.id)
		} else {
			Debugf("[http1Stream=%d]", r.stream.(*http1Stream).conn.id)
		}
		Debugf("-------> [%s%s%s]\n", r.vector[0], r.vector[1], r.vector[2])
	}
	if err := r.writeVector1(&r.vector); err != nil {
		return err
	}
	r.fieldsEdge = 0 // now r.fields is used by trailers (if any), so reset it.
	return nil
}
func (r *httpOutMessage_) writeBlock1(block *Block, chunked bool) error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	if block.IsBlob() {
		return r._writeBlob1(block, chunked)
	} else {
		return r._writeFile1(block, chunked)
	}
}
func (r *httpOutMessage_) _writeFile1(block *Block, chunked bool) error {
	buffer := GetNK(block.size)
	defer PutNK(buffer)
	nRead := int64(0)
	for { // we don't use sendfile(2).
		if nRead == block.size {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := block.size - nRead; sizeLeft < readSize {
			readSize = sizeLeft
		}
		n, err := block.file.ReadAt(buffer[:readSize], nRead)
		nRead += int64(n)
		if err != nil && nRead != block.size {
			r.stream.markBroken()
			return err
		}
		if err = r._beforeWrite(); err != nil {
			r.stream.markBroken()
			return err
		}
		if chunked {
			sizeBuffer := r.stream.tinyBuffer()
			k := i64ToHex(int64(n), sizeBuffer)
			sizeBuffer[k] = '\r'
			sizeBuffer[k+1] = '\n'
			k += 2
			r.vector = r.fixedVector[0:3]
			r.vector[0] = sizeBuffer[:k]
			r.vector[1] = buffer[:n]
			r.vector[2] = sizeBuffer[k-2 : k]
			_, err = r.stream.writev(&r.vector)
		} else {
			_, err = r.stream.write(buffer[0:n])
		}
		if err == nil && (r.maxSendTimeout > 0 && time.Now().Sub(r.sendTime) >= r.maxSendTimeout) {
			err = httpWriteTooSlow
		}
		if err != nil {
			r.stream.markBroken()
			return err
		}
	}
}
func (r *httpOutMessage_) _writeBlob1(block *Block, chunked bool) error { // blob
	if chunked { // HTTP/1.1
		sizeBuffer := r.stream.tinyBuffer() // buffer is enough for chunk size
		n := i64ToHex(block.size, sizeBuffer)
		sizeBuffer[n] = '\r'
		sizeBuffer[n+1] = '\n'
		n += 2
		r.vector = r.fixedVector[0:3] // we reuse r.vector and r.fixedVector
		r.vector[0] = sizeBuffer[0:n]
		r.vector[1] = block.Blob()
		r.vector[2] = sizeBuffer[n-2 : n]
	} else { // HTTP/1.0, or raw blob
		r.vector = r.fixedVector[0:1] // we reuse r.vector and r.fixedVector
		r.vector[0] = block.Blob()
	}
	return r.writeVector1(&r.vector)
}
func (r *httpOutMessage_) writeVector1(vector *net.Buffers) error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	for {
		if err := r._beforeWrite(); err != nil {
			r.stream.markBroken()
			return err
		}
		_, err := r.stream.writev(vector)
		if err == nil && (r.maxSendTimeout > 0 && time.Now().Sub(r.sendTime) >= r.maxSendTimeout) {
			err = httpWriteTooSlow
		}
		if err != nil {
			r.stream.markBroken()
			return err
		}
		return nil
	}
}

// HTTP/1 protocol elements.

var ( // HTTP/1 byteses
	http1BytesContinue             = []byte("HTTP/1.1 100 Continue\r\n\r\n")
	http1BytesConnectionClose      = []byte("connection: close\r\n")
	http1BytesConnectionKeepAlive  = []byte("connection: keep-alive\r\n")
	http1BytesContentTypeHTMLUTF8  = []byte("content-type: text/html; charset=utf-8\r\n")
	http1BytesTransferChunked      = []byte("transfer-encoding: chunked\r\n")
	http1BytesVaryEncoding         = []byte("vary: accept-encoding\r\n")
	http1BytesLocationHTTP         = []byte("location: http://")
	http1BytesLocationHTTPS        = []byte("location: https://")
	http1BytesFixedRequestHeaders  = []byte("client: gorox\r\n\r\n")
	http1BytesFixedResponseHeaders = []byte("server: gorox\r\n\r\n")
	http1BytesZeroCRLF             = []byte("0\r\n")
	http1BytesZeroCRLFCRLF         = []byte("0\r\n\r\n")
)

var http1Mysterious = [32]byte{'H', 'T', 'T', 'P', '/', '1', '.', '1', ' ', 'x', 'x', 'x', ' ', 'M', 'y', 's', 't', 'e', 'r', 'i', 'o', 'u', 's', ' ', 'S', 't', 'a', 't', 'u', 's', '\r', '\n'}
var http1Controls = [...][]byte{ // size: 512*24B=12K
	// 1XX
	StatusContinue:           []byte("HTTP/1.1 100 Continue\r\n"),
	StatusSwitchingProtocols: []byte("HTTP/1.1 101 Switching Protocols\r\n"),
	StatusProcessing:         []byte("HTTP/1.1 102 Processing\r\n"),
	StatusEarlyHints:         []byte("HTTP/1.1 103 Early Hints\r\n"),
	// 2XX
	StatusOK:                         []byte("HTTP/1.1 200 OK\r\n"),
	StatusCreated:                    []byte("HTTP/1.1 201 Created\r\n"),
	StatusAccepted:                   []byte("HTTP/1.1 202 Accepted\r\n"),
	StatusNonAuthoritativeInfomation: []byte("HTTP/1.1 203 Non-Authoritative Information\r\n"),
	StatusNoContent:                  []byte("HTTP/1.1 204 No Content\r\n"),
	StatusResetContent:               []byte("HTTP/1.1 205 Reset Content\r\n"),
	StatusPartialContent:             []byte("HTTP/1.1 206 Partial Content\r\n"),
	StatusMultiStatus:                []byte("HTTP/1.1 207 Multi-Status\r\n"),
	StatusAlreadyReported:            []byte("HTTP/1.1 208 Already Reported\r\n"),
	StatusIMUsed:                     []byte("HTTP/1.1 226 IM Used\r\n"),
	// 3XX
	StatusMultipleChoices:   []byte("HTTP/1.1 300 Multiple Choices\r\n"),
	StatusMovedPermanently:  []byte("HTTP/1.1 301 Moved Permanently\r\n"),
	StatusFound:             []byte("HTTP/1.1 302 Found\r\n"),
	StatusSeeOther:          []byte("HTTP/1.1 303 See Other\r\n"),
	StatusNotModified:       []byte("HTTP/1.1 304 Not Modified\r\n"),
	StatusUseProxy:          []byte("HTTP/1.1 305 Use Proxy\r\n"),
	StatusTemporaryRedirect: []byte("HTTP/1.1 307 Temporary Redirect\r\n"),
	StatusPermanentRedirect: []byte("HTTP/1.1 308 Permanent Redirect\r\n"),
	// 4XX
	StatusBadRequest:                  []byte("HTTP/1.1 400 Bad Request\r\n"),
	StatusUnauthorized:                []byte("HTTP/1.1 401 Unauthorized\r\n"),
	StatusPaymentRequired:             []byte("HTTP/1.1 402 Payment Required\r\n"),
	StatusForbidden:                   []byte("HTTP/1.1 403 Forbidden\r\n"),
	StatusNotFound:                    []byte("HTTP/1.1 404 Not Found\r\n"),
	StatusMethodNotAllowed:            []byte("HTTP/1.1 405 Method Not Allowed\r\n"),
	StatusNotAcceptable:               []byte("HTTP/1.1 406 Not Acceptable\r\n"),
	StatusProxyAuthenticationRequired: []byte("HTTP/1.1 407 Proxy Authentication Required\r\n"),
	StatusRequestTimeout:              []byte("HTTP/1.1 408 Request Timeout\r\n"),
	StatusConflict:                    []byte("HTTP/1.1 409 Conflict\r\n"),
	StatusGone:                        []byte("HTTP/1.1 410 Gone\r\n"),
	StatusLengthRequired:              []byte("HTTP/1.1 411 Length Required\r\n"),
	StatusPreconditionFailed:          []byte("HTTP/1.1 412 Precondition Failed\r\n"),
	StatusContentTooLarge:             []byte("HTTP/1.1 413 Content Too Large\r\n"),
	StatusURITooLong:                  []byte("HTTP/1.1 414 URI Too Long\r\n"),
	StatusUnsupportedMediaType:        []byte("HTTP/1.1 415 Unsupported Media Type\r\n"),
	StatusRangeNotSatisfiable:         []byte("HTTP/1.1 416 Range Not Satisfiable\r\n"),
	StatusExpectationFailed:           []byte("HTTP/1.1 417 Expectation Failed\r\n"),
	StatusMisdirectedRequest:          []byte("HTTP/1.1 421 Misdirected Request\r\n"),
	StatusUnprocessableEntity:         []byte("HTTP/1.1 422 Unprocessable Entity\r\n"),
	StatusLocked:                      []byte("HTTP/1.1 423 Locked\r\n"),
	StatusFailedDependency:            []byte("HTTP/1.1 424 Failed Dependency\r\n"),
	StatusTooEarly:                    []byte("HTTP/1.1 425 Too Early\r\n"),
	StatusUpgradeRequired:             []byte("HTTP/1.1 426 Upgrade Required\r\n"),
	StatusPreconditionRequired:        []byte("HTTP/1.1 428 Precondition Required\r\n"),
	StatusTooManyRequests:             []byte("HTTP/1.1 429 Too Many Requests\r\n"),
	StatusRequestHeaderFieldsTooLarge: []byte("HTTP/1.1 431 Request Header Fields Too Large\r\n"),
	StatusUnavailableForLegalReasons:  []byte("HTTP/1.1 451 Unavailable For Legal Reasons\r\n"),
	// 5XX
	StatusInternalServerError:           []byte("HTTP/1.1 500 Internal Server Error\r\n"),
	StatusNotImplemented:                []byte("HTTP/1.1 501 Not Implemented\r\n"),
	StatusBadGateway:                    []byte("HTTP/1.1 502 Bad Gateway\r\n"),
	StatusServiceUnavailable:            []byte("HTTP/1.1 503 Service Unavailable\r\n"),
	StatusGatewayTimeout:                []byte("HTTP/1.1 504 Gateway Timeout\r\n"),
	StatusHTTPVersionNotSupported:       []byte("HTTP/1.1 505 HTTP Version Not Supported\r\n"),
	StatusVariantAlsoNegotiates:         []byte("HTTP/1.1 506 Variant Also Negotiates\r\n"),
	StatusInsufficientStorage:           []byte("HTTP/1.1 507 Insufficient Storage\r\n"),
	StatusLoopDetected:                  []byte("HTTP/1.1 508 Loop Detected\r\n"),
	StatusNotExtended:                   []byte("HTTP/1.1 510 Not Extended\r\n"),
	StatusNetworkAuthenticationRequired: []byte("HTTP/1.1 511 Network Authentication Required\r\n"),
}
