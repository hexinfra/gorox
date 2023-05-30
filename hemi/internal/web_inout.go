// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General Web incoming and outgoing messages.

package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
)

// webKeeper is a webServer or webClient which keeps its connections and streams.
type webKeeper interface {
	Stage() *Stage
	TLSMode() bool
	ReadTimeout() time.Duration  // timeout of a read operation
	WriteTimeout() time.Duration // timeout of a write operation
	RecvTimeout() time.Duration  // timeout to recv the whole message content
	SendTimeout() time.Duration  // timeout to send the whole message
	MaxContentSize() int64       // allowed
	SaveContentFilesDir() string
}

// webKeeper_ is the mixin for webServer_, webOutgate_, and webBackend_.
type webKeeper_ struct {
	// States
	recvTimeout    time.Duration // timeout to recv the whole message content
	sendTimeout    time.Duration // timeout to send the whole message
	maxContentSize int64         // max content size allowed
}

func (k *webKeeper_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
	// sendTimeout
	shell.ConfigureDuration("sendTimeout", &k.sendTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, sendTimeout)

	// recvTimeout
	shell.ConfigureDuration("recvTimeout", &k.recvTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, recvTimeout)

	// maxContentSize
	shell.ConfigureInt64("maxContentSize", &k.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)
}

func (k *webKeeper_) RecvTimeout() time.Duration { return k.recvTimeout }
func (k *webKeeper_) SendTimeout() time.Duration { return k.sendTimeout }
func (k *webKeeper_) MaxContentSize() int64      { return k.maxContentSize }

// webStream is the interface for *http[1-3]Stream, *hwebExchan, *H[1-3]Stream, and *HExchan.
type webStream interface {
	webKeeper() webKeeper
	peerAddr() net.Addr

	buffer256() []byte
	unsafeMake(size int) []byte
	makeTempName(p []byte, unixTime int64) (from int, edge int)

	setReadDeadline(deadline time.Time) error
	setWriteDeadline(deadline time.Time) error

	read(p []byte) (int, error)
	readFull(p []byte) (int, error)
	write(p []byte) (int, error)
	writev(vector *net.Buffers) (int64, error)

	isBroken() bool // if either side is broken, then the stream is broken
	markBroken()
}

// webStream_ is the mixin for http[1-3]Stream, hwebExchan, H[1-3]Stream, and HExchan.
type webStream_ struct {
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	region Region // a region-based memory pool
	// Stream states (zeros)
	mode int8 // working mode of current stream. see streamModeXXX
}

const ( // stream modes
	streamModeExchan = 0 // request & response, must be 0
	streamModeSocket = 1 // upgrade: websocket
	streamModeTCPTun = 2 // CONNECT method
	streamModeUDPTun = 3 // upgrade: connect-udp
)

func (s *webStream_) onUse() { // for non-zeros
	s.region.Init()
	s.mode = streamModeExchan
}
func (s *webStream_) onEnd() { // for zeros
	s.region.Free()
}

func (s *webStream_) buffer256() []byte          { return s.stockBuffer[:] }
func (s *webStream_) unsafeMake(size int) []byte { return s.region.Make(size) }

// webIn is the interface for *http[1-3]Request, *hwebRequest, *H[1-3]Response, and *HResponse. Used as shell by webIn_.
type webIn interface {
	ContentSize() int64
	IsUnsized() bool
	readContent() (p []byte, err error)
	applyTrailer(index uint8) bool
	HasTrailers() bool
	forTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool
	arrayCopy(p []byte) bool
	saveContentFilesDir() string
}

// webIn_ is a mixin for serverRequest_ and clientResponse_.
type webIn_ struct { // incoming. needs parsing
	// Assocs
	shell  webIn     // *http[1-3]Request, *hwebRequest, *H[1-3]Response, *HResponse
	stream webStream // *http[1-3]Stream, *hwebExchan, *H[1-3]Stream, *HExchan
	// Stream states (stocks)
	stockInput  [1536]byte // for r.input
	stockArray  [768]byte  // for r.array
	stockPrimes [40]pair   // for r.primes
	stockExtras [30]pair   // for r.extras
	// Stream states (controlled)
	mainPair       pair     // to overcome the limitation of Go's escape analysis when receiving pairs
	contentCodings [4]uint8 // content-encoding flags, controlled by r.nContentCodings. see httpCodingXXX. values: none compress deflate gzip br
	acceptCodings  [4]uint8 // accept-encoding flags, controlled by r.nAcceptCodings. see httpCodingXXX. values: identity(none) compress deflate gzip br
	inputNext      int32    // HTTP/1 request only. next request begins from r.input[r.inputNext]. exists because HTTP/1 supports pipelining
	inputEdge      int32    // edge position of current message head is at r.input[r.inputEdge]. placed here to make it compatible with HTTP/1 pipelining
	// Stream states (non-zeros)
	input          []byte        // bytes of incoming message heads. [<r.stockInput>/4K/16K]
	array          []byte        // store dynamic incoming data. [<r.stockArray>/4K/16K/64K1/(make <= 1G)]
	primes         []pair        // hold prime queries, headers(main+subs), cookies, forms, and trailers(main+subs). [<r.stockPrimes>/max]
	extras         []pair        // hold extra queries, headers(main+subs), cookies, forms, trailers(main+subs), and params. [<r.stockExtras>/max]
	recvTimeout    time.Duration // timeout to recv the whole message content
	maxContentSize int64         // max content size allowed for current message. if content is unsized, size is calculated when receiving
	contentSize    int64         // info of incoming content. >=0: content size, -1: no content, -2: unsized content
	versionCode    uint8         // Version1_0, Version1_1, Version2, Version3
	asResponse     bool          // treat the incoming message as response?
	keepAlive      int8          // HTTP/1 only. -1: no connection header, 0: connection close, 1: connection keep-alive
	_              byte          // padding
	headResult     int16         // result of receiving message head. values are same as http status for convenience
	bodyResult     int16         // result of receiving message body. values are same as http status for convenience
	// Stream states (zeros)
	failReason  string    // the reason of headResult or bodyResult
	bodyWindow  []byte    // a window used for receiving body. sizes must be same with r.input for HTTP/1. [HTTP/1=<none>/16K, HTTP/2/3=<none>/4K/16K/64K1]
	recvTime    time.Time // the time when receiving message
	bodyTime    time.Time // the time when first body read operation is performed on this stream
	contentText []byte    // if loadable, the received and loaded content of current message is at r.contentText[:r.receivedSize]. [<none>/r.input/4K/16K/64K1/(make)]
	contentFile *os.File  // used by r.takeContent(), if content is tempFile. will be closed on stream ends
	webIn0                // all values must be zero by default in this struct!
}
type webIn0 struct { // for fast reset, entirely
	pBack            int32   // element begins from. for parsing control & headers & content & trailers elements
	pFore            int32   // element spanning to. for parsing control & headers & content & trailers elements
	head             span    // head (control + headers) of current message -> r.input. set after head is received. only for debugging
	imme             span    // HTTP/1 only. immediate data after current message head is at r.input[r.imme.from:r.imme.edge]
	hasExtra         [8]bool // see kindXXX for indexes
	dateTime         int64   // parsed unix time of date
	arrayEdge        int32   // next usable position of r.array is at r.array[r.arrayEdge]. used when writing r.array
	arrayKind        int8    // kind of current r.array. see arrayKindXXX
	receiving        int8    // currently receiving. see httpSectionXXX
	headers          zone    // headers ->r.primes
	hasRevisers      bool    // are there any incoming revisers hooked on this incoming message?
	upgradeSocket    bool    // upgrade: websocket?
	upgradeUDPTun    bool    // upgrade: connect-udp?
	acceptGzip       bool    // does peer accept gzip content coding? i.e. accept-encoding: gzip, deflate
	acceptBrotli     bool    // does peer accept brotli content coding? i.e. accept-encoding: gzip, br
	nContentCodings  int8    // num of content-encoding flags, controls r.contentCodings
	nAcceptCodings   int8    // num of accept-encoding flags, controls r.acceptCodings
	iContentLength   uint8   // index of content-length header in r.primes
	iContentLocation uint8   // index of content-location header in r.primes
	iContentRange    uint8   // index of content-range header in r.primes
	iContentType     uint8   // index of content-type header in r.primes
	iDate            uint8   // index of date header in r.primes
	_                [2]byte // padding
	zConnection      zone    // zone of connection headers in r.primes. may not be continuous
	zContentLanguage zone    // zone of content-language headers in r.primes. may not be continuous
	zTrailer         zone    // zone of trailer headers in r.primes. may not be continuous
	zVia             zone    // zone of via headers in r.primes. may not be continuous
	contentReceived  bool    // is content received? if message has no content, it is true (received)
	contentTextKind  int8    // kind of current r.contentText if it is text. see webContentTextXXX
	receivedSize     int64   // bytes of currently received content. used by both sized & unsized content receiver
	chunkSize        int64   // left size of current chunk if the chunk is too large to receive in one call. HTTP/1.1 chunked only
	cBack            int32   // for parsing chunked elements. HTTP/1.1 chunked only
	cFore            int32   // for parsing chunked elements. HTTP/1.1 chunked only
	chunkEdge        int32   // edge position of the filled chunked data in r.bodyWindow. HTTP/1.1 chunked only
	transferChunked  bool    // transfer-encoding: chunked? HTTP/1.1 only
	overChunked      bool    // for HTTP/1.1 requests, if chunked receiver over received in r.bodyWindow, then r.bodyWindow will be used as r.input on ends
	trailers         zone    // trailers -> r.primes. set after trailer section is received and parsed
}

