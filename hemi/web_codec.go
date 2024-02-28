// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General Web incoming and outgoing messages implementation. See RFC 9110 and 9111.

package hemi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
)

// webAgent collects shared methods between webServer or webBackend.
type webAgent interface {
	// Imports
	agent
	contentSaver
	// Methods
	RecvTimeout() time.Duration // timeout to recv the whole message content
	SendTimeout() time.Duration // timeout to send the whole message
	MaxContentSizeAllowed() int64
	MaxMemoryContentSize() int32
}

// _webAgent_ is a mixin for webServer_ and webBackend_.
type _webAgent_ struct {
	// States
	recvTimeout           time.Duration // timeout to recv the whole message content
	sendTimeout           time.Duration // timeout to send the whole message
	maxContentSizeAllowed int64         // max content size allowed
	maxMemoryContentSize  int32         // max content size that can be loaded into memory directly
}

func (a *_webAgent_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
	// recvTimeout
	shell.ConfigureDuration("recvTimeout", &a.recvTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, recvTimeout)

	// sendTimeout
	shell.ConfigureDuration("sendTimeout", &a.sendTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, sendTimeout)

	// maxContentSizeAllowed
	shell.ConfigureInt64("maxContentSizeAllowed", &a.maxContentSizeAllowed, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSizeAllowed has an invalid value")
	}, _1T)

	// maxMemoryContentSize
	shell.ConfigureInt32("maxMemoryContentSize", &a.maxMemoryContentSize, func(value int32) error {
		if value > 0 && value <= _1G {
			return nil
		}
		return errors.New(".maxMemoryContentSize has an invalid value")
	}, _16M) // DO NOT CHANGE THIS, otherwise integer overflow may occur
}
func (a *_webAgent_) onPrepare(shell Component) {
	// Currently nothing
}

func (a *_webAgent_) RecvTimeout() time.Duration   { return a.recvTimeout }
func (a *_webAgent_) SendTimeout() time.Duration   { return a.sendTimeout }
func (a *_webAgent_) MaxContentSizeAllowed() int64 { return a.maxContentSizeAllowed }
func (a *_webAgent_) MaxMemoryContentSize() int32  { return a.maxMemoryContentSize }

// webConn collects shared methods between *http[1-3]Conn and *H[1-3]Conn.
type webConn interface {
	ID() int64
	IsUDS() bool
	IsTLS() bool
	makeTempName(p []byte, unixTime int64) int
	setKeepConn(keepConn bool)
	isBroken() bool
	markBroken()
}

// _webConn_ is a mixin for webServerConn_ and webBackendConn_.
type _webConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	keepConn bool // keep the connection after current stream? true by default
	// Conn states (zeros)
	counter     atomic.Int64 // can be used to generate a random number
	usedStreams atomic.Int32 // num of streams served or used
	broken      atomic.Bool  // is conn broken?
}

func (c *_webConn_) onGet() {
	c.keepConn = true
}
func (c *_webConn_) onPut() {
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *_webConn_) setKeepConn(keepConn bool) { c.keepConn = keepConn }

func (c *_webConn_) isBroken() bool { return c.broken.Load() }
func (c *_webConn_) markBroken()    { c.broken.Store(true) }

// webStream collects shared methods between *http[1-3]Stream and *H[1-3]Stream.
type webStream interface {
	webAgent() webAgent
	webConn() webConn
	remoteAddr() net.Addr

	buffer256() []byte
	unsafeMake(size int) []byte
	makeTempName(p []byte, unixTime int64) int // temp name is small enough to be placed in buffer256() of stream

	setReadDeadline(deadline time.Time) error
	setWriteDeadline(deadline time.Time) error

	read(p []byte) (int, error)
	readFull(p []byte) (int, error)
	write(p []byte) (int, error)
	writev(vector *net.Buffers) (int64, error)

	isBroken() bool // if either side of the stream is broken, then it is broken
	markBroken()    // mark stream as broken
}

// _webStream_ is a mixin for webServerStream_ and webBackendStream_.
type _webStream_ struct {
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	isSocket bool // is websocket?
}

func (s *_webStream_) onUse() {
}
func (s *_webStream_) onEnd() {
	s.isSocket = false
}

