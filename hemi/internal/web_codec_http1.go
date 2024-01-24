// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 incoming message and outgoing message implementation. See RFC 9112.

package internal

import (
	"bytes"
	"io"
	"net"
)

// HTTP/1 incoming

func (r *webIn_) growHead1() bool { // HTTP/1 is not a binary protocol, we don't know how many bytes to grow, so just grow.
	// Is r.input full?
	if inputSize := int32(cap(r.input)); r.inputEdge == inputSize { // r.inputEdge reached end, so r.input is full
		if inputSize == _16K { // max r.input size is 16K, we cannot use a larger input anymore
			if r.receiving == webSectionControl {
				r.headResult = StatusURITooLong
			} else { // webSectionHeaders
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
func (r *webIn_) recvHeaders1() bool { // *( field-name ":" OWS field-value OWS CRLF ) CRLF
	r.headers.from = uint8(len(r.primes))
	r.headers.edge = r.headers.from
	header := &r.mainPair
	header.zero()
	header.kind = kindHeader
	header.place = placeInput // all received headers are in r.input
	// r.pFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.pFore]; b == '\r' {
			// Skip '\r'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '\n' {
				r.headResult, r.failReason = StatusBadRequest, "bad end of headers"
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		// header-field = field-name ":" OWS field-value OWS

		// field-name = token
		// token = 1*tchar

		r.pBack = r.pFore // now r.pBack is at header-field
		for {
			b := r.input[r.pFore]
			if t := webTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.pFore] = b
			} else if t == 3 { // '_'
				header.setUnderscore()
			} else if b == ':' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			header.hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		if nameSize := r.pFore - r.pBack; nameSize > 0 && nameSize <= 255 {
			header.nameFrom, header.nameSize = r.pBack, uint8(nameSize)
		} else {
			r.headResult, r.failReason = StatusBadRequest, "header name out of range"
			return false
		}
		// Skip ':'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
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
			if b := r.input[r.pFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
				if r.input[r.pFore] != '\n' {
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
		// r.pFore is at '\n'
		fore := r.pFore
		if r.input[fore-1] == '\r' {
			fore--
		}
		if fore > r.pBack { // field-value is not empty. now trim OWS after field-value
			for r.input[fore-1] == ' ' || r.input[fore-1] == '\t' {
				fore--
			}
		}
		header.value.set(r.pBack, fore)

		// Header is received in general algorithm. Now add it
		if !r.addHeader(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
		// r.pFore is now at the next header or end of headers.
		header.hash, header.flags = 0, 0 // reset for next header
	}
	r.receiving = webSectionContent
	// Skip end of headers
	r.pFore++
	// Now the head is received, and r.pFore is at the beginning of content (if exists) or next message (if exists and is pipelined).
	r.head.set(0, r.pFore)

	return true
}

func (r *webIn_) readContent1() (p []byte, err error) {
	if r.contentSize >= 0 { // sized
		return r._readSizedContent1()
	} else { // vague. must be -2. -1 (no content) is excluded priorly
		return r._readVagueContent1()
	}
}
func (r *webIn_) _readSizedContent1() (p []byte, err error) {
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
	if err = r._beforeRead(&r.bodyTime); err != nil {
		return nil, err
	}
	readSize := int64(cap(r.bodyWindow))
	if sizeLeft := r.contentSize - r.receivedSize; sizeLeft < readSize {
		readSize = sizeLeft
	}
	size, err := r.stream.readFull(r.bodyWindow[:readSize])
	if err == nil {
		if !r._tooSlow() {
			r.receivedSize += int64(size)
			return r.bodyWindow[:size], nil
		}
		err = webInTooSlow
	}
	return nil, err
}
func (r *webIn_) _readVagueContent1() (p []byte, err error) {
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends. 16K is a tradeoff between performance and memory consumption, and can fit r.imme and trailers
	}
	if r.imme.notEmpty() {
		r.chunkEdge = int32(copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge])) // r.input is not larger than r.bodyWindow
		r.imme.zero()
	}
	if r.chunkEdge == 0 && !r.growChunked1() { // r.bodyWindow is empty. must fill
		goto badRead
	}
	switch r.chunkSize { // size left in receiving current chunk
	case -2: // got chunk-data. needs CRLF or LF
		if r.bodyWindow[r.cFore] == '\r' {
			if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
				goto badRead
			}
		}
		fallthrough
	case -1: // got chunk-data CR. needs LF
		if r.bodyWindow[r.cFore] != '\n' {
			goto badRead
		}
		// Skip '\n'
		if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
			goto badRead
		}
		fallthrough
	case 0: // start a new chunk = chunk-size [chunk-ext] CRLF chunk-data CRLF
		r.cBack = r.cFore // now r.bodyWindow is used for receiving: chunk-size [chunk-ext] CRLF
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
				goto badRead
			}
		}
		if chunkSize < 0 { // bad chunk size.
			goto badRead
		}
		if b := r.bodyWindow[r.cFore]; b == ';' { // ignore chunk-ext = *( ";" chunk-ext-name [ "=" chunk-ext-val ] )
			for r.bodyWindow[r.cFore] != '\n' {
				if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
					goto badRead
				}
			}
		} else if b == '\r' {
			// Skip '\r'
			if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
				goto badRead
			}
		}
		// Must be LF
		if r.bodyWindow[r.cFore] != '\n' {
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
		if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
			goto badRead
		}
		// Last chunk?
		if r.chunkSize == 0 { // last-chunk = 1*("0") [chunk-ext] CRLF
			// last-chunk trailer-section CRLF
			if r.bodyWindow[r.cFore] == '\r' {
				// Skip '\r'
				if r.cFore++; r.cFore == r.chunkEdge && !r.growChunked1() {
					goto badRead
				}
				if r.bodyWindow[r.cFore] != '\n' {
					goto badRead
				}
			} else if r.bodyWindow[r.cFore] != '\n' { // must be trailer-section = *( field-line CRLF)
				r.receiving = webSectionTrailers
				if !r.recvTrailers1() || !r.examineTail() {
					goto badRead
				}
				// r.recvTrailers1() must ends with r.cFore being at the last '\n' after trailer-section.
			}
			// Skip the last '\n'
			r.cFore++ // now the whole vague content is received and r.cFore is immediately after the vague content.
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
		// Not last chunk, now r.cFore is at the beginning of: chunk-data CRLF
		fallthrough
	default: // r.chunkSize > 0, we are receiving: chunk-data CRLF
		r.cBack = 0   // so growChunked1() works correctly
		var data span // the chunk data we are receiving
		data.from = r.cFore
		if haveSize := int64(r.chunkEdge - r.cFore); haveSize <= r.chunkSize { // 1 <= haveSize <= r.chunkSize. chunk-data can be taken entirely
			r.receivedSize += haveSize
			data.edge = r.chunkEdge
			if haveSize == r.chunkSize { // exact chunk-data
				r.chunkSize = -2 // got chunk-data, needs CRLF or LF
			} else { // haveSize < r.chunkSize, not enough data.
				r.chunkSize -= haveSize
			}
			r.cFore, r.chunkEdge = 0, 0 // all data taken
		} else { // haveSize > r.chunkSize, more than chunk-data
			r.receivedSize += r.chunkSize
			data.edge = r.cFore + int32(r.chunkSize)
			if sizeLeft := r.chunkEdge - data.edge; sizeLeft == 1 { // chunk-data ?
				if b := r.bodyWindow[data.edge]; b == '\r' { // exact chunk-data CR
					r.chunkSize = -1 // got chunk-data CR, needs LF
				} else if b == '\n' { // exact chunk-data LF
					r.chunkSize = 0
				} else { // chunk-data X
					goto badRead
				}
				r.cFore, r.chunkEdge = 0, 0 // all data taken
			} else if r.bodyWindow[data.edge] == '\r' && r.bodyWindow[data.edge+1] == '\n' { // chunk-data CRLF..
				r.chunkSize = 0
				if sizeLeft == 2 { // exact chunk-data CRLF
					r.cFore, r.chunkEdge = 0, 0 // all data taken
				} else { // > 2, chunk-data CRLF X
					r.cFore = data.edge + 2
				}
			} else if r.bodyWindow[data.edge] == '\n' { // >=2, chunk-data LF X
				r.chunkSize = 0
				r.cFore = data.edge + 1
			} else { // >=2, chunk-data XX
				goto badRead
			}
		}
		return r.bodyWindow[data.from:data.edge], nil
	}