func (r *webIn_) onUse(versionCode uint8, asResponse bool) { // for non-zeros
	if versionCode >= Version2 || asResponse {
		r.input = r.stockInput[:]
	} else {
		// HTTP/1 supports request pipelining, so input related are not reset here.
	}
	r.array = r.stockArray[:]
	r.primes = r.stockPrimes[0:1:cap(r.stockPrimes)] // use append(). r.primes[0] is skipped due to zero value of pair indexes.
	r.extras = r.stockExtras[0:0:cap(r.stockExtras)] // use append()
	r.recvTimeout = r.stream.webKeeper().RecvTimeout()
	r.maxContentSize = r.stream.webKeeper().MaxContentSize()
	r.contentSize = -1 // no content
	r.versionCode = versionCode
	r.asResponse = asResponse
	r.keepAlive = -1 // no connection header
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *webIn_) onEnd() { // for zeros
	if r.versionCode >= Version2 || r.asResponse { // as we don't use pipelining for outgoing requests, so responses are not pipelined.
		if cap(r.input) != cap(r.stockInput) {
			PutNK(r.input)
		}
		r.input = nil
		r.inputNext, r.inputEdge = 0, 0
	} else {
		// HTTP/1 supports request pipelining, so input related are not reset here.
	}
	if r.arrayKind == arrayKindPool {
		PutNK(r.array)
	}
	r.array = nil // other array kinds are only references, just reset.
	if cap(r.primes) != cap(r.stockPrimes) {
		putPairs(r.primes)
		r.primes = nil
	}
	if cap(r.extras) != cap(r.stockExtras) {
		putPairs(r.extras)
		r.extras = nil
	}

	r.failReason = ""

	if r.inputNext != 0 { // only happens in HTTP/1.1 request pipelining
		if r.overChunked { // only happens in HTTP/1.1 chunked mode
			// Use bytes over received in r.bodyWindow as new r.input.
			// This means the size list for r.bodyWindow must sync with r.input!
			if cap(r.input) != cap(r.stockInput) {
				PutNK(r.input)
			}
			r.input = r.bodyWindow // use r.bodyWindow as new r.input
		}
		// slide r.input. r.inputNext and r.inputEdge have already been set
		copy(r.input, r.input[r.inputNext:r.inputEdge])
		r.inputEdge -= r.inputNext
		r.inputNext = 0
	} else if r.bodyWindow != nil { // r.bodyWindow was used to receive content and failed to free. we free it here.
		PutNK(r.bodyWindow)
	}
	r.bodyWindow = nil

	r.recvTime = time.Time{}
	r.bodyTime = time.Time{}

	if r.contentTextKind == webContentTextPool {
		PutNK(r.contentText)
	}
	r.contentText = nil // other content text kinds are only references, just reset.

	if r.contentFile != nil {
		r.contentFile.Close()
		if IsDebug(2) {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(r.contentFile.Name()); err != nil {
			// TODO: log?
		}
		r.contentFile = nil
	}

	r.webIn0 = webIn0{}
}

func (r *webIn_) UnsafeMake(size int) []byte { return r.stream.unsafeMake(size) }
func (r *webIn_) PeerAddr() net.Addr         { return r.stream.peerAddr() }

func (r *webIn_) VersionCode() uint8    { return r.versionCode }
func (r *webIn_) IsHTTP1_0() bool       { return r.versionCode == Version1_0 }
func (r *webIn_) IsHTTP1_1() bool       { return r.versionCode == Version1_1 }
func (r *webIn_) IsHTTP1() bool         { return r.versionCode <= Version1_1 }
func (r *webIn_) IsHTTP2() bool         { return r.versionCode == Version2 }
func (r *webIn_) IsHTTP3() bool         { return r.versionCode == Version3 }
func (r *webIn_) Version() string       { return httpVersionStrings[r.versionCode] }
func (r *webIn_) UnsafeVersion() []byte { return httpVersionByteses[r.versionCode] }

func (r *webIn_) addHeader(header *pair) bool { // as prime
	if edge, ok := r._addPrime(header); ok {
		r.headers.edge = edge
		return true
	}
	r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many headers"
	return false
}
func (r *webIn_) AddHeader(name string, value string) bool { // as extra
	// TODO: add restrictions on what headers are allowed to add? should we check the value?
	// TODO: parse and check?
	// setFlags?
	return r.addExtra(name, value, 0, kindHeader)
}
func (r *webIn_) HasHeaders() bool                  { return r.hasPairs(r.headers, kindHeader) }
func (r *webIn_) AllHeaders() (headers [][2]string) { return r.allPairs(r.headers, kindHeader) }
func (r *webIn_) H(name string) string {
	value, _ := r.Header(name)
	return value
}
func (r *webIn_) Hstr(name string, defaultValue string) string {
	if value, ok := r.Header(name); ok {
		return value
	}
	return defaultValue
}
func (r *webIn_) Hint(name string, defaultValue int) int {
	if value, ok := r.Header(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *webIn_) Header(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.headers, kindHeader)
	return string(v), ok
}
func (r *webIn_) UnsafeHeader(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.headers, kindHeader)
}
func (r *webIn_) Headers(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.headers, kindHeader)
}
func (r *webIn_) HasHeader(name string) bool {
	_, ok := r.getPair(name, 0, r.headers, kindHeader)
	return ok
}
func (r *webIn_) DelHeader(name string) (deleted bool) {
	// TODO: add restrictions on what headers are allowed to del?
	return r.delPair(name, 0, r.headers, kindHeader)
}
func (r *webIn_) delHeader(name []byte, hash uint16) {
	r.delPair(risky.WeakString(name), hash, r.headers, kindHeader)
}