// webIn_ is the parent for webServerRequest_ and webBackendResponse_.
type webIn_ struct { // incoming. needs parsing
	// Assocs
	shell interface { // *http[1-3]Request, *H[1-3]Response
		readContent() (p []byte, err error)
		examineTail() bool
	}
	stream webStream // *http[1-3]Stream, *H[1-3]Stream
	// Stream states (stocks)
	stockInput  [1536]byte // for r.input
	stockArray  [768]byte  // for r.array
	stockPrimes [40]pair   // for r.primes
	stockExtras [30]pair   // for r.extras
	// Stream states (controlled)
	mainPair       pair     // to overcome the limitation of Go's escape analysis when receiving pairs
	contentCodings [4]uint8 // content-encoding flags, controlled by r.nContentCodings. see webCodingXXX. values: none compress deflate gzip br
	acceptCodings  [4]uint8 // accept-encoding flags, controlled by r.nAcceptCodings. see webCodingXXX. values: identity(none) compress deflate gzip br
	inputNext      int32    // HTTP/1 request only. next request begins from r.input[r.inputNext]. exists because HTTP/1 supports pipelining
	inputEdge      int32    // edge position of current message head is at r.input[r.inputEdge]. placed here to make it compatible with HTTP/1 pipelining
	// Stream states (non-zeros)
	input                 []byte        // bytes of incoming message heads. [<r.stockInput>/4K/16K]
	array                 []byte        // store parsed, dynamic incoming data. [<r.stockArray>/4K/16K/64K1/(make <= 1G)]
	primes                []pair        // hold prime queries, headers(main+subs), cookies, forms, and trailers(main+subs). [<r.stockPrimes>/max]
	extras                []pair        // hold extra queries, headers(main+subs), cookies, forms, trailers(main+subs), and params. [<r.stockExtras>/max]
	recvTimeout           time.Duration // timeout to recv the whole message content
	maxContentSizeAllowed int64         // max content size allowed for current message. if content is vague, size will be calculated on receiving
	contentSize           int64         // info about incoming content. >=0: content size, -1: no content, -2: vague content
	versionCode           uint8         // Version1_0, Version1_1, Version2, Version3
	asResponse            bool          // treat this message as response?
	keepAlive             int8          // HTTP/1 only. -1: no connection header, 0: connection close, 1: connection keep-alive
	_                     byte          // padding
	headResult            int16         // result of receiving message head. values are same as http status for convenience
	bodyResult            int16         // result of receiving message body. values are same as http status for convenience
	// Stream states (zeros)
	failReason  string    // the reason of headResult or bodyResult
	bodyWindow  []byte    // a window used for receiving body. sizes must be same with r.input for HTTP/1. [HTTP/1=<none>/16K, HTTP/2/3=<none>/4K/16K/64K1]
	recvTime    time.Time // the time when we begin receiving message
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
	hasExtra         [8]bool // has extra pairs? see kindXXX for indexes
	dateTime         int64   // parsed unix time of the date header
	arrayEdge        int32   // next usable position of r.array is at r.array[r.arrayEdge]. used when writing r.array
	arrayKind        int8    // kind of current r.array. see arrayKindXXX
	receiving        int8    // what section of the message are we currently receiving. see webSectionXXX
	headers          zone    // headers ->r.primes
	hasRevisers      bool    // are there any incoming revisers hooked on this incoming message?
	upgradeSocket    bool    // upgrade: websocket?
	acceptGzip       bool    // does peer accept gzip content coding? i.e. accept-encoding: gzip, deflate
	acceptBrotli     bool    // does peer accept brotli content coding? i.e. accept-encoding: gzip, br
	nContentCodings  int8    // num of content-encoding flags, controls r.contentCodings
	nAcceptCodings   int8    // num of accept-encoding flags, controls r.acceptCodings
	iContentLength   uint8   // index of content-length header in r.primes
	iContentLocation uint8   // index of content-location header in r.primes
	iContentRange    uint8   // index of content-range header in r.primes
	iContentType     uint8   // index of content-type header in r.primes
	iDate            uint8   // index of date header in r.primes
	_                [3]byte // padding
	zConnection      zone    // zone of connection headers in r.primes. may not be continuous
	zContentLanguage zone    // zone of content-language headers in r.primes. may not be continuous
	zTrailer         zone    // zone of trailer headers in r.primes. may not be continuous
	zVia             zone    // zone of via headers in r.primes. may not be continuous
	contentReceived  bool    // is content received? if message has no content, it is true (received)
	contentTextKind  int8    // kind of current r.contentText if it is text. see webContentTextXXX
	receivedSize     int64   // bytes of currently received content. used by both sized & vague content receiver
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
	agent := r.stream.webAgent()
	r.recvTimeout = agent.RecvTimeout()
	r.maxContentSizeAllowed = agent.MaxContentSizeAllowed()
	r.contentSize = -1 // no content
	r.versionCode = versionCode
	r.asResponse = asResponse
	r.keepAlive = -1 // no connection header
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *webIn_) onEnd() { // for zeros
	if r.versionCode >= Version2 || r.asResponse { // as we don't use pipelining for outgoing requests, incoming responses are not pipelined.
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
	r.array = nil // array of other kinds is only a reference, so just reset.
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
	r.contentText = nil // contentText of other kinds is only a reference, so just reset.

	if r.contentFile != nil {
		r.contentFile.Close()
		if Debug() >= 2 {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(r.contentFile.Name()); err != nil {
			// TODO: log?
		}
		r.contentFile = nil
	}

	r.webIn0 = webIn0{}
}

func (r *webIn_) UnsafeMake(size int) []byte { return r.stream.unsafeMake(size) }
func (r *webIn_) RemoteAddr() net.Addr       { return r.stream.remoteAddr() }

func (r *webIn_) VersionCode() uint8    { return r.versionCode }
func (r *webIn_) IsHTTP1_0() bool       { return r.versionCode == Version1_0 }
func (r *webIn_) IsHTTP1_1() bool       { return r.versionCode == Version1_1 }
func (r *webIn_) IsHTTP1() bool         { return r.versionCode <= Version1_1 }
func (r *webIn_) IsHTTP2() bool         { return r.versionCode == Version2 }
func (r *webIn_) IsHTTP3() bool         { return r.versionCode == Version3 }
func (r *webIn_) Version() string       { return webVersionStrings[r.versionCode] }
func (r *webIn_) UnsafeVersion() []byte { return webVersionByteses[r.versionCode] }

func (r *webIn_) KeepAlive() int8   { return r.keepAlive }
func (r *webIn_) HeadResult() int16 { return r.headResult }
func (r *webIn_) BodyResult() int16 { return r.bodyResult }

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
			if webTchar[p[text.edge]] == 0 {
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
			for text.edge < field.value.edge && webTchar[p[text.edge]] != 0 {
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
	} else if numSubs == 0 {
		field.dataEdge = field.value.edge
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
	if r.contentSize == -1 { // r.contentSize can only be -1 or >= 0 here. -2 is set after all of the headers are received if the content is vague
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
	// TODO: check syntax
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
		if r.nAcceptCodings == int8(cap(r.acceptCodings)) {
			break // ignore too many codings
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
			coding = webCodingGzip
		} else if bytes.Equal(data, bytesBrotli) {
			r.acceptBrotli = true
			coding = webCodingBrotli
		} else if bytes.Equal(data, bytesDeflate) {
			coding = webCodingDeflate
		} else if bytes.Equal(data, bytesCompress) {
			coding = webCodingCompress
		} else if bytes.Equal(data, bytesIdentity) {
			coding = webCodingIdentity
		} else {
			coding = webCodingUnknown
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
			coding = webCodingGzip
		} else if bytes.Equal(data, bytesBrotli) {
			coding = webCodingBrotli
		} else if bytes.Equal(data, bytesDeflate) { // this is in fact zlib format
			coding = webCodingDeflate // some non-conformant implementations send the "deflate" compressed data without the zlib wrapper :(
		} else if bytes.Equal(data, bytesCompress) {
			coding = webCodingCompress
		} else {
			coding = webCodingUnknown
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
		r.contentSize = -2 // vague
	} else if r.versionCode >= Version2 && r.contentSize == -1 { // no content-length header
		// TODO: if there is no content, HTTP/2 and HTTP/3 should mark END_STREAM in headers frame. use this to decide!
		r.contentSize = -2 // if there is no content-length in HTTP/2 or HTTP/3, we treat it as vague
	}
	return true
}
func (r *webIn_) IsVague() bool { return r.contentSize == -2 }

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
func (r *webIn_) loadContent() { // into memory. [0, r.maxContentSizeAllowed]
	if r.contentReceived {
		// Content is in r.contentText already.
		return
	}
	r.contentReceived = true
	switch content := r.recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		r.contentText = content // real content is r.contentText[:r.receivedSize]
		r.contentTextKind = webContentTextPool
	case tempFile: // [0, r.maxContentSizeAllowed]. case happens when sized content > 64K1, or content is vague.
		contentFile := content.(*os.File)
		if r.receivedSize == 0 { // vague content can has 0 size
			r.contentText = r.input
			r.contentTextKind = webContentTextInput
		} else { // r.receivedSize > 0
			if r.receivedSize <= _64K1 { // must be vague content because sized content is a []byte if <= _64K1
				r.contentText = GetNK(r.receivedSize) // 4K/16K/64K1. real content is r.content[:r.receivedSize]
				r.contentTextKind = webContentTextPool
			} else { // r.receivedSize > 64K1, content can be sized or vague. just alloc
				r.contentText = make([]byte, r.receivedSize)
				r.contentTextKind = webContentTextMake
			}
			if _, err := io.ReadFull(contentFile, r.contentText[:r.receivedSize]); err != nil {
				// TODO: r.webapp.log
			}
		}
		contentFile.Close()
		if Debug() >= 2 {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(contentFile.Name()); err != nil {
			// TODO: r.webapp.log
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
	case tempFile: // [0, r.maxContentSizeAllowed]. case happens when sized content > 64K1, or content is vague.
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
	case tempFile: // [0, r.maxContentSizeAllowed]. case happens when sized content > 64K1, or content is vague.
		if content != fakeFile { // this must not happen!
			BugExitln("temp file is not fake when dropping content")
		}
	case error: // i/o error or unexpected EOF
		// TODO: log error?
		r.stream.markBroken()
	}
}
func (r *webIn_) recvContent(retain bool) any { // to []byte (for small content <= 64K1) or tempFile (for large content > 64K1, or vague content)
	if r.contentSize > 0 && r.contentSize <= _64K1 { // (0, 64K1]. save to []byte. must be received in a timeout
		if err := r.stream.setReadDeadline(time.Now().Add(r.stream.webAgent().ReadTimeout())); err != nil {
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
	} else { // (64K1, r.maxContentSizeAllowed] when sized, or [0, r.maxContentSizeAllowed] when vague. save to tempFile and return the file
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

func (r *webIn_) _addPrime(prime *pair) (edge uint8, ok bool) {
	if len(r.primes) == cap(r.primes) { // full
		if cap(r.primes) != cap(r.stockPrimes) { // too many primes
			return 0, false
		}
		if Debug() >= 2 {
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
		if Debug() >= 2 {
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
	if extraKind == kindHeader || extraKind == kindTrailer { // skip sub fields, only collects values of main fields
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
	if name == "" {
		return
	}
	if hash == 0 {
		hash = stringHash(name)
	}
	if extraKind == kindHeader || extraKind == kindTrailer { // skip comma fields, only collects data of fields without comma
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
	return
}
func (r *webIn_) getPairs(name string, hash uint16, primes zone, extraKind int8) (values []string, ok bool) {
	if name == "" {
		return
	}
	if hash == 0 {
		hash = stringHash(name)
	}
	if extraKind == kindHeader || extraKind == kindTrailer { // skip comma fields, only collects data of fields without comma
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
	return
}
func (r *webIn_) delPair(name string, hash uint16, primes zone, extraKind int8) (deleted bool) {
	if name == "" {
		return
	}
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
	return
}
func (r *webIn_) _placeOf(pair *pair) []byte {
	var place []byte
	switch pair.place {
	case placeInput:
		place = r.input
	case placeArray:
		place = r.array
	case placeStatic2:
		place = http2BytesStatic
	case placeStatic3:
		place = http3BytesStatic
	default:
		BugExitln("unknown pair.place")
	}
	return place
}

func (r *webIn_) delHopHeaders() {
	r._delHopFields(r.headers, kindHeader, r.delHeader)
}
func (r *webIn_) delHopTrailers() {
	r._delHopFields(r.trailers, kindTrailer, r.delTrailer)
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

func (r *webIn_) forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool { // by webOut.proxyCopyHead(). excluding sub headers
	return r._forMainFields(r.headers, kindHeader, callback)
}
func (r *webIn_) forTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool { // by webOut.proxyCopyTail(). excluding sub trailers
	return r._forMainFields(r.trailers, kindTrailer, callback)
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

func (r *webIn_) arrayCopy(p []byte) bool {
	if len(p) > 0 {
		edge := r.arrayEdge + int32(len(p))
		if edge < r.arrayEdge { // overflow
			return false
		}
		if edge > r.stream.webAgent().MaxMemoryContentSize() {
			return false
		}
		if !r._growArray(int32(len(p))) {
			return false
		}
		r.arrayEdge += int32(copy(r.array[r.arrayEdge:], p))
	}
	return true
}
func (r *webIn_) arrayPush(b byte) {
	r.array[r.arrayEdge] = b
	if r.arrayEdge++; r.arrayEdge == int32(cap(r.array)) {
		r._growArray(1)
	}
}
func (r *webIn_) _growArray(size int32) bool { // stock(<4K)->4K->16K->64K1->(128K->...->1G)
	edge := r.arrayEdge + size
	if edge < 0 || edge > _1G { // cannot overflow hard limit: 1G
		return false
	}
	if edge <= int32(cap(r.array)) { // existing array is enough
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

func (r *webIn_) saveContentFilesDir() string {
	return r.stream.webAgent().SaveContentFilesDir()
}
func (r *webIn_) _newTempFile(retain bool) (tempFile, error) { // to save content to
	if !retain { // since data is not used by upper caller, we don't need to actually write data to file.
		return fakeFile, nil
	}
	filesDir := r.saveContentFilesDir()
	filePath := r.UnsafeMake(len(filesDir) + 19) // 19 bytes is enough for an int64
	n := copy(filePath, filesDir)
	n += r.stream.makeTempName(filePath[n:], r.recvTime.Unix())
	return os.OpenFile(risky.WeakString(filePath[:n]), os.O_RDWR|os.O_CREATE, 0644)
}
func (r *webIn_) _beforeRead(toTime *time.Time) error {
	now := time.Now()
	if toTime.IsZero() {
		*toTime = now
	}
	return r.stream.setReadDeadline(now.Add(r.stream.webAgent().ReadTimeout()))
}
func (r *webIn_) _tooSlow() bool { // reports whether the speed of incoming content is too slow
	return r.recvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.recvTimeout
}

var ( // web incoming message errors
	webInBadChunk = errors.New("bad chunk")
	webInTooSlow  = errors.New("web incoming too slow")
)

// webOut_ is the parent for webServerResponse_ and webBackendRequest_.
type webOut_ struct { // outgoing. needs building
	// Assocs
	shell interface { // *http[1-3]Response, *H[1-3]Request
		control() []byte
		addHeader(name []byte, value []byte) bool
		header(name []byte) (value []byte, ok bool)
		hasHeader(name []byte) bool
		delHeader(name []byte) (deleted bool)
		delHeaderAt(i uint8)
		insertHeader(hash uint16, name []byte, value []byte) bool
		removeHeader(hash uint16, name []byte) (deleted bool)
		addedHeaders() []byte
		fixedHeaders() []byte
		finalizeHeaders()
		beforeSend()
		doSend() error
		sendChain() error // content
		beforeEcho()
		echoHeaders() error
		doEcho() error
		echoChain() error // chunks
		addTrailer(name []byte, value []byte) bool
		trailer(name []byte) (value []byte, ok bool)
		finalizeVague() error
		passHeaders() error
		passBytes(p []byte) error
	}
	stream webStream // *http[1-3]Stream, *H[1-3]Stream
	// Stream states (stocks)
	stockFields [1536]byte // for r.fields
	// Stream states (controlled)
	edges [128]uint16 // edges of headers or trailers in r.fields. not used at the same time. controlled by r.nHeaders or r.nTrailers. edges[0] is not used!
	piece Piece       // for r.chain. used when sending content or echoing chunks
	chain Chain       // outgoing piece chain. used when sending content or echoing chunks
	// Stream states (non-zeros)
	fields      []byte        // bytes of the headers or trailers which are not manipulated at the same time. [<r.stockFields>/4K/16K]
	sendTimeout time.Duration // timeout to send the whole message
	contentSize int64         // info of outgoing content. -1: not set, -2: vague, >=0: size
	versionCode uint8         // Version1_1, Version2, Version3
	asRequest   bool          // treat this message as request?
	nHeaders    uint8         // 1+num of added headers, starts from 1 because edges[0] is not used
	nTrailers   uint8         // 1+num of added trailers, starts from 1 because edges[0] is not used
	// Stream states (zeros)
	sendTime    time.Time   // the time when first send operation is performed
	ranges      []Range     // if content is ranged, this is set
	rangeType   string      // if content is ranged, this is the content type for each range
	vector      net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector [4][]byte   // for sending/echoing message. reset after stream
	webOut0                 // all values must be zero by default in this struct!
}
type webOut0 struct { // for fast reset, entirely
	controlEdge   uint16 // edge of control in r.fields. only used by request to mark the method and request-target
	fieldsEdge    uint16 // edge of r.fields. max size of r.fields must be <= 16K. used by both headers and trailers because they are not manipulated at the same time
	hasRevisers   bool   // are there any outgoing revisers hooked on this outgoing message?
	isSent        bool   // whether the message is sent
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
	iContentType  uint8  // position of content-type in r.edges
	iDate         uint8  // position of date in r.edges
}

func (r *webOut_) onUse(versionCode uint8, asRequest bool) { // for non-zeros
	r.fields = r.stockFields[:]
	r.sendTimeout = r.stream.webAgent().SendTimeout()
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
	// r.piece was reset in echo(), and will be reset here if send() was used. double free doesn't matter
	r.chain.free()

	r.sendTime = time.Time{}
	r.ranges = nil
	r.rangeType = ""
	r.vector = nil
	r.fixedVector = [4][]byte{}
	r.webOut0 = webOut0{}
}

func (r *webOut_) unsafeMake(size int) []byte { return r.stream.unsafeMake(size) }

func (r *webOut_) AddContentType(contentType string) bool {
	return r.AddHeaderBytes(bytesContentType, risky.ConstBytes(contentType))
}
func (r *webOut_) AddContentTypeBytes(contentType []byte) bool {
	return r.AddHeaderBytes(bytesContentType, contentType)
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
	nameBuffer := r.stream.buffer256()
	for i := 0; i < n; i++ {
		b := name[i]
		if b >= 'A' && b <= 'Z' {
			b += 0x20 // to lower
		} else if !(b >= 'a' && b <= 'z' || b == '-') {
			return 0, false, nil
		}
		hash += uint16(b)
		nameBuffer[i] = b
	}
	return hash, true, nameBuffer[:n]
}

func (r *webOut_) isVague() bool { return r.contentSize == -2 }
func (r *webOut_) IsSent() bool  { return r.isSent }

func (r *webOut_) appendContentType(contentType []byte) (ok bool) {
	return r._appendSingleton(&r.iContentType, bytesContentType, contentType)
}
func (r *webOut_) appendDate(date []byte) (ok bool) { // rarely used in backendRequest
	return r._appendSingleton(&r.iDate, bytesDate, date)
}

func (r *webOut_) deleteContentType() (deleted bool) { return r._deleteSingleton(&r.iContentType) }
func (r *webOut_) deleteDate() (deleted bool)        { return r._deleteSingleton(&r.iDate) }

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
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.shell.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = unixTime
	return true
}
func (r *webOut_) _addUnixTime(pUnixTime *int64, pIndex *uint8, name []byte, httpDate []byte) bool {
	if *pUnixTime == -2 { // was set through general api, must delete it
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
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.shell.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = -1
	return true
}

func (r *webOut_) SetSendTimeout(timeout time.Duration) { r.sendTimeout = timeout }

func (r *webOut_) Send(content string) error      { return r.sendText(risky.ConstBytes(content)) }
func (r *webOut_) SendBytes(content []byte) error { return r.sendText(content) }
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
func (r *webOut_) SendJSON(content any) error { // TODO: optimize performance
	r.AddContentTypeBytes(bytesTypeJSON)
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	return r.sendText(data)
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

func (r *webOut_) proxyPost(content any, hasTrailers bool) error { // post held content to the other side
	if contentText, ok := content.([]byte); ok {
		if hasTrailers { // if (in the future) we supports taking vague content in buffer, this happens
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
		if hasTrailers { // we must use vague
			return r.echoFile(contentFile, fileInfo, false) // false means don't close on end. this file doesn't belong to r
		} else {
			return r.sendFile(contentFile, fileInfo, false) // false means don't close on end. this file doesn't belong to r
		}
	} else { // nil means no content.
		if err := r._beforeSend(); err != nil {
			return err
		}
		r.forbidContent = true
		return r.shell.doSend()
	}
}

func (r *webOut_) employRanges(ranges []Range, rangeType string) {
	r.ranges = ranges
	r.rangeType = rangeType
}

func (r *webOut_) sendText(content []byte) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content)) // initial size, may be changed by revisers
	return r.shell.doSend()
}
func (r *webOut_) sendFile(content *os.File, info os.FileInfo, shut bool) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetFile(content, info, shut)
	r.chain.PushTail(&r.piece)
	r.contentSize = info.Size() // initial size, may be changed by revisers
	return r.shell.doSend()
}
func (r *webOut_) _beforeSend() error {
	if r.isSent {
		return webOutAlreadySent
	}
	r.isSent = true
	if r.hasRevisers {
		r.shell.beforeSend()
	}
	return nil
}

func (r *webOut_) echoText(chunk []byte) error {
	if err := r._beforeEcho(); err != nil {
		return err
	}
	if len(chunk) == 0 { // empty chunk is not actually sent, since it is used to indicate the end. pretend to succeed
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
	if info.Size() == 0 { // empty chunk is not actually sent, since it is used to indicate the end. pretend to succeed
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
	if r.isSent {
		return nil
	}
	if r.contentSize != -1 {
		return webOutMixedContent
	}
	r.isSent = true
	r.contentSize = -2
	if r.hasRevisers {
		r.shell.beforeEcho()
	}
	return r.shell.echoHeaders()
}

func (r *webOut_) growHeader(size int) (from int, edge int, ok bool) { // headers and trailers are not manipulated at the same time
	if r.nHeaders == uint8(cap(r.edges)) { // too many headers
		return
	}
	return r._growFields(size)
}
func (r *webOut_) growTrailer(size int) (from int, edge int, ok bool) { // headers and trailers are not manipulated at the same time
	if r.nTrailers == uint8(cap(r.edges)) { // too many trailers
		return
	}
	return r._growFields(size)
}
func (r *webOut_) _growFields(size int) (from int, edge int, ok bool) { // used by growHeader first and growTrailer later as they are not manipulated at the same time
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
	if last > uint16(cap(r.fields)) { // cap < last <= _16K
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
	if r.sendTime.IsZero() { // only once
		r.sendTime = now
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.webAgent().WriteTimeout()))
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
func (r *webOut_) _tooSlow() bool { // reports whether the speed of outgoing content is too slow
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

// webSocket_
type webSocket_ struct {
	// TODO
}

func (s *webSocket_) onUse() {
}
func (s *webSocket_) onEnd() {
}

func (s *webSocket_) example() {
}

const ( // version codes
	Version1_0 = 0 // must be 0
	Version1_1 = 1
	Version2   = 2
	Version3   = 3
)

var ( // version strings and byteses
	stringHTTP1_0     = "HTTP/1.0"
	stringHTTP1_1     = "HTTP/1.1"
	stringHTTP2       = "HTTP/2"
	stringHTTP3       = "HTTP/3"
	bytesHTTP1_0      = []byte(stringHTTP1_0)
	bytesHTTP1_1      = []byte(stringHTTP1_1)
	bytesHTTP2        = []byte(stringHTTP2)
	bytesHTTP3        = []byte(stringHTTP3)
	webVersionStrings = [...]string{
		Version1_0: stringHTTP1_0,
		Version1_1: stringHTTP1_1,
		Version2:   stringHTTP2,
		Version3:   stringHTTP3,
	}
	webVersionByteses = [...][]byte{
		Version1_0: bytesHTTP1_0,
		Version1_1: bytesHTTP1_1,
		Version2:   bytesHTTP2,
		Version3:   bytesHTTP3,
	}
)

const ( // scheme codes
	SchemeHTTP  = 0 // must be 0
	SchemeHTTPS = 1
)

var ( // scheme strings and byteses
	stringHTTP       = "http"
	stringHTTPS      = "https"
	bytesHTTP        = []byte(stringHTTP)
	bytesHTTPS       = []byte(stringHTTPS)
	webSchemeStrings = [...]string{
		SchemeHTTP:  stringHTTP,
		SchemeHTTPS: stringHTTPS,
	}
	webSchemeByteses = [...][]byte{
		SchemeHTTP:  bytesHTTP,
		SchemeHTTPS: bytesHTTPS,
	}
)

const ( // method codes
	MethodGET     = 0x00000001
	MethodHEAD    = 0x00000002
	MethodPOST    = 0x00000004
	MethodPUT     = 0x00000008
	MethodDELETE  = 0x00000010
	MethodCONNECT = 0x00000020
	MethodOPTIONS = 0x00000040
	MethodTRACE   = 0x00000080
)

var ( // method hash table
	webMethodBytes = []byte("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE")
	webMethodTable = [8]struct {
		hash uint16
		from uint8
		edge uint8
		code uint32
	}{
		0: {326, 9, 13, MethodPOST},
		1: {274, 4, 8, MethodHEAD},
		2: {249, 14, 17, MethodPUT},
		3: {224, 0, 3, MethodGET},
		4: {556, 33, 40, MethodOPTIONS},
		5: {522, 25, 32, MethodCONNECT},
		6: {435, 18, 24, MethodDELETE},
		7: {367, 41, 46, MethodTRACE},
	}
	webMethodFind = func(hash uint16) int { return (2610 / int(hash)) % 8 }
)

const ( // status codes
	// 1XX
	StatusContinue           = 100
	StatusSwitchingProtocols = 101
	StatusProcessing         = 102
	StatusEarlyHints         = 103
	// 2XX
	StatusOK                         = 200
	StatusCreated                    = 201
	StatusAccepted                   = 202
	StatusNonAuthoritativeInfomation = 203
	StatusNoContent                  = 204
	StatusResetContent               = 205
	StatusPartialContent             = 206
	StatusMultiStatus                = 207
	StatusAlreadyReported            = 208
	StatusIMUsed                     = 226
	// 3XX
	StatusMultipleChoices   = 300
	StatusMovedPermanently  = 301
	StatusFound             = 302
	StatusSeeOther          = 303
	StatusNotModified       = 304
	StatusUseProxy          = 305
	StatusTemporaryRedirect = 307
	StatusPermanentRedirect = 308
	// 4XX
	StatusBadRequest                  = 400
	StatusUnauthorized                = 401
	StatusPaymentRequired             = 402
	StatusForbidden                   = 403
	StatusNotFound                    = 404
	StatusMethodNotAllowed            = 405
	StatusNotAcceptable               = 406
	StatusProxyAuthenticationRequired = 407
	StatusRequestTimeout              = 408
	StatusConflict                    = 409
	StatusGone                        = 410
	StatusLengthRequired              = 411
	StatusPreconditionFailed          = 412
	StatusContentTooLarge             = 413
	StatusURITooLong                  = 414
	StatusUnsupportedMediaType        = 415
	StatusRangeNotSatisfiable         = 416
	StatusExpectationFailed           = 417
	StatusMisdirectedRequest          = 421
	StatusUnprocessableEntity         = 422
	StatusLocked                      = 423
	StatusFailedDependency            = 424
	StatusTooEarly                    = 425
	StatusUpgradeRequired             = 426
	StatusPreconditionRequired        = 428
	StatusTooManyRequests             = 429
	StatusRequestHeaderFieldsTooLarge = 431
	StatusUnavailableForLegalReasons  = 451
	// 5XX
	StatusInternalServerError           = 500
	StatusNotImplemented                = 501
	StatusBadGateway                    = 502
	StatusServiceUnavailable            = 503
	StatusGatewayTimeout                = 504
	StatusHTTPVersionNotSupported       = 505
	StatusVariantAlsoNegotiates         = 506
	StatusInsufficientStorage           = 507
	StatusLoopDetected                  = 508
	StatusNotExtended                   = 510
	StatusNetworkAuthenticationRequired = 511
)

const ( // misc web types
	webTargetOrigin    = 0 // must be 0
	webTargetAbsolute  = 1 // scheme "://" hostname [ ":" port ] path-abempty [ "?" query ]
	webTargetAuthority = 2 // hostname:port, /path/to/unix.sock
	webTargetAsterisk  = 3 // *

	webSectionControl  = 0 // must be 0
	webSectionHeaders  = 1
	webSectionContent  = 2
	webSectionTrailers = 3

	webCodingIdentity = 0 // must be 0
	webCodingCompress = 1
	webCodingDeflate  = 2 // this is in fact zlib format
	webCodingGzip     = 3
	webCodingBrotli   = 4
	webCodingUnknown  = 5

	webFormNotForm    = 0 // must be 0
	webFormURLEncoded = 1 // application/x-www-form-urlencoded
	webFormMultipart  = 2 // multipart/form-data

	webContentTextNone  = 0 // must be 0
	webContentTextInput = 1 // refers to r.input
	webContentTextPool  = 2 // fetched from pool
	webContentTextMake  = 3 // direct make
)

const ( // hashes of web fields. value is calculated by adding all ASCII values.
	// Pseudo headers
	hashAuthority = 1059 // :authority
	hashMethod    = 699  // :method
	hashPath      = 487  // :path
	hashProtocol  = 940  // :protocol
	hashScheme    = 687  // :scheme
	hashStatus    = 734  // :status
	// General fields
	hashAcceptEncoding     = 1508
	hashCacheControl       = 1314 // same with hashLastModified
	hashConnection         = 1072
	hashContentDisposition = 2013
	hashContentEncoding    = 1647
	hashContentLanguage    = 1644
	hashContentLength      = 1450
	hashContentLocation    = 1665
	hashContentRange       = 1333
	hashContentType        = 1258
	hashDate               = 414
	hashKeepAlive          = 995
	hashTrailer            = 755
	hashTransferEncoding   = 1753
	hashUpgrade            = 744
	hashVia                = 320
	// Request fields
	hashAccept             = 624
	hashAcceptCharset      = 1415
	hashAcceptLanguage     = 1505
	hashAuthorization      = 1425
	hashCookie             = 634
	hashExpect             = 649
	hashForwarded          = 958
	hashHost               = 446
	hashIfMatch            = 777 // same with hashIfRange
	hashIfModifiedSince    = 1660
	hashIfNoneMatch        = 1254
	hashIfRange            = 777 // same with hashIfMatch
	hashIfUnmodifiedSince  = 1887
	hashProxyAuthorization = 2048
	hashProxyConnection    = 1695
	hashRange              = 525
	hashTE                 = 217
	hashUserAgent          = 1019
	hashXForwardedFor      = 1495
	// Response fields
	hashAcceptRanges      = 1309
	hashAge               = 301
	hashAllow             = 543
	hashAltSvc            = 698
	hashCacheStatus       = 1221
	hashCDNCacheControl   = 1668
	hashETag              = 417
	hashExpires           = 768
	hashLastModified      = 1314 // same with hashCacheControl
	hashLocation          = 857
	hashProxyAuthenticate = 1902
	hashRetryAfter        = 1141
	hashServer            = 663
	hashSetCookie         = 1011
	hashVary              = 450
	hashWWWAuthenticate   = 1681
)

var ( // byteses of web fields.
	// Pseudo headers
	bytesAuthority = []byte(":authority")
	bytesMethod    = []byte(":method")
	bytesPath      = []byte(":path")
	bytesProtocol  = []byte(":protocol")
	bytesScheme    = []byte(":scheme")
	bytesStatus    = []byte(":status")
	// General fields
	bytesAcceptEncoding     = []byte("accept-encoding")
	bytesCacheControl       = []byte("cache-control")
	bytesConnection         = []byte("connection")
	bytesContentDisposition = []byte("content-disposition")
	bytesContentEncoding    = []byte("content-encoding")
	bytesContentLanguage    = []byte("content-language")
	bytesContentLength      = []byte("content-length")
	bytesContentLocation    = []byte("content-location")
	bytesContentRange       = []byte("content-range")
	bytesContentType        = []byte("content-type")
	bytesDate               = []byte("date")
	bytesKeepAlive          = []byte("keep-alive")
	bytesTrailer            = []byte("trailer")
	bytesTransferEncoding   = []byte("transfer-encoding")
	bytesUpgrade            = []byte("upgrade")
	bytesVia                = []byte("via")
	// Request fields
	bytesAccept             = []byte("accept")
	bytesAcceptCharset      = []byte("accept-charset")
	bytesAcceptLanguage     = []byte("accept-language")
	bytesAuthorization      = []byte("authorization")
	bytesCookie             = []byte("cookie")
	bytesExpect             = []byte("expect")
	bytesForwarded          = []byte("forwarded")
	bytesHost               = []byte("host")
	bytesIfMatch            = []byte("if-match")
	bytesIfModifiedSince    = []byte("if-modified-since")
	bytesIfNoneMatch        = []byte("if-none-match")
	bytesIfRange            = []byte("if-range")
	bytesIfUnmodifiedSince  = []byte("if-unmodified-since")
	bytesProxyAuthorization = []byte("proxy-authorization")
	bytesProxyConnection    = []byte("proxy-connection")
	bytesRange              = []byte("range")
	bytesTE                 = []byte("te")
	bytesUserAgent          = []byte("user-agent")
	bytesXForwardedFor      = []byte("x-forwarded-for")
	// Response fields
	bytesAcceptRanges      = []byte("accept-ranges")
	bytesAge               = []byte("age")
	bytesAllow             = []byte("allow")
	bytesAltSvc            = []byte("alt-svc")
	bytesCacheStatus       = []byte("cache-status")
	bytesCDNCacheControl   = []byte("cdn-cache-control")
	bytesETag              = []byte("etag")
	bytesExpires           = []byte("expires")
	bytesLastModified      = []byte("last-modified")
	bytesLocation          = []byte("location")
	bytesProxyAuthenticate = []byte("proxy-authenticate")
	bytesRetryAfter        = []byte("retry-after")
	bytesServer            = []byte("server")
	bytesSetCookie         = []byte("set-cookie")
	bytesVary              = []byte("vary")
	bytesWWWAuthenticate   = []byte("www-authenticate")
)

const ( // hashes of misc web strings & byteses.
	hashBoundary = 868
	hashFilename = 833
	hashName     = 417
)

var ( // misc web strings & byteses.
	// Strings
	stringColonPort80  = ":80"
	stringColonPort443 = ":443"
	stringSlash        = "/"
	stringAsterisk     = "*"
	// Byteses
	bytesColonPort80    = []byte(stringColonPort80)
	bytesColonPort443   = []byte(stringColonPort443)
	bytesSlash          = []byte(stringSlash)
	bytesAsterisk       = []byte(stringAsterisk)
	bytesGET            = []byte("GET")
	bytes100Continue    = []byte("100-continue")
	bytesBoundary       = []byte("boundary")
	bytesBytes          = []byte("bytes")
	bytesBytesEqual     = []byte("bytes=")
	bytesBytesStarSlash = []byte("bytes */")
	bytesChunked        = []byte("chunked")
	bytesClose          = []byte("close")
	bytesColonSpace     = []byte(": ")
	bytesCompress       = []byte("compress")
	bytesCRLF           = []byte("\r\n")
	bytesDeflate        = []byte("deflate")
	bytesFilename       = []byte("filename")
	bytesFormData       = []byte("form-data")
	bytesGzip           = []byte("gzip")
	bytesBrotli         = []byte("br")
	bytesIdentity       = []byte("identity")
	bytesTypeHTMLUTF8   = []byte("text/html; charset=utf-8")
	bytesTypeJSON       = []byte("application/json")
	bytesURLEncodedForm = []byte("application/x-www-form-urlencoded")
	bytesMultipartForm  = []byte("multipart/form-data")
	bytesName           = []byte("name")
	bytesNone           = []byte("none")
	bytesTrailers       = []byte("trailers")
	bytesWebSocket      = []byte("websocket")
	bytesGorox          = []byte("gorox")
	// HTTP/2 and HTTP/3 byteses, TODO
	bytesSchemeHTTP           = []byte(":scheme http")
	bytesSchemeHTTPS          = []byte(":scheme https")
	bytesFixedRequestHeaders  = []byte("client gorox")
	bytesFixedResponseHeaders = []byte("server gorox")
)

var webTchar = [256]int8{ // tchar = ALPHA / DIGIT / "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~"
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 1, 1, 1, 1, 1, 0, 0, 1, 1, 0, 1, 1, 0, //   !   # $ % & '     * +   - .
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, // 0 1 2 3 4 5 6 7 8 9
	0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, //   A B C D E F G H I J K L M N O
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 0, 0, 0, 1, 3, // P Q R S T U V W X Y Z       ^ _
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // ` a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 1, 0, // p q r s t u v w x y z   |   ~
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var webPchar = [256]int8{ // pchar = ALPHA / DIGIT / "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / "-" / "." / ":" / ";" / "=" / "@" / "_" / "~" / pct-encoded. '/' is pchar to improve performance.
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   !     $   & ' ( ) * + , - . /
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 2, // 0 1 2 3 4 5 6 7 8 9 : ;   =   ? // '?' is set to 2 to improve performance
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // @ A B C D E F G H I J K L M N O
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 1, // P Q R S T U V W X Y Z         _
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 0, // p q r s t u v w x y z       ~
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var webKchar = [256]int8{ // cookie-octet = 0x21 / 0x23-0x2B / 0x2D-0x3A / 0x3C-0x5B / 0x5D-0x7E
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, //   !   # $ % & ' ( ) * +   - . /
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, 1, // 0 1 2 3 4 5 6 7 8 9 :   < = > ?
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // @ A B C D E F G H I J K L M N O
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, // P Q R S T U V W X Y Z [   ] ^ _
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // ` a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, // p q r s t u v w x y z { | } ~
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var webNchar = [256]int8{ // for hostname
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, //                           - .
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, // 0 1 2 3 4 5 6 7 8 9
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, //
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, //
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, // p q r s t u v w x y z
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

// zone
type zone struct { // 2 bytes
	from, edge uint8 // edge is ensured to be <= 255
}

func (z *zone) zero() { *z = zone{} }

func (z *zone) size() int      { return int(z.edge - z.from) }
func (z *zone) isEmpty() bool  { return z.from == z.edge }
func (z *zone) notEmpty() bool { return z.from != z.edge }

// span
type span struct { // 8 bytes
	from, edge int32 // p[from:edge] is the bytes. edge is ensured to be <= 2147483647
}

func (s *span) zero() { *s = span{} }

func (s *span) size() int      { return int(s.edge - s.from) }
func (s *span) isEmpty() bool  { return s.from == s.edge }
func (s *span) notEmpty() bool { return s.from != s.edge }

func (s *span) set(from int32, edge int32) {
	s.from, s.edge = from, edge
}
func (s *span) sub(delta int32) {
	if s.from >= delta {
		s.from -= delta
		s.edge -= delta
	}
}

// defaultDesc
var defaultDesc = &desc{ // sec-ch-ua: "Microsoft Edge";v="111", "Not(A:Brand";v="8", "Chromium";v="111"
	allowQuote: true,
	allowEmpty: false,
	allowParam: true,
	hasComment: false,
}

// desc describes a web field.
type desc struct {
	hash       uint16 // name hash
	allowQuote bool   // allow data quote or not
	allowEmpty bool   // allow empty data or not
	allowParam bool   // allow parameters or not
	hasComment bool   // has comment or not
	name       []byte // field name
}

// poolPairs
var poolPairs sync.Pool

const maxPairs = 250 // 24B*250=6000B

func getPairs() []pair {
	if x := poolPairs.Get(); x == nil {
		return make([]pair, 0, maxPairs)
	} else {
		return x.([]pair)
	}
}
func putPairs(pairs []pair) {
	if cap(pairs) != maxPairs {
		BugExitln("bad pairs")
	}
	pairs = pairs[0:0:maxPairs] // reset
	poolPairs.Put(pairs)
}

// pair is used to hold queries, headers, cookies, forms, trailers, and params.
type pair struct { // 24 bytes
	hash     uint16 // name hash, to support fast search. 0 means empty
	kind     int8   // see pair kinds
	nameSize uint8  // name ends at nameFrom+nameSize
	nameFrom int32  // name begins from
	value    span   // the value
	place    int8   // see pair places
	flags    byte   // fields only. see field flags
	params   zone   // fields only. refers to a zone of pairs
	dataEdge int32  // fields only. data ends at
}

// If "accept-type" field is defined as: `allowQuote=true allowEmpty=false allowParam=true`, then a non-comma "accept-type" field may looks like this:
//
//                                 [         params         )
//                     [               value                )
//        [   name   )  [  data   )  [    param1    )[param2)
//       +--------------------------------------------------+
//       |accept-type: "text/plain"; charset="utf-8";lang=en|
//       +--------------------------------------------------+
//        ^          ^ ^^         ^                         ^
//        |          | ||         |                         |
// nameFrom          | ||         dataEdge                  |
//   nameFrom+nameSize ||                                   |
//            value.from|                          value.edge
//                      |
//                      dataFrom=value.from+(flags&flagQuoted)
//
// For dataFrom, if data is quoted, then flagQuoted is set, so flags&flagQuoted is 1, which skips '"' exactly.
//
// A has-comma "accept-types" field may looks like this (needs further parsing into sub fields):
//
// +-----------------------------------------------------------------------------------------------------------------+
// |accept-types: "text/plain"; ;charset="utf-8";langs="en,zh" ,,; ;charset="" ,,application/octet-stream ;,image/png|
// +-----------------------------------------------------------------------------------------------------------------+

const ( // pair kinds
	kindUnknown = iota
	kindQuery
	kindHeader // field
	kindCookie
	kindForm
	kindTrailer // field
	kindParam   // parameter of fields
)

const ( // pair places
	placeInput   = iota
	placeArray   // parsed
	placeStatic2 // http/2 static table
	placeStatic3 // http/3 static table
)

const ( // field flags
	flagParsed     = 0b10000000 // data and params are parsed or not
	flagSingleton  = 0b01000000 // singleton or not. mainly used by proxies
	flagSubField   = 0b00100000 // sub field or not. mainly used by webapps
	flagLiteral    = 0b00010000 // keep literal or not. used in HTTP/2 and HTTP/3
	flagPseudo     = 0b00001000 // pseudo header or not. used in HTTP/2 and HTTP/3
	flagUnderscore = 0b00000100 // name contains '_' or not. some proxies need this information
	flagCommaValue = 0b00000010 // value has comma or not
	flagQuoted     = 0b00000001 // data is quoted or not. for non comma-value field only. MUST be 0b00000001
)

func (p *pair) zero() { *p = pair{} }

func (p *pair) nameAt(t []byte) []byte { return t[p.nameFrom : p.nameFrom+int32(p.nameSize)] }
func (p *pair) nameEqualString(t []byte, x string) bool {
	return int(p.nameSize) == len(x) && string(t[p.nameFrom:p.nameFrom+int32(p.nameSize)]) == x
}
func (p *pair) nameEqualBytes(t []byte, x []byte) bool {
	return int(p.nameSize) == len(x) && bytes.Equal(t[p.nameFrom:p.nameFrom+int32(p.nameSize)], x)
}
func (p *pair) valueAt(t []byte) []byte { return t[p.value.from:p.value.edge] }

func (p *pair) setParsed()     { p.flags |= flagParsed }
func (p *pair) setSingleton()  { p.flags |= flagSingleton }
func (p *pair) setSubField()   { p.flags |= flagSubField }
func (p *pair) setLiteral()    { p.flags |= flagLiteral }
func (p *pair) setPseudo()     { p.flags |= flagPseudo }
func (p *pair) setUnderscore() { p.flags |= flagUnderscore }
func (p *pair) setCommaValue() { p.flags |= flagCommaValue }
func (p *pair) setQuoted()     { p.flags |= flagQuoted }

func (p *pair) isParsed() bool     { return p.flags&flagParsed > 0 }
func (p *pair) isSingleton() bool  { return p.flags&flagSingleton > 0 }
func (p *pair) isSubField() bool   { return p.flags&flagSubField > 0 }
func (p *pair) isLiteral() bool    { return p.flags&flagLiteral > 0 }
func (p *pair) isPseudo() bool     { return p.flags&flagPseudo > 0 }
func (p *pair) isUnderscore() bool { return p.flags&flagUnderscore > 0 }
func (p *pair) isCommaValue() bool { return p.flags&flagCommaValue > 0 }
func (p *pair) isQuoted() bool     { return p.flags&flagQuoted > 0 }

func (p *pair) dataAt(t []byte) []byte { return t[p.value.from+int32(p.flags&flagQuoted) : p.dataEdge] }
func (p *pair) dataEmpty() bool        { return p.value.from+int32(p.flags&flagQuoted) == p.dataEdge }

func (p *pair) show(place []byte) { // TODO: optimize, or simply remove
	var kind string
	switch p.kind {
	case kindQuery:
		kind = "query"
	case kindHeader:
		kind = "header"
	case kindCookie:
		kind = "cookie"
	case kindForm:
		kind = "form"
	case kindTrailer:
		kind = "trailer"
	case kindParam:
		kind = "param"
	default:
		kind = "unknown"
	}
	var plase string
	switch p.place {
	case placeInput:
		plase = "input"
	case placeArray:
		plase = "array"
	case placeStatic2:
		plase = "static2"
	case placeStatic3:
		plase = "static3"
	default:
		plase = "unknown"
	}
	var flags []string
	if p.isParsed() {
		flags = append(flags, "parsed")
	}
	if p.isSingleton() {
		flags = append(flags, "singleton")
	}
	if p.isSubField() {
		flags = append(flags, "subField")
	}
	if p.isCommaValue() {
		flags = append(flags, "commaValue")
	}
	if p.isQuoted() {
		flags = append(flags, "quoted")
	}
	if len(flags) == 0 {
		flags = append(flags, "nothing")
	}
	Printf("{hash=%4d kind=%7s place=[%7s] flags=[%s] dataEdge=%d params=%v value=%v %s=%s}\n", p.hash, kind, plase, strings.Join(flags, ","), p.dataEdge, p.params, p.value, p.nameAt(place), p.valueAt(place))
}

// para is a name-value parameter in multipart/form-data.
type para struct { // 16 bytes
	name, value span
}

// Range defines a range.
type Range struct { // 16 bytes
	from, last int64 // [from:last], inclusive
}

// poolPiece
var poolPiece sync.Pool

func GetPiece() *Piece {
	if x := poolPiece.Get(); x == nil {
		piece := new(Piece)
		piece.pool = true // other pieces are not pooled.
		return piece
	} else {
		return x.(*Piece)
	}
}
func putPiece(piece *Piece) { poolPiece.Put(piece) }

// Piece is a member of content chain.
type Piece struct { // 64 bytes
	next *Piece   // next piece
	pool bool     // true if this piece is got from poolPiece. don't change this after set!
	shut bool     // close file on free()?
	kind int8     // 0:text 1:*os.File
	_    [5]byte  // padding
	text []byte   // text
	file *os.File // file
	size int64    // size of text or file
	time int64    // file mod time
}

func (p *Piece) zero() {
	p.closeFile()
	p.next = nil
	p.shut = false
	p.kind = 0
	p.text = nil
	p.file = nil
	p.size = 0
	p.time = 0
}
func (p *Piece) closeFile() {
	if p.IsText() {
		return
	}
	if p.shut {
		p.file.Close()
	}
	if Debug() >= 2 {
		if p.shut {
			Println("file closed in Piece.closeFile()")
		} else {
			Println("file *NOT* closed in Piece.closeFile()")
		}
	}
}

func (p *Piece) copyTo(buffer []byte) error { // buffer is large enough, and p is a file.
	if p.IsText() {
		BugExitln("copyTo when piece is text")
	}
	sizeRead := int64(0)
	for {
		if sizeRead == p.size {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := p.size - sizeRead; sizeLeft < readSize {
			readSize = sizeLeft
		}
		n, err := p.file.ReadAt(buffer[:readSize], sizeRead)
		sizeRead += int64(n)
		if err != nil && sizeRead != p.size {
			return err
		}
	}
}

func (p *Piece) Next() *Piece { return p.next }

func (p *Piece) IsText() bool { return p.kind == 0 }
func (p *Piece) IsFile() bool { return p.kind == 1 }

func (p *Piece) SetText(text []byte) {
	p.closeFile()
	p.shut = false
	p.kind = 0
	p.text = text
	p.file = nil
	p.size = int64(len(text))
	p.time = 0
}
func (p *Piece) SetFile(file *os.File, info os.FileInfo, shut bool) {
	p.closeFile()
	p.shut = shut
	p.kind = 1
	p.text = nil
	p.file = file
	p.size = info.Size()
	p.time = info.ModTime().Unix()
}

func (p *Piece) Text() []byte {
	if !p.IsText() {
		BugExitln("piece is not text")
	}
	if p.size == 0 {
		return nil
	}
	return p.text
}
func (p *Piece) File() *os.File {
	if !p.IsFile() {
		BugExitln("piece is not file")
	}
	return p.file
}

// Chain is a linked-list of pieces.
type Chain struct { // 24 bytes
	head *Piece
	tail *Piece
	qnty int
}

func (c *Chain) free() {
	if Debug() >= 2 {
		Printf("chain.free() called, qnty=%d\n", c.qnty)
	}
	if c.qnty == 0 {
		return
	}
	piece := c.head
	c.head, c.tail = nil, nil
	qnty := 0
	for piece != nil {
		next := piece.next
		piece.zero()
		if piece.pool { // only put those got from poolPiece because they are not fixed
			putPiece(piece)
		}
		qnty++
		piece = next
	}
	if qnty != c.qnty {
		BugExitf("bad chain: qnty=%d c.qnty=%d\n", qnty, c.qnty)
	}
	c.qnty = 0
}

func (c *Chain) Qnty() int { return c.qnty }
func (c *Chain) Size() (int64, bool) {
	size := int64(0)
	for piece := c.head; piece != nil; piece = piece.next {
		size += piece.size
		if size < 0 {
			return 0, false
		}
	}
	return size, true
}

func (c *Chain) PushHead(piece *Piece) {
	if piece == nil {
		return
	}
	if c.qnty == 0 {
		c.head, c.tail = piece, piece
	} else {
		piece.next = c.head
		c.head = piece
	}
	c.qnty++
}
func (c *Chain) PushTail(piece *Piece) {
	if piece == nil {
		return
	}
	if c.qnty == 0 {
		c.head, c.tail = piece, piece
	} else {
		c.tail.next = piece
		c.tail = piece
	}
	c.qnty++
}