badRead:
	return nil, webInBadChunk
}

func (r *webIn_) recvTrailers1() bool { // trailer-section = *( field-line CRLF)
	copy(r.bodyWindow, r.bodyWindow[r.cFore:r.chunkEdge]) // slide to start, we need a clean r.bodyWindow
	r.chunkEdge -= r.cFore
	r.cBack, r.cFore = 0, 0 // setting r.cBack = 0 means r.bodyWindow will not slide, so the whole trailers must fit in r.bodyWindow.
	r.pBack, r.pFore = 0, 0 // for parsing trailer fields

	r.trailers.from = uint8(len(r.primes))
	r.trailers.edge = r.trailers.from
	trailer := &r.mainPair
	trailer.zero()
	trailer.kind = kindTrailer
	trailer.place = placeArray // all received trailers are placed in r.array
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

		r.pBack = r.pFore // for field-name
		for {
			b := r.bodyWindow[r.pFore]
			if t := webTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.bodyWindow[r.pFore] = b
			} else if t == 3 { // '_'
				trailer.setUnderscore()
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
		if nameSize := r.pFore - r.pBack; nameSize > 0 && nameSize <= 255 {
			trailer.nameFrom, trailer.nameSize = r.pBack, uint8(nameSize)
		} else {
			return false
		}
		// Skip ':'
		if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.bodyWindow[r.pFore] == ' ' || r.bodyWindow[r.pFore] == '\t' {
			if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
		}
		r.pBack = r.pFore // for field-value or EOL
		for {
			if b := r.bodyWindow[r.pFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
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
		if fore > r.pBack { // field-value is not empty. now trim OWS after field-value
			for r.bodyWindow[fore-1] == ' ' || r.bodyWindow[fore-1] == '\t' {
				fore--
			}
		}
		trailer.value.set(r.pBack, fore)

		// Copy trailer data to r.array
		fore = r.arrayEdge
		if !r.shell.arrayCopy(trailer.nameAt(r.bodyWindow)) {
			return false
		}
		trailer.nameFrom = fore
		fore = r.arrayEdge
		if !r.shell.arrayCopy(trailer.valueAt(r.bodyWindow)) {
			return false
		}
		trailer.value.set(fore, r.arrayEdge)

		// Trailer is received in general algorithm. Now add it
		if !r.addTrailer(trailer) {
			return false
		}

		// Trailer is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.chunkEdge && !r.growChunked1() {
			return false
		}
		// r.pFore is now at the next trailer or end of trailers.
		trailer.hash, trailer.flags = 0, 0 // reset for next trailer
	}
	r.cFore = r.pFore // r.cFore must ends at the last '\n'
	return true
}
func (r *webIn_) growChunked1() bool { // HTTP/1 is not a binary protocol, we don't know how many bytes to grow, so just grow.
	if r.chunkEdge == int32(cap(r.bodyWindow)) && r.cBack == 0 { // r.bodyWindow is full and we can't slide
		return false // element is too large
	}
	if r.cBack > 0 { // has previously used data, but now useless. slide to start so we can read more
		copy(r.bodyWindow, r.bodyWindow[r.cBack:r.chunkEdge])
		r.chunkEdge -= r.cBack
		r.cFore -= r.cBack
		r.cBack = 0
	}
	err := r._beforeRead(&r.bodyTime)
	if err == nil {
		n, e := r.stream.read(r.bodyWindow[r.chunkEdge:])
		r.chunkEdge += int32(n)
		if e == nil {
			if !r._tooSlow() {
				return true
			}
			e = webInTooSlow
		}
		err = e // including io.EOF which is unexpected here
	}
	// err != nil. TODO: log err
	return false
}

// HTTP/1 outgoing

func (r *webOut_) addHeader1(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	headerSize := len(name) + len(bytesColonSpace) + len(value) + len(bytesCRLF) // name: value\r\n
	if from, _, ok := r.growHeader(headerSize); ok {
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
func (r *webOut_) header1(name []byte) (value []byte, ok bool) {
	if r.nHeaders > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nHeaders; i++ {
			edge := r.edges[i]
			header := r.fields[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return header[p+len(bytesColonSpace) : len(header)-len(bytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *webOut_) hasHeader1(name []byte) bool {
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
func (r *webOut_) delHeader1(name []byte) (deleted bool) {
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
func (r *webOut_) delHeaderAt1(i uint8) {
	if i == 0 {
		BugExitln("delHeaderAt1: i == 0 which must not happen")
	}
	from := r.edges[i-1]
	edge := r.edges[i]
	size := edge - from
	copy(r.fields[from:], r.fields[edge:])
	for j := i + 1; j < r.nHeaders; j++ {
		r.edges[j] -= size
	}
	r.fieldsEdge -= size
	r.nHeaders--
}
func (r *webOut_) _addCRLFHeader1(from int) {
	r.fields[from] = '\r'
	r.fields[from+1] = '\n'
	r.edges[r.nHeaders] = uint16(from + 2)
	r.nHeaders++
}
func (r *webOut_) _addFixedHeader1(name []byte, value []byte) { // used by finalizeHeaders
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], name))
	r.fields[r.fieldsEdge] = ':'
	r.fields[r.fieldsEdge+1] = ' '
	r.fieldsEdge += 2
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], value))
	r.fields[r.fieldsEdge] = '\r'
	r.fields[r.fieldsEdge+1] = '\n'
	r.fieldsEdge += 2
}

func (r *webOut_) sendChain1() error { // TODO: if conn is TLS, don't use writev as it uses many Write() which might be slower than make+copy+write.
	return r._sendEntireChain1()
	// TODO
	nRanges := len(r.ranges)
	if nRanges == 0 {
		return r._sendEntireChain1()
	}
	if !r.asRequest {
		r.shell.(Response).SetStatus(StatusPartialContent)
	}
	if nRanges == 1 {
		return r._sendSingleRange1()
	} else {
		return r._sendMultiRanges1()
	}
}
func (r *webOut_) _sendEntireChain1() error {
	r.shell.finalizeHeaders()
	vector := r._prepareVector1() // waiting to write
	if Debug() >= 2 {
		if r.asRequest {
			Printf("[H1Stream=%d]=======> ", r.stream.(*H1Stream).conn.id)
		} else {
			Printf("[http1Stream=%d]-------> ", r.stream.(*http1Stream).conn.id)
		}
		Printf("[%s%s%s]\n", vector[0], vector[1], vector[2])
	}
	vFrom, vEdge := 0, 3
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if piece.size == 0 {
			continue
		}
		if piece.IsText() { // plain text
			vector[vEdge] = piece.Text()
			vEdge++
		} else if piece.size <= _16K { // small file, <= 16K
			buffer := GetNK(piece.size) // 4K/16K
			if err := piece.copyTo(buffer); err != nil {
				r.stream.markBroken()
				PutNK(buffer)
				return err
			}
			vector[vEdge] = buffer[0:piece.size]
			vEdge++
			r.vector = vector[vFrom:vEdge]
			if err := r.writeVector1(); err != nil {
				PutNK(buffer)
				return err
			}
			PutNK(buffer)
			vFrom, vEdge = 0, 0
		} else { // large file, > 16K
			if vFrom < vEdge {
				r.vector = vector[vFrom:vEdge]
				if err := r.writeVector1(); err != nil { // texts
					return err
				}
				vFrom, vEdge = 0, 0
			}
			if err := r.writePiece1(piece, false); err != nil { // the file
				return err
			}
		}
	}
	if vFrom < vEdge {
		r.vector = vector[vFrom:vEdge]
		return r.writeVector1()
	}
	return nil
}
func (r *webOut_) _sendSingleRange1() error {
	r.AddContentType(r.rangeType)
	r.shell.finalizeHeaders()
	vector := r._prepareVector1() // waiting to write
	vFrom, vEdge := 0, 3
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if piece.size == 0 {
			continue
		}
		if piece.IsText() { // plain text
			vector[vEdge] = piece.Text()
			vEdge++
		} else if piece.size <= _16K { // small file, <= 16K
			buffer := GetNK(piece.size) // 4K/16K
			if err := piece.copyTo(buffer); err != nil {
				r.stream.markBroken()
				PutNK(buffer)
				return err
			}
			vector[vEdge] = buffer[0:piece.size]
			vEdge++
			r.vector = vector[vFrom:vEdge]
			if err := r.writeVector1(); err != nil {
				PutNK(buffer)
				return err
			}
			PutNK(buffer)
			vFrom, vEdge = 0, 0
		} else { // large file, > 16K
			if vFrom < vEdge {
				r.vector = vector[vFrom:vEdge]
				if err := r.writeVector1(); err != nil { // texts
					return err
				}
				vFrom, vEdge = 0, 0
			}
			if err := r.writePiece1(piece, false); err != nil { // the file
				return err
			}
		}
	}
	if vFrom < vEdge {
		r.vector = vector[vFrom:vEdge]
		return r.writeVector1()
	}
	return nil
}
func (r *webOut_) _sendMultiRanges1() error {
	buffer := r.stream.buffer256()
	n := copy(buffer, bytesMultipartRanges)
	n += copy(buffer[n:], "xsd3lxT9b5c")
	r.AddHeaderBytes(bytesContentType, buffer[:n])
	// TODO
	return nil
}
func (r *webOut_) _prepareVector1() [][]byte {
	var vector [][]byte // waiting for write
	if r.forbidContent {
		vector = r.fixedVector[0:3]
		r.chain.free()
	} else if nPieces := r.chain.NumPieces(); nPieces == 1 { // content chain has exactly one piece
		vector = r.fixedVector[0:4]
	} else { // nPieces >= 2
		vector = make([][]byte, 3+nPieces) // TODO(diogin): get from pool? defer pool.put()
	}
	vector[0] = r.shell.control()
	vector[1] = r.shell.addedHeaders()
	vector[2] = r.shell.fixedHeaders()
	return vector
}

func (r *webOut_) echoChain1(chunked bool) error { // TODO: coalesce?
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if err := r.writePiece1(piece, chunked); err != nil {
			return err
		}
	}
	return nil
}