func (r *webIn_) _parseField(field *pair, fdesc *desc, p []byte, fully bool) bool { // for field data and value params
	field.setParsed()

	if field.value.isEmpty() {
		if fdesc.allowEmpty {
			field.dataEdge = field.value.edge
			return true
		} else {
			r.failReason = "field can't be empty"
			return false
		}
	}

	// Now parse field value.
	text := field.value
	if p[text.from] != '"' { // field value is normal text
	forData:
		for pSpace := int32(0); text.from < field.value.edge; text.from++ {
			switch b := p[text.from]; b {
			case ' ', '\t':
				if pSpace == 0 {
					pSpace = text.from
				}
			case ';':
				if pSpace == 0 {
					field.dataEdge = text.from
				} else {
					field.dataEdge = pSpace
				}
				break forData
			case ',':
				if !fully {
					field.value.edge = text.from
					field.dataEdge = text.from
					return true
				}
				pSpace = 0
			case '(':
				// TODO: comments can nest
				if fdesc.hasComment {
					text.from++
					for {
						if text.from == field.value.edge {
							r.failReason = "bad comment"
							return false
						}
						if p[text.from] == ')' {
							break
						}
						text.from++
					}
				} else { // comment is not allowed. treat as normal character and reset pSpace
					pSpace = 0
				}
			default: // normal character. reset pSpace
				pSpace = 0
			}
		}
		if text.from == field.value.edge { // exact data
			field.dataEdge = text.from
			return true
		}
	} else { // field value begins with '"'
		text.from++
		for {
			if text.from == field.value.edge { // "...
				field.dataEdge = text.from
				return true
			}
			if p[text.from] == '"' {
				break
			}
			text.from++
		}
		// "..."
		if !fdesc.allowQuote {
			r.failReason = "DQUOTE is not allowed"
			return false
		}
		if text.from-field.value.from == 1 && !fdesc.allowEmpty { // ""
			r.failReason = "field cannot be empty"
			return false
		}
		field.setQuoted()
		field.dataEdge = text.from
		if text.from++; text.from == field.value.edge { // exact "..."
			return true
		}
	afterValue:
		for {
			switch b := p[text.from]; b {
			case ';':
				break afterValue
			case ' ', '\t':
				text.from++
			case ',':
				if fully {
					r.failReason = "comma after dquote"
					return false
				} else {
					field.value.edge = text.from
					return true
				}
			default:
				r.failReason = "malformed DQUOTE and normal text"
				return false
			}
			if text.from == field.value.edge {
				return true
			}
		}
	}
	// text.from is now at ';'
	if !fdesc.allowParam {
		r.failReason = "parameters are not allowed"
		return false
	}

	// Now parse value params.
	field.params.from = uint8(len(r.extras))
	for { // each *( OWS ";" OWS [ token "=" ( token / quoted-string ) ] )
		haveSemic := false
	forSemic:
		for {
			if text.from == field.value.edge {
				return true
			}
			switch b := p[text.from]; b {
			case ' ', '\t':
				text.from++
			case ';':
				haveSemic = true
				text.from++
			case ',':
				if fully {
					r.failReason = "invalid parameter"
					return false
				} else {
					field.value.edge = text.from
					return true
				}
			default:
				break forSemic
			}
		}
		if !haveSemic {
			r.failReason = "semicolon required in parameters"
			return false
		}
		// parameter-name = token
		text.edge = text.from
		for {
			if httpTchar[p[text.edge]] == 0 {
				break
			}
			text.edge++
			if text.edge == field.value.edge {
				r.failReason = "only parameter-name is provided"
				return false
			}
		}
		nameSize := text.edge - text.from
		if nameSize == 0 || nameSize > 255 {
			r.failReason = "parameter-name out of range"
			return false
		}
		if p[text.edge] != '=' {
			r.failReason = "token '=' required"
			return false
		}
		var param pair
		param.hash = bytesHash(p[text.from:text.edge])
		param.kind = kindParam
		param.nameSize = uint8(nameSize)
		param.nameFrom = text.from
		param.place = field.place
		// parameter-value = ( token / quoted-string )
		if text.edge++; text.edge == field.value.edge {
			r.failReason = "missing parameter-value"
			return false
		}
		if p[text.edge] == '"' { // quoted-string = DQUOTE *( qdtext / quoted-pair ) DQUOTE
			text.edge++
			text.from = text.edge
			for {
				// TODO: detect qdtext
				if text.edge == field.value.edge {
					r.failReason = "invalid quoted-string"
					return false
				}
				if p[text.edge] == '"' {
					break
				}
				text.edge++
			}
			param.value = text
			text.edge++
		} else { // token
			text.from = text.edge
			for text.edge < field.value.edge && httpTchar[p[text.edge]] != 0 {
				text.edge++
			}
			if text.edge == text.from {
				r.failReason = "empty parameter-value is not allowed"
				return false
			}
			param.value = text
		}
		if !r._addExtra(&param) {
			r.failReason = "too many extras"
			return false
		}
		field.params.edge = uint8(len(r.extras))

		text.from = text.edge // for next parameter
	}
}
func (r *webIn_) _splitField(field *pair, fdesc *desc, p []byte) bool { // split: #element => [ element ] *( OWS "," OWS [ element ] )
	field.setParsed()

	subField := *field
	subField.setSubField()
	var bakField pair
	numSubs, needComma := 0, false

	for { // each sub value
		haveComma := false
	forComma:
		for subField.value.from < field.value.edge {
			switch b := p[subField.value.from]; b {
			case ' ', '\t':
				subField.value.from++
			case ',':
				haveComma = true
				subField.value.from++
			default:
				break forComma
			}
		}
		if subField.value.from == field.value.edge {
			break
		}
		if needComma && !haveComma {
			r.failReason = "comma needed in multi-value field"
			return false
		}
		subField.value.edge = field.value.edge
		if !r._parseField(&subField, fdesc, p, false) { // parse one sub field
			// r.failReason is set.
			return false
		}
		if numSubs == 0 { // first sub field, save as backup
			bakField = subField
		} else { // numSubs >= 1, sub fields exist
			if numSubs == 1 { // got the second sub field
				field.setCommaValue() // mark main field as comma-value
				if !r._addExtra(&bakField) {
					r.failReason = "too many extra fields"
					return false
				}
			}
			if !r._addExtra(&subField) {
				r.failReason = "too many sub fields"
				return false
			}
		}
		numSubs++
		subField.value.from = subField.value.edge
		needComma = true
	}
	if numSubs == 1 {
		if bakField.isQuoted() {
			field.setQuoted()
		}
		field.params = bakField.params
		field.dataEdge = bakField.dataEdge
	}
	return true
}

func (r *webIn_) checkContentLength(header *pair, index uint8) bool { // Content-Length = 1*DIGIT
	// RFC 7230 (section 3.3.2):
	// If a message is received that has multiple Content-Length header
	// fields with field-values consisting of the same decimal value, or a
	// single Content-Length header field with a field value containing a
	// list of identical decimal values (e.g., "Content-Length: 42, 42"),
	// indicating that duplicate Content-Length header fields have been
	// generated or combined by an upstream message processor, then the
	// recipient MUST either reject the message as invalid or replace the
	// duplicated field-values with a single valid Content-Length field
	// containing that decimal value prior to determining the message body
	// length or forwarding the message.
	if r.contentSize == -1 { // r.contentSize can only be -1 or >= 0 here. -2 is set later if the content is unsized
		if size, ok := decToI64(header.valueAt(r.input)); ok {
			r.contentSize = size
			r.iContentLength = index
			return true
		}
	}
	// RFC 7230 (section 3.3.3):
	// If a message is received without Transfer-Encoding and with
	// either multiple Content-Length header fields having differing
	// field-values or a single Content-Length header field having an
	// invalid value, then the message framing is invalid and the
	// recipient MUST treat it as an unrecoverable error.  If this is a
	// request message, the server MUST respond with a 400 (Bad Request)
	// status code and then close the connection.
	r.headResult, r.failReason = StatusBadRequest, "bad content-length"
	return false
}
func (r *webIn_) checkContentLocation(header *pair, index uint8) bool { // Content-Location = absolute-URI / partial-URI
	if r.iContentLocation == 0 && header.value.notEmpty() {
		r.iContentLocation = index
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-location"
	return false
}
func (r *webIn_) checkContentRange(header *pair, index uint8) bool { // Content-Range = range-unit SP ( range-resp / unsatisfied-range )
	// TODO
	if r.iContentRange == 0 && header.value.notEmpty() {
		r.iContentRange = index
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-range"
	return false
}
func (r *webIn_) checkContentType(header *pair, index uint8) bool { // Content-Type = media-type
	// media-type = type "/" subtype *( OWS ";" OWS parameter )
	// type = token
	// subtype = token
	// parameter = token "=" ( token / quoted-string )
	if r.iContentType == 0 && !header.dataEmpty() {
		r.iContentType = index
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-type"
	return false
}
func (r *webIn_) checkDate(header *pair, index uint8) bool { // Date = HTTP-date
	return r._checkHTTPDate(header, index, &r.iDate, &r.dateTime)
}
func (r *webIn_) _checkHTTPDate(header *pair, index uint8, pIndex *uint8, toTime *int64) bool { // HTTP-date = day-name "," SP day SP month SP year SP hour ":" minute ":" second SP GMT
	if *pIndex == 0 {
		if httpDate, ok := clockParseHTTPDate(header.valueAt(r.input)); ok {
			*pIndex = index
			*toTime = httpDate
			return true
		}
	}
	r.headResult, r.failReason = StatusBadRequest, "bad http-date"
	return false
}

func (r *webIn_) checkAcceptEncoding(pairs []pair, from uint8, edge uint8) bool { // Accept-Encoding = #( codings [ weight ] )
	// codings        = content-coding / "identity" / "*"
	// content-coding = token
	for i := from; i < edge; i++ {
		if r.nAcceptCodings == int8(cap(r.acceptCodings)) { // ignore too many codings
			break
		}
		pair := &pairs[i]
		if pair.kind != kindHeader {
			continue
		}
		data := pair.dataAt(r.input)
		bytesToLower(data)
		var coding uint8
		if bytes.Equal(data, bytesGzip) {
			r.acceptGzip = true
			coding = httpCodingGzip
		} else if bytes.Equal(data, bytesBrotli) {
			r.acceptBrotli = true
			coding = httpCodingBrotli
		} else if bytes.Equal(data, bytesDeflate) {
			coding = httpCodingDeflate
		} else if bytes.Equal(data, bytesCompress) {
			coding = httpCodingCompress
		} else if bytes.Equal(data, bytesIdentity) {
			coding = httpCodingIdentity
		} else {
			// Empty or unknown content-coding, ignored
			continue
		}
		r.acceptCodings[r.nAcceptCodings] = coding
		r.nAcceptCodings++
	}
	return true
}
func (r *webIn_) checkConnection(pairs []pair, from uint8, edge uint8) bool { // Connection = #connection-option
	if r.versionCode >= Version2 {
		r.headResult, r.failReason = StatusBadRequest, "connection header is not allowed in HTTP/2 and HTTP/3"
		return false
	}
	if r.zConnection.isEmpty() {
		r.zConnection.from = from
	}
	r.zConnection.edge = edge
	// connection-option = token
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		bytesToLower(data) // connection options are case-insensitive.
		if bytes.Equal(data, bytesKeepAlive) {
			r.keepAlive = 1 // to be compatible with HTTP/1.0
		} else if bytes.Equal(data, bytesClose) {
			// Furthermore, the header field-name "Close" has been registered as
			// "reserved", since using that name as an HTTP header field might
			// conflict with the "close" connection option of the Connection header
			// field (Section 6.1).
			r.keepAlive = 0
		}
	}
	return true
}
func (r *webIn_) checkContentEncoding(pairs []pair, from uint8, edge uint8) bool { // Content-Encoding = #content-coding
	// content-coding = token
	for i := from; i < edge; i++ {
		if r.nContentCodings == int8(cap(r.contentCodings)) {
			r.headResult, r.failReason = StatusBadRequest, "too many content codings applied to content"
			return false
		}
		data := pairs[i].dataAt(r.input)
		bytesToLower(data)
		var coding uint8
		if bytes.Equal(data, bytesGzip) {
			coding = httpCodingGzip
		} else if bytes.Equal(data, bytesBrotli) {
			coding = httpCodingBrotli
		} else if bytes.Equal(data, bytesDeflate) { // this is in fact zlib format
			coding = httpCodingDeflate // some non-conformant implementations send the "deflate" compressed data without the zlib wrapper :(
		} else if bytes.Equal(data, bytesCompress) {
			coding = httpCodingCompress
		} else {
			// RFC 7231 (section 3.1.2.2):
			// An origin server MAY respond with a status code of 415 (Unsupported
			// Media Type) if a representation in the request message has a content
			// coding that is not acceptable.

			// TODO: but we can be proxies too...
			r.headResult, r.failReason = StatusUnsupportedMediaType, "currently only gzip, deflate, compress, and br are supported"
			return false
		}
		r.contentCodings[r.nContentCodings] = coding
		r.nContentCodings++
	}
	return true
}
func (r *webIn_) checkContentLanguage(pairs []pair, from uint8, edge uint8) bool { // Content-Language = #language-tag
	if r.zContentLanguage.isEmpty() {
		r.zContentLanguage.from = from
	}
	r.zContentLanguage.edge = edge
	return true
}
func (r *webIn_) checkTrailer(pairs []pair, from uint8, edge uint8) bool { // Trailer = #field-name
	// field-name = token
	if r.zTrailer.isEmpty() {
		r.zTrailer.from = from
	}
	r.zTrailer.edge = edge
	return true
}
func (r *webIn_) checkTransferEncoding(pairs []pair, from uint8, edge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.versionCode != Version1_1 {
		r.headResult, r.failReason = StatusBadRequest, "transfer-encoding is only allowed in http/1.1"
		return false
	}
	// transfer-coding = "chunked" / "compress" / "deflate" / "gzip"
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		bytesToLower(data)
		if bytes.Equal(data, bytesChunked) {
			r.transferChunked = true
		} else {
			// RFC 7230 (section 3.3.1):
			// A server that receives a request message with a transfer coding it
			// does not understand SHOULD respond with 501 (Not Implemented).
			r.headResult, r.failReason = StatusNotImplemented, "unknown transfer coding"
			return false
		}
	}
	return true
}
func (r *webIn_) checkVia(pairs []pair, from uint8, edge uint8) bool { // Via = #( received-protocol RWS received-by [ RWS comment ] )
	if r.zVia.isEmpty() {
		r.zVia.from = from
	}
	r.zVia.edge = edge
	return true
}

func (r *webIn_) determineContentMode() bool {
	if r.transferChunked { // must be HTTP/1.1 and there is a transfer-encoding: chunked
		if r.contentSize != -1 { // there is a content-length: nnn
			// RFC 7230 (section 3.3.3):
			// If a message is received with both a Transfer-Encoding and a
			// Content-Length header field, the Transfer-Encoding overrides the
			// Content-Length.  Such a message might indicate an attempt to
			// perform request smuggling (Section 9.5) or response splitting
			// (Section 9.4) and ought to be handled as an error.  A sender MUST
			// remove the received Content-Length field prior to forwarding such
			// a message downstream.
			r.headResult, r.failReason = StatusBadRequest, "transfer-encoding conflits with content-length"
			return false
		}
		r.contentSize = -2 // unsized
	} else if r.versionCode >= Version2 && r.contentSize == -1 { // no content-length header
		// TODO: if there is no content, HTTP/2 and HTTP/3 will mark END_STREAM in headers frame. use this to decide!
		r.contentSize = -2 // if there is no content-length in HTTP/2 or HTTP/3, we treat it as unsized
	}
	return true
}
func (r *webIn_) markUnsized()    { r.contentSize = -2 }
func (r *webIn_) IsUnsized() bool { return r.contentSize == -2 }

func (r *webIn_) ContentSize() int64 { return r.contentSize }
func (r *webIn_) UnsafeContentLength() []byte {
	if r.iContentLength == 0 {
		return nil
	}
	return r.primes[r.iContentLength].valueAt(r.input)
}
func (r *webIn_) ContentType() string { return string(r.UnsafeContentType()) }
func (r *webIn_) UnsafeContentType() []byte {
	if r.iContentType == 0 {
		return nil
	}
	return r.primes[r.iContentType].dataAt(r.input)
}

func (r *webIn_) SetRecvTimeout(timeout time.Duration) { r.recvTimeout = timeout }

func (r *webIn_) unsafeContent() []byte {
	r.loadContent()
	if r.stream.isBroken() {
		return nil
	}
	return r.contentText[0:r.receivedSize]
}
func (r *webIn_) loadContent() { // into memory. [0, r.maxContentSize]
	if r.contentReceived {
		// Content is in r.contentText already.
		return
	}
	r.contentReceived = true
	switch content := r.recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		r.contentText = content // real content is r.contentText[:r.receivedSize]
		r.contentTextKind = webContentTextPool
	case tempFile: // [0, r.maxContentSize]. case happens when sized content > 64K1, or content is unsized.
		contentFile := content.(*os.File)
		if r.receivedSize == 0 { // unsized content can has 0 size
			r.contentText = r.input
			r.contentTextKind = webContentTextInput
		} else { // r.receivedSize > 0
			if r.receivedSize <= _64K1 { // must be unsized content because sized content is a []byte if <= _64K1
				r.contentText = GetNK(r.receivedSize) // 4K/16K/64K1. real content is r.content[:r.receivedSize]
				r.contentTextKind = webContentTextPool
			} else { // r.receivedSize > 64K1, content can be sized or unsized. just alloc
				r.contentText = make([]byte, r.receivedSize)
				r.contentTextKind = webContentTextMake
			}
			if _, err := io.ReadFull(contentFile, r.contentText[:r.receivedSize]); err != nil {
				// TODO: r.app.log
			}
		}
		contentFile.Close()
		if IsDebug(2) {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(contentFile.Name()); err != nil {
			// TODO: r.app.log
		}
	case error: // i/o error or unexpected EOF
		// TODO: log error?
		r.stream.markBroken()
	}
}
func (r *webIn_) takeContent() any { // used by proxies
	if r.contentReceived {
		if r.contentFile == nil {
			return r.contentText // immediate
		}
		return r.contentFile
	}
	r.contentReceived = true
	switch content := r.recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		r.contentText = content
		r.contentTextKind = webContentTextPool // so r.contentText can be freed on end
		return r.contentText[0:r.receivedSize]
	case tempFile: // [0, r.maxContentSize]. case happens when sized content > 64K1, or content is unsized.
		r.contentFile = content.(*os.File)
		return r.contentFile
	case error: // i/o error or unexpected EOF
		// TODO: log err?
	}
	r.stream.markBroken()
	return nil
}
func (r *webIn_) dropContent() { // if message content is not received, this will be called at last
	switch content := r.recvContent(false).(type) { // don't retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		PutNK(content)
	case tempFile: // [0, r.maxContentSize]. case happens when sized content > 64K1, or content is unsized.
		if content != fakeFile { // this must not happen!
			BugExitln("temp file is not fake when dropping content")
		}
	case error: // i/o error or unexpected EOF
		// TODO: log error?
		r.stream.markBroken()
	}
}
func (r *webIn_) recvContent(retain bool) any { // to []byte (for small content <= 64K1) or tempFile (for large content > 64K1, or unsized content)
	if r.contentSize > 0 && r.contentSize <= _64K1 { // (0, 64K1]. save to []byte. must be received in a timeout
		if err := r.stream.setReadDeadline(time.Now().Add(r.stream.webKeeper().ReadTimeout())); err != nil {
			return err
		}
		// Since content is small, r.bodyWindow and tempFile are not needed.
		contentText := GetNK(r.contentSize) // 4K/16K/64K1. max size of content is 64K1
		r.receivedSize = int64(r.imme.size())
		if r.receivedSize > 0 { // r.imme has data
			copy(contentText, r.input[r.imme.from:r.imme.edge])
			r.imme.zero()
		}
		n, err := r.stream.readFull(contentText[r.receivedSize:r.contentSize])
		if err != nil {
			PutNK(contentText)
			return err
		}
		r.receivedSize += int64(n)
		return contentText // []byte, fetched from pool
	} else { // (64K1, r.maxContentSize] when sized, or [0, r.maxContentSize] when unsized. save to tempFile and return the file
		contentFile, err := r._newTempFile(retain)
		if err != nil {
			return err
		}
		var p []byte
		for {
			p, err = r.shell.readContent()
			if len(p) > 0 { // skip 0, nothing to write
				if _, e := contentFile.Write(p); e != nil {
					err = e
					goto badRead
				}
			}
			if err == io.EOF {
				break
			} else if err != nil {
				goto badRead
			}
		}
		if _, err = contentFile.Seek(0, 0); err != nil {
			goto badRead
		}
		return contentFile // the tempFile
	badRead:
		contentFile.Close()
		if retain { // the tempFile is not fake, so must remove.
			os.Remove(contentFile.Name())
		}
		return err
	}
}