func (r *webOut_) addTrailer1(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	trailerSize := len(name) + len(bytesColonSpace) + len(value) + len(bytesCRLF) // name: value\r\n
	if from, _, ok := r.growTrailer(trailerSize); ok {
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
func (r *webOut_) trailer1(name []byte) (value []byte, ok bool) {
	if r.nTrailers > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nTrailers; i++ {
			edge := r.edges[i]
			trailer := r.fields[from:edge]
			if p := bytes.IndexByte(trailer, ':'); p != -1 && bytes.Equal(trailer[0:p], name) {
				return trailer[p+len(bytesColonSpace) : len(trailer)-len(bytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *webOut_) trailers1() []byte { return r.fields[0:r.fieldsEdge] } // Headers and trailers are not present at the same time, so after headers is sent, r.fields is used by trailers.

func (r *webOut_) passBytes1(p []byte) error { return r.writeBytes1(p) }

func (r *webOut_) finalizeVague1() error {
	if r.nTrailers == 1 { // no trailers
		r.vector = r.fixedVector[0:1]
		r.vector[0] = http1BytesZeroCRLFCRLF // 0\r\n\r\n
	} else { // with trailers
		r.vector = r.fixedVector[0:3]
		r.vector[0] = http1BytesZeroCRLF // 0\r\n
		r.vector[1] = r.trailers1()      // field-name: field-value\r\n
		r.vector[2] = bytesCRLF          // \r\n
	}
	return r.writeVector1()
}

func (r *webOut_) writeHeaders1() error { // used by echo and pass
	r.shell.finalizeHeaders()
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.shell.control()
	r.vector[1] = r.shell.addedHeaders()
	r.vector[2] = r.shell.fixedHeaders()
	if Debug() >= 2 {
		if r.asRequest {
			Printf("[H1Stream=%d]", r.stream.(*H1Stream).conn.id)
		} else {
			Printf("[http1Stream=%d]", r.stream.(*http1Stream).conn.id)
		}
		Printf("-------> [%s%s%s]\n", r.vector[0], r.vector[1], r.vector[2])
	}
	if err := r.writeVector1(); err != nil {
		return err
	}
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *webOut_) writePiece1(piece *Piece, chunked bool) error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if piece.IsText() { // text piece
		if chunked { // HTTP/1.1 chunked data
			sizeBuffer := r.stream.buffer256() // buffer is enough for chunk size
			n := i64ToHex(piece.size, sizeBuffer)
			sizeBuffer[n] = '\r'
			sizeBuffer[n+1] = '\n'
			n += 2
			r.vector = r.fixedVector[0:3] // we reuse r.vector and r.fixedVector
			r.vector[0] = sizeBuffer[0:n]
			r.vector[1] = piece.Text()
			r.vector[2] = sizeBuffer[n-2 : n]
		} else { // HTTP/1.0, or raw data
			r.vector = r.fixedVector[0:1] // we reuse r.vector and r.fixedVector
			r.vector[0] = piece.Text()
		}
		return r.writeVector1()
	}
	// file piece. we don't use sendfile(2).
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
		if err = r._beforeWrite(); err != nil {
			r.stream.markBroken()
			return err
		}
		if chunked { // use HTTP/1.1 chunked mode
			sizeBuffer := r.stream.buffer256()
			k := i64ToHex(int64(n), sizeBuffer)
			sizeBuffer[k] = '\r'
			sizeBuffer[k+1] = '\n'
			k += 2
			r.vector = r.fixedVector[0:3]
			r.vector[0] = sizeBuffer[:k]
			r.vector[1] = buffer[:n]
			r.vector[2] = sizeBuffer[k-2 : k]
			_, err = r.stream.writev(&r.vector)
		} else { // HTTP/1.0, or identity content
			_, err = r.stream.write(buffer[0:n])
		}
		if err = r._slowCheck(err); err != nil {
			return err
		}
	}
}
func (r *webOut_) writeBytes1(p []byte) error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if len(p) == 0 {
		return nil
	}
	if err := r._beforeWrite(); err != nil {
		r.stream.markBroken()
		return err
	}
	_, err := r.stream.write(p)
	return r._slowCheck(err)
}
func (r *webOut_) writeVector1() error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if len(r.vector) == 1 && len(r.vector[0]) == 0 {
		return nil
	}
	if err := r._beforeWrite(); err != nil {
		r.stream.markBroken()
		return err
	}
	_, err := r.stream.writev(&r.vector)
	return r._slowCheck(err)
}