func (r *webIn_) addTrailer(trailer *pair) bool { // as prime
	if edge, ok := r._addPrime(trailer); ok {
		r.trailers.edge = edge
		return true
	}
	r.bodyResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many trailers"
	return false
}
func (r *webIn_) AddTrailer(name string, value string) bool { // as extra
	// TODO: add restrictions on what trailers are allowed to add? should we check the value?
	// TODO: parse and check?
	// setFlags?
	return r.addExtra(name, value, 0, kindTrailer)
}
func (r *webIn_) HasTrailers() bool                   { return r.hasPairs(r.trailers, kindTrailer) }
func (r *webIn_) AllTrailers() (trailers [][2]string) { return r.allPairs(r.trailers, kindTrailer) }
func (r *webIn_) T(name string) string {
	value, _ := r.Trailer(name)
	return value
}
func (r *webIn_) Tstr(name string, defaultValue string) string {
	if value, ok := r.Trailer(name); ok {
		return value
	}
	return defaultValue
}
func (r *webIn_) Tint(name string, defaultValue int) int {
	if value, ok := r.Trailer(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *webIn_) Trailer(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.trailers, kindTrailer)
	return string(v), ok
}
func (r *webIn_) UnsafeTrailer(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.trailers, kindTrailer)
}
func (r *webIn_) Trailers(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.trailers, kindTrailer)
}
func (r *webIn_) HasTrailer(name string) bool {
	_, ok := r.getPair(name, 0, r.trailers, kindTrailer)
	return ok
}
func (r *webIn_) DelTrailer(name string) (deleted bool) {
	return r.delPair(name, 0, r.trailers, kindTrailer)
}
func (r *webIn_) delTrailer(name []byte, hash uint16) {
	r.delPair(risky.WeakString(name), hash, r.trailers, kindTrailer)
}

func (r *webIn_) examineTail() bool {
	for i := r.trailers.from; i < r.trailers.edge; i++ {
		if !r.shell.applyTrailer(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}

func (r *webIn_) arrayPush(b byte) {
	r.array[r.arrayEdge] = b
	if r.arrayEdge++; r.arrayEdge == int32(cap(r.array)) {
		r._growArray(1)
	}
}
func (r *webIn_) _growArray(size int32) bool { // stock->4K->16K->64K1->(128K->...->1G)
	edge := r.arrayEdge + size
	if edge < 0 || edge > _1G { // cannot overflow hard limit: 1G
		return false
	}
	if edge <= int32(cap(r.array)) {
		return true
	}
	arrayKind := r.arrayKind
	var array []byte
	if edge <= _64K1 { // (stock, 64K1]
		r.arrayKind = arrayKindPool
		array = GetNK(int64(edge)) // 4K/16K/64K1
	} else { // > _64K1
		r.arrayKind = arrayKindMake
		if edge <= _128K {
			array = make([]byte, _128K)
		} else if edge <= _256K {
			array = make([]byte, _256K)
		} else if edge <= _512K {
			array = make([]byte, _512K)
		} else if edge <= _1M {
			array = make([]byte, _1M)
		} else if edge <= _2M {
			array = make([]byte, _2M)
		} else if edge <= _4M {
			array = make([]byte, _4M)
		} else if edge <= _8M {
			array = make([]byte, _8M)
		} else if edge <= _16M {
			array = make([]byte, _16M)
		} else if edge <= _32M {
			array = make([]byte, _32M)
		} else if edge <= _64M {
			array = make([]byte, _64M)
		} else if edge <= _128M {
			array = make([]byte, _128M)
		} else if edge <= _256M {
			array = make([]byte, _256M)
		} else if edge <= _512M {
			array = make([]byte, _512M)
		} else { // <= _1G
			array = make([]byte, _1G)
		}
	}
	copy(array, r.array[0:r.arrayEdge])
	if arrayKind == arrayKindPool {
		PutNK(r.array)
	}
	r.array = array
	return true
}

func (r *webIn_) _addPrime(prime *pair) (edge uint8, ok bool) {
	if len(r.primes) == cap(r.primes) { // full
		if cap(r.primes) != cap(r.stockPrimes) { // too many primes
			return 0, false
		}
		if IsDebug(2) {
			Println("use large primes!")
		}
		r.primes = getPairs()
		r.primes = append(r.primes, r.stockPrimes[:]...)
	}
	r.primes = append(r.primes, *prime)
	return uint8(len(r.primes)), true
}
func (r *webIn_) _delPrime(i uint8) { r.primes[i].zero() }

func (r *webIn_) addExtra(name string, value string, hash uint16, extraKind int8) bool {
	nameSize := len(name)
	if nameSize == 0 || nameSize > 255 { // name size is limited at 255
		return false
	}
	valueSize := len(value)
	if extraKind == kindForm { // for forms, max value size is 1G
		if valueSize > _1G {
			return false
		}
	} else if valueSize > _16K { // for non-forms, max value size is 16K
		return false
	}
	if !r._growArray(int32(nameSize + valueSize)) { // extras are always placed in r.array
		return false
	}
	extra := &r.mainPair
	extra.zero()
	if hash == 0 {
		extra.hash = stringHash(name)
	} else {
		extra.hash = hash
	}
	extra.kind = extraKind
	extra.place = placeArray
	extra.nameFrom = r.arrayEdge
	extra.nameSize = uint8(nameSize)
	r.arrayEdge += int32(copy(r.array[r.arrayEdge:], name))
	extra.value.from = r.arrayEdge
	r.arrayEdge += int32(copy(r.array[r.arrayEdge:], value))
	extra.value.edge = r.arrayEdge
	return r._addExtra(extra)
}
func (r *webIn_) _addExtra(extra *pair) bool {
	if len(r.extras) == cap(r.extras) { // full
		if cap(r.extras) != cap(r.stockExtras) { // too many extras
			return false
		}
		if IsDebug(2) {
			Println("use large extras!")
		}
		r.extras = getPairs()
		r.extras = append(r.extras, r.stockExtras[:]...)
	}
	r.extras = append(r.extras, *extra)
	r.hasExtra[extra.kind] = true
	return true
}

func (r *webIn_) hasPairs(primes zone, extraKind int8) bool {
	return primes.notEmpty() || r.hasExtra[extraKind]
}
func (r *webIn_) allPairs(primes zone, extraKind int8) [][2]string {
	var pairs [][2]string
	if extraKind == kindHeader || extraKind == kindTrailer { // skip sub fields, only collect values of main fields
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.hash != 0 {
				p := r._placeOf(prime)
				pairs = append(pairs, [2]string{string(prime.nameAt(p)), string(prime.valueAt(p))})
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.hash != 0 && extra.kind == extraKind && !extra.isSubField() {
					pairs = append(pairs, [2]string{string(extra.nameAt(r.array)), string(extra.valueAt(r.array))})
				}
			}
		}
	} else { // queries, cookies, forms, and params
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.hash != 0 {
				p := r._placeOf(prime)
				pairs = append(pairs, [2]string{string(prime.nameAt(p)), string(prime.valueAt(p))})
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.hash != 0 && extra.kind == extraKind {
					pairs = append(pairs, [2]string{string(extra.nameAt(r.array)), string(extra.valueAt(r.array))})
				}
			}
		}
	}
	return pairs
}
func (r *webIn_) getPair(name string, hash uint16, primes zone, extraKind int8) (value []byte, ok bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		if extraKind == kindHeader || extraKind == kindTrailer { // skip comma fields, only collect data of fields without comma
			for i := primes.from; i < primes.edge; i++ {
				if prime := &r.primes[i]; prime.hash == hash {
					if p := r._placeOf(prime); prime.nameEqualString(p, name) {
						if !prime.isParsed() && !r._splitField(prime, defaultDesc, p) {
							continue
						}
						if !prime.isCommaValue() {
							return prime.dataAt(p), true
						}
					}
				}
			}
			if r.hasExtra[extraKind] {
				for i := 0; i < len(r.extras); i++ {
					if extra := &r.extras[i]; extra.hash == hash && extra.kind == extraKind && !extra.isCommaValue() {
						if p := r._placeOf(extra); extra.nameEqualString(p, name) {
							return extra.dataAt(p), true
						}
					}
				}
			}
		} else { // queries, cookies, forms, and params
			for i := primes.from; i < primes.edge; i++ {
				if prime := &r.primes[i]; prime.hash == hash {
					if p := r._placeOf(prime); prime.nameEqualString(p, name) {
						return prime.valueAt(p), true
					}
				}
			}
			if r.hasExtra[extraKind] {
				for i := 0; i < len(r.extras); i++ {
					if extra := &r.extras[i]; extra.hash == hash && extra.kind == extraKind && extra.nameEqualString(r.array, name) {
						return extra.valueAt(r.array), true
					}
				}
			}
		}
	}
	return
}
func (r *webIn_) getPairs(name string, hash uint16, primes zone, extraKind int8) (values []string, ok bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		if extraKind == kindHeader || extraKind == kindTrailer { // skip comma fields, only collect data of fields without comma
			for i := primes.from; i < primes.edge; i++ {
				if prime := &r.primes[i]; prime.hash == hash {
					if p := r._placeOf(prime); prime.nameEqualString(p, name) {
						if !prime.isParsed() && !r._splitField(prime, defaultDesc, p) {
							continue
						}
						if !prime.isCommaValue() {
							values = append(values, string(prime.dataAt(p)))
						}
					}
				}
			}
			if r.hasExtra[extraKind] {
				for i := 0; i < len(r.extras); i++ {
					if extra := &r.extras[i]; extra.hash == hash && extra.kind == extraKind && !extra.isCommaValue() {
						if p := r._placeOf(extra); extra.nameEqualString(p, name) {
							values = append(values, string(extra.dataAt(p)))
						}
					}
				}
			}
		} else { // queries, cookies, forms, and params
			for i := primes.from; i < primes.edge; i++ {
				if prime := &r.primes[i]; prime.hash == hash {
					if p := r._placeOf(prime); prime.nameEqualString(p, name) {
						values = append(values, string(prime.valueAt(p)))
					}
				}
			}
			if r.hasExtra[extraKind] {
				for i := 0; i < len(r.extras); i++ {
					if extra := &r.extras[i]; extra.hash == hash && extra.kind == extraKind && extra.nameEqualString(r.array, name) {
						values = append(values, string(extra.valueAt(r.array)))
					}
				}
			}
		}
		if len(values) > 0 {
			ok = true
		}
	}
	return
}
func (r *webIn_) delPair(name string, hash uint16, primes zone, extraKind int8) (deleted bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.hash == hash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					prime.zero()
					deleted = true
				}
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.hash == hash && extra.kind == extraKind && extra.nameEqualString(r.array, name) {
					extra.zero()
					deleted = true
				}
			}
		}
	}
	return
}
func (r *webIn_) _placeOf(pair *pair) []byte {
	var place []byte
	if pair.place == placeInput {
		place = r.input
	} else if pair.place == placeArray {
		place = r.array
	} else if pair.place == placeStatic2 {
		place = http2BytesStatic
	} else if pair.place == placeStatic3 {
		place = http3BytesStatic
	} else {
		BugExitln("unknown pair.place")
	}
	return place
}

func (r *webIn_) delHopHeaders() { // used by proxies
	r._delHopFields(r.headers, kindHeader, r.delHeader)
}
func (r *webIn_) forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool { // by webOut.copyHeadFrom(). excluding sub headers
	return r._forMainFields(r.headers, kindHeader, callback)
}

func (r *webIn_) delHopTrailers() { // used by proxies
	r._delHopFields(r.trailers, kindTrailer, r.delTrailer)
}
func (r *webIn_) forTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool { // by webOut.copyTrailersFrom(). excluding sub trailers
	return r._forMainFields(r.trailers, kindTrailer, callback)
}

func (r *webIn_) _delHopFields(fields zone, extraKind int8, delField func(name []byte, hash uint16)) { // TODO: improve performance
	// These fields should be removed anyway: proxy-connection, keep-alive, te, transfer-encoding, upgrade
	delField(bytesProxyConnection, hashProxyConnection)
	delField(bytesKeepAlive, hashKeepAlive)
	if !r.asResponse { // as request
		delField(bytesTE, hashTE)
	}
	delField(bytesTransferEncoding, hashTransferEncoding)
	delField(bytesUpgrade, hashUpgrade)

	for i := r.zConnection.from; i < r.zConnection.edge; i++ {
		prime := &r.primes[i]
		// Skip fields that are not "connection: xxx"
		if prime.hash != hashConnection || !prime.nameEqualBytes(r.input, bytesConnection) {
			continue
		}
		p := r._placeOf(prime)
		optionName := prime.dataAt(p)
		optionHash := bytesHash(optionName)
		// Skip options that are "connection: connection"
		if optionHash == hashConnection && bytes.Equal(optionName, bytesConnection) {
			continue
		}
		// Now remove options in primes and extras. Note: we don't remove ("connection: xxx") itself, since we simply ignore it when acting as a proxy.
		for j := fields.from; j < fields.edge; j++ {
			if field := &r.primes[j]; field.hash == optionHash && field.nameEqualBytes(p, optionName) {
				field.zero()
			}
		}
		if !r.hasExtra[extraKind] {
			continue
		}
		for i := 0; i < len(r.extras); i++ {
			if extra := &r.extras[i]; extra.hash == optionHash && extra.kind == extraKind {
				if p := r._placeOf(extra); extra.nameEqualBytes(p, optionName) {
					extra.zero()
				}
			}
		}
	}
}
func (r *webIn_) _forMainFields(fields zone, extraKind int8, callback func(field *pair, name []byte, value []byte) bool) bool {
	for i := fields.from; i < fields.edge; i++ {
		if field := &r.primes[i]; field.hash != 0 {
			p := r._placeOf(field)
			if !callback(field, field.nameAt(p), field.valueAt(p)) {
				return false
			}
		}
	}
	if r.hasExtra[extraKind] {
		for i := 0; i < len(r.extras); i++ {
			if field := &r.extras[i]; field.hash != 0 && field.kind == extraKind && !field.isSubField() {
				if !callback(field, field.nameAt(r.array), field.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *webIn_) _newTempFile(retain bool) (tempFile, error) { // to save content to
	if !retain { // since data is not used by upper caller, we don't need to actually write data to file.
		return fakeFile, nil
	}
	filesDir := r.shell.saveContentFilesDir()
	pathSize := len(filesDir)
	filePath := r.UnsafeMake(pathSize + 19) // 19 bytes is enough for int64
	copy(filePath, filesDir)
	from, edge := r.stream.makeTempName(filePath[pathSize:], r.recvTime.Unix())
	pathSize += copy(filePath[pathSize:], filePath[pathSize+from:pathSize+edge])
	return os.OpenFile(risky.WeakString(filePath[:pathSize]), os.O_RDWR|os.O_CREATE, 0644)
}
func (r *webIn_) _beforeRead(toTime *time.Time) error {
	now := time.Now()
	if toTime.IsZero() {
		*toTime = now
	}
	return r.stream.setReadDeadline(now.Add(r.stream.webKeeper().ReadTimeout()))
}
func (r *webIn_) _tooSlow() bool {
	return r.recvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.recvTimeout
}

const ( // Web incoming content text kinds
	webContentTextNone  = iota // must be 0
	webContentTextInput        // refers to r.input
	webContentTextPool         // fetched from pool
	webContentTextMake         // direct make
)

var ( // web incoming message errors
	webInBadChunk = errors.New("bad chunk")
	webInTooSlow  = errors.New("http incoming too slow")
)

// webOut is the interface for *http[1-3]Response, *hwebResponse, *H[1-3]Request, and *HRequest. Used as shell by webOut_.
type webOut interface {
	control() []byte
	addHeader(name []byte, value []byte) bool
	header(name []byte) (value []byte, ok bool)
	hasHeader(name []byte) bool
	delHeader(name []byte) (deleted bool)
	delHeaderAt(o uint8)
	insertHeader(hash uint16, name []byte, value []byte) bool
	removeHeader(hash uint16, name []byte) (deleted bool)
	addedHeaders() []byte
	fixedHeaders() []byte
	finalizeHeaders()
	doSend() error
	sendChain() error // content
	beforeRevise()
	echoHeaders() error
	doEcho() error
	echoChain() error // chunks
	addTrailer(name []byte, value []byte) bool
	trailer(name []byte) (value []byte, ok bool)
	finalizeUnsized() error
	passHeaders() error       // used by proxies
	passBytes(p []byte) error // used by proxies
}

// webOut_ is a mixin for serverResponse_ and clientRequest_.
type webOut_ struct { // outgoing. needs building
	// Assocs
	shell  webOut    // *http[1-3]Response, *hwebResponse, *H[1-3]Request, *HRequest
	stream webStream // *http[1-3]Stream, *hwebExchan, *H[1-3]Stream, *HExchan
	// Stream states (stocks)
	stockFields [1536]byte // for r.fields
	// Stream states (controlled)
	edges [128]uint16 // edges of headers or trailers in r.fields. not used at the same time. controlled by r.nHeaders or r.nTrailers. edges[0] is not used!
	piece Piece       // for r.chain. used when sending content or echoing chunks
	chain Chain       // outgoing piece chain. used when sending content or echoing chunks
	// Stream states (non-zeros)
	fields      []byte        // bytes of the headers or trailers which are not present at the same time. [<r.stockFields>/4K/16K]
	sendTimeout time.Duration // timeout to send the whole message
	contentSize int64         // info of outgoing content. -1: not set, -2: unsized, >=0: size
	versionCode uint8         // Version1_1, Version2, Version3
	asRequest   bool          // use message as request?
	nHeaders    uint8         // 1+num of added headers, starts from 1 because edges[0] is not used
	nTrailers   uint8         // 1+num of added trailers, starts from 1 because edges[0] is not used
	// Stream states (zeros)
	sendTime    time.Time   // the time when first send operation is performed
	vector      net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector [4][]byte   // for sending/echoing message. reset after stream
	webOut0                 // all values must be zero by default in this struct!
}
type webOut0 struct { // for fast reset, entirely
	controlEdge   uint16 // edge of control in r.fields. only used by request to mark the method and request-target
	fieldsEdge    uint16 // edge of r.fields. max size of r.fields must be <= 16K. used by both headers and trailers because they are not present at the same time
	hasRevisers   bool   // are there any outgoing revisers hooked on this outgoing message?
	isSent        bool   // whether the message is sent
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
	oContentType  uint8  // position of content-type in r.edges
	oDate         uint8  // position of date in r.edges
}

func (r *webOut_) onUse(versionCode uint8, asRequest bool) { // for non-zeros
	r.fields = r.stockFields[:]
	r.sendTimeout = r.stream.webKeeper().SendTimeout()
	r.contentSize = -1 // not set
	r.versionCode = versionCode
	r.asRequest = asRequest
	r.nHeaders, r.nTrailers = 1, 1 // r.edges[0] is not used
}
func (r *webOut_) onEnd() { // for zeros
	if cap(r.fields) != cap(r.stockFields) {
		PutNK(r.fields)
		r.fields = nil
	}
	// r.piece is reset in echo(), and will be reset below if send() is used. double free doesn't matter
	r.chain.free()

	r.sendTime = time.Time{}
	r.vector = nil
	r.fixedVector = [4][]byte{}
	r.webOut0 = webOut0{}
}

func (r *webOut_) unsafeMake(size int) []byte { return r.stream.unsafeMake(size) }

func (r *webOut_) AddContentType(contentType string) bool {
	return r.AddHeaderBytes(bytesContentType, risky.ConstBytes(contentType))
}

func (r *webOut_) Header(name string) (value string, ok bool) {
	v, ok := r.shell.header(risky.ConstBytes(name))
	return string(v), ok
}
func (r *webOut_) HasHeader(name string) bool {
	return r.shell.hasHeader(risky.ConstBytes(name))
}
func (r *webOut_) AddHeader(name string, value string) bool {
	return r.AddHeaderBytes(risky.ConstBytes(name), risky.ConstBytes(value))
}
func (r *webOut_) AddHeaderBytes(name []byte, value []byte) bool {
	hash, valid, lower := r._nameCheck(name)
	if !valid {
		return false
	}
	for _, b := range value { // to prevent response splitting
		if b == '\r' || b == '\n' {
			return false
		}
	}
	return r.shell.insertHeader(hash, lower, value)
}
func (r *webOut_) DelHeader(name string) bool {
	return r.DelHeaderBytes(risky.ConstBytes(name))
}
func (r *webOut_) DelHeaderBytes(name []byte) bool {
	hash, valid, lower := r._nameCheck(name)
	if !valid {
		return false
	}
	return r.shell.removeHeader(hash, lower)
}
func (r *webOut_) _nameCheck(name []byte) (hash uint16, valid bool, lower []byte) { // TODO: improve performance
	n := len(name)
	if n == 0 || n > 255 {
		return 0, false, nil
	}
	allLower := true
	for i := 0; i < n; i++ {
		if b := name[i]; b >= 'a' && b <= 'z' || b == '-' {
			hash += uint16(b)
		} else {
			hash = 0
			allLower = false
			break
		}
	}
	if allLower {
		return hash, true, name
	}
	buffer := r.stream.buffer256()
	for i := 0; i < n; i++ {
		b := name[i]
		if b >= 'A' && b <= 'Z' {
			b += 0x20 // to lower
		} else if !(b >= 'a' && b <= 'z' || b == '-') {
			return 0, false, nil
		}
		hash += uint16(b)
		buffer[i] = b
	}
	return hash, true, buffer[:n]
}

func (r *webOut_) markUnsized()    { r.contentSize = -2 }
func (r *webOut_) isUnsized() bool { return r.contentSize == -2 }
func (r *webOut_) markSent()       { r.isSent = true }
func (r *webOut_) IsSent() bool    { return r.isSent }

func (r *webOut_) appendContentType(contentType []byte) (ok bool) {
	return r._appendSingleton(&r.oContentType, bytesContentType, contentType)
}
func (r *webOut_) appendDate(date []byte) (ok bool) {
	return r._appendSingleton(&r.oDate, bytesDate, date)
}

func (r *webOut_) deleteContentType() (deleted bool) { return r._deleteSingleton(&r.oContentType) }
func (r *webOut_) deleteDate() (deleted bool)        { return r._deleteSingleton(&r.oDate) }

func (r *webOut_) _appendSingleton(pIndex *uint8, name []byte, value []byte) bool {
	if *pIndex > 0 || !r.shell.addHeader(name, value) {
		return false
	}
	*pIndex = r.nHeaders - 1 // r.nHeaders begins from 1, so must minus one
	return true
}
func (r *webOut_) _deleteSingleton(pIndex *uint8) bool {
	if *pIndex == 0 { // not exist
		return false
	}
	r.shell.delHeaderAt(*pIndex)
	*pIndex = 0
	return true
}

func (r *webOut_) _setUnixTime(pUnixTime *int64, pIndex *uint8, unixTime int64) bool {
	if unixTime < 0 {
		return false
	}
	if *pUnixTime == -2 { // set through general api
		r.shell.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = unixTime
	return true
}
func (r *webOut_) _addUnixTime(pUnixTime *int64, pIndex *uint8, name []byte, httpDate []byte) bool {
	if *pUnixTime == -2 {
		r.shell.delHeaderAt(*pIndex)
		*pIndex = 0
	} else { // >= 0 or -1
		*pUnixTime = -2
	}
	if !r.shell.addHeader(name, httpDate) {
		return false
	}
	*pIndex = r.nHeaders - 1 // r.nHeaders begins from 1, so must minus one
	return true
}
func (r *webOut_) _delUnixTime(pUnixTime *int64, pIndex *uint8) bool {
	if *pUnixTime == -1 {
		return false
	}
	if *pUnixTime == -2 {
		r.shell.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = -1
	return true
}

func (r *webOut_) SetSendTimeout(timeout time.Duration) { r.sendTimeout = timeout }

func (r *webOut_) Send(content string) error      { return r.sendText(risky.ConstBytes(content)) }
func (r *webOut_) SendBytes(content []byte) error { return r.sendText(content) }
func (r *webOut_) SendJSON(content any) error {
	// TODO: optimize performance
	r.appendContentType(bytesTypeJSON)
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	return r.sendText(data)
}
func (r *webOut_) SendFile(contentPath string) error {
	file, err := os.Open(contentPath)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	return r.sendFile(file, info, true) // true to close on end
}

func (r *webOut_) Echo(chunk string) error      { return r.echoText(risky.ConstBytes(chunk)) }
func (r *webOut_) EchoBytes(chunk []byte) error { return r.echoText(chunk) }
func (r *webOut_) EchoFile(chunkPath string) error {
	file, err := os.Open(chunkPath)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	return r.echoFile(file, info, true) // true to close on end
}

func (r *webOut_) AddTrailer(name string, value string) bool {
	return r.AddTrailerBytes(risky.ConstBytes(name), risky.ConstBytes(value))
}
func (r *webOut_) AddTrailerBytes(name []byte, value []byte) bool {
	if r.isSent { // trailers must be added after headers & content are sent, otherwise r.fields will be messed up
		return r.shell.addTrailer(name, value)
	}
	return false
}
func (r *webOut_) Trailer(name string) (value string, ok bool) {
	v, ok := r.shell.trailer(risky.ConstBytes(name))
	return string(v), ok
}

func (r *webOut_) pass(in webIn) error { // used by proxes, to sync content directly
	pass := r.shell.passBytes
	if in.IsUnsized() || r.hasRevisers { // if we need to revise, we always use unsized no matter the original content is sized or unsized
		pass = r.EchoBytes
	} else { // in is sized and there are no revisers, use passBytes
		r.isSent = true
		r.contentSize = in.ContentSize()
		// TODO: find a way to reduce i/o syscalls if content is small?
		if err := r.shell.passHeaders(); err != nil {
			return err
		}
	}
	for {
		p, err := in.readContent()
		if len(p) >= 0 {
			if e := pass(p); e != nil {
				return e
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if in.HasTrailers() { // added trailers will be written by upper code eventually.
		if !in.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
			return r.shell.addTrailer(name, value)
		}) {
			return webOutTrailerFailed
		}
	}
	return nil
}
func (r *webOut_) post(content any, hasTrailers bool) error { // used by proxies, to post held content
	if contentText, ok := content.([]byte); ok {
		if hasTrailers { // if (in the future) we supports taking unsized content in buffer, this happens
			return r.echoText(contentText)
		} else {
			return r.sendText(contentText)
		}
	} else if contentFile, ok := content.(*os.File); ok {
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		if hasTrailers { // we must use unsized
			return r.echoFile(contentFile, fileInfo, false) // false means don't close on end. this file belongs to webIn
		} else {
			return r.sendFile(contentFile, fileInfo, false) // false means don't close on end. this file belongs to webIn
		}
	} else { // nil means no content.
		if err := r._beforeSend(); err != nil {
			return err
		}
		r.forbidContent = true
		return r.shell.doSend()
	}
}

func (r *webOut_) sendText(content []byte) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content)) // original size, may be revised
	return r.shell.doSend()
}
func (r *webOut_) sendFile(content *os.File, info os.FileInfo, shut bool) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetFile(content, info, shut)
	r.chain.PushTail(&r.piece)
	r.contentSize = info.Size() // original size, may be revised
	return r.shell.doSend()
}
func (r *webOut_) _beforeSend() error {
	if r.isSent {
		return webOutAlreadySent
	}
	r.isSent = true
	return nil
}

func (r *webOut_) echoText(chunk []byte) error {
	if err := r._beforeEcho(); err != nil {
		return err
	}
	if len(chunk) == 0 { // empty chunk is not actually sent, since it is used to indicate the end
		return nil
	}
	r.piece.SetText(chunk)
	defer r.piece.zero()
	return r.shell.doEcho()
}
func (r *webOut_) echoFile(chunk *os.File, info os.FileInfo, shut bool) error {
	if err := r._beforeEcho(); err != nil {
		return err
	}
	if info.Size() == 0 { // empty chunk is not actually sent, since it is used to indicate the end
		if shut {
			chunk.Close()
		}
		return nil
	}
	r.piece.SetFile(chunk, info, shut)
	defer r.piece.zero()
	return r.shell.doEcho()
}
func (r *webOut_) _beforeEcho() error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if r.IsSent() {
		return nil
	}
	if r.contentSize != -1 {
		return webOutMixedContent
	}
	r.markSent()
	r.markUnsized()
	/*
		if r.hasRevisers {
			r.shell.beforeRevise()
		}
	*/
	return r.shell.echoHeaders()
}

func (r *webOut_) growHeader(size int) (from int, edge int, ok bool) { // headers and trailers are not present at the same time
	if r.nHeaders == uint8(cap(r.edges)) { // too many headers
		return
	}
	return r._growFields(size)
}
func (r *webOut_) growTrailer(size int) (from int, edge int, ok bool) { // headers and trailers are not present at the same time
	if r.nTrailers == uint8(cap(r.edges)) { // too many trailers
		return
	}
	return r._growFields(size)
}
func (r *webOut_) _growFields(size int) (from int, edge int, ok bool) { // used by both growHeader and growTrailer as they are not used at the same time
	if size <= 0 || size > _16K { // size allowed: (0, 16K]
		BugExitln("invalid size in _growFields")
	}
	from = int(r.fieldsEdge)
	ceil := r.fieldsEdge + uint16(size)
	last := ceil + 256 // we reserve 256 bytes at the end of r.fields for finalizeHeaders()
	if ceil < r.fieldsEdge || last > _16K || last < ceil {
		// Overflow
		return
	}
	if last > uint16(cap(r.fields)) { // last <= _16K
		fields := GetNK(int64(last)) // 4K/16K
		copy(fields, r.fields[0:r.fieldsEdge])
		if cap(r.fields) != cap(r.stockFields) {
			PutNK(r.fields)
		}
		r.fields = fields
	}
	r.fieldsEdge = ceil
	edge, ok = int(r.fieldsEdge), true
	return
}

func (r *webOut_) _beforeWrite() error {
	now := time.Now()
	if r.sendTime.IsZero() {
		r.sendTime = now
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.webKeeper().WriteTimeout()))
}
func (r *webOut_) _slowCheck(err error) error {
	if err == nil && r._tooSlow() {
		err = webOutTooSlow
	}
	if err != nil {
		r.stream.markBroken()
	}
	return err
}
func (r *webOut_) _tooSlow() bool {
	return r.sendTimeout > 0 && time.Now().Sub(r.sendTime) >= r.sendTimeout
}

var ( // web outgoing message errors
	webOutTooSlow       = errors.New("web outgoing too slow")
	webOutWriteBroken   = errors.New("write broken")
	webOutUnknownStatus = errors.New("unknown status")
	webOutAlreadySent   = errors.New("already sent")
	webOutTooLarge      = errors.New("content too large")
	webOutMixedContent  = errors.New("mixed content mode")
	webOutTrailerFailed = errors.New("add trailer failed")
)

var webTemplate = [11]byte{':', 's', 't', 'a', 't', 'u', 's', ' ', 'x', 'x', 'x'}
var webControls = [...][]byte{ // size: 512*24B=12K. for both HTTP/2 and HTTP/3
	// 1XX
	StatusContinue:           []byte(":status 100"),
	StatusSwitchingProtocols: []byte(":status 101"),
	StatusProcessing:         []byte(":status 102"),
	StatusEarlyHints:         []byte(":status 103"),
	// 2XX
	StatusOK:                         []byte(":status 200"),
	StatusCreated:                    []byte(":status 201"),
	StatusAccepted:                   []byte(":status 202"),
	StatusNonAuthoritativeInfomation: []byte(":status 203"),
	StatusNoContent:                  []byte(":status 204"),
	StatusResetContent:               []byte(":status 205"),
	StatusPartialContent:             []byte(":status 206"),
	StatusMultiStatus:                []byte(":status 207"),
	StatusAlreadyReported:            []byte(":status 208"),
	StatusIMUsed:                     []byte(":status 226"),
	// 3XX
	StatusMultipleChoices:   []byte(":status 300"),
	StatusMovedPermanently:  []byte(":status 301"),
	StatusFound:             []byte(":status 302"),
	StatusSeeOther:          []byte(":status 303"),
	StatusNotModified:       []byte(":status 304"),
	StatusUseProxy:          []byte(":status 305"),
	StatusTemporaryRedirect: []byte(":status 307"),
	StatusPermanentRedirect: []byte(":status 308"),
	// 4XX
	StatusBadRequest:                  []byte(":status 400"),
	StatusUnauthorized:                []byte(":status 401"),
	StatusPaymentRequired:             []byte(":status 402"),
	StatusForbidden:                   []byte(":status 403"),
	StatusNotFound:                    []byte(":status 404"),
	StatusMethodNotAllowed:            []byte(":status 405"),
	StatusNotAcceptable:               []byte(":status 406"),
	StatusProxyAuthenticationRequired: []byte(":status 407"),
	StatusRequestTimeout:              []byte(":status 408"),
	StatusConflict:                    []byte(":status 409"),
	StatusGone:                        []byte(":status 410"),
	StatusLengthRequired:              []byte(":status 411"),
	StatusPreconditionFailed:          []byte(":status 412"),
	StatusContentTooLarge:             []byte(":status 413"),
	StatusURITooLong:                  []byte(":status 414"),
	StatusUnsupportedMediaType:        []byte(":status 415"),
	StatusRangeNotSatisfiable:         []byte(":status 416"),
	StatusExpectationFailed:           []byte(":status 417"),
	StatusMisdirectedRequest:          []byte(":status 421"),
	StatusUnprocessableEntity:         []byte(":status 422"),
	StatusLocked:                      []byte(":status 423"),
	StatusFailedDependency:            []byte(":status 424"),
	StatusTooEarly:                    []byte(":status 425"),
	StatusUpgradeRequired:             []byte(":status 426"),
	StatusPreconditionRequired:        []byte(":status 428"),
	StatusTooManyRequests:             []byte(":status 429"),
	StatusRequestHeaderFieldsTooLarge: []byte(":status 431"),
	StatusUnavailableForLegalReasons:  []byte(":status 451"),
	// 5XX
	StatusInternalServerError:           []byte(":status 500"),
	StatusNotImplemented:                []byte(":status 501"),
	StatusBadGateway:                    []byte(":status 502"),
	StatusServiceUnavailable:            []byte(":status 503"),
	StatusGatewayTimeout:                []byte(":status 504"),
	StatusHTTPVersionNotSupported:       []byte(":status 505"),
	StatusVariantAlsoNegotiates:         []byte(":status 506"),
	StatusInsufficientStorage:           []byte(":status 507"),
	StatusLoopDetected:                  []byte(":status 508"),
	StatusNotExtended:                   []byte(":status 510"),
	StatusNetworkAuthenticationRequired: []byte(":status 511"),
}

var webErrorPages = func() map[int16][]byte {
	const template = `<!doctype html>
<html lang="en">
<head>
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<meta charset="utf-8">
<title>%d %s</title>
<style type="text/css">
body{text-align:center;}
header{font-size:72pt;}
main{font-size:36pt;}
footer{padding:20px;}
</style>
</head>
<body>
	<header>%d</header>
	<main>%s</main>
	<footer>Powered by Gorox</footer>
</body>
</html>`
	pages := make(map[int16][]byte)
	for status, control := range http1Controls {
		if status < 400 || control == nil {
			continue
		}
		phrase := control[len("HTTP/1.1 XXX ") : len(control)-2]
		pages[int16(status)] = []byte(fmt.Sprintf(template, status, phrase, status, phrase))
	}
	return pages
}()
