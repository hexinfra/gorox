// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP server and backend implementation. See RFC 9110.

package hemi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//////////////////////////////////////// HTTP general implementation ////////////////////////////////////////

// httpHolder collects shared methods between *http[x3]Gate and *http[1-3]Node.
type httpHolder interface {
	// Imports
	contentSaver
	// Methods
	MaxCumulativeStreamsPerConn() int32
	MaxMemoryContentSize() int32 // allowed to load into memory
}

// _httpHolder_ is a mixin for httpServer_, httpGate_, and httpNode_.
type _httpHolder_ struct {
	// Mixins
	_contentSaver_ // so responses can save their large contents in local file system.
	// States
	maxCumulativeStreamsPerConn int32 // max cumulative streams of one conn. 0 means infinite
	maxMemoryContentSize        int32 // max content size that can be loaded into memory directly
}

func (h *_httpHolder_) onConfigure(component Component, defaultDir string, defaultRecv time.Duration, defaultSend time.Duration) {
	h._contentSaver_.onConfigure(component, defaultDir, defaultRecv, defaultSend)

	// maxCumulativeStreamsPerConn
	component.ConfigureInt32("maxCumulativeStreamsPerConn", &h.maxCumulativeStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxCumulativeStreamsPerConn has an invalid value")
	}, 1000)

	// maxMemoryContentSize
	component.ConfigureInt32("maxMemoryContentSize", &h.maxMemoryContentSize, func(value int32) error {
		if value > 0 && value <= _1G { // DO NOT CHANGE THIS, otherwise integer overflow may occur
			return nil
		}
		return errors.New(".maxMemoryContentSize has an invalid value")
	}, _16M)
}
func (h *_httpHolder_) onPrepare(component Component, perm os.FileMode) {
	h._contentSaver_.onPrepare(component, perm)
}

func (h *_httpHolder_) MaxCumulativeStreamsPerConn() int32 { return h.maxCumulativeStreamsPerConn }
func (h *_httpHolder_) MaxMemoryContentSize() int32        { return h.maxMemoryContentSize }

// httpConn collects shared methods between *server[1-3]Conn and *backend[1-3]Conn.
type httpConn interface {
	ID() int64
	UDSMode() bool
	TLSMode() bool
	MakeTempName(dst []byte, unixTime int64) int
	remoteAddr() net.Addr
	markBroken()
	isBroken() bool
}

// httpConn_ is the parent for *http[1-3]Conn_.
type httpConn_ struct {
	// Conn states (non-zeros)
	id           int64         // the conn id
	stageID      int32         // current stage id, for convenience
	udsMode      bool          // for convenience
	tlsMode      bool          // for convenience
	readTimeout  time.Duration // for convenience
	writeTimeout time.Duration // for convenience
	// Conn states (zeros)
	cumulativeStreams atomic.Int32 // cumulative num of streams served or fired by this conn
	broken            atomic.Bool  // is conn broken?
	counter           atomic.Int64 // can be used to generate a random number
	lastWrite         time.Time    // deadline of last write operation
	lastRead          time.Time    // deadline of last read operation
}

func (c *httpConn_) onGet(id int64, stageID int32, udsMode bool, tlsMode bool, readTimeout time.Duration, writeTimeout time.Duration) {
	c.id = id
	c.stageID = stageID
	c.udsMode = udsMode
	c.tlsMode = tlsMode
	c.readTimeout = readTimeout
	c.writeTimeout = writeTimeout
}
func (c *httpConn_) onPut() {
	c.cumulativeStreams.Store(0)
	c.broken.Store(false)
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *httpConn_) ID() int64 { return c.id }

func (c *httpConn_) UDSMode() bool { return c.udsMode }
func (c *httpConn_) TLSMode() bool { return c.tlsMode }

func (c *httpConn_) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.stageID, c.id, unixTime, c.counter.Add(1))
}

func (c *httpConn_) markBroken()    { c.broken.Store(true) }
func (c *httpConn_) isBroken() bool { return c.broken.Load() }

// httpStream collects shared methods between *server[1-3]Stream and *backend[1-3]Stream.
type httpStream interface {
	Holder() httpHolder
	Conn() httpConn

	remoteAddr() net.Addr
	buffer256() []byte
	unsafeMake(size int) []byte
	isBroken() bool // returns true if either side of the stream is broken
	markBroken()    // mark stream as broken
	setReadDeadline() error
	setWriteDeadline() error
	read(dst []byte) (int, error)
	readFull(dst []byte) (int, error)
	write(src []byte) (int, error)
	writev(srcVec *net.Buffers) (int64, error)
}

// httpStream_ is the parent for *http[1-3]Stream_.
type httpStream_ struct {
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	region Region // a region-based memory pool
	// Stream states (zeros)
}

func (s *httpStream_) onUse() {
	s.region.Init()
}
func (s *httpStream_) onEnd() {
	s.region.Free()
}

func (s *httpStream_) buffer256() []byte          { return s.stockBuffer[:] }
func (s *httpStream_) unsafeMake(size int) []byte { return s.region.Make(size) }

//////////////////////////////////////// HTTP incoming implementation ////////////////////////////////////////

// httpIn collects shared methods between *server[1-3]Request and *backend[1-3]Response.
type httpIn interface {
	ContentSize() int64
	IsVague() bool
	HasTrailers() bool

	readContent() (data []byte, err error)
	examineTail() bool
	proxyWalkTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool
}

// httpIn_ is the parent for serverRequest_ and backendResponse_.
type httpIn_ struct { // incoming. needs parsing
	// Assocs
	stream    httpStream // *server[1-3]Stream, *backend[1-3]Stream
	inMessage httpIn     // *server[1-3]Request, *backend[1-3]Response
	// Stream states (stocks)
	stockPrimes [40]pair   // for r.primes
	stockExtras [30]pair   // for r.extras
	stockArray  [768]byte  // for r.array
	stockInput  [1536]byte // for r.input
	// Stream states (controlled)
	inputNext      int32    // HTTP/1.x request only. next request begins from r.input[r.inputNext]. exists because HTTP/1.1 supports pipelining
	inputEdge      int32    // edge position of current message head is at r.input[r.inputEdge]. placed here to make it compatible with HTTP/1.1 pipelining
	mainPair       pair     // to overcome the limitation of Go's escape analysis when receiving pairs
	contentCodings [4]uint8 // content-encoding flags, controlled by r.numContentCodings. see httpCodingXXX for values
	acceptCodings  [4]uint8 // accept-encoding flags, controlled by r.numAcceptCodings. see httpCodingXXX for values
	// Stream states (non-zeros)
	primes               []pair        // hold prime queries, headers(main+subs), cookies, forms, and trailers(main+subs). [<r.stockPrimes>/max]
	extras               []pair        // hold extra queries, headers(main+subs), cookies, forms, trailers(main+subs), and params. [<r.stockExtras>/max]
	array                []byte        // store parsed, dynamic incoming data. [<r.stockArray>/4K/16K/64K1/(make <= 1G)]
	input                []byte        // bytes of raw incoming message heads. [<r.stockInput>/4K/16K]
	recvTimeout          time.Duration // timeout to recv the whole message content. zero means no timeout
	maxContentSize       int64         // max content size allowed for current message. if the content is vague, size will be calculated on receiving
	maxMemoryContentSize int32         // max content size allowed for loading the content into memory
	_                    int32         // padding
	contentSize          int64         // size info about incoming content. >=0: content size, -1: no content, -2: vague content
	httpVersion          uint8         // Version1_0, Version1_1, Version2, Version3
	asResponse           bool          // treat this incoming message as a response?
	keepAlive            int8          // used by HTTP/1.x only. -1: no connection header, 0: connection close, 1: connection keep-alive
	_                    byte          // padding
	headResult           int16         // result of receiving message head. values are as same as http status for convenience
	bodyResult           int16         // result of receiving message body. values are as same as http status for convenience
	// Stream states (zeros)
	failReason  string    // the fail reason of headResult or bodyResult
	bodyWindow  []byte    // a window used for receiving body. for HTTP/1.x, sizes must be same with r.input. [HTTP/1.x=<none>/16K, HTTP/2/3=<none>/4K/16K/64K1]
	bodyTime    time.Time // the time when first body read operation is performed on this stream
	contentText []byte    // if loadable, the received and loaded content of current message is at r.contentText[:r.receivedSize]. [<none>/r.input/4K/16K/64K1/(make)]
	contentFile *os.File  // used by r.holdContent(), if content is tempFile. will be closed on stream ends
	_httpIn0              // all values in this struct must be zero by default!
}
type _httpIn0 struct { // for fast reset, entirely
	elemBack          int32   // element begins from. for parsing elements in control & headers & content & trailers
	elemFore          int32   // element spanning to. for parsing elements in control & headers & content & trailers
	head              span    // head (control + headers) of current message -> r.input. set after head is received. only for debugging
	imme              span    // HTTP/1.x only. immediate data after current message head is at r.input[r.imme.from:r.imme.edge]
	hasExtra          [8]bool // has extra pairs? see pairXXX for indexes
	dateTime          int64   // parsed unix time of the date header
	arrayEdge         int32   // next usable position of r.array is at r.array[r.arrayEdge]. used when writing r.array
	arrayKind         int8    // kind of current r.array. see arrayKindXXX
	receiving         int8    // what section of the message are we currently receiving. see httpSectionXXX
	headers           zone    // headers ->r.primes
	hasRevisers       bool    // are there any incoming revisers hooked on this incoming message?
	upgradeSocket     bool    // upgrade: websocket?
	acceptGzip        bool    // does the peer accept gzip content coding? i.e. accept-encoding: gzip, deflate
	acceptBrotli      bool    // does the peer accept brotli content coding? i.e. accept-encoding: gzip, br
	numContentCodings int8    // num of content-encoding flags, controls r.contentCodings
	numAcceptCodings  int8    // num of accept-encoding flags, controls r.acceptCodings
	iContentLength    uint8   // index of content-length header in r.primes
	iContentLocation  uint8   // index of content-location header in r.primes
	iContentRange     uint8   // index of content-range header in r.primes
	iContentType      uint8   // index of content-type header in r.primes
	iDate             uint8   // index of date header in r.primes
	_                 [3]byte // padding
	zConnection       zone    // zone of connection headers in r.primes. may not be continuous
	zContentLanguage  zone    // zone of content-language headers in r.primes. may not be continuous
	zTrailer          zone    // zone of trailer headers in r.primes. may not be continuous
	zVia              zone    // zone of via headers in r.primes. may not be continuous
	contentReceived   bool    // is the content received? true if the message has no content or the content is received
	contentTextKind   int8    // kind of current r.contentText if it is text. see httpContentTextXXX
	receivedSize      int64   // bytes of currently received content. used by both sized & vague content receiver
	chunkSize         int64   // left size of current chunk if the chunk is too large to receive in one call. HTTP/1.1 chunked only
	chunkBack         int32   // for parsing chunked elements. HTTP/1.1 chunked only
	chunkFore         int32   // for parsing chunked elements. HTTP/1.1 chunked only
	chunkEdge         int32   // edge position of the filled chunked data in r.bodyWindow. HTTP/1.1 chunked only
	transferChunked   bool    // transfer-encoding: chunked? HTTP/1.1 only
	overChunked       bool    // for HTTP/1.1 requests, if chunked receiver over received in r.bodyWindow, then r.bodyWindow will be used as r.input on ends
	trailers          zone    // trailers -> r.primes. set after trailer section is received and parsed
}

func (r *httpIn_) onUse(httpVersion uint8, asResponse bool) { // for non-zeros
	r.primes = r.stockPrimes[0:1:cap(r.stockPrimes)] // use append(). r.primes[0] is skipped due to zero value of pair indexes.
	r.extras = r.stockExtras[0:0:cap(r.stockExtras)] // use append()
	r.array = r.stockArray[:]
	if httpVersion >= Version2 || asResponse {
		r.input = r.stockInput[:]
	} else {
		// HTTP/1.1 supports request pipelining, so input related are not set here.
	}
	httpHolder := r.stream.Holder()
	r.recvTimeout = httpHolder.RecvTimeout()
	r.maxContentSize = httpHolder.MaxContentSize()
	r.maxMemoryContentSize = httpHolder.MaxMemoryContentSize()
	r.contentSize = -1 // no content
	r.httpVersion = httpVersion
	r.asResponse = asResponse
	r.keepAlive = -1 // no connection header
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *httpIn_) onEnd() { // for zeros
	if cap(r.primes) != cap(r.stockPrimes) {
		putPairs(r.primes)
		r.primes = nil
	}
	if cap(r.extras) != cap(r.stockExtras) {
		putPairs(r.extras)
		r.extras = nil
	}
	if r.arrayKind == arrayKindPool {
		PutNK(r.array)
	}
	r.array = nil                                  // array of other kinds is only a reference, so just reset.
	if r.httpVersion >= Version2 || r.asResponse { // as we don't pipeline outgoing requests, incoming responses are not pipelined too.
		if cap(r.input) != cap(r.stockInput) {
			PutNK(r.input)
		}
		r.input = nil
		r.inputNext, r.inputEdge = 0, 0
	} else {
		// HTTP/1.1 supports request pipelining, so input related are not reset here.
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

	r.bodyTime = time.Time{}

	if r.contentTextKind == httpContentTextPool {
		PutNK(r.contentText)
	}
	r.contentText = nil // contentText of other kinds is only a reference, so just reset.

	if r.contentFile != nil {
		r.contentFile.Close()
		if DebugLevel() >= 2 {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(r.contentFile.Name()); err != nil {
			// TODO: log?
		}
		r.contentFile = nil
	}

	r._httpIn0 = _httpIn0{}
}

func (r *httpIn_) UnsafeMake(size int) []byte { return r.stream.unsafeMake(size) }
func (r *httpIn_) RemoteAddr() net.Addr       { return r.stream.remoteAddr() }

func (r *httpIn_) VersionCode() uint8    { return r.httpVersion }
func (r *httpIn_) IsHTTP1() bool         { return r.httpVersion <= Version1_1 }
func (r *httpIn_) IsHTTP1_0() bool       { return r.httpVersion == Version1_0 }
func (r *httpIn_) IsHTTP1_1() bool       { return r.httpVersion == Version1_1 }
func (r *httpIn_) IsHTTP2() bool         { return r.httpVersion == Version2 }
func (r *httpIn_) IsHTTP3() bool         { return r.httpVersion == Version3 }
func (r *httpIn_) Version() string       { return httpVersionStrings[r.httpVersion] }
func (r *httpIn_) UnsafeVersion() []byte { return httpVersionByteses[r.httpVersion] }

func (r *httpIn_) KeepAlive() int8   { return r.keepAlive }
func (r *httpIn_) HeadResult() int16 { return r.headResult }
func (r *httpIn_) BodyResult() int16 { return r.bodyResult }

func (r *httpIn_) addHeader(header *pair) bool { // as prime
	if edge, ok := r._addPrime(header); ok {
		r.headers.edge = edge
		return true
	}
	r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many headers"
	return false
}
func (r *httpIn_) HasHeaders() bool                  { return r.hasPairs(r.headers, pairHeader) }
func (r *httpIn_) AllHeaders() (headers [][2]string) { return r.allPairs(r.headers, pairHeader) }
func (r *httpIn_) H(name string) string {
	value, _ := r.Header(name)
	return value
}
func (r *httpIn_) Hstr(name string, defaultValue string) string {
	if value, ok := r.Header(name); ok {
		return value
	}
	return defaultValue
}
func (r *httpIn_) Hint(name string, defaultValue int) int {
	if value, ok := r.Header(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *httpIn_) Header(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.headers, pairHeader)
	return string(v), ok
}
func (r *httpIn_) UnsafeHeader(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.headers, pairHeader)
}
func (r *httpIn_) Headers(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.headers, pairHeader)
}
func (r *httpIn_) HasHeader(name string) bool {
	_, ok := r.getPair(name, 0, r.headers, pairHeader)
	return ok
}
func (r *httpIn_) DelHeader(name string) (deleted bool) {
	// TODO: add restrictions on what headers are allowed to del?
	return r.delPair(name, 0, r.headers, pairHeader)
}
func (r *httpIn_) delHeader(name []byte, nameHash uint16) {
	r.delPair(WeakString(name), nameHash, r.headers, pairHeader)
}
func (r *httpIn_) AddHeader(name string, value string) bool { // as extra, by webapp
	// TODO: add restrictions on what headers are allowed to add? should we check the value?
	// TODO: parse and check?
	// setFlags?
	return r.addExtra(name, value, 0, pairHeader)
}

func (r *httpIn_) _splitField(field *pair, fdesc *fdesc, p []byte) bool { // split: #element => [ element ] *( OWS "," OWS [ element ] )
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
func (r *httpIn_) _parseField(field *pair, fdesc *fdesc, p []byte, fully bool) bool { // for field data and value params
	field.setParsed()

	if field.value.isEmpty() {
		if fdesc.allowEmpty {
			field.dataEdge = field.value.edge
			return true
		}
		r.failReason = "field can't be empty"
		return false
	}

	// Now parse field value which is not empty.
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
		param.nameHash = bytesHash(p[text.from:text.edge])
		param.kind = pairParam
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

func (r *httpIn_) checkContentLength(header *pair, index uint8) bool { // Content-Length = 1*DIGIT
	// RFC 9110 (section 8.6):
	// Likewise, a sender MUST NOT forward a message with a Content-Length
	// header field value that does not match the ABNF above, with one
	// exception: a recipient of a Content-Length header field value
	// consisting of the same decimal value repeated as a comma-separated
	// list (e.g, "Content-Length: 42, 42") MAY either reject the message as
	// invalid or replace that invalid field value with a single instance of
	// the decimal value, since this likely indicates that a duplicate was
	// generated or combined by an upstream message processor.
	if r.contentSize == -1 { // r.contentSize can only be -1 or >= 0 here. -2 is set after all of the headers are received if the content is vague
		if size, ok := decToI64(header.valueAt(r.input)); ok {
			r.contentSize = size
			r.iContentLength = index
			return true
		}
	}
	r.headResult, r.failReason = StatusBadRequest, "bad content-length"
	return false
}
func (r *httpIn_) checkContentLocation(header *pair, index uint8) bool { // Content-Location = absolute-URI / partial-URI
	if r.iContentLocation == 0 && header.value.notEmpty() {
		// TODO: check syntax
		r.iContentLocation = index
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-location"
	return false
}
func (r *httpIn_) checkContentRange(header *pair, index uint8) bool { // Content-Range = range-unit SP ( range-resp / unsatisfied-range )
	if r.iContentRange == 0 && header.value.notEmpty() {
		// TODO: check syntax
		r.iContentRange = index
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-range"
	return false
}
func (r *httpIn_) checkContentType(header *pair, index uint8) bool { // Content-Type = media-type
	// media-type = type "/" subtype *( OWS ";" OWS parameter )
	// type = token
	// subtype = token
	// parameter = token "=" ( token / quoted-string )
	if r.iContentType == 0 && !header.dataEmpty() {
		// TODO: check syntax
		r.iContentType = index
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-type"
	return false
}
func (r *httpIn_) checkDate(header *pair, index uint8) bool { // Date = HTTP-date
	return r._checkHTTPDate(header, index, &r.iDate, &r.dateTime)
}
func (r *httpIn_) _checkHTTPDate(header *pair, index uint8, pIndex *uint8, toTime *int64) bool { // HTTP-date = day-name "," SP day SP month SP year SP hour ":" minute ":" second SP GMT
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

func (r *httpIn_) checkAcceptEncoding(pairs []pair, from uint8, edge uint8) bool { // Accept-Encoding = #( codings [ weight ] )
	// codings = content-coding / "identity" / "*"
	// content-coding = token
	for i := from; i < edge; i++ {
		if r.numAcceptCodings == int8(cap(r.acceptCodings)) {
			break // ignore too many codings
		}
		pair := &pairs[i]
		if pair.kind != pairHeader {
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
			coding = httpCodingUnknown
		}
		r.acceptCodings[r.numAcceptCodings] = coding
		r.numAcceptCodings++
	}
	return true
}
func (r *httpIn_) checkConnection(pairs []pair, from uint8, edge uint8) bool { // Connection = #connection-option
	if r.httpVersion >= Version2 {
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
func (r *httpIn_) checkContentEncoding(pairs []pair, from uint8, edge uint8) bool { // Content-Encoding = #content-coding
	// content-coding = token
	for i := from; i < edge; i++ {
		if r.numContentCodings == int8(cap(r.contentCodings)) {
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
			coding = httpCodingUnknown
		}
		r.contentCodings[r.numContentCodings] = coding
		r.numContentCodings++
	}
	return true
}
func (r *httpIn_) checkContentLanguage(pairs []pair, from uint8, edge uint8) bool { // Content-Language = #language-tag
	if r.zContentLanguage.isEmpty() {
		r.zContentLanguage.from = from
	}
	r.zContentLanguage.edge = edge
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *httpIn_) checkTrailer(pairs []pair, from uint8, edge uint8) bool { // Trailer = #field-name
	if r.zTrailer.isEmpty() {
		r.zTrailer.from = from
	}
	r.zTrailer.edge = edge
	// field-name = token
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *httpIn_) checkTransferEncoding(pairs []pair, from uint8, edge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.httpVersion != Version1_1 {
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
			// RFC 9112 (section 6.1):
			// A server that receives a request message with a transfer coding it does not understand SHOULD respond with 501 (Not Implemented).
			r.headResult, r.failReason = StatusNotImplemented, "unknown transfer coding"
			return false
		}
	}
	return true
}
func (r *httpIn_) checkVia(pairs []pair, from uint8, edge uint8) bool { // Via = #( received-protocol RWS received-by [ RWS comment ] )
	if r.zVia.isEmpty() {
		r.zVia.from = from
	}
	r.zVia.edge = edge
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}

func (r *httpIn_) determineContentMode() bool {
	if r.transferChunked { // must be HTTP/1.1 and there is a transfer-encoding: chunked
		if r.contentSize != -1 { // there is also a content-length: nnn
			// RFC 9112 (section 6.3):
			// If a message is received with both a Transfer-Encoding and a Content-Length header field,
			// the Transfer-Encoding overrides the Content-Length. Such a message might indicate an attempt to perform
			// request smuggling (Section 11.2) or response splitting (Section 11.1) and ought to be handled as an error.
			r.headResult, r.failReason = StatusBadRequest, "transfer-encoding conflits with content-length"
			return false
		}
		r.contentSize = -2 // vague
	} else if r.httpVersion >= Version2 && r.contentSize == -1 { // no content-length header
		// TODO: if there is no content, HTTP/2 and HTTP/3 should mark END_STREAM in headers frame. use this to decide!
		r.contentSize = -2 // if there is no content-length in HTTP/2 or HTTP/3, we treat it as vague
	}
	return true
}
func (r *httpIn_) IsVague() bool { return r.contentSize == -2 }

func (r *httpIn_) ContentSize() int64  { return r.contentSize }
func (r *httpIn_) ContentType() string { return string(r.UnsafeContentType()) }
func (r *httpIn_) UnsafeContentLength() []byte {
	if r.iContentLength == 0 {
		return nil
	}
	return r.primes[r.iContentLength].valueAt(r.input)
}
func (r *httpIn_) UnsafeContentType() []byte {
	if r.iContentType == 0 {
		return nil
	}
	return r.primes[r.iContentType].dataAt(r.input)
}

func (r *httpIn_) SetRecvTimeout(timeout time.Duration) { r.recvTimeout = timeout }
func (r *httpIn_) unsafeContent() []byte { // load message content into memory
	r._loadContent()
	if r.stream.isBroken() {
		return nil
	}
	return r.contentText[0:r.receivedSize]
}
func (r *httpIn_) _loadContent() { // into memory. [0, r.maxContentSize]
	if r.contentReceived {
		// Content is in r.contentText already.
		return
	}
	r.contentReceived = true
	switch content := r._recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		r.contentText = content // real content is r.contentText[:r.receivedSize]
		r.contentTextKind = httpContentTextPool
	case tempFile: // [0, r.maxContentSize]. case happens when sized content > 64K1, or content is vague.
		contentFile := content.(*os.File)
		if r.receivedSize == 0 { // vague content can has 0 size
			r.contentText = r.input
			r.contentTextKind = httpContentTextInput
		} else { // r.receivedSize > 0
			if r.receivedSize <= _64K1 { // must be vague content because sized content is a []byte if size <= _64K1
				r.contentText = GetNK(r.receivedSize) // 4K/16K/64K1. real content is r.content[:r.receivedSize]
				r.contentTextKind = httpContentTextPool
			} else { // r.receivedSize > 64K1, content can be sized or vague. just alloc
				r.contentText = make([]byte, r.receivedSize)
				r.contentTextKind = httpContentTextMake
			}
			if _, err := io.ReadFull(contentFile, r.contentText[:r.receivedSize]); err != nil {
				// TODO: r.webapp.log
			}
		}
		contentFile.Close()
		if DebugLevel() >= 2 {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(contentFile.Name()); err != nil {
			// TODO: r.webapp.log
		}
	case error: // i/o error or unexpected EOF
		// TODO: log error?
		r.stream.markBroken()
	}
}
func (r *httpIn_) proxyTakeContent() any {
	if r.contentReceived {
		if r.contentFile == nil {
			return r.contentText // immediate
		}
		return r.contentFile
	}
	r.contentReceived = true
	switch content := r._recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		r.contentText = content
		r.contentTextKind = httpContentTextPool // so r.contentText can be freed on end
		return r.contentText[0:r.receivedSize]
	case tempFile: // [0, r.maxContentSize]. case happens when sized content > 64K1, or content is vague.
		r.contentFile = content.(*os.File)
		return r.contentFile
	case error: // i/o error or unexpected EOF
		// TODO: log err?
	}
	r.stream.markBroken()
	return nil
}
func (r *httpIn_) _dropContent() { // if message content is not received, this will be called at last
	switch content := r._recvContent(false).(type) { // don't retain
	case []byte: // (0, 64K1]. case happens when sized content <= 64K1
		PutNK(content)
	case tempFile: // [0, r.maxContentSize]. case happens when sized content > 64K1, or content is vague.
		if content != fakeFile { // this must not happen!
			BugExitln("temp file is not fake when dropping content")
		}
	case error: // i/o error or unexpected EOF
		// TODO: log error?
		r.stream.markBroken()
	}
}
func (r *httpIn_) _recvContent(retain bool) any { // to []byte (for small content <= 64K1) or tempFile (for large content > 64K1, or content is vague)
	if r.contentSize > 0 && r.contentSize <= _64K1 { // (0, 64K1]. save to []byte. must be received in a timeout
		if err := r.stream.setReadDeadline(); err != nil {
			return err
		}
		// Since content is small, r.bodyWindow and tempFile are not needed.
		contentText := GetNK(r.contentSize) // 4K/16K/64K1. max size of content is 64K1
		r.receivedSize = int64(r.imme.size())
		if r.receivedSize > 0 { // r.imme has data
			copy(contentText, r.input[r.imme.from:r.imme.edge])
			r.imme.zero()
		}
		if n, err := r.stream.readFull(contentText[r.receivedSize:r.contentSize]); err == nil {
			r.receivedSize += int64(n)
			return contentText // []byte, fetched from pool
		} else {
			PutNK(contentText)
			return err
		}
	} else { // (64K1, r.maxContentSize] when sized, or [0, r.maxContentSize] when vague. save to tempFile and return the file
		contentFile, err := r._newTempFile(retain)
		if err != nil {
			return err
		}
		var data []byte
		for {
			data, err = r.inMessage.readContent()
			if len(data) > 0 { // skip 0, nothing to write
				if _, e := contentFile.Write(data); e != nil {
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

func (r *httpIn_) addTrailer(trailer *pair) bool { // as prime
	if edge, ok := r._addPrime(trailer); ok {
		r.trailers.edge = edge
		return true
	}
	r.bodyResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many trailers"
	return false
}
func (r *httpIn_) HasTrailers() bool                   { return r.hasPairs(r.trailers, pairTrailer) }
func (r *httpIn_) AllTrailers() (trailers [][2]string) { return r.allPairs(r.trailers, pairTrailer) }
func (r *httpIn_) T(name string) string {
	value, _ := r.Trailer(name)
	return value
}
func (r *httpIn_) Tstr(name string, defaultValue string) string {
	if value, ok := r.Trailer(name); ok {
		return value
	}
	return defaultValue
}
func (r *httpIn_) Tint(name string, defaultValue int) int {
	if value, ok := r.Trailer(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *httpIn_) Trailer(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.trailers, pairTrailer)
	return string(v), ok
}
func (r *httpIn_) UnsafeTrailer(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.trailers, pairTrailer)
}
func (r *httpIn_) Trailers(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.trailers, pairTrailer)
}
func (r *httpIn_) HasTrailer(name string) bool {
	_, ok := r.getPair(name, 0, r.trailers, pairTrailer)
	return ok
}
func (r *httpIn_) DelTrailer(name string) (deleted bool) {
	return r.delPair(name, 0, r.trailers, pairTrailer)
}
func (r *httpIn_) delTrailer(name []byte, nameHash uint16) {
	r.delPair(WeakString(name), nameHash, r.trailers, pairTrailer)
}
func (r *httpIn_) AddTrailer(name string, value string) bool { // as extra, by webapp
	// TODO: add restrictions on what trailers are allowed to add? should we check the value?
	// TODO: parse and check?
	// setFlags?
	return r.addExtra(name, value, 0, pairTrailer)
}

func (r *httpIn_) _addPrime(prime *pair) (edge uint8, ok bool) {
	if len(r.primes) == cap(r.primes) { // full
		if cap(r.primes) != cap(r.stockPrimes) { // too many primes
			return 0, false
		}
		if DebugLevel() >= 2 {
			Println("use large primes!")
		}
		r.primes = getPairs()
		r.primes = append(r.primes, r.stockPrimes[:]...)
	}
	r.primes = append(r.primes, *prime)
	return uint8(len(r.primes)), true
}
func (r *httpIn_) _delPrime(i uint8) { r.primes[i].zero() }

func (r *httpIn_) addExtra(name string, value string, nameHash uint16, extraKind int8) bool {
	nameSize := len(name)
	if nameSize == 0 || nameSize > 255 { // name size is limited at 255
		return false
	}
	valueSize := len(value)
	if extraKind == pairForm { // for forms, max value size is 1G
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
	if nameHash == 0 {
		extra.nameHash = stringHash(name)
	} else {
		extra.nameHash = nameHash
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
func (r *httpIn_) _addExtra(extra *pair) bool {
	if len(r.extras) == cap(r.extras) { // full
		if cap(r.extras) != cap(r.stockExtras) { // too many extras
			return false
		}
		if DebugLevel() >= 2 {
			Println("use large extras!")
		}
		r.extras = getPairs()
		r.extras = append(r.extras, r.stockExtras[:]...)
	}
	r.extras = append(r.extras, *extra)
	r.hasExtra[extra.kind] = true
	return true
}

func (r *httpIn_) hasPairs(primes zone, extraKind int8) bool {
	return primes.notEmpty() || r.hasExtra[extraKind]
}
func (r *httpIn_) allPairs(primes zone, extraKind int8) [][2]string {
	var pairs [][2]string
	if extraKind == pairHeader || extraKind == pairTrailer { // skip sub fields, only collects values of main fields
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash != 0 {
				p := r._placeOf(prime)
				pairs = append(pairs, [2]string{string(prime.nameAt(p)), string(prime.valueAt(p))})
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.nameHash != 0 && extra.kind == extraKind && !extra.isSubField() {
					pairs = append(pairs, [2]string{string(extra.nameAt(r.array)), string(extra.valueAt(r.array))})
				}
			}
		}
	} else { // queries, cookies, forms, and params
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash != 0 {
				p := r._placeOf(prime)
				pairs = append(pairs, [2]string{string(prime.nameAt(p)), string(prime.valueAt(p))})
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.nameHash != 0 && extra.kind == extraKind {
					pairs = append(pairs, [2]string{string(extra.nameAt(r.array)), string(extra.valueAt(r.array))})
				}
			}
		}
	}
	return pairs
}
func (r *httpIn_) getPair(name string, nameHash uint16, primes zone, extraKind int8) (value []byte, ok bool) {
	if name == "" {
		return
	}
	if nameHash == 0 {
		nameHash = stringHash(name)
	}
	if extraKind == pairHeader || extraKind == pairTrailer { // skip comma fields, only collects data of fields without comma
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash == nameHash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					if !prime.isParsed() && !r._splitField(prime, defaultFdesc, p) {
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
				if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && !extra.isCommaValue() {
					if p := r._placeOf(extra); extra.nameEqualString(p, name) {
						return extra.dataAt(p), true
					}
				}
			}
		}
	} else { // queries, cookies, forms, and params
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash == nameHash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					return prime.valueAt(p), true
				}
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && extra.nameEqualString(r.array, name) {
					return extra.valueAt(r.array), true
				}
			}
		}
	}
	return
}
func (r *httpIn_) getPairs(name string, nameHash uint16, primes zone, extraKind int8) (values []string, ok bool) {
	if name == "" {
		return
	}
	if nameHash == 0 {
		nameHash = stringHash(name)
	}
	if extraKind == pairHeader || extraKind == pairTrailer { // skip comma fields, only collects data of fields without comma
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash == nameHash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					if !prime.isParsed() && !r._splitField(prime, defaultFdesc, p) {
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
				if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && !extra.isCommaValue() {
					if p := r._placeOf(extra); extra.nameEqualString(p, name) {
						values = append(values, string(extra.dataAt(p)))
					}
				}
			}
		}
	} else { // queries, cookies, forms, and params
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash == nameHash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					values = append(values, string(prime.valueAt(p)))
				}
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && extra.nameEqualString(r.array, name) {
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
func (r *httpIn_) delPair(name string, nameHash uint16, primes zone, extraKind int8) (deleted bool) {
	if name == "" {
		return
	}
	if nameHash == 0 {
		nameHash = stringHash(name)
	}
	for i := primes.from; i < primes.edge; i++ {
		if prime := &r.primes[i]; prime.nameHash == nameHash {
			if p := r._placeOf(prime); prime.nameEqualString(p, name) {
				prime.zero()
				deleted = true
			}
		}
	}
	if r.hasExtra[extraKind] {
		for i := 0; i < len(r.extras); i++ {
			if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && extra.nameEqualString(r.array, name) {
				extra.zero()
				deleted = true
			}
		}
	}
	return
}
func (r *httpIn_) _placeOf(pair *pair) []byte {
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

func (r *httpIn_) proxyDelHopHeaders() {
	r._proxyDelHopFields(r.headers, pairHeader, r.delHeader)
}
func (r *httpIn_) proxyDelHopTrailers() {
	r._proxyDelHopFields(r.trailers, pairTrailer, r.delTrailer)
}
func (r *httpIn_) _proxyDelHopFields(fields zone, extraKind int8, delField func(name []byte, nameHash uint16)) { // TODO: improve performance
	// These fields should be removed anyway: proxy-connection, keep-alive, te, transfer-encoding, upgrade
	delField(bytesProxyConnection, hashProxyConnection)
	delField(bytesKeepAlive, hashKeepAlive)
	if !r.asResponse { // as request
		delField(bytesTE, hashTE)
	}
	delField(bytesTransferEncoding, hashTransferEncoding)
	delField(bytesUpgrade, hashUpgrade)

	// Now remove connection options in primes and extras.
	// Note: we don't remove ("connection: xxx, yyy") itself here, we simply restrict it from being copied or inserted when acting as a proxy.
	for i := r.zConnection.from; i < r.zConnection.edge; i++ {
		prime := &r.primes[i]
		// Skip fields that are not "connection: xxx, yyy"
		if prime.nameHash != hashConnection || !prime.nameEqualBytes(r.input, bytesConnection) {
			continue
		}
		p := r._placeOf(prime)
		optionName := prime.dataAt(p)
		optionHash := bytesHash(optionName)
		// Skip options that are "connection: connection"
		if optionHash == hashConnection && bytes.Equal(optionName, bytesConnection) {
			continue
		}
		// Got a "connection: xxx" option, remove it from fields
		for j := fields.from; j < fields.edge; j++ {
			if field := &r.primes[j]; field.nameHash == optionHash && field.nameEqualBytes(p, optionName) {
				field.zero()
			}
		}
		if r.hasExtra[extraKind] {
			for j := 0; j < len(r.extras); j++ {
				if extra := &r.extras[j]; extra.nameHash == optionHash && extra.kind == extraKind {
					if p := r._placeOf(extra); extra.nameEqualBytes(p, optionName) {
						extra.zero()
					}
				}
			}
		}
	}
}

func (r *httpIn_) proxyWalkHeaders(callback func(header *pair, name []byte, value []byte) bool) bool { // excluding sub headers
	return r._proxyWalkMainFields(r.headers, pairHeader, callback)
}
func (r *httpIn_) proxyWalkTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool { // excluding sub trailers
	return r._proxyWalkMainFields(r.trailers, pairTrailer, callback)
}
func (r *httpIn_) _proxyWalkMainFields(fields zone, extraKind int8, callback func(field *pair, name []byte, value []byte) bool) bool {
	for i := fields.from; i < fields.edge; i++ {
		if field := &r.primes[i]; field.nameHash != 0 {
			p := r._placeOf(field)
			if !callback(field, field.nameAt(p), field.valueAt(p)) {
				return false
			}
		}
	}
	if r.hasExtra[extraKind] {
		for i := 0; i < len(r.extras); i++ {
			if field := &r.extras[i]; field.nameHash != 0 && field.kind == extraKind && !field.isSubField() {
				if !callback(field, field.nameAt(r.array), field.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *httpIn_) arrayCopy(src []byte) bool { // callers don't guarantee the intended memory cost is limited
	if len(src) > 0 {
		edge := r.arrayEdge + int32(len(src))
		if edge < r.arrayEdge { // overflow
			return false
		}
		if edge > r.maxMemoryContentSize {
			return false
		}
		if !r._growArray(int32(len(src))) {
			return false
		}
		r.arrayEdge += int32(copy(r.array[r.arrayEdge:], src))
	}
	return true
}
func (r *httpIn_) arrayPush(b byte) { // callers must ensure the intended memory cost is limited
	r.array[r.arrayEdge] = b
	if r.arrayEdge++; r.arrayEdge == int32(cap(r.array)) {
		r._growArray(1)
	}
}
func (r *httpIn_) _growArray(size int32) bool { // stock(<4K)->4K->16K->64K1->(128K->...->1G)
	edge := r.arrayEdge + size
	if edge < 0 || edge > _1G { // cannot overflow hard limit: 1G
		return false
	}
	if edge <= int32(cap(r.array)) { // existing array is enough
		return true
	}
	lastKind := r.arrayKind
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
	if lastKind == arrayKindPool {
		PutNK(r.array)
	}
	r.array = array
	return true
}

func (r *httpIn_) saveContentFilesDir() string { return r.stream.Holder().SaveContentFilesDir() }

func (r *httpIn_) _newTempFile(retain bool) (tempFile, error) { // to save content to
	if retain {
		filesDir := r.saveContentFilesDir()
		pathBuffer := r.UnsafeMake(len(filesDir) + 19) // 19 bytes is enough for an int64
		n := copy(pathBuffer, filesDir)
		n += r.stream.Conn().MakeTempName(pathBuffer[n:], time.Now().Unix())
		return os.OpenFile(WeakString(pathBuffer[:n]), os.O_RDWR|os.O_CREATE, 0644)
	} else { // since data is not used by upper caller, we don't need to actually write data to file.
		return fakeFile, nil
	}
}

func (r *httpIn_) _isLongTime() bool { // reports whether the receiving of incoming content costs a long time
	return r.recvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.recvTimeout
}

var ( // httpIn_ errors
	httpInBadChunk = errors.New("bad incoming http chunk")
	httpInLongTime = errors.New("http incoming costs a long time")
)

//////////////////////////////////////// HTTP outgoing implementation ////////////////////////////////////////

// httpOut collects shared methods between *server[1-3]Response and *backend[1-3]Request.
type httpOut interface {
	control() []byte
	addHeader(name []byte, value []byte) bool
	header(name []byte) (value []byte, ok bool)
	hasHeader(name []byte) bool
	delHeader(name []byte) (deleted bool)
	delHeaderAt(i uint8)
	insertHeader(nameHash uint16, name []byte, value []byte) bool
	removeHeader(nameHash uint16, name []byte) (deleted bool)
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
	proxyPassHeaders() error
	proxyPassBytes(data []byte) error
}

// httpOut_ is the parent for serverResponse_ and backendRequest_.
type httpOut_ struct { // outgoing. needs building
	// Assocs
	stream     httpStream // *server[1-3]Stream, *backend[1-3]Stream
	outMessage httpOut    // *server[1-3]Response, *backend[1-3]Request
	// Stream states (stocks)
	stockFields [1536]byte // for r.fields
	// Stream states (controlled)
	edges [128]uint16 // edges of headers or trailers in r.fields, but not used at the same time. controlled by r.numHeaders or r.numTrailers. edges[0] is not used!
	piece Piece       // for r.chain. used when sending content or echoing chunks
	chain Chain       // outgoing piece chain. used when sending content or echoing chunks
	// Stream states (non-zeros)
	fields      []byte        // bytes of the headers or trailers which are not manipulated at the same time. [<r.stockFields>/4K/16K]
	sendTimeout time.Duration // timeout to send the whole message. zero means no timeout
	contentSize int64         // info of outgoing content. -1: not set, -2: vague, >=0: size
	httpVersion uint8         // Version1_1, Version2, Version3
	asRequest   bool          // treat this outgoing message as request?
	numHeaders  uint8         // 1+num of added headers, starts from 1 because edges[0] is not used
	numTrailers uint8         // 1+num of added trailers, starts from 1 because edges[0] is not used
	// Stream states (zeros)
	sendTime      time.Time   // the time when first write operation is performed
	contentRanges []Range     // if outgoing content is ranged, this will be set
	rangeType     string      // if outgoing content is ranged, this will be the content type for each range
	vector        net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector   [4][]byte   // for sending/echoing message. reset after stream
	_httpOut0                 // all values in this struct must be zero by default!
}
type _httpOut0 struct { // for fast reset, entirely
	controlEdge   uint16 // edge of control in r.fields. only used by request to mark the method and request-target
	fieldsEdge    uint16 // edge of r.fields. max size of r.fields must be <= 16K. used by both headers and trailers because they are not manipulated at the same time
	hasRevisers   bool   // are there any outgoing revisers hooked on this outgoing message?
	isSent        bool   // whether the message is sent
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
	iContentType  uint8  // position of content-type in r.edges
	iDate         uint8  // position of date in r.edges
}

func (r *httpOut_) onUse(httpVersion uint8, asRequest bool) { // for non-zeros
	r.fields = r.stockFields[:]
	httpHolder := r.stream.Holder()
	r.sendTimeout = httpHolder.SendTimeout()
	r.contentSize = -1 // not set
	r.httpVersion = httpVersion
	r.asRequest = asRequest
	r.numHeaders, r.numTrailers = 1, 1 // r.edges[0] is not used
}
func (r *httpOut_) onEnd() { // for zeros
	if cap(r.fields) != cap(r.stockFields) {
		PutNK(r.fields)
		r.fields = nil
	}
	// r.piece was reset in echo(), and will be reset here if send() was used. double free doesn't matter
	r.chain.free()

	r.sendTime = time.Time{}
	r.contentRanges = nil
	r.rangeType = ""
	r.vector = nil
	r.fixedVector = [4][]byte{}
	r._httpOut0 = _httpOut0{}
}

func (r *httpOut_) unsafeMake(size int) []byte { return r.stream.unsafeMake(size) }

func (r *httpOut_) AddContentType(contentType string) bool {
	return r.AddHeaderBytes(bytesContentType, ConstBytes(contentType))
}
func (r *httpOut_) AddContentTypeBytes(contentType []byte) bool {
	return r.AddHeaderBytes(bytesContentType, contentType)
}

func (r *httpOut_) Header(name string) (value string, ok bool) {
	v, ok := r.outMessage.header(ConstBytes(name))
	return string(v), ok
}
func (r *httpOut_) HasHeader(name string) bool {
	return r.outMessage.hasHeader(ConstBytes(name))
}
func (r *httpOut_) AddHeader(name string, value string) bool {
	return r.AddHeaderBytes(ConstBytes(name), ConstBytes(value))
}
func (r *httpOut_) AddHeaderBytes(name []byte, value []byte) bool {
	nameHash, valid, lower := r._nameCheck(name)
	if !valid {
		return false
	}
	for _, b := range value { // to prevent response splitting
		if b == '\r' || b == '\n' {
			return false
		}
	}
	return r.outMessage.insertHeader(nameHash, lower, value) // some headers are restricted
}
func (r *httpOut_) DelHeader(name string) bool {
	return r.DelHeaderBytes(ConstBytes(name))
}
func (r *httpOut_) DelHeaderBytes(name []byte) bool {
	nameHash, valid, lower := r._nameCheck(name)
	if !valid {
		return false
	}
	return r.outMessage.removeHeader(nameHash, lower)
}
func (r *httpOut_) _nameCheck(name []byte) (nameHash uint16, valid bool, lower []byte) { // TODO: improve performance
	n := len(name)
	if n == 0 || n > 255 {
		return 0, false, nil
	}
	allLower := true
	for i := 0; i < n; i++ {
		if b := name[i]; b >= 'a' && b <= 'z' || b == '-' {
			nameHash += uint16(b)
		} else {
			nameHash = 0
			allLower = false
			break
		}
	}
	if allLower {
		return nameHash, true, name
	}
	nameBuffer := r.stream.buffer256()
	for i := 0; i < n; i++ {
		b := name[i]
		if b >= 'A' && b <= 'Z' {
			b += 0x20 // to lower
		} else if !(b >= 'a' && b <= 'z' || b == '-') {
			return 0, false, nil
		}
		nameHash += uint16(b)
		nameBuffer[i] = b
	}
	return nameHash, true, nameBuffer[:n]
}

func (r *httpOut_) isVague() bool { return r.contentSize == -2 }
func (r *httpOut_) IsSent() bool  { return r.isSent }

func (r *httpOut_) _insertContentType(contentType []byte) (ok bool) {
	return r._appendSingleton(&r.iContentType, bytesContentType, contentType)
}
func (r *httpOut_) _insertDate(date []byte) (ok bool) { // rarely used in backend request
	return r._appendSingleton(&r.iDate, bytesDate, date)
}
func (r *httpOut_) _appendSingleton(pIndex *uint8, name []byte, value []byte) bool {
	if *pIndex > 0 || !r.outMessage.addHeader(name, value) {
		return false
	}
	*pIndex = r.numHeaders - 1 // r.numHeaders begins from 1, so must minus one
	return true
}

func (r *httpOut_) _removeContentType() (deleted bool) { return r._deleteSingleton(&r.iContentType) }
func (r *httpOut_) _removeDate() (deleted bool)        { return r._deleteSingleton(&r.iDate) }
func (r *httpOut_) _deleteSingleton(pIndex *uint8) bool {
	if *pIndex == 0 { // not exist
		return false
	}
	r.outMessage.delHeaderAt(*pIndex)
	*pIndex = 0
	return true
}

func (r *httpOut_) _setUnixTime(pUnixTime *int64, pIndex *uint8, unixTime int64) bool {
	if unixTime < 0 {
		return false
	}
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.outMessage.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = unixTime
	return true
}
func (r *httpOut_) _addUnixTime(pUnixTime *int64, pIndex *uint8, name []byte, httpDate []byte) bool {
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.outMessage.delHeaderAt(*pIndex)
		*pIndex = 0
	} else { // >= 0 or -1
		*pUnixTime = -2
	}
	if !r.outMessage.addHeader(name, httpDate) {
		return false
	}
	*pIndex = r.numHeaders - 1 // r.numHeaders begins from 1, so must minus one
	return true
}
func (r *httpOut_) _delUnixTime(pUnixTime *int64, pIndex *uint8) bool {
	if *pUnixTime == -1 {
		return false
	}
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.outMessage.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = -1
	return true
}

func (r *httpOut_) pickRanges(contentRanges []Range, rangeType string) {
	r.contentRanges = contentRanges
	r.rangeType = rangeType
}

func (r *httpOut_) SetSendTimeout(timeout time.Duration) { r.sendTimeout = timeout }

func (r *httpOut_) Send(content string) error      { return r.sendText(ConstBytes(content)) }
func (r *httpOut_) SendBytes(content []byte) error { return r.sendText(content) }
func (r *httpOut_) SendFile(contentPath string) error {
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
func (r *httpOut_) SendJSON(content any) error { // TODO: optimize performance
	r.AddContentTypeBytes(bytesTypeJSON)
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	return r.sendText(data)
}

func (r *httpOut_) Echo(chunk string) error      { return r.echoText(ConstBytes(chunk)) }
func (r *httpOut_) EchoBytes(chunk []byte) error { return r.echoText(chunk) }
func (r *httpOut_) EchoFile(chunkPath string) error {
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
func (r *httpOut_) AddTrailer(name string, value string) bool {
	return r.AddTrailerBytes(ConstBytes(name), ConstBytes(value))
}
func (r *httpOut_) AddTrailerBytes(name []byte, value []byte) bool {
	if !r.isSent { // trailers must be added after headers & content are sent, otherwise r.fields will be messed up
		return false
	}
	return r.outMessage.addTrailer(name, value)
}
func (r *httpOut_) Trailer(name string) (value string, ok bool) {
	v, ok := r.outMessage.trailer(ConstBytes(name))
	return string(v), ok
}

func (r *httpOut_) _proxyPassMessage(inMessage httpIn) error {
	proxyPass := r.outMessage.proxyPassBytes
	if inMessage.IsVague() || r.hasRevisers { // if we need to revise, we always use vague no matter the original content is sized or vague
		proxyPass = r.EchoBytes
	} else { // inMessage is sized and there are no revisers, use proxyPassBytes
		r.isSent = true
		r.contentSize = inMessage.ContentSize()
		// TODO: find a way to reduce i/o syscalls if content is small?
		if err := r.outMessage.proxyPassHeaders(); err != nil {
			return err
		}
	}
	for {
		data, err := inMessage.readContent()
		if len(data) >= 0 {
			if e := proxyPass(data); e != nil {
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
	if inMessage.HasTrailers() { // added trailers will be written by upper code eventually.
		if !inMessage.proxyWalkTrailers(func(trailer *pair, name []byte, value []byte) bool {
			return r.outMessage.addTrailer(name, value)
		}) {
			return httpOutTrailerFailed
		}
	}
	return nil
}
func (r *httpOut_) proxyPostMessage(content any, hasTrailers bool) error {
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
		return r.outMessage.doSend()
	}
}
func (r *httpOut_) _proxyCopyTrailers(inMessage httpIn, proxyConfig *WebExchanProxyConfig) bool {
	return inMessage.proxyWalkTrailers(func(trailer *pair, name []byte, value []byte) bool {
		return r.outMessage.addTrailer(name, value)
	})
}

func (r *httpOut_) sendText(content []byte) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content)) // initial size, may be changed by revisers
	return r.outMessage.doSend()
}
func (r *httpOut_) sendFile(content *os.File, info os.FileInfo, shut bool) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetFile(content, info, shut)
	r.chain.PushTail(&r.piece)
	r.contentSize = info.Size() // initial size, may be changed by revisers
	return r.outMessage.doSend()
}
func (r *httpOut_) _beforeSend() error {
	if r.isSent {
		return httpOutAlreadySent
	}
	r.isSent = true
	if r.hasRevisers {
		r.outMessage.beforeSend()
	}
	return nil
}

func (r *httpOut_) echoText(chunk []byte) error {
	if err := r._beforeEcho(); err != nil {
		return err
	}
	if len(chunk) == 0 { // empty chunk is not actually sent, since it is used to indicate the end. pretend to succeed
		return nil
	}
	r.piece.SetText(chunk)
	defer r.piece.zero()
	return r.outMessage.doEcho()
}
func (r *httpOut_) echoFile(chunk *os.File, info os.FileInfo, shut bool) error {
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
	return r.outMessage.doEcho()
}
func (r *httpOut_) _beforeEcho() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if r.isSent {
		return nil
	}
	if r.contentSize != -1 { // is set, either sized or vague
		return httpOutMixedContent
	}
	r.isSent = true
	r.contentSize = -2 // vague
	if r.hasRevisers {
		r.outMessage.beforeEcho()
	}
	return r.outMessage.echoHeaders()
}

func (r *httpOut_) growHeader(size int) (from int, edge int, ok bool) { // headers and trailers are not manipulated at the same time
	if r.numHeaders == uint8(cap(r.edges)) { // too many headers
		return
	}
	return r._growFields(size)
}
func (r *httpOut_) growTrailer(size int) (from int, edge int, ok bool) { // headers and trailers are not manipulated at the same time
	if r.numTrailers == uint8(cap(r.edges)) { // too many trailers
		return
	}
	return r._growFields(size)
}
func (r *httpOut_) _growFields(size int) (from int, edge int, ok bool) { // used by growHeader first and growTrailer later as they are not manipulated at the same time
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

func (r *httpOut_) _longTimeCheck(err error) error {
	if err == nil && r._isLongTime() {
		err = httpOutLongTime
	}
	if err != nil {
		r.stream.markBroken()
	}
	return err
}
func (r *httpOut_) _isLongTime() bool { // reports whether the sending of outgoing content costs a long time
	return r.sendTimeout > 0 && time.Now().Sub(r.sendTime) >= r.sendTimeout
}

var ( // httpOut_ errors
	httpOutLongTime      = errors.New("http outgoing costs a long time")
	httpOutWriteBroken   = errors.New("write broken")
	httpOutUnknownStatus = errors.New("unknown status")
	httpOutAlreadySent   = errors.New("already sent")
	httpOutTooLarge      = errors.New("content too large")
	httpOutMixedContent  = errors.New("mixed content mode")
	httpOutTrailerFailed = errors.New("add trailer failed")
)

//////////////////////////////////////// HTTP socket implementation ////////////////////////////////////////

// httpSocket
type httpSocket interface {
	Read(dst []byte) (int, error)
	Write(src []byte) (int, error)
	Close() error
}

// httpSocket_
type httpSocket_ struct { // incoming and outgoing
	// Assocs
	stream httpStream // *server[1-3]Stream, *backend[1-3]Stream
	socket httpSocket // *server[1-3]Socket, *backend[1-3]Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	asServer bool // treat this socket as a server socket?
	// Stream states (zeros)
	_httpSocket0 // all values in this struct must be zero by default!
}
type _httpSocket0 struct { // for fast reset, entirely
}

func (s *httpSocket_) onUse(asServer bool) {
	s.asServer = asServer
}
func (s *httpSocket_) onEnd() {
	s._httpSocket0 = _httpSocket0{}
}

func (s *httpSocket_) todo() {
}

var ( // httpSocket_ errors
	httpSocketWriteBroken = errors.New("write broken")
)

//////////////////////////////////////// HTTP server implementation ////////////////////////////////////////

// HTTPServer
type HTTPServer interface { // for *http[x3]Server
	// Imports
	Server
	// Methods
	MaxConcurrentConnsPerGate() int32
	bindWebapps()
	httpHolder() _httpHolder_
}

// httpServer_ is the parent for http[x3]Server.
type httpServer_[G httpGate] struct {
	// Parent
	Server_[G]
	// Mixins
	_httpHolder_ // to carry configs used by gates
	// Assocs
	defaultWebapp *Webapp // default webapp if not found
	// States
	webapps                   []string               // for what webapps
	exactWebapps              []*hostnameTo[*Webapp] // like: ("example.com")
	suffixWebapps             []*hostnameTo[*Webapp] // like: ("*.example.com")
	prefixWebapps             []*hostnameTo[*Webapp] // like: ("www.example.*")
	forceScheme               int8                   // scheme (http/https) that must be used
	alignScheme               bool                   // use https scheme for TLS and http scheme for others?
	maxConcurrentConnsPerGate int32                  // max concurrent connections allowed per gate
}

func (s *httpServer_[G]) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)

	s.forceScheme = -1 // not forced
}

func (s *httpServer_[G]) onConfigure() {
	s.Server_.OnConfigure()
	s._httpHolder_.onConfigure(s, TmpDir()+"/web/servers/"+s.name, 0*time.Second, 0*time.Second)

	// webapps
	s.ConfigureStringList("webapps", &s.webapps, nil, []string{})

	// forceScheme
	var scheme string
	s.ConfigureString("forceScheme", &scheme, func(value string) error {
		if value != "http" && value != "https" {
			return errors.New(".forceScheme has an invalid value")
		}
		return nil
	}, "")
	switch scheme {
	case "http":
		s.forceScheme = SchemeHTTP
	case "https":
		s.forceScheme = SchemeHTTPS
	}

	// alignScheme
	s.ConfigureBool("alignScheme", &s.alignScheme, true)

	// maxConcurrentConnsPerGate
	s.ConfigureInt32("maxConcurrentConnsPerGate", &s.maxConcurrentConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConcurrentConnsPerGate has an invalid value")
	}, 10000)
}
func (s *httpServer_[G]) onPrepare() {
	s.Server_.OnPrepare()
	s._httpHolder_.onPrepare(s, 0755)
}

func (s *httpServer_[G]) MaxConcurrentConnsPerGate() int32 { return s.maxConcurrentConnsPerGate }

func (s *httpServer_[G]) bindWebapps() {
	for _, webappName := range s.webapps {
		webapp := s.stage.Webapp(webappName)
		if webapp == nil {
			continue
		}
		if s.TLSMode() {
			if webapp.tlsCertificate == "" || webapp.tlsPrivateKey == "" {
				UseExitln("webapps that bound to tls server must have certificates and private keys")
			}
			certificate, err := tls.LoadX509KeyPair(webapp.tlsCertificate, webapp.tlsPrivateKey)
			if err != nil {
				UseExitln(err.Error())
			}
			if DebugLevel() >= 1 {
				Printf("adding certificate to %s\n", s.Colonport())
			}
			s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, certificate)
		}
		webapp.bindServer(s.shell.(HTTPServer))
		if webapp.isDefault {
			s.defaultWebapp = webapp
		}
		// TODO: use hash table?
		for _, hostname := range webapp.exactHostnames {
			s.exactWebapps = append(s.exactWebapps, &hostnameTo[*Webapp]{hostname, webapp})
		}
		// TODO: use radix trie?
		for _, hostname := range webapp.suffixHostnames {
			s.suffixWebapps = append(s.suffixWebapps, &hostnameTo[*Webapp]{hostname, webapp})
		}
		// TODO: use radix trie?
		for _, hostname := range webapp.prefixHostnames {
			s.prefixWebapps = append(s.prefixWebapps, &hostnameTo[*Webapp]{hostname, webapp})
		}
	}
}
func (s *httpServer_[G]) findWebapp(hostname []byte) *Webapp {
	// TODO: use hash table?
	for _, exactMap := range s.exactWebapps {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixWebapps {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixWebapps {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return s.defaultWebapp // may be nil
}

func (s *httpServer_[G]) httpHolder() _httpHolder_ { return s._httpHolder_ }

// httpGate
type httpGate interface {
	// Imports
	Gate
	httpHolder
	// Methods
}

// httpGate_ is the parent for http[x3]Gate.
type httpGate_[S HTTPServer] struct {
	// Parent
	Gate_[S]
	// Mixins
	_httpHolder_
	// States
	maxConcurrentConns int32        // max concurrent conns allowed for this gate
	concurrentConns    atomic.Int32 // current concurrent conns. TODO: false sharing
}

func (g *httpGate_[S]) onNew(server S, id int32) {
	g.Gate_.OnNew(server, id)
	g._httpHolder_ = server.httpHolder()
	g.maxConcurrentConns = server.MaxConcurrentConnsPerGate()
	g.concurrentConns.Store(0)
}

func (g *httpGate_[S]) DecConcurrentConns() int32 { return g.concurrentConns.Add(-1) }
func (g *httpGate_[S]) IncConcurrentConns() int32 { return g.concurrentConns.Add(1) }
func (g *httpGate_[S]) ReachLimit(concurrentConns int32) bool {
	return concurrentConns > g.maxConcurrentConns
}

// Request is the server-side http request.
type Request interface { // for *server[1-3]Request
	RemoteAddr() net.Addr
	Webapp() *Webapp

	IsAbsoluteForm() bool    // TODO: what about HTTP/2 and HTTP/3?
	IsAsteriskOptions() bool // OPTIONS *

	VersionCode() uint8
	IsHTTP1() bool
	IsHTTP1_0() bool
	IsHTTP1_1() bool
	IsHTTP2() bool
	IsHTTP3() bool
	Version() string // HTTP/1.0, HTTP/1.1, HTTP/2, HTTP/3
	UnsafeVersion() []byte

	SchemeCode() uint8 // SchemeHTTP, SchemeHTTPS
	IsHTTP() bool
	IsHTTPS() bool
	Scheme() string // http, https
	UnsafeScheme() []byte

	IsGET() bool
	IsHEAD() bool
	IsPOST() bool
	IsPUT() bool
	IsDELETE() bool
	IsCONNECT() bool
	IsOPTIONS() bool
	IsTRACE() bool
	Method() string // GET, POST, ...
	UnsafeMethod() []byte

	Authority() string       // hostname[:port]
	UnsafeAuthority() []byte // hostname[:port]
	Hostname() string        // hostname
	UnsafeHostname() []byte  // hostname
	Colonport() string       // :port
	UnsafeColonport() []byte // :port

	URI() string               // /encodedPath?queryString
	UnsafeURI() []byte         // /encodedPath?queryString
	Path() string              // /decodedPath
	UnsafePath() []byte        // /decodedPath
	EncodedPath() string       // /encodedPath
	UnsafeEncodedPath() []byte // /encodedPath
	QueryString() string       // including '?' if query string exists, otherwise empty
	UnsafeQueryString() []byte // including '?' if query string exists, otherwise empty

	HasQueries() bool
	AllQueries() (queries [][2]string)
	Q(name string) string
	Qstr(name string, defaultValue string) string
	Qint(name string, defaultValue int) int
	Query(name string) (value string, ok bool)
	UnsafeQuery(name string) (value []byte, ok bool)
	Queries(name string) (values []string, ok bool)
	HasQuery(name string) bool
	DelQuery(name string) (deleted bool)
	AddQuery(name string, value string) bool

	HasHeaders() bool
	AllHeaders() (headers [][2]string)
	H(name string) string
	Hstr(name string, defaultValue string) string
	Hint(name string, defaultValue int) int
	Header(name string) (value string, ok bool)
	UnsafeHeader(name string) (value []byte, ok bool)
	Headers(name string) (values []string, ok bool)
	HasHeader(name string) bool
	DelHeader(name string) (deleted bool)
	AddHeader(name string, value string) bool

	UserAgent() string
	UnsafeUserAgent() []byte

	ContentType() string
	UnsafeContentType() []byte

	ContentSize() int64
	UnsafeContentLength() []byte

	AcceptTrailers() bool

	EvalPreconditions(date int64, etag []byte, asOrigin bool) (status int16, normal bool)

	HasIfRange() bool
	EvalIfRange(date int64, etag []byte, asOrigin bool) (canRange bool)

	HasRanges() bool
	EvalRanges(size int64) []Range

	HasCookies() bool
	AllCookies() (cookies [][2]string)
	C(name string) string
	Cstr(name string, defaultValue string) string
	Cint(name string, defaultValue int) int
	Cookie(name string) (value string, ok bool)
	UnsafeCookie(name string) (value []byte, ok bool)
	Cookies(name string) (values []string, ok bool)
	HasCookie(name string) bool
	DelCookie(name string) (deleted bool)
	AddCookie(name string, value string) bool

	SetRecvTimeout(timeout time.Duration) // to defend against slowloris attack
	HasContent() bool                     // true if content exists
	IsVague() bool                        // true if content exists and is not sized
	Content() string
	UnsafeContent() []byte

	HasForms() bool
	AllForms() (forms [][2]string)
	F(name string) string
	Fstr(name string, defaultValue string) string
	Fint(name string, defaultValue int) int
	Form(name string) (value string, ok bool)
	UnsafeForm(name string) (value []byte, ok bool)
	Forms(name string) (values []string, ok bool)
	HasForm(name string) bool
	AddForm(name string, value string) bool

	HasUpfiles() bool
	AllUpfiles() (upfiles []*Upfile)
	U(name string) *Upfile
	Upfile(name string) (upfile *Upfile, ok bool)
	Upfiles(name string) (upfiles []*Upfile, ok bool)
	HasUpfile(name string) bool

	HasTrailers() bool
	AllTrailers() (trailers [][2]string)
	T(name string) string
	Tstr(name string, defaultValue string) string
	Tint(name string, defaultValue int) int
	Trailer(name string) (value string, ok bool)
	UnsafeTrailer(name string) (value []byte, ok bool)
	Trailers(name string) (values []string, ok bool)
	HasTrailer(name string) bool
	DelTrailer(name string) (deleted bool)
	AddTrailer(name string, value string) bool

	UnsafeMake(size int) []byte

	// Internal only
	getPathInfo() os.FileInfo
	unsafeAbsPath() []byte
	makeAbsPath()
	proxyDelHopHeaders()
	proxyDelHopTrailers()
	proxyWalkHeaders(callback func(header *pair, name []byte, value []byte) bool) bool
	proxyWalkTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool
	proxyWalkCookies(callback func(cookie *pair, name []byte, value []byte) bool) bool
	proxyUnsetHost()
	proxyTakeContent() any
	readContent() (data []byte, err error)
	examineTail() bool
	hookReviser(reviser Reviser)
	unsafeVariable(code int16, name string) (value []byte)
}

// serverRequest_ is the parent for server[1-3]Request.
type serverRequest_ struct { // incoming. needs parsing
	// Parent
	httpIn_ // incoming http request
	// Stream states (stocks)
	stockUpfiles [2]Upfile // for r.upfiles. 96B
	// Stream states (controlled)
	ranges [4]Range // parsed range fields. at most 4 range fields are allowed. controlled by r.numRanges
	// Stream states (non-zeros)
	upfiles []Upfile // decoded upfiles -> r.array (for metadata) and temp files in local file system. [<r.stockUpfiles>/(make=16/128)]
	// Stream states (zeros)
	webapp          *Webapp     // target webapp of this request. set before executing the stream
	path            []byte      // decoded path. only a reference. refers to r.array or region if rewrited, so can't be a span
	absPath         []byte      // webapp.webRoot + r.UnsafePath(). if webapp.webRoot is not set then this is nil. set when dispatching to handlets. only a reference
	pathInfo        os.FileInfo // cached result of os.Stat(r.absPath) if r.absPath is not nil
	formWindow      []byte      // a window used for reading and parsing content as multipart/form-data. [<none>/r.contentText/4K/16K]
	_serverRequest0             // all values in this struct must be zero by default!
}
type _serverRequest0 struct { // for fast reset, entirely
	gotInput        bool     // got some input from client? for request timeout handling
	targetForm      int8     // request-target form. see httpTargetXXX
	asteriskOptions bool     // true if method and uri is: OPTIONS *
	schemeCode      uint8    // SchemeHTTP, SchemeHTTPS
	methodCode      uint32   // known method code. 0: unknown method
	method          span     // raw method -> r.input
	authority       span     // raw hostname[:port] -> r.input
	hostname        span     // raw hostname (without :port) -> r.input
	colonport       span     // raw colon port (:port, with ':') -> r.input
	uri             span     // raw uri (raw path & raw query string) -> r.input
	encodedPath     span     // raw path -> r.input
	queryString     span     // raw query string (with '?') -> r.input
	boundary        span     // boundary parameter of "multipart/form-data" if exists -> r.input
	queries         zone     // decoded queries -> r.array
	cookies         zone     // cookies ->r.input. temporarily used when checking cookie headers, set after cookie header is parsed
	forms           zone     // decoded forms -> r.array
	ifMatch         int8     // -1: if-match *, 0: no if-match field, >0: number of if-match: 1#entity-tag
	ifNoneMatch     int8     // -1: if-none-match *, 0: no if-none-match field, >0: number of if-none-match: 1#entity-tag
	numRanges       int8     // num of ranges. controls r.ranges
	maxForwards     int8     // parsed value of "Max-Forwards" header, must <= 127
	expectContinue  bool     // expect: 100-continue?
	acceptTrailers  bool     // does client accept trailers? i.e. te: trailers
	pathInfoGot     bool     // is r.pathInfo got?
	_               [3]byte  // padding
	indexes         struct { // indexes of some selected singleton headers, for fast accessing
		authorization      uint8 // authorization header ->r.input
		host               uint8 // host header ->r.input
		ifModifiedSince    uint8 // if-modified-since header ->r.input
		ifRange            uint8 // if-range header ->r.input
		ifUnmodifiedSince  uint8 // if-unmodified-since header ->r.input
		maxForwards        uint8 // max-forwards header ->r.input
		proxyAuthorization uint8 // proxy-authorization header ->r.input
		userAgent          uint8 // user-agent header ->r.input
	}
	zones struct { // zones (may not be continuous) of some selected headers, for fast accessing
		acceptLanguage zone
		expect         zone
		forwarded      zone
		ifMatch        zone // the zone of if-match in r.primes
		ifNoneMatch    zone // the zone of if-none-match in r.primes
		xForwardedFor  zone
		_              [4]byte // padding
	}
	unixTimes struct { // parsed unix times in seconds
		ifModifiedSince   int64 // parsed unix time of if-modified-since
		ifRange           int64 // parsed unix time of if-range if is http-date format
		ifUnmodifiedSince int64 // parsed unix time of if-unmodified-since
	}
	cacheControl struct { // the cache-control info
		noCache      bool  // no-cache directive in cache-control
		noStore      bool  // no-store directive in cache-control
		noTransform  bool  // no-transform directive in cache-control
		onlyIfCached bool  // only-if-cached directive in cache-control
		maxAge       int32 // max-age directive in cache-control
		maxStale     int32 // max-stale directive in cache-control
		minFresh     int32 // min-fresh directive in cache-control
	}
	revisers     [32]uint8 // reviser ids which will apply on this request. indexed by reviser order
	_            [2]byte   // padding
	formReceived bool      // if content is a form, is it received?
	formKind     int8      // deducted type of form. 0:not form. see formXXX
	formEdge     int32     // edge position of the filled content in r.formWindow
	pFieldName   span      // field name. used during receiving and parsing multipart form in case of sliding r.formWindow
	consumedSize int64     // bytes of consumed content when consuming received tempFile. used by, for example, _recvMultipartForm.
}

func (r *serverRequest_) onUse(httpVersion uint8) { // for non-zeros
	const asResponse = false
	r.httpIn_.onUse(httpVersion, asResponse)

	r.upfiles = r.stockUpfiles[0:0:cap(r.stockUpfiles)] // use append()
}
func (r *serverRequest_) onEnd() { // for zeros
	for _, upfile := range r.upfiles {
		if upfile.isMoved() { // file was moved, don't remove it
			continue
		}
		var filePath string
		if upfile.metaSet() {
			filePath = upfile.Path()
		} else {
			filePath = WeakString(r.array[upfile.pathFrom : upfile.pathFrom+int32(upfile.pathSize)])
		}
		if err := os.Remove(filePath); err != nil {
			r.webapp.Logf("failed to remove uploaded file: %s, error: %s\n", filePath, err.Error())
		}
	}
	r.upfiles = nil

	r.webapp = nil
	r.path = nil
	r.absPath = nil
	r.pathInfo = nil
	r.formWindow = nil // if r.formWindow is fetched from pool, it's put into pool on return. so just set as nil
	r._serverRequest0 = _serverRequest0{}

	r.httpIn_.onEnd()
}

func (r *serverRequest_) Webapp() *Webapp { return r.webapp }

func (r *serverRequest_) IsAbsoluteForm() bool    { return r.targetForm == httpTargetAbsolute }
func (r *serverRequest_) IsAsteriskOptions() bool { return r.asteriskOptions }

func (r *serverRequest_) SchemeCode() uint8    { return r.schemeCode }
func (r *serverRequest_) IsHTTP() bool         { return r.schemeCode == SchemeHTTP }
func (r *serverRequest_) IsHTTPS() bool        { return r.schemeCode == SchemeHTTPS }
func (r *serverRequest_) Scheme() string       { return httpSchemeStrings[r.schemeCode] }
func (r *serverRequest_) UnsafeScheme() []byte { return httpSchemeByteses[r.schemeCode] }

func (r *serverRequest_) IsGET() bool          { return r.methodCode == MethodGET }
func (r *serverRequest_) IsHEAD() bool         { return r.methodCode == MethodHEAD }
func (r *serverRequest_) IsPOST() bool         { return r.methodCode == MethodPOST }
func (r *serverRequest_) IsPUT() bool          { return r.methodCode == MethodPUT }
func (r *serverRequest_) IsDELETE() bool       { return r.methodCode == MethodDELETE }
func (r *serverRequest_) IsCONNECT() bool      { return r.methodCode == MethodCONNECT }
func (r *serverRequest_) IsOPTIONS() bool      { return r.methodCode == MethodOPTIONS }
func (r *serverRequest_) IsTRACE() bool        { return r.methodCode == MethodTRACE }
func (r *serverRequest_) Method() string       { return string(r.UnsafeMethod()) }
func (r *serverRequest_) UnsafeMethod() []byte { return r.input[r.method.from:r.method.edge] }
func (r *serverRequest_) recognizeMethod(method []byte, methodHash uint16) {
	if m := serverMethodTable[serverMethodFind(methodHash)]; m.hash == methodHash && bytes.Equal(serverMethodBytes[m.from:m.edge], method) {
		r.methodCode = m.code
	}
}

var ( // perfect hash table for best known http methods
	serverMethodBytes = []byte("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE")
	serverMethodTable = [8]struct {
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
	serverMethodFind = func(methodHash uint16) int { return (2610 / int(methodHash)) % len(serverMethodTable) }
)

func (r *serverRequest_) Authority() string { return string(r.UnsafeAuthority()) }
func (r *serverRequest_) UnsafeAuthority() []byte {
	return r.input[r.authority.from:r.authority.edge]
}
func (r *serverRequest_) Hostname() string       { return string(r.UnsafeHostname()) }
func (r *serverRequest_) UnsafeHostname() []byte { return r.input[r.hostname.from:r.hostname.edge] }
func (r *serverRequest_) Colonport() string {
	if r.colonport.notEmpty() {
		return string(r.input[r.colonport.from:r.colonport.edge])
	}
	if r.schemeCode == SchemeHTTPS {
		return stringColonport443
	} else {
		return stringColonport80
	}
}
func (r *serverRequest_) UnsafeColonport() []byte {
	if r.colonport.notEmpty() {
		return r.input[r.colonport.from:r.colonport.edge]
	}
	if r.schemeCode == SchemeHTTPS {
		return bytesColonport443
	} else {
		return bytesColonport80
	}
}

func (r *serverRequest_) URI() string {
	if r.uri.notEmpty() {
		return string(r.input[r.uri.from:r.uri.edge])
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) UnsafeURI() []byte {
	if r.uri.notEmpty() {
		return r.input[r.uri.from:r.uri.edge]
	} else { // use "/"
		return bytesSlash
	}
}
func (r *serverRequest_) EncodedPath() string {
	if r.encodedPath.notEmpty() {
		return string(r.input[r.encodedPath.from:r.encodedPath.edge])
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) UnsafeEncodedPath() []byte {
	if r.encodedPath.notEmpty() {
		return r.input[r.encodedPath.from:r.encodedPath.edge]
	} else { // use "/"
		return bytesSlash
	}
}
func (r *serverRequest_) Path() string {
	if len(r.path) != 0 {
		return string(r.path)
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) UnsafePath() []byte {
	if len(r.path) != 0 {
		return r.path
	} else { // use "/"
		return bytesSlash
	}
}
func (r *serverRequest_) cleanPath() {
	nPath := len(r.path)
	if nPath <= 1 {
		// Must be '/'.
		return
	}
	slashed := r.path[nPath-1] == '/'
	pOrig, pReal := 1, 1
	for pOrig < nPath {
		if b := r.path[pOrig]; b == '/' {
			pOrig++
		} else if b == '.' && (pOrig+1 == nPath || r.path[pOrig+1] == '/') {
			pOrig++
		} else if b == '.' && r.path[pOrig+1] == '.' && (pOrig+2 == nPath || r.path[pOrig+2] == '/') {
			pOrig += 2
			if pReal > 1 {
				pReal--
				for pReal > 1 && r.path[pReal] != '/' {
					pReal--
				}
			}
		} else {
			if pReal != 1 {
				r.path[pReal] = '/'
				pReal++
			}
			for pOrig < nPath && r.path[pOrig] != '/' {
				r.path[pReal] = r.path[pOrig]
				pReal++
				pOrig++
			}
		}
	}
	if pReal != nPath {
		if slashed && pReal > 1 {
			r.path[pReal] = '/'
			pReal++
		}
		r.path = r.path[:pReal]
	}
}
func (r *serverRequest_) unsafeAbsPath() []byte { return r.absPath }
func (r *serverRequest_) makeAbsPath() {
	if r.webapp.webRoot == "" { // if webapp's webRoot is empty, r.absPath is not used either. so it's safe to do nothing
		return
	}
	webRoot := r.webapp.webRoot
	r.absPath = r.UnsafeMake(len(webRoot) + len(r.UnsafePath()))
	n := copy(r.absPath, webRoot)
	copy(r.absPath[n:], r.UnsafePath())
}
func (r *serverRequest_) getPathInfo() os.FileInfo {
	if !r.pathInfoGot {
		r.pathInfoGot = true
		if pathInfo, err := os.Stat(WeakString(r.absPath)); err == nil {
			r.pathInfo = pathInfo
		}
	}
	return r.pathInfo
}
func (r *serverRequest_) QueryString() string { return string(r.UnsafeQueryString()) }
func (r *serverRequest_) UnsafeQueryString() []byte {
	return r.input[r.queryString.from:r.queryString.edge]
}

func (r *serverRequest_) addQuery(query *pair) bool { // as prime
	if edge, ok := r._addPrime(query); ok {
		r.queries.edge = edge
		return true
	}
	r.headResult, r.failReason = StatusURITooLong, "too many queries"
	return false
}
func (r *serverRequest_) HasQueries() bool { return r.hasPairs(r.queries, pairQuery) }
func (r *serverRequest_) AllQueries() (queries [][2]string) {
	return r.allPairs(r.queries, pairQuery)
}
func (r *serverRequest_) Q(name string) string {
	value, _ := r.Query(name)
	return value
}
func (r *serverRequest_) Qstr(name string, defaultValue string) string {
	if value, ok := r.Query(name); ok {
		return value
	}
	return defaultValue
}
func (r *serverRequest_) Qint(name string, defaultValue int) int {
	if value, ok := r.Query(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *serverRequest_) Query(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.queries, pairQuery)
	return string(v), ok
}
func (r *serverRequest_) UnsafeQuery(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.queries, pairQuery)
}
func (r *serverRequest_) Queries(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.queries, pairQuery)
}
func (r *serverRequest_) HasQuery(name string) bool {
	_, ok := r.getPair(name, 0, r.queries, pairQuery)
	return ok
}
func (r *serverRequest_) DelQuery(name string) (deleted bool) {
	return r.delPair(name, 0, r.queries, pairQuery)
}
func (r *serverRequest_) AddQuery(name string, value string) bool { // as extra, by webapp
	return r.addExtra(name, value, 0, pairQuery)
}

func (r *serverRequest_) examineHead() bool {
	for i := r.headers.from; i < r.headers.edge; i++ {
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	if r.cookies.notEmpty() { // in HTTP/2 and HTTP/3, there can be multiple cookie fields.
		cookies := r.cookies // make a copy, as r.cookies will be changed as cookie pairs below
		r.cookies.from = uint8(len(r.primes))
		for i := cookies.from; i < cookies.edge; i++ {
			cookie := &r.primes[i]
			if cookie.nameHash != hashCookie || !cookie.nameEqualBytes(r.input, bytesCookie) { // cookies may not be consecutive
				continue
			}
			if !r.parseCookie(cookie.value) { // r.cookies.edge is set in r.addCookie().
				return false
			}
		}
	}
	if DebugLevel() >= 3 {
		Println("======primes======")
		for i := 0; i < len(r.primes); i++ {
			prime := &r.primes[i]
			prime.show(r._placeOf(prime))
		}
		Println("======extras======")
		for i := 0; i < len(r.extras); i++ {
			extra := &r.extras[i]
			extra.show(r._placeOf(extra))
		}
	}

	// RFC 9110 (section 5.3):
	// A server MUST NOT apply a request to the target resource until it receives the entire request header section,
	// since later header field lines might include conditionals, authentication credentials,
	// or deliberately misleading duplicate header fields that could impact request processing.

	// Basic checks against versions
	switch r.httpVersion {
	case Version1_0:
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 0 // default is close for HTTP/1.0
		}
	case Version1_1:
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
		if r.indexes.host == 0 {
			// RFC 9112 (section 3.2):
			// A server MUST respond with a 400 (Bad Request) status code to any HTTP/1.1 request message that lacks a Host header field.
			r.headResult, r.failReason = StatusBadRequest, "MUST send a Host header field in all HTTP/1.1 request messages"
			return false
		}
	default: // HTTP/2 and HTTP/3
		r.keepAlive = 1 // default is keep-alive for HTTP/2 and HTTP/3
		// TODO: Add other checks here
	}

	if !r.determineContentMode() {
		// r.headResult is set.
		return false
	}
	if r.contentSize > r.maxContentSize {
		r.headResult, r.failReason = StatusContentTooLarge, "content size exceeds server's limit"
		return false
	}

	if r.upgradeSocket {
		// RFC 6455 (section 4.1):
		// The method of the request MUST be GET, and the HTTP version MUST be at least 1.1.
		if !r.IsGET() || r.httpVersion == Version1_0 || r.contentSize != -1 {
			r.headResult, r.failReason = StatusMethodNotAllowed, "webSocket only supports GET method and HTTP version >= 1.1, without content"
			return false
		}
	}
	if r.methodCode&(MethodCONNECT|MethodOPTIONS|MethodTRACE) != 0 {
		// RFC 9110 (section 13.2.1):
		// Likewise, a server MUST ignore the conditional request header
		// fields defined by this specification when received with a request
		// method that does not involve the selection or modification of a
		// selected representation, such as CONNECT, OPTIONS, or TRACE.
		if r.ifMatch != 0 {
			r.delHeader(bytesIfMatch, hashIfMatch)
			r.ifMatch = 0
		}
		if r.ifNoneMatch != 0 {
			r.delHeader(bytesIfNoneMatch, hashIfNoneMatch)
			r.ifNoneMatch = 0
		}
		if r.indexes.ifModifiedSince != 0 {
			r._delPrime(r.indexes.ifModifiedSince)
			r.indexes.ifModifiedSince = 0
		}
		if r.indexes.ifUnmodifiedSince != 0 {
			r._delPrime(r.indexes.ifUnmodifiedSince)
			r.indexes.ifUnmodifiedSince = 0
		}
		if r.indexes.ifRange != 0 {
			r._delPrime(r.indexes.ifRange)
			r.indexes.ifRange = 0
		}
	} else {
		// RFC 9110 (section 13.1.3):
		// A recipient MUST ignore the If-Modified-Since header field if the
		// received field value is not a valid HTTP-date, the field value has
		// more than one member, or if the request method is neither GET nor HEAD.
		if r.indexes.ifModifiedSince != 0 && r.methodCode&(MethodGET|MethodHEAD) == 0 {
			r._delPrime(r.indexes.ifModifiedSince) // we delete it.
			r.indexes.ifModifiedSince = 0
		}
		// A server MUST ignore an If-Range header field received in a request that does not contain a Range header field.
		if r.indexes.ifRange != 0 && r.numRanges == 0 {
			r._delPrime(r.indexes.ifRange) // we delete it.
			r.indexes.ifRange = 0
		}
	}
	if r.contentSize == -1 { // no content
		if r.expectContinue { // expect is used to send large content.
			r.headResult, r.failReason = StatusBadRequest, "cannot use expect header without content"
			return false
		}
		if r.methodCode&(MethodPOST|MethodPUT) != 0 {
			r.headResult, r.failReason = StatusLengthRequired, "POST and PUT must contain a content"
			return false
		}
	} else { // content exists (sized or vague)
		// Content is not allowed in some methods, according to RFC 9110.
		if r.methodCode&(MethodCONNECT|MethodTRACE) != 0 {
			r.headResult, r.failReason = StatusBadRequest, "content is not allowed in CONNECT and TRACE method"
			return false
		}
		if r.numContentCodings > 0 { // request has content-encoding
			if r.numContentCodings > 1 || r.contentCodings[0] != httpCodingGzip {
				r.headResult, r.failReason = StatusUnsupportedMediaType, "currently only gzip content coding is supported in request"
				return false
			}
		}
		if r.iContentType == 0 { // no content-type
			if r.IsOPTIONS() {
				// RFC 9110 (section 9.3.7):
				// A client that generates an OPTIONS request containing content MUST send
				// a valid Content-Type header field describing the representation media type.
				r.headResult, r.failReason = StatusBadRequest, "OPTIONS with content but without a content-type"
				return false
			}
		} else { // content-type exists
			header := &r.primes[r.iContentType]
			contentType := header.dataAt(r.input)
			bytesToLower(contentType)
			if bytes.Equal(contentType, bytesURLEncodedForm) {
				r.formKind = httpFormURLEncoded
			} else if bytes.Equal(contentType, bytesMultipartForm) { // multipart/form-data; boundary=xxxxxx
				for i := header.params.from; i < header.params.edge; i++ {
					param := &r.extras[i]
					if param.nameHash != hashBoundary || !param.nameEqualBytes(r.input, bytesBoundary) {
						continue
					}
					if boundary := param.value; boundary.notEmpty() && boundary.size() <= 70 && r.input[boundary.edge-1] != ' ' {
						// boundary := 0*69<bchars> bcharsnospace
						// bchars := bcharsnospace / " "
						// bcharsnospace := DIGIT / ALPHA / "'" / "(" / ")" / "+" / "_" / "," / "-" / "." / "/" / ":" / "=" / "?"
						r.boundary = boundary
						r.formKind = httpFormMultipart
						break
					}
				}
				if r.formKind != httpFormMultipart {
					r.headResult, r.failReason = StatusBadRequest, "bad boundary"
					return false
				}
			}
			if r.formKind != httpFormNotForm && r.numContentCodings > 0 {
				r.headResult, r.failReason = StatusUnsupportedMediaType, "a form with content coding is not supported yet"
				return false
			}
		}
	}

	return true
}
func (r *serverRequest_) applyHeader(index uint8) bool {
	header := &r.primes[index]
	name := header.nameAt(r.input)
	if sh := &serverRequestSingletonHeaderTable[serverRequestSingletonHeaderFind(header.nameHash)]; sh.nameHash == header.nameHash && bytes.Equal(sh.name, name) {
		header.setSingleton()
		if !sh.parse { // unnecessary to parse generally
			header.setParsed()
			header.dataEdge = header.value.edge
		} else if !r._parseField(header, &sh.fdesc, r.input, true) { // fully
			r.headResult = StatusBadRequest
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &serverRequestImportantHeaderTable[serverRequestImportantHeaderFind(header.nameHash)]; mh.nameHash == header.nameHash && bytes.Equal(mh.name, name) {
		extraFrom := uint8(len(r.extras))
		if !r._splitField(header, &mh.fdesc, r.input) {
			r.headResult = StatusBadRequest
			return false
		}
		if header.isCommaValue() { // has sub headers, check them
			if extraEdge := uint8(len(r.extras)); !mh.check(r, r.extras, extraFrom, extraEdge) {
				// r.headResult is set.
				return false
			}
		} else if !mh.check(r, r.primes, index, index+1) { // no sub headers. check it
			// r.headResult is set.
			return false
		}
	} else {
		// All other headers are treated as list-based headers.
	}
	return true
}

var ( // perfect hash table for singleton request headers
	serverRequestSingletonHeaderTable = [13]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, *pair, uint8) bool
	}{ // authorization content-length content-type cookie date host if-modified-since if-range if-unmodified-since max-forwards proxy-authorization range user-agent
		0:  {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*serverRequest_).checkContentLength},
		1:  {false, fdesc{hashIfUnmodifiedSince, false, false, false, false, bytesIfUnmodifiedSince}, (*serverRequest_).checkIfUnmodifiedSince},
		2:  {false, fdesc{hashMaxForwards, false, false, false, false, bytesMaxForwards}, (*serverRequest_).checkMaxForwards},
		3:  {false, fdesc{hashUserAgent, false, false, false, true, bytesUserAgent}, (*serverRequest_).checkUserAgent},
		4:  {false, fdesc{hashIfRange, false, false, false, false, bytesIfRange}, (*serverRequest_).checkIfRange},
		5:  {false, fdesc{hashCookie, false, false, false, false, bytesCookie}, (*serverRequest_).checkCookie}, // `a=b; c=d; e=f` is cookie list, not parameters
		6:  {false, fdesc{hashProxyAuthorization, false, false, false, false, bytesProxyAuthorization}, (*serverRequest_).checkProxyAuthorization},
		7:  {false, fdesc{hashIfModifiedSince, false, false, false, false, bytesIfModifiedSince}, (*serverRequest_).checkIfModifiedSince},
		8:  {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*serverRequest_).checkContentType},
		9:  {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*serverRequest_).checkDate},
		10: {false, fdesc{hashAuthorization, false, false, false, false, bytesAuthorization}, (*serverRequest_).checkAuthorization},
		11: {false, fdesc{hashRange, false, false, false, false, bytesRange}, (*serverRequest_).checkRange},
		12: {false, fdesc{hashHost, false, false, false, false, bytesHost}, (*serverRequest_).checkHost},
	}
	serverRequestSingletonHeaderFind = func(nameHash uint16) int { return (811410 / int(nameHash)) % len(serverRequestSingletonHeaderTable) }
)

func (r *serverRequest_) checkAuthorization(header *pair, index uint8) bool { // Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	// TODO
	if r.indexes.authorization != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated authorization header"
		return false
	}
	r.indexes.authorization = index
	return true
}
func (r *serverRequest_) checkCookie(header *pair, index uint8) bool { // Cookie = cookie-string
	if header.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty cookie"
		return false
	}
	if index == 255 {
		r.headResult, r.failReason = StatusBadRequest, "too many pairs"
		return false
	}
	// HTTP/2 and HTTP/3 allows multiple cookie headers, so we have to mark all the cookie headers.
	if r.cookies.isEmpty() {
		r.cookies.from = index
	}
	// And we can't inject cookies into headers zone while receiving headers, this will break the continuous nature of headers zone.
	r.cookies.edge = index + 1 // so we postpone cookie parsing after the request head is entirely received. only mark the edge
	return true
}
func (r *serverRequest_) checkHost(header *pair, index uint8) bool { // Host = host [ ":" port ]
	// RFC 9112 (section 3.2):
	// A server MUST respond with a 400 (Bad Request) status code to any HTTP/1.1 request message that lacks a Host header field and
	// to any request message that contains more than one Host header field line or a Host header field with an invalid field value.
	if r.indexes.host != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicate host header"
		return false
	}
	host := header.value
	if host.notEmpty() {
		// RFC 9110 (section 4.2.3):
		// The scheme and host are case-insensitive and normally provided in lowercase;
		// all other components are compared in a case-sensitive manner.
		bytesToLower(r.input[host.from:host.edge])
		if !r.parseAuthority(host.from, host.edge, r.authority.isEmpty()) {
			r.headResult, r.failReason = StatusBadRequest, "bad host value"
			return false
		}
	}
	r.indexes.host = index
	return true
}
func (r *serverRequest_) checkIfModifiedSince(header *pair, index uint8) bool { // If-Modified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifModifiedSince, &r.unixTimes.ifModifiedSince)
}
func (r *serverRequest_) checkIfRange(header *pair, index uint8) bool { // If-Range = entity-tag / HTTP-date
	if r.indexes.ifRange != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated if-range header"
		return false
	}
	if date, ok := clockParseHTTPDate(header.valueAt(r.input)); ok {
		r.unixTimes.ifRange = date
	}
	r.indexes.ifRange = index
	return true
}
func (r *serverRequest_) checkIfUnmodifiedSince(header *pair, index uint8) bool { // If-Unmodified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifUnmodifiedSince, &r.unixTimes.ifUnmodifiedSince)
}
func (r *serverRequest_) checkMaxForwards(header *pair, index uint8) bool { // Max-Forwards = Max-Forwards = 1*DIGIT
	if r.indexes.maxForwards != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated max-forwards header"
		return false
	}
	// TODO: parse header.valueAt(r.input) as 1*DIGIT into r.maxForwards
	r.indexes.maxForwards = index
	return true
}
func (r *serverRequest_) checkProxyAuthorization(header *pair, index uint8) bool { // Proxy-Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	if r.indexes.proxyAuthorization != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated proxyAuthorization header"
		return false
	}
	// TODO: check
	r.indexes.proxyAuthorization = index
	return true
}
func (r *serverRequest_) checkRange(header *pair, index uint8) bool { // Range = ranges-specifier
	if !r.IsGET() {
		// A server MUST ignore a Range header field received with a request method that is unrecognized or for which range handling is not defined.
		// For this specification, GET is the only method for which range handling is defined.
		r._delPrime(index)
		return true
	}
	if r.numRanges > 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated range header"
		return false
	}
	// Range        = range-unit "=" range-set
	// range-set    = 1#range-spec
	// range-spec   = int-range / suffix-range
	// int-range    = first-pos "-" [ last-pos ]
	// suffix-range = "-" suffix-length
	rangeSet := header.valueAt(r.input)
	nPrefix := len(bytesBytesEqual) // bytes=
	if !bytes.Equal(rangeSet[0:nPrefix], bytesBytesEqual) {
		r.headResult, r.failReason = StatusBadRequest, "unsupported range unit"
		return false
	}
	rangeSet = rangeSet[nPrefix:]
	if len(rangeSet) == 0 {
		r.headResult, r.failReason = StatusBadRequest, "empty range-set"
		return false
	}
	var rang Range // [from-last], inclusive, begins from 0
	state := 0
	for i, n := 0, len(rangeSet); i < n; i++ {
		b := rangeSet[i]
		switch state {
		case 0: // select int-range or suffix-range
			if b >= '0' && b <= '9' {
				rang.From = int64(b - '0')
				state = 1 // int-range
			} else if b == '-' {
				rang.From = -1
				rang.Last = 0
				state = 4 // suffix-range
			} else if b != ',' && b != ' ' {
				goto badRange
			}
		case 1: // in first-pos = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					rang.From = rang.From*10 + int64(b-'0')
					if rang.From < 0 { // overflow
						goto badRange
					}
				} else if b == '-' {
					state = 2 // select last-pos or not
					break
				} else {
					goto badRange
				}
			}
		case 2: // select last-pos or not
			if b >= '0' && b <= '9' { // last-pos
				rang.Last = int64(b - '0')
				state = 3 // first-pos "-" last-pos
			} else if b == ',' || b == ' ' { // got: first-pos "-"
				rang.Last = -1
				if !r._addRange(rang) {
					return false
				}
				state = 0 // select int-range or suffix-range
			} else {
				goto badRange
			}
		case 3: // in last-pos = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					rang.Last = rang.Last*10 + int64(b-'0')
					if rang.Last < 0 { // overflow
						goto badRange
					}
				} else if b == ',' || b == ' ' { // got: first-pos "-" last-pos
					// An int-range is invalid if the last-pos value is present and less than the first-pos.
					if rang.From > rang.Last {
						goto badRange
					}
					if !r._addRange(rang) {
						return false
					}
					state = 0 // select int-range or suffix-range
					break
				} else {
					goto badRange
				}
			}
		case 4: // in suffix-length = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					rang.Last = rang.Last*10 + int64(b-'0')
					if rang.Last < 0 { // overflow
						goto badRange
					}
				} else if b == ',' || b == ' ' { // got: "-" suffix-length
					if !r._addRange(rang) {
						return false
					}
					state = 0 // select int-range or suffix-range
					break
				} else {
					goto badRange
				}
			}
		}
	}
	if state == 1 || state == 4 && rangeSet[len(rangeSet)-1] == '-' {
		goto badRange
	}
	if state == 2 {
		rang.Last = -1
	}
	if (state == 2 || state == 3 || state == 4) && !r._addRange(rang) {
		return false
	}
	return true
badRange:
	r.headResult, r.failReason = StatusBadRequest, "invalid range"
	return false
}
func (r *serverRequest_) _addRange(rang Range) bool {
	if r.numRanges == int8(cap(r.ranges)) {
		r.headResult, r.failReason = StatusBadRequest, "too many ranges"
		return false
	}
	r.ranges[r.numRanges] = rang
	r.numRanges++
	return true
}
func (r *serverRequest_) checkUserAgent(header *pair, index uint8) bool { // User-Agent = product *( RWS ( product / comment ) )
	if r.indexes.userAgent != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated user-agent header"
		return false
	}
	r.indexes.userAgent = index
	return true
}

var ( // perfect hash table for important request headers
	serverRequestImportantHeaderTable = [16]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-language cache-control connection content-encoding content-language expect forwarded if-match if-none-match te trailer transfer-encoding upgrade via x-forwarded-for
		0:  {fdesc{hashIfMatch, true, false, false, false, bytesIfMatch}, (*serverRequest_).checkIfMatch},
		1:  {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*serverRequest_).checkContentLanguage},
		2:  {fdesc{hashVia, false, false, false, true, bytesVia}, (*serverRequest_).checkVia},
		3:  {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*serverRequest_).checkTransferEncoding}, // deliberately false
		4:  {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*serverRequest_).checkCacheControl},
		5:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*serverRequest_).checkConnection},
		6:  {fdesc{hashForwarded, false, false, false, false, bytesForwarded}, (*serverRequest_).checkForwarded}, // `for=192.0.2.60;proto=http;by=203.0.113.43` is not parameters
		7:  {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*serverRequest_).checkUpgrade},
		8:  {fdesc{hashXForwardedFor, false, false, false, false, bytesXForwardedFor}, (*serverRequest_).checkXForwardedFor},
		9:  {fdesc{hashExpect, false, false, true, false, bytesExpect}, (*serverRequest_).checkExpect},
		10: {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*serverRequest_).checkAcceptEncoding},
		11: {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*serverRequest_).checkContentEncoding},
		12: {fdesc{hashAcceptLanguage, false, false, true, false, bytesAcceptLanguage}, (*serverRequest_).checkAcceptLanguage},
		13: {fdesc{hashIfNoneMatch, true, false, false, false, bytesIfNoneMatch}, (*serverRequest_).checkIfNoneMatch},
		14: {fdesc{hashTE, false, false, true, false, bytesTE}, (*serverRequest_).checkTE},
		15: {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*serverRequest_).checkTrailer},
	}
	serverRequestImportantHeaderFind = func(nameHash uint16) int { return (49454765 / int(nameHash)) % len(serverRequestImportantHeaderTable) }
)

func (r *serverRequest_) checkAcceptLanguage(pairs []pair, from uint8, edge uint8) bool { // Accept-Language = #( language-range [ weight ] )
	// language-range = <language-range, see [RFC4647], Section 2.1>
	// weight = OWS ";" OWS "q=" qvalue
	// qvalue = ( "0" [ "." *3DIGIT ] ) / ( "1" [ "." *3"0" ] )
	if r.zones.acceptLanguage.isEmpty() {
		r.zones.acceptLanguage.from = from
	}
	r.zones.acceptLanguage.edge = edge
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *serverRequest_) checkCacheControl(pairs []pair, from uint8, edge uint8) bool { // Cache-Control = #cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *serverRequest_) checkExpect(pairs []pair, from uint8, edge uint8) bool { // Expect = #expectation
	// expectation = token [ "=" ( token / quoted-string ) parameters ]
	if r.httpVersion >= Version1_1 {
		if r.zones.expect.isEmpty() {
			r.zones.expect.from = from
		}
		r.zones.expect.edge = edge
		for i := from; i < edge; i++ {
			pair := &pairs[i]
			if pair.kind != pairHeader {
				continue
			}
			data := pair.dataAt(r.input)
			bytesToLower(data) // the Expect field-value is case-insensitive.
			if bytes.Equal(data, bytes100Continue) {
				r.expectContinue = true
			} else {
				// Unknown expectation, ignored.
			}
		}
	} else { // HTTP/1.0
		// RFC 9110 (section 10.1.1):
		// A server that receives a 100-continue expectation in an HTTP/1.0 request MUST ignore that expectation.
		for i := from; i < edge; i++ {
			pairs[i].zero() // since HTTP/1.0 doesn't support 1xx status codes, we delete the expect.
		}
	}
	return true
}
func (r *serverRequest_) checkForwarded(pairs []pair, from uint8, edge uint8) bool { // Forwarded = 1#forwarded-element
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "forwarded = 1#forwarded-element"
		return false
	}
	// forwarded-element = [ forwarded-pair ] *( ";" [ forwarded-pair ] )
	// forwarded-pair    = token "=" value
	// value             = token / quoted-string
	if r.zones.forwarded.isEmpty() {
		r.zones.forwarded.from = from
	}
	r.zones.forwarded.edge = edge
	return true
}
func (r *serverRequest_) checkIfMatch(pairs []pair, from uint8, edge uint8) bool { // If-Match = "*" / #entity-tag
	return r._checkMatch(pairs, from, edge, &r.zones.ifMatch, &r.ifMatch)
}
func (r *serverRequest_) checkIfNoneMatch(pairs []pair, from uint8, edge uint8) bool { // If-None-Match = "*" / #entity-tag
	return r._checkMatch(pairs, from, edge, &r.zones.ifNoneMatch, &r.ifNoneMatch)
}
func (r *serverRequest_) _checkMatch(pairs []pair, from uint8, edge uint8, zMatch *zone, match *int8) bool {
	if zMatch.isEmpty() {
		zMatch.from = from
	}
	zMatch.edge = edge
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		nMatch := *match // -1:*, 0:nonexist, >0:num
		if len(data) == 1 && data[0] == '*' {
			if nMatch != 0 {
				r.headResult, r.failReason = StatusBadRequest, "mix using of * and entity-tag"
				return false
			}
			*match = -1 // *
		} else { // entity-tag = [ weak ] DQUOTE *etagc DQUOTE
			if nMatch == -1 { // *
				r.headResult, r.failReason = StatusBadRequest, "mix using of entity-tag and *"
				return false
			}
			if nMatch > 16 {
				r.headResult, r.failReason = StatusBadRequest, "too many entity-tag"
				return false
			}
			*match++ // *match is 0 by default
		}
	}
	return true
}
func (r *serverRequest_) checkTE(pairs []pair, from uint8, edge uint8) bool { // TE = #t-codings
	// t-codings = "trailers" / ( transfer-coding [ t-ranking ] )
	// t-ranking = OWS ";" OWS "q=" rank
	for i := from; i < edge; i++ {
		pair := &pairs[i]
		if pair.kind != pairHeader {
			continue
		}
		data := pair.dataAt(r.input)
		bytesToLower(data)
		if bytes.Equal(data, bytesTrailers) {
			r.acceptTrailers = true
		} else if r.httpVersion > Version1_1 {
			r.headResult, r.failReason = StatusBadRequest, "te codings other than trailers are not allowed in http/2 and http/3"
			return false
		}
	}
	return true
}
func (r *serverRequest_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	if r.httpVersion > Version1_1 {
		r.headResult, r.failReason = StatusBadRequest, "http upgrade is only supported in http/1.1"
		return false
	}
	if r.IsCONNECT() {
		// TODO: confirm this
		return true
	}
	if r.httpVersion == Version1_1 {
		// protocol         = protocol-name ["/" protocol-version]
		// protocol-name    = token
		// protocol-version = token
		for i := from; i < edge; i++ {
			data := pairs[i].dataAt(r.input)
			bytesToLower(data)
			if bytes.Equal(data, bytesWebSocket) {
				r.upgradeSocket = true
			} else {
				// Unknown protocol. Ignored. We don't support "Upgrade: h2c" either.
			}
		}
	} else { // HTTP/1.0
		// RFC 9110 (section 7.8):
		// A server that receives an Upgrade header field in an HTTP/1.0 request MUST ignore that Upgrade field.
		for i := from; i < edge; i++ {
			pairs[i].zero() // we delete it.
		}
	}
	return true
}
func (r *serverRequest_) checkXForwardedFor(pairs []pair, from uint8, edge uint8) bool { // X-Forwarded-For: <client>, <proxy1>, <proxy2>
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "empty x-forwarded-for"
		return false
	}
	if r.zones.xForwardedFor.isEmpty() {
		r.zones.xForwardedFor.from = from
	}
	r.zones.xForwardedFor.edge = edge
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}

func (r *serverRequest_) parseAuthority(from int32, edge int32, save bool) bool { // authority = host [ ":" port ]
	if save {
		r.authority.set(from, edge)
	}
	// host = IP-literal / IPv4address / reg-name
	// IP-literal = "[" ( IPv6address / IPvFuture  ) "]"
	// port = *DIGIT
	back, fore := from, from
	if r.input[back] == '[' { // IP-literal
		back++
		fore = back
		for fore < edge {
			if b := r.input[fore]; (b >= 'a' && b <= 'f') || (b >= '0' && b <= '9') || b == ':' {
				fore++
			} else if b == ']' {
				break
			} else {
				return false
			}
		}
		if fore == edge || fore-back == 1 { // "[]" is illegal
			return false
		}
		if save {
			r.hostname.set(back, fore)
		}
		fore++
		if fore == edge {
			return true
		}
		if r.input[fore] != ':' {
			return false
		}
	} else { // IPv4address or reg-name
		for fore < edge {
			if b := r.input[fore]; httpHchar[b] == 1 {
				fore++
			} else if b == ':' {
				break
			} else {
				return false
			}
		}
		if save {
			r.hostname.set(back, fore)
		}
		if fore == edge {
			return true
		}
	}
	// Now fore is at ':'. cases are: ":", ":88"
	back = fore
	fore++
	for fore < edge {
		if b := r.input[fore]; b >= '0' && b <= '9' {
			fore++
		} else {
			return false
		}
	}
	if n := fore - back; n > 6 { // max len(":65535") == 6
		return false
	} else if n > 1 && save { // ":" alone is ignored
		r.colonport.set(back, fore)
	}
	return true
}
func (r *serverRequest_) parseCookie(cookieString span) bool { // cookie-string = cookie-pair *( ";" SP cookie-pair )
	// cookie-pair = token "=" cookie-value
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	// exclude these: %x22=`"`  %2C=`,`  %3B=`;`  %5C=`\`
	cookie := &r.mainPair
	cookie.zero()
	cookie.kind = pairCookie
	cookie.place = placeInput // all received cookies are in r.input
	cookie.nameFrom = cookieString.from
	state := 0
	for p := cookieString.from; p < cookieString.edge; p++ {
		b := r.input[p]
		switch state {
		case 0: // expecting '=' to get cookie-name
			if b == '=' {
				if nameSize := p - cookie.nameFrom; nameSize > 0 && nameSize <= 255 {
					cookie.nameSize = uint8(nameSize)
					cookie.value.from = p + 1 // skip '='
				} else {
					r.headResult, r.failReason = StatusBadRequest, "cookie name out of range"
					return false
				}
				state = 1
			} else if httpTchar[b] != 0 {
				cookie.nameHash += uint16(b)
			} else {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie name"
				return false
			}
		case 1: // DQUOTE or not?
			if b == '"' {
				cookie.value.from++ // skip '"'
				state = 3
				continue
			}
			state = 2
			fallthrough
		case 2: // *cookie-octet, expecting ';'
			if b == ';' {
				cookie.value.edge = p
				if !r.addCookie(cookie) {
					return false
				}
				state = 5
			} else if b < 0x21 || b == '"' || b == ',' || b == '\\' || b > 0x7e {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie value"
				return false
			}
		case 3: // (DQUOTE *cookie-octet DQUOTE), expecting '"'
			if b == '"' {
				cookie.value.edge = p
				if !r.addCookie(cookie) {
					return false
				}
				state = 4
			} else if b < 0x20 || b == ';' || b == '\\' || b > 0x7e { // ` ` and `,` are allowed here!
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie value"
				return false
			}
		case 4: // expecting ';'
			if b != ';' {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie separator"
				return false
			}
			state = 5
		case 5: // expecting SP
			if b != ' ' {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie SP"
				return false
			}
			cookie.nameHash = 0     // reset for next cookie
			cookie.nameFrom = p + 1 // skip ' '
			state = 0
		}
	}
	if state == 2 { // ';' not found
		cookie.value.edge = cookieString.edge
		if !r.addCookie(cookie) {
			return false
		}
	} else if state == 4 { // ';' not found
		if !r.addCookie(cookie) {
			return false
		}
	} else { // 0, 1, 3, 5
		r.headResult, r.failReason = StatusBadRequest, "invalid cookie string"
		return false
	}
	return true
}

func (r *serverRequest_) AcceptTrailers() bool { return r.acceptTrailers }
func (r *serverRequest_) HasRanges() bool      { return r.numRanges > 0 }
func (r *serverRequest_) HasIfRange() bool     { return r.indexes.ifRange != 0 }
func (r *serverRequest_) UserAgent() string    { return string(r.UnsafeUserAgent()) }
func (r *serverRequest_) UnsafeUserAgent() []byte {
	if r.indexes.userAgent == 0 {
		return nil
	}
	return r.primes[r.indexes.userAgent].valueAt(r.input)
}

func (r *serverRequest_) addCookie(cookie *pair) bool { // as prime
	if edge, ok := r._addPrime(cookie); ok {
		r.cookies.edge = edge
		return true
	}
	r.headResult = StatusRequestHeaderFieldsTooLarge
	return false
}
func (r *serverRequest_) HasCookies() bool { return r.hasPairs(r.cookies, pairCookie) }
func (r *serverRequest_) AllCookies() (cookies [][2]string) {
	return r.allPairs(r.cookies, pairCookie)
}
func (r *serverRequest_) C(name string) string {
	value, _ := r.Cookie(name)
	return value
}
func (r *serverRequest_) Cstr(name string, defaultValue string) string {
	if value, ok := r.Cookie(name); ok {
		return value
	}
	return defaultValue
}
func (r *serverRequest_) Cint(name string, defaultValue int) int {
	if value, ok := r.Cookie(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *serverRequest_) Cookie(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.cookies, pairCookie)
	return string(v), ok
}
func (r *serverRequest_) UnsafeCookie(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.cookies, pairCookie)
}
func (r *serverRequest_) Cookies(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.cookies, pairCookie)
}
func (r *serverRequest_) HasCookie(name string) bool {
	_, ok := r.getPair(name, 0, r.cookies, pairCookie)
	return ok
}
func (r *serverRequest_) DelCookie(name string) (deleted bool) {
	return r.delPair(name, 0, r.cookies, pairCookie)
}
func (r *serverRequest_) AddCookie(name string, value string) bool { // as extra, by webapp
	return r.addExtra(name, value, 0, pairCookie)
}
func (r *serverRequest_) proxyWalkCookies(callback func(cookie *pair, name []byte, value []byte) bool) bool {
	for i := r.cookies.from; i < r.cookies.edge; i++ {
		if cookie := &r.primes[i]; cookie.nameHash != 0 {
			if !callback(cookie, cookie.nameAt(r.input), cookie.valueAt(r.input)) {
				return false
			}
		}
	}
	if r.hasExtra[pairCookie] {
		for i := 0; i < len(r.extras); i++ {
			if extra := &r.extras[i]; extra.nameHash != 0 && extra.kind == pairCookie {
				if !callback(extra, extra.nameAt(r.array), extra.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *serverRequest_) EvalPreconditions(date int64, etag []byte, asOrigin bool) (status int16, normal bool) { // to test against preconditons intentionally
	// Get effective etag without ""
	if n := len(etag); n >= 2 && etag[0] == '"' && etag[n-1] == '"' {
		etag = etag[1 : n-1]
	}
	// RFC 9110 (section 13.2.2):
	if asOrigin { // proxies may ignore if-match and if-unmodified-since.
		if r.ifMatch != 0 { // if-match is present
			if !r._evalIfMatch(etag) {
				return StatusPreconditionFailed, false
			}
		} else if r.indexes.ifUnmodifiedSince != 0 { // if-match is not present and if-unmodified-since is present
			if !r._evalIfUnmodifiedSince(date) {
				return StatusPreconditionFailed, false
			}
		}
	}
	getOrHead := r.methodCode&(MethodGET|MethodHEAD) != 0
	if r.ifNoneMatch != 0 { // if-none-match is present
		if !r._evalIfNoneMatch(etag) {
			if getOrHead {
				return StatusNotModified, false
			} else {
				return StatusPreconditionFailed, false
			}
		}
	} else if getOrHead && r.indexes.ifModifiedSince != 0 { // if-none-match is not present and if-modified-since is present
		if !r._evalIfModifiedSince(date) {
			return StatusNotModified, false
		}
	}
	return StatusOK, true
}
func (r *serverRequest_) _evalIfMatch(etag []byte) (pass bool) {
	if r.ifMatch == -1 { // *
		// If the field value is "*", the condition is true if the origin server has a current representation for the target resource.
		return true
	}
	for i := r.zones.ifMatch.from; i < r.zones.ifMatch.edge; i++ {
		header := &r.primes[i]
		if header.nameHash != hashIfMatch || !header.nameEqualBytes(r.input, bytesIfMatch) {
			continue
		}
		data := header.dataAt(r.input)
		if size := len(data); !(size >= 4 && data[0] == 'W' && data[1] == '/' && data[2] == '"' && data[size-1] == '"') && bytes.Equal(data, etag) {
			// If the field value is a list of entity tags, the condition is true if any of the listed tags match the entity tag of the selected representation.
			return true
		}
	}
	// TODO: r.extras?
	return false
}
func (r *serverRequest_) _evalIfNoneMatch(etag []byte) (pass bool) {
	if r.ifNoneMatch == -1 { // *
		// If the field value is "*", the condition is false if the origin server has a current representation for the target resource.
		return false
	}
	for i := r.zones.ifNoneMatch.from; i < r.zones.ifNoneMatch.edge; i++ {
		header := &r.primes[i]
		if header.nameHash != hashIfNoneMatch || !header.nameEqualBytes(r.input, bytesIfNoneMatch) {
			continue
		}
		if bytes.Equal(header.valueAt(r.input), etag) {
			// If the field value is a list of entity tags, the condition is false if one of the listed tags matches the entity tag of the selected representation.
			return false
		}
	}
	// TODO: r.extras?
	return true
}
func (r *serverRequest_) _evalIfModifiedSince(date int64) (pass bool) {
	// If the selected representation's last modification date is earlier than or equal to the date provided in the field value, the condition is false.
	return date > r.unixTimes.ifModifiedSince
}
func (r *serverRequest_) _evalIfUnmodifiedSince(date int64) (pass bool) {
	// If the selected representation's last modification date is earlier than or equal to the date provided in the field value, the condition is true.
	return date <= r.unixTimes.ifUnmodifiedSince
}

func (r *serverRequest_) EvalIfRange(date int64, etag []byte, asOrigin bool) (canRange bool) {
	if r.unixTimes.ifRange == 0 {
		if r._evalIfRangeETag(etag) {
			return true
		}
	} else if r._evalIfRangeDate(date) {
		return true
	}
	return false
}
func (r *serverRequest_) _evalIfRangeETag(etag []byte) (pass bool) {
	ifRange := &r.primes[r.indexes.ifRange] // TODO: r.extras?
	data := ifRange.dataAt(r.input)
	if size := len(data); !(size >= 4 && data[0] == 'W' && data[1] == '/' && data[2] == '"' && data[size-1] == '"') && bytes.Equal(data, etag) {
		// If the entity-tag validator provided exactly matches the ETag field value for the selected representation using the strong comparison function (Section 8.8.3.2), the condition is true.
		return true
	}
	return false
}
func (r *serverRequest_) _evalIfRangeDate(date int64) (pass bool) {
	// If the HTTP-date validator provided exactly matches the Last-Modified field value for the selected representation, the condition is true.
	return r.unixTimes.ifRange == date
}

func (r *serverRequest_) EvalRanges(contentSize int64) []Range { // returned ranges are converted from [from:last] to the format of [from:edge)
	rangedSize := int64(0)
	for i := int8(0); i < r.numRanges; i++ {
		rang := &r.ranges[i]
		if rang.From == -1 { // "-" suffix-length, means the last `suffix-length` bytes
			if rang.Last == 0 {
				return nil
			}
			if rang.Last >= contentSize {
				rang.From = 0
			} else {
				rang.From = contentSize - rang.Last
			}
			rang.Last = contentSize
		} else { // first-pos "-" [ last-pos ]
			if rang.From >= contentSize {
				return nil
			}
			if rang.Last == -1 { // first-pos "-", to the end if last-pos is not present
				rang.Last = contentSize
			} else { // first-pos "-" last-pos
				if rang.Last >= contentSize {
					rang.Last = contentSize
				} else {
					rang.Last++
				}
			}
		}
		rangedSize += rang.Last - rang.From
		if rangedSize > contentSize { // possible attack
			return nil
		}
	}
	return r.ranges[:r.numRanges]
}

func (r *serverRequest_) proxyUnsetHost() {
	r._delPrime(r.indexes.host) // zero safe
}

func (r *serverRequest_) HasContent() bool { return r.contentSize >= 0 || r.IsVague() }
func (r *serverRequest_) Content() string  { return string(r.UnsafeContent()) }
func (r *serverRequest_) UnsafeContent() []byte {
	if r.formKind == httpFormMultipart { // loading multipart form into memory is not allowed!
		return nil
	}
	return r.unsafeContent()
}

func (r *serverRequest_) parseHTMLForm() { // called on need to populate r.forms and r.upfiles
	if r.formKind == httpFormNotForm || r.formReceived {
		return
	}
	r.formReceived = true
	r.forms.from = uint8(len(r.primes))
	r.forms.edge = r.forms.from
	if r.formKind == httpFormURLEncoded { // application/x-www-form-urlencoded
		r._loadURLEncodedForm()
	} else { // multipart/form-data
		r._recvMultipartForm()
	}
}
func (r *serverRequest_) _loadURLEncodedForm() { // into memory entirely
	r._loadContent()
	if r.stream.isBroken() {
		return
	}
	var (
		state = 2 // to be consistent with HTTP/1.x
		octet byte
	)
	form := &r.mainPair
	form.zero()
	form.kind = pairForm
	form.place = placeArray // all received forms are placed in r.array
	form.nameFrom = r.arrayEdge
	for i := int64(0); i < r.receivedSize; i++ { // TODO: use a better algorithm to improve performance
		b := r.contentText[i]
		switch state {
		case 2: // expecting '=' to get a name
			if b == '=' {
				if nameSize := r.arrayEdge - form.nameFrom; nameSize <= 255 {
					form.nameSize = uint8(nameSize)
					form.value.from = r.arrayEdge
				} else {
					r.bodyResult, r.failReason = StatusBadRequest, "form name too long"
					return
				}
				state = 3
			} else if httpPchar[b] > 0 { // including '?'
				if b == '+' {
					b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
				}
				form.nameHash += uint16(b)
				r.arrayPush(b)
			} else if b == '%' {
				state = 0x2f // '2' means from state 2
			} else {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid form name"
				return
			}
		case 3: // expecting '&' to get a value
			if b == '&' {
				form.value.edge = r.arrayEdge
				if form.nameSize > 0 {
					r.addForm(form)
				}
				form.nameHash = 0 // reset for next form
				form.nameFrom = r.arrayEdge
				state = 2
			} else if httpPchar[b] > 0 { // including '?'
				if b == '+' {
					b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
				}
				r.arrayPush(b)
			} else if b == '%' {
				state = 0x3f // '3' means from state 3
			} else {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid form value"
				return
			}
		default: // expecting HEXDIG
			nybble, ok := byteFromHex(b)
			if !ok {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid pct encoding"
				return
			}
			if state&0xf == 0xf { // expecting the first HEXDIG
				octet = nybble << 4
				state &= 0xf0 // this reserves last state and leads to the state of second HEXDIG
			} else { // expecting the second HEXDIG
				octet |= nybble
				if state == 0x20 { // in name, calculate name hash
					form.nameHash += uint16(octet)
				}
				r.arrayPush(octet)
				state >>= 4 // restore last state
			}
		}
	}
	// Reaches the end of content.
	if state == 3 { // '&' not found
		form.value.edge = r.arrayEdge
		if form.nameSize > 0 {
			r.addForm(form)
		}
	} else { // '=' not found, or incomplete pct-encoded
		r.bodyResult, r.failReason = StatusBadRequest, "incomplete pct-encoded"
	}
}
func (r *serverRequest_) _recvMultipartForm() { // into memory or tempFile. see RFC 7578: https://datatracker.ietf.org/doc/html/rfc7578
	r.elemBack, r.elemFore = 0, 0
	r.consumedSize = r.receivedSize
	if r.contentReceived { // (0, 64K1)
		// r.contentText is set, r.contentTextKind == httpContentTextInput. r.formWindow refers to the exact r.contentText.
		r.formWindow = r.contentText
		r.formEdge = int32(len(r.formWindow))
	} else { // content is not received
		r.contentReceived = true
		switch content := r._recvContent(true).(type) { // retain
		case []byte: // (0, 64K1]. case happens when sized content <= 64K1
			r.contentText = content
			r.contentTextKind = httpContentTextPool        // so r.contentText can be freed on end
			r.formWindow = r.contentText[0:r.receivedSize] // r.formWindow refers to the exact r.content.
			r.formEdge = int32(r.receivedSize)
		case tempFile: // [0, r.webapp.maxMultiformSize]. case happens when sized content > 64K1, or content is vague.
			r.contentFile = content.(*os.File)
			if r.receivedSize == 0 {
				return // vague content can be empty
			}
			// We need a window to read and parse. An adaptive r.formWindow is used
			if r.receivedSize <= _4K {
				r.formWindow = Get4K()
			} else {
				r.formWindow = Get16K()
			}
			defer func() {
				PutNK(r.formWindow)
				r.formWindow = nil
			}()
			r.formEdge = 0     // no initial data, will fill below
			r.consumedSize = 0 // increases when we grow content
			if !r._growMultipartForm() {
				return
			}
		case error:
			// TODO: log err
			r.stream.markBroken()
			return
		}
	}
	template := r.UnsafeMake(3 + r.boundary.size() + 2) // \n--boundary--
	template[0], template[1], template[2] = '\n', '-', '-'
	n := 3 + copy(template[3:], r.input[r.boundary.from:r.boundary.edge])
	separator := template[0:n] // \n--boundary
	template[n], template[n+1] = '-', '-'
	for { // each part in multipart
		// Now r.formWindow is used for receiving --boundary-- EOL or --boundary EOL
		for r.formWindow[r.elemFore] != '\n' {
			if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
				return
			}
		}
		if r.elemBack == r.elemFore {
			r.stream.markBroken()
			return
		}
		fore := r.elemFore
		if fore >= 1 && r.formWindow[fore-1] == '\r' {
			fore--
		}
		if bytes.Equal(r.formWindow[r.elemBack:fore], template[1:n+2]) { // end of multipart (--boundary--)
			// All parts are received.
			if DebugLevel() >= 2 {
				Println(r.arrayEdge, cap(r.array), string(r.array[0:r.arrayEdge]))
			}
			return
		} else if !bytes.Equal(r.formWindow[r.elemBack:fore], template[1:n]) { // not start of multipart (--boundary)
			r.stream.markBroken()
			return
		}
		// Skip '\n'
		if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
			return
		}
		// r.elemFore is at fields of current part.
		var part struct { // current part
			valid  bool     // true if "name" parameter in "content-disposition" field is found
			isFile bool     // true if "filename" parameter in "content-disposition" field is found
			hash   uint16   // name hash
			name   span     // to r.array. like: "avatar"
			base   span     // to r.array. like: "michael.jpg", or empty if part is not a file
			type_  span     // to r.array. like: "image/jpeg", or empty if part is not a file
			path   span     // to r.array. like: "/path/to/391384576", or empty if part is not a file
			osFile *os.File // if part is a file, this is used
			form   pair     // if part is a form, this is used
			upfile Upfile   // if part is a file, this is used. zeroed
		}
		part.form.kind = pairForm
		part.form.place = placeArray // all received forms are placed in r.array
		for {                        // each field in current part
			// End of part fields?
			if b := r.formWindow[r.elemFore]; b == '\r' {
				if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
					return
				}
				if r.formWindow[r.elemFore] != '\n' {
					r.stream.markBroken()
					return
				}
				break
			} else if b == '\n' {
				break
			}
			r.elemBack = r.elemFore // now r.formWindow is used for receiving field-name and onward
			for {                   // field name
				b := r.formWindow[r.elemFore]
				if t := httpTchar[b]; t == 1 {
					// Fast path, do nothing
				} else if t == 2 { // A-Z
					r.formWindow[r.elemFore] = b + 0x20 // to lower
				} else if t == 3 { // '_'
					// For forms, do nothing
				} else if b == ':' {
					break
				} else {
					r.stream.markBroken()
					return
				}
				if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
					return
				}
			}
			if r.elemBack == r.elemFore { // field-name cannot be empty
				r.stream.markBroken()
				return
			}
			r.pFieldName.set(r.elemBack, r.elemFore) // in case of sliding r.formWindow when r._growMultipartForm()
			// Skip ':'
			if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
				return
			}
			// Skip OWS before field value
			for r.formWindow[r.elemFore] == ' ' || r.formWindow[r.elemFore] == '\t' {
				if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
					return
				}
			}
			r.elemBack = r.elemFore
			// Now r.formWindow is used for receiving field-value and onward. at this time we can still use r.pFieldName, no risk of sliding
			if fieldName := r.formWindow[r.pFieldName.from:r.pFieldName.edge]; bytes.Equal(fieldName, bytesContentDisposition) { // content-disposition
				// form-data; name="avatar"; filename="michael.jpg"
				for r.formWindow[r.elemFore] != ';' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				if r.elemBack == r.elemFore || !bytes.Equal(r.formWindow[r.elemBack:r.elemFore], bytesFormData) {
					r.stream.markBroken()
					return
				}
				r.elemBack = r.elemFore // now r.formWindow is used for receiving parameters and onward
				for r.formWindow[r.elemFore] != '\n' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				fore := r.elemFore
				if r.formWindow[fore-1] == '\r' {
					fore--
				}
				// Skip OWS after field value
				for r.formWindow[fore-1] == ' ' || r.formWindow[fore-1] == '\t' {
					fore--
				}
				paras := make([]para, 2) // for name & filename. won't escape to heap
				n, ok := r._parseParas(r.formWindow, r.elemBack, fore, paras)
				if !ok {
					r.stream.markBroken()
					return
				}
				for i := 0; i < n; i++ { // each para in field (; name="avatar"; filename="michael.jpg")
					para := &paras[i]
					if paraName := r.formWindow[para.name.from:para.name.edge]; bytes.Equal(paraName, bytesName) { // name="avatar"
						if m := para.value.size(); m == 0 || m > 255 {
							r.stream.markBroken()
							return
						}
						part.valid = true // as long as we got a name, this part is valid
						part.name.from = r.arrayEdge
						if !r.arrayCopy(r.formWindow[para.value.from:para.value.edge]) { // add "avatar"
							r.stream.markBroken()
							return
						}
						part.name.edge = r.arrayEdge
						// TODO: Is this a good implementation? If size is too large, just use bytes.Equal? Use a special hash value (like 0xffff) to hint this?
						for p := para.value.from; p < para.value.edge; p++ {
							part.hash += uint16(r.formWindow[p])
						}
					} else if bytes.Equal(paraName, bytesFilename) { // filename="michael.jpg"
						if m := para.value.size(); m == 0 || m > 255 {
							r.stream.markBroken()
							return
						}
						part.isFile = true

						part.base.from = r.arrayEdge
						if !r.arrayCopy(r.formWindow[para.value.from:para.value.edge]) { // add "michael.jpg"
							r.stream.markBroken()
							return
						}
						part.base.edge = r.arrayEdge

						part.path.from = r.arrayEdge
						if !r.arrayCopy(ConstBytes(r.saveContentFilesDir())) { // add "/path/to/"
							r.stream.markBroken()
							return
						}
						nameBuffer := r.stream.buffer256() // enough for tempName
						m := r.stream.Conn().MakeTempName(nameBuffer, time.Now().Unix())
						if !r.arrayCopy(nameBuffer[:m]) { // add "391384576"
							r.stream.markBroken()
							return
						}
						part.path.edge = r.arrayEdge // pathSize is ensured to be <= 255.
					} else {
						// Other parameters are invalid.
						r.stream.markBroken()
						return
					}
				}
			} else if bytes.Equal(fieldName, bytesContentType) { // content-type
				// image/jpeg
				for r.formWindow[r.elemFore] != '\n' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				fore := r.elemFore
				if r.formWindow[fore-1] == '\r' {
					fore--
				}
				// Skip OWS after field value
				for r.formWindow[fore-1] == ' ' || r.formWindow[fore-1] == '\t' {
					fore--
				}
				if n := fore - r.elemBack; n == 0 || n > 255 {
					r.stream.markBroken()
					return
				}
				part.type_.from = r.arrayEdge
				if !r.arrayCopy(r.formWindow[r.elemBack:fore]) { // add "image/jpeg"
					r.stream.markBroken()
					return
				}
				part.type_.edge = r.arrayEdge
			} else { // other fields are ignored
				for r.formWindow[r.elemFore] != '\n' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
			}
			// Skip '\n' and goto next field or end of fields
			if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
				return
			}
		}
		if !part.valid { // no valid fields
			r.stream.markBroken()
			return
		}
		// Now all fields of the part are received. Skip end of fields and goto part data
		if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
			return
		}
		if part.isFile {
			// TODO: upload code
			part.upfile.nameHash = part.hash
			part.upfile.nameSize, part.upfile.nameFrom = uint8(part.name.size()), part.name.from
			part.upfile.baseSize, part.upfile.baseFrom = uint8(part.base.size()), part.base.from
			part.upfile.typeSize, part.upfile.typeFrom = uint8(part.type_.size()), part.type_.from
			part.upfile.pathSize, part.upfile.pathFrom = uint8(part.path.size()), part.path.from
			if osFile, err := os.OpenFile(WeakString(r.array[part.path.from:part.path.edge]), os.O_RDWR|os.O_CREATE, 0644); err == nil {
				if DebugLevel() >= 2 {
					Println("OPENED")
				}
				part.osFile = osFile
			} else {
				if DebugLevel() >= 2 {
					Println(err.Error())
				}
				part.osFile = nil
			}
		} else { // part must be a form
			part.form.nameHash = part.hash
			part.form.nameFrom = part.name.from
			part.form.nameSize = uint8(part.name.size())
			part.form.value.from = r.arrayEdge
		}
		r.elemBack = r.elemFore // now r.formWindow is used for receiving part data and onward
		for {                   // each partial in current part
			partial := r.formWindow[r.elemBack:r.formEdge]
			r.elemFore = r.formEdge
			mode := 0 // by default, we assume end of part ("\n--boundary") is not in partial
			var i int
			if i = bytes.Index(partial, separator); i >= 0 {
				mode = 1 // end of part ("\n--boundary") is found in partial
			} else if i = bytes.LastIndexByte(partial, '\n'); i >= 0 && bytes.HasPrefix(separator, partial[i:]) {
				mode = 2 // partial ends with prefix of end of part ("\n--boundary")
			}
			if mode > 0 { // found "\n" at i
				r.elemFore = r.elemBack + int32(i)
				if r.elemFore > r.elemBack && r.formWindow[r.elemFore-1] == '\r' {
					r.elemFore--
				}
				partial = r.formWindow[r.elemBack:r.elemFore] // pure data
			}
			if !part.isFile {
				if !r.arrayCopy(partial) { // join form value
					r.stream.markBroken()
					return
				}
				if mode == 1 { // form part ends
					part.form.value.edge = r.arrayEdge
					r.addForm(&part.form)
				}
			} else if part.osFile != nil {
				part.osFile.Write(partial)
				if mode == 1 { // file part ends
					r.addUpfile(&part.upfile)
					part.osFile.Close()
					if DebugLevel() >= 2 {
						Println("CLOSED")
					}
				}
			}
			if mode == 1 {
				r.elemBack += int32(i + 1) // at the first '-' of "--boundary"
				r.elemFore = r.elemBack    // next part starts here
				break                      // part is received.
			}
			if mode == 2 {
				r.elemBack = r.elemFore // from EOL (\r or \n). need more and continue
			} else { // mode == 0
				r.elemBack, r.formEdge = 0, 0 // pure data, clean r.formWindow. need more and continue
			}
			// Grow more
			if !r._growMultipartForm() {
				return
			}
		}
	}
}
func (r *serverRequest_) _growMultipartForm() bool { // caller needs more data from content file
	if r.consumedSize == r.receivedSize || (r.formEdge == int32(len(r.formWindow)) && r.elemBack == 0) {
		r.stream.markBroken()
		return false
	}
	if r.elemBack > 0 { // have useless data. slide to start
		copy(r.formWindow, r.formWindow[r.elemBack:r.formEdge])
		r.formEdge -= r.elemBack
		r.elemFore -= r.elemBack
		if r.pFieldName.notEmpty() {
			r.pFieldName.sub(r.elemBack) // for fields in multipart/form-data, not for trailers
		}
		r.elemBack = 0
	}
	n, err := r.contentFile.Read(r.formWindow[r.formEdge:])
	r.formEdge += int32(n)
	r.consumedSize += int64(n)
	if err == io.EOF {
		if r.consumedSize == r.receivedSize {
			err = nil
		} else {
			err = io.ErrUnexpectedEOF
		}
	}
	if err != nil {
		r.stream.markBroken()
		return false
	}
	return true
}
func (r *serverRequest_) _parseParas(p []byte, from int32, edge int32, paras []para) (int, bool) {
	// param-string = *( OWS ";" OWS param-pair )
	// param-pair   = token "=" param-value
	// param-value  = *param-octet / ( DQUOTE *param-octet DQUOTE )
	// param-octet  = ?
	back, fore := from, from
	nAdd := 0
	for {
		nSemic := 0
		for fore < edge {
			if b := p[fore]; b == ' ' || b == '\t' {
				fore++
			} else if b == ';' {
				nSemic++
				fore++
			} else {
				break
			}
		}
		if fore == edge || nSemic != 1 {
			// `; ` and ` ` and `;;` are invalid
			return nAdd, false
		}
		back = fore // for name
		for fore < edge {
			if b := p[fore]; b == '=' {
				break
			} else if b == ';' || b == ' ' || b == '\t' {
				// `; a; ` is invalid
				return nAdd, false
			} else {
				fore++
			}
		}
		if fore == edge || back == fore {
			// `; a` and `; ="b"` are invalid
			return nAdd, false
		}
		para := &paras[nAdd]
		para.name.set(back, fore)
		fore++ // skip '='
		if fore == edge {
			para.value.zero()
			nAdd++
			return nAdd, true
		}
		back = fore
		if p[fore] == '"' {
			fore++
			for fore < edge && p[fore] != '"' {
				fore++
			}
			if fore == edge {
				para.value.set(back, fore) // value is "...
			} else {
				para.value.set(back+1, fore) // strip ""
				fore++
			}
		} else {
			for fore < edge && p[fore] != ';' && p[fore] != ' ' && p[fore] != '\t' {
				fore++
			}
			para.value.set(back, fore)
		}
		nAdd++
		if nAdd == len(paras) || fore == edge {
			return nAdd, true
		}
	}
}

func (r *serverRequest_) addForm(form *pair) bool { // as prime
	if edge, ok := r._addPrime(form); ok {
		r.forms.edge = edge
		return true
	}
	r.bodyResult, r.failReason = StatusURITooLong, "too many forms"
	return false
}
func (r *serverRequest_) HasForms() bool {
	r.parseHTMLForm()
	return r.hasPairs(r.forms, pairForm)
}
func (r *serverRequest_) AllForms() (forms [][2]string) {
	r.parseHTMLForm()
	return r.allPairs(r.forms, pairForm)
}
func (r *serverRequest_) F(name string) string {
	value, _ := r.Form(name)
	return value
}
func (r *serverRequest_) Fstr(name string, defaultValue string) string {
	if value, ok := r.Form(name); ok {
		return value
	}
	return defaultValue
}
func (r *serverRequest_) Fint(name string, defaultValue int) int {
	if value, ok := r.Form(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *serverRequest_) Form(name string) (value string, ok bool) {
	r.parseHTMLForm()
	v, ok := r.getPair(name, 0, r.forms, pairForm)
	return string(v), ok
}
func (r *serverRequest_) UnsafeForm(name string) (value []byte, ok bool) {
	r.parseHTMLForm()
	return r.getPair(name, 0, r.forms, pairForm)
}
func (r *serverRequest_) Forms(name string) (values []string, ok bool) {
	r.parseHTMLForm()
	return r.getPairs(name, 0, r.forms, pairForm)
}
func (r *serverRequest_) HasForm(name string) bool {
	r.parseHTMLForm()
	_, ok := r.getPair(name, 0, r.forms, pairForm)
	return ok
}
func (r *serverRequest_) DelForm(name string) (deleted bool) {
	r.parseHTMLForm()
	return r.delPair(name, 0, r.forms, pairForm)
}
func (r *serverRequest_) AddForm(name string, value string) bool { // as extra, by webapp
	return r.addExtra(name, value, 0, pairForm)
}

func (r *serverRequest_) addUpfile(upfile *Upfile) {
	if len(r.upfiles) == cap(r.upfiles) {
		if cap(r.upfiles) == cap(r.stockUpfiles) {
			upfiles := make([]Upfile, 0, 16)
			r.upfiles = append(upfiles, r.upfiles...)
		} else if cap(r.upfiles) == 16 {
			upfiles := make([]Upfile, 0, 128)
			r.upfiles = append(upfiles, r.upfiles...)
		} else {
			// Ignore too many upfiles
			return
		}
	}
	r.upfiles = append(r.upfiles, *upfile)
}
func (r *serverRequest_) HasUpfiles() bool {
	r.parseHTMLForm()
	return len(r.upfiles) != 0
}
func (r *serverRequest_) AllUpfiles() (upfiles []*Upfile) {
	r.parseHTMLForm()
	for i := 0; i < len(r.upfiles); i++ {
		upfile := &r.upfiles[i]
		upfile.setMeta(r.array)
		upfiles = append(upfiles, upfile)
	}
	return upfiles
}
func (r *serverRequest_) U(name string) *Upfile {
	upfile, _ := r.Upfile(name)
	return upfile
}
func (r *serverRequest_) Upfile(name string) (upfile *Upfile, ok bool) {
	r.parseHTMLForm()
	if n := len(r.upfiles); n > 0 && name != "" {
		nameHash := stringHash(name)
		for i := 0; i < n; i++ {
			if upfile := &r.upfiles[i]; upfile.nameHash == nameHash && upfile.nameEqualString(r.array, name) {
				upfile.setMeta(r.array)
				return upfile, true
			}
		}
	}
	return
}
func (r *serverRequest_) Upfiles(name string) (upfiles []*Upfile, ok bool) {
	r.parseHTMLForm()
	if n := len(r.upfiles); n > 0 && name != "" {
		nameHash := stringHash(name)
		for i := 0; i < n; i++ {
			if upfile := &r.upfiles[i]; upfile.nameHash == nameHash && upfile.nameEqualString(r.array, name) {
				upfile.setMeta(r.array)
				upfiles = append(upfiles, upfile)
			}
		}
		if len(upfiles) > 0 {
			ok = true
		}
	}
	return
}
func (r *serverRequest_) HasUpfile(name string) bool {
	r.parseHTMLForm()
	_, ok := r.Upfile(name)
	return ok
}

func (r *serverRequest_) examineTail() bool {
	for i := r.trailers.from; i < r.trailers.edge; i++ {
		if !r.applyTrailer(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *serverRequest_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

func (r *serverRequest_) hookReviser(reviser Reviser) { // to revise input content
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

func (r *serverRequest_) unsafeVariable(code int16, name string) (value []byte) {
	if code != -1 {
		return serverRequestVariables[code](r)
	}
	if strings.HasPrefix(name, "header_") {
		name = name[len("header_"):]
		if v, ok := r.UnsafeHeader(name); ok {
			return v
		}
	} else if strings.HasPrefix(name, "cookie_") {
		name = name[len("cookie_"):]
		if v, ok := r.UnsafeCookie(name); ok {
			return v
		}
	} else if strings.HasPrefix(name, "query_") {
		name = name[len("query_"):]
		if v, ok := r.UnsafeQuery(name); ok {
			return v
		}
	}
	return nil
}

var serverRequestVariables = [...]func(*serverRequest_) []byte{ // keep sync with varCodes
	0: (*serverRequest_).UnsafeMethod,      // method
	1: (*serverRequest_).UnsafeScheme,      // scheme
	2: (*serverRequest_).UnsafeAuthority,   // authority
	3: (*serverRequest_).UnsafeHostname,    // hostname
	4: (*serverRequest_).UnsafeColonport,   // colonport
	5: (*serverRequest_).UnsafePath,        // path
	6: (*serverRequest_).UnsafeURI,         // uri
	7: (*serverRequest_).UnsafeEncodedPath, // encodedPath
	8: (*serverRequest_).UnsafeQueryString, // queryString
	9: (*serverRequest_).UnsafeContentType, // contentType
}

// Response is the server-side http response.
type Response interface { // for *server[1-3]Response
	Request() Request

	SetStatus(status int16) error
	Status() int16

	MakeETagFrom(date int64, size int64) ([]byte, bool) // with `""`
	SetExpires(expires int64) bool
	SetLastModified(lastModified int64) bool
	AddContentType(contentType string) bool
	AddContentTypeBytes(contentType []byte) bool
	AddHTTPSRedirection(authority string) bool
	AddHostnameRedirection(hostname string) bool
	AddDirectoryRedirection() bool

	AddCookie(cookie *Cookie) bool

	AddHeader(name string, value string) bool
	AddHeaderBytes(name []byte, value []byte) bool
	Header(name string) (value string, ok bool)
	HasHeader(name string) bool
	DelHeader(name string) bool
	DelHeaderBytes(name []byte) bool

	IsSent() bool
	SetSendTimeout(timeout time.Duration) // to defend against slowloris attack

	Send(content string) error
	SendBytes(content []byte) error
	SendFile(contentPath string) error
	SendJSON(content any) error
	SendBadRequest(content []byte) error                             // 400
	SendForbidden(content []byte) error                              // 403
	SendNotFound(content []byte) error                               // 404
	SendMethodNotAllowed(allow string, content []byte) error         // 405
	SendRangeNotSatisfiable(contentSize int64, content []byte) error // 416
	SendInternalServerError(content []byte) error                    // 500
	SendNotImplemented(content []byte) error                         // 501
	SendBadGateway(content []byte) error                             // 502
	SendGatewayTimeout(content []byte) error                         // 504

	Echo(chunk string) error
	EchoBytes(chunk []byte) error
	EchoFile(chunkPath string) error
	AddTrailer(name string, value string) bool
	AddTrailerBytes(name []byte, value []byte) bool

	// Internal only
	addHeader(name []byte, value []byte) bool
	header(name []byte) (value []byte, ok bool)
	hasHeader(name []byte) bool
	delHeader(name []byte) bool
	pickRanges(ranges []Range, rangeType string)
	sendText(content []byte) error
	sendFile(content *os.File, info os.FileInfo, shut bool) error // will close content after sent
	sendChain() error                                             // content
	echoHeaders() error
	echoChain() error // chunks
	addTrailer(name []byte, value []byte) bool
	endVague() error
	proxyPass1xx(backResp backendResponse) bool
	proxyPassMessage(backResp backendResponse) error              // pass content to client directly
	proxyPostMessage(backContent any, backHasTrailers bool) error // post held content to client
	proxyCopyHeaders(backResp backendResponse, proxyConfig *WebExchanProxyConfig) bool
	proxyCopyTrailers(backResp backendResponse, proxyConfig *WebExchanProxyConfig) bool
	hookReviser(reviser Reviser)
	unsafeMake(size int) []byte
}

// serverResponse_ is the parent for server[1-3]Response.
type serverResponse_ struct { // outgoing. needs building
	// Parent
	httpOut_ // outgoing http response
	// Assocs
	request Request // related request
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	status    int16    // 200, 302, 404, 500, ...
	_         [6]byte  // padding
	start     [16]byte // exactly 16 bytes for "HTTP/1.1 NNN X\r\n". also used by HTTP/2 and HTTP/3, but shorter
	unixTimes struct { // in seconds
		expires      int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		lastModified int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	webapp           *Webapp // associated webapp
	_serverResponse0         // all values in this struct must be zero by default!
}
type _serverResponse0 struct { // for fast reset, entirely
	indexes struct {
		expires      uint8
		lastModified uint8
		_            [6]byte // padding
	}
	revisers [32]uint8 // reviser ids which will apply on this response. indexed by reviser order
}

func (r *serverResponse_) onUse(httpVersion uint8) { // for non-zeros
	const asRequest = false
	r.httpOut_.onUse(httpVersion, asRequest)

	r.status = StatusOK
	r.unixTimes.expires = -1      // not set
	r.unixTimes.lastModified = -1 // not set
}
func (r *serverResponse_) onEnd() { // for zeros
	r.webapp = nil
	r._serverResponse0 = _serverResponse0{}

	r.httpOut_.onEnd()
}

func (r *serverResponse_) Request() Request { return r.request }

func (r *serverResponse_) SetStatus(status int16) error {
	if status >= 200 && status <= 999 {
		r.status = status
		if status == StatusNoContent {
			r.forbidFraming = true
			r.forbidContent = true
		} else if status == StatusNotModified {
			// A server MAY send a Content-Length header field in a 304 (Not Modified) response to a conditional GET request.
			r.forbidFraming = true // we forbid it.
			r.forbidContent = true
		}
		return nil
	} else { // 1xx are not allowed to set through SetStatus()
		return httpOutUnknownStatus
	}
}
func (r *serverResponse_) Status() int16 { return r.status }

func (r *serverResponse_) MakeETagFrom(date int64, size int64) ([]byte, bool) { // with ""
	if date < 0 || size < 0 {
		return nil, false
	}
	p := r.unsafeMake(32)
	p[0] = '"'
	etag := p[1:]
	n := i64ToHex(date, etag)
	etag[n] = '-'
	if n++; n > 13 {
		return nil, false
	}
	n = 1 + n + i64ToHex(size, etag[n:])
	p[n] = '"'
	return p[0 : n+1], true
}
func (r *serverResponse_) SetExpires(expires int64) bool {
	return r._setUnixTime(&r.unixTimes.expires, &r.indexes.expires, expires)
}
func (r *serverResponse_) SetLastModified(lastModified int64) bool {
	return r._setUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified, lastModified)
}

func (r *serverResponse_) SendBadRequest(content []byte) error { // 400
	return r.sendError(StatusBadRequest, content)
}
func (r *serverResponse_) SendForbidden(content []byte) error { // 403
	return r.sendError(StatusForbidden, content)
}
func (r *serverResponse_) SendNotFound(content []byte) error { // 404
	return r.sendError(StatusNotFound, content)
}
func (r *serverResponse_) SendMethodNotAllowed(allow string, content []byte) error { // 405
	r.AddHeaderBytes(bytesAllow, ConstBytes(allow))
	return r.sendError(StatusMethodNotAllowed, content)
}
func (r *serverResponse_) SendRangeNotSatisfiable(contentSize int64, content []byte) error { // 416
	// add a header like: content-range: bytes */1234
	valueBuffer := r.stream.buffer256()
	n := copy(valueBuffer, bytesBytesStarSlash)
	n += i64ToDec(contentSize, valueBuffer[n:])
	r.AddHeaderBytes(bytesContentRange, valueBuffer[:n])
	return r.sendError(StatusRangeNotSatisfiable, content)
}
func (r *serverResponse_) SendInternalServerError(content []byte) error { // 500
	return r.sendError(StatusInternalServerError, content)
}
func (r *serverResponse_) SendNotImplemented(content []byte) error { // 501
	return r.sendError(StatusNotImplemented, content)
}
func (r *serverResponse_) SendBadGateway(content []byte) error { // 502
	return r.sendError(StatusBadGateway, content)
}
func (r *serverResponse_) SendGatewayTimeout(content []byte) error { // 504
	return r.sendError(StatusGatewayTimeout, content)
}
func (r *serverResponse_) sendError(status int16, content []byte) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	if err := r.SetStatus(status); err != nil {
		return err
	}
	if content == nil {
		content = serverErrorPages[status]
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content))
	return r.outMessage.sendChain()
}

var serverErrorPages = func() map[int16][]byte {
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
		phrase := control[len("HTTP/1.1 NNN ") : len(control)-2]
		pages[int16(status)] = []byte(fmt.Sprintf(template, status, phrase, status, phrase))
	}
	return pages
}()

func (r *serverResponse_) beforeSend() {
	resp := r.outMessage.(Response)
	for _, id := range r.revisers {
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.webapp.reviserByID(id)
		reviser.BeforeSend(resp.Request(), resp) // revise headers
	}
}
func (r *serverResponse_) doSend() error {
	if r.hasRevisers {
		resp := r.outMessage.(Response)
		for _, id := range r.revisers { // revise sized content
			if id == 0 {
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.OnOutput(resp.Request(), resp, &r.chain)
		}
		// Because r.chain may be altered by revisers, content size must be recalculated
		if contentSize, ok := r.chain.Size(); ok {
			r.contentSize = contentSize
		} else {
			return httpOutTooLarge
		}
	}
	return r.outMessage.sendChain()
}

func (r *serverResponse_) beforeEcho() {
	resp := r.outMessage.(Response)
	for _, id := range r.revisers { // revise headers
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.webapp.reviserByID(id)
		reviser.BeforeEcho(resp.Request(), resp)
	}
}
func (r *serverResponse_) doEcho() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	if r.hasRevisers {
		resp := r.outMessage.(Response)
		for _, id := range r.revisers { // revise vague content
			if id == 0 { // id of effective reviser is ensured to be > 0
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.OnOutput(resp.Request(), resp, &r.chain)
		}
	}
	return r.outMessage.echoChain()
}
func (r *serverResponse_) endVague() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if r.hasRevisers {
		resp := r.outMessage.(Response)
		for _, id := range r.revisers { // finish vague content
			if id == 0 { // id of effective reviser is ensured to be > 0
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.FinishEcho(resp.Request(), resp)
		}
	}
	return r.outMessage.finalizeVague()
}

var ( // perfect hash table for response critical headers
	serverResponseCriticalHeaderTable = [10]struct {
		hash uint16
		name []byte
		fAdd func(*serverResponse_, []byte) (ok bool)
		fDel func(*serverResponse_) (deleted bool)
	}{ // connection content-length content-type date expires last-modified server set-cookie transfer-encoding upgrade
		0: {hashServer, bytesServer, nil, nil},       // restricted
		1: {hashSetCookie, bytesSetCookie, nil, nil}, // restricted
		2: {hashUpgrade, bytesUpgrade, nil, nil},     // restricted
		3: {hashDate, bytesDate, (*serverResponse_)._insertDate, (*serverResponse_)._removeDate},
		4: {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted
		5: {hashConnection, bytesConnection, nil, nil},             // restricted
		6: {hashLastModified, bytesLastModified, (*serverResponse_)._insertLastModified, (*serverResponse_)._removeLastModified},
		7: {hashExpires, bytesExpires, (*serverResponse_)._insertExpires, (*serverResponse_)._removeExpires},
		8: {hashContentLength, bytesContentLength, nil, nil}, // restricted
		9: {hashContentType, bytesContentType, (*serverResponse_)._insertContentType, (*serverResponse_)._removeContentType},
	}
	serverResponseCriticalHeaderFind = func(nameHash uint16) int { return (113100 / int(nameHash)) % len(serverResponseCriticalHeaderTable) }
)

func (r *serverResponse_) insertHeader(nameHash uint16, name []byte, value []byte) bool {
	h := &serverResponseCriticalHeaderTable[serverResponseCriticalHeaderFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is restricted to insert
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.outMessage.addHeader(name, value)
}
func (r *serverResponse_) _insertExpires(expires []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.expires, &r.indexes.expires, bytesExpires, expires)
}
func (r *serverResponse_) _insertLastModified(lastModified []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified, bytesLastModified, lastModified)
}

func (r *serverResponse_) removeHeader(nameHash uint16, name []byte) bool {
	h := &serverResponseCriticalHeaderTable[serverResponseCriticalHeaderFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header is restricted to remove
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.outMessage.delHeader(name)
}
func (r *serverResponse_) _removeExpires() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.expires, &r.indexes.expires)
}
func (r *serverResponse_) _removeLastModified() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified)
}

func (r *serverResponse_) proxyPassMessage(backResp backendResponse) error {
	return r._proxyPassMessage(backResp)
}
func (r *serverResponse_) proxyCopyHeaders(backResp backendResponse, proxyConfig *WebExchanProxyConfig) bool {
	backResp.proxyDelHopHeaders()

	// copy control (:status)
	r.SetStatus(backResp.Status())

	// copy selective forbidden headers (excluding set-cookie, which is copied directly) from backResp

	// copy remaining headers from backResp
	if !backResp.proxyWalkHeaders(func(header *pair, name []byte, value []byte) bool {
		if header.nameHash == hashSetCookie && bytes.Equal(name, bytesSetCookie) { // set-cookie is copied directly
			return r.outMessage.addHeader(name, value)
		} else {
			return r.outMessage.insertHeader(header.nameHash, name, value) // some headers are restricted
		}
	}) {
		return false
	}

	for _, name := range proxyConfig.DelResponseHeaders {
		r.outMessage.delHeader(name)
	}

	return true
}
func (r *serverResponse_) proxyCopyTrailers(backResp backendResponse, proxyConfig *WebExchanProxyConfig) bool {
	return r._proxyCopyTrailers(backResp, proxyConfig)
}

func (r *serverResponse_) hookReviser(reviser Reviser) { // to revise output content
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

// Socket is the server-side webSocket.
type Socket interface { // for *server[1-3]Socket
	// TODO
	Read(dst []byte) (int, error)
	Write(src []byte) (int, error)
	Close() error
}

// serverSocket_ is the parent for server[1-3]Socket.
type serverSocket_ struct { // incoming and outgoing
	// Parent
	httpSocket_
	// Assocs
	// Stream states (non-zeros)
	// Stream states (zeros)
	_serverSocket0 // all values in this struct must be zero by default!
}
type _serverSocket0 struct { // for fast reset, entirely
}

func (s *serverSocket_) onUse() {
	const asServer = true
	s.httpSocket_.onUse(asServer)
}
func (s *serverSocket_) onEnd() {
	s._serverSocket0 = _serverSocket0{}

	s.httpSocket_.onEnd()
}

func (s *serverSocket_) serverTodo() {
}

//////////////////////////////////////// HTTP backend implementation ////////////////////////////////////////

// HTTPBackend
type HTTPBackend interface { // for *HTTP[1-3]Backend
	// Imports
	Backend
	// Methods
	FetchStream() (backendStream, error)
	StoreStream(stream backendStream)
}

// httpBackend_ is the parent for http[1-3]Backend.
type httpBackend_[N HTTPNode] struct {
	// Parent
	Backend_[N]
	// States
}

func (b *httpBackend_[N]) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *httpBackend_[N]) onConfigure() {
	b.Backend_.OnConfigure()
}
func (b *httpBackend_[N]) onPrepare() {
	b.Backend_.OnPrepare()
}

// HTTPNode
type HTTPNode interface {
	// Imports
	Node
	httpHolder
	// Methods
}

// httpNode_ is the parent for http[1-3]Node.
type httpNode_[B HTTPBackend] struct {
	// Parent
	Node_[B]
	// Mixins
	_httpHolder_
	// States
	keepAliveConns int32         // max conns to keep alive
	idleTimeout    time.Duration // conn idle timeout
	maxLifetime    time.Duration // conn's max lifetime
}

func (n *httpNode_[B]) onCreate(name string, stage *Stage, backend B) {
	n.Node_.OnCreate(name, stage, backend)
}

func (n *httpNode_[B]) onConfigure() {
	n.Node_.OnConfigure()
	n._httpHolder_.onConfigure(n, TmpDir()+"/web/backends/"+n.backend.Name()+"/"+n.name, 0*time.Second, 0*time.Second)

	// keepAliveConns
	n.ConfigureInt32("keepAliveConns", &n.keepAliveConns, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad keepAliveConns in node")
	}, 10)

	// idleTimeout
	n.ConfigureDuration("idleTimeout", &n.idleTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".idleTimeout has an invalid value")
	}, 2*time.Second)

	// maxLifetime
	n.ConfigureDuration("maxLifetime", &n.maxLifetime, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxLifetime has an invalid value")
	}, 1*time.Minute)
}
func (n *httpNode_[B]) onPrepare() {
	n.Node_.OnPrepare()
	n._httpHolder_.onPrepare(n, 0755)
}

// backendStream is the backend-side http stream.
type backendStream interface { // for *backend[1-3]Stream
	Request() backendRequest
	Response() backendResponse
	Socket() backendSocket

	isBroken() bool
	markBroken()
}

// backendRequest is the backend-side http request.
type backendRequest interface { // for *backend[1-3]Request
	setMethodURI(method []byte, uri []byte, hasContent bool) bool
	proxySetAuthority(hostname []byte, colonport []byte) bool
	proxyCopyCookies(foreReq Request) bool // NOTE: HTTP 1.x/2/3 have different requirements on "cookie" header
	proxyCopyHeaders(foreReq Request, proxyConfig *WebExchanProxyConfig) bool
	proxyPassMessage(foreReq Request) error                       // pass content to backend directly
	proxyPostMessage(foreContent any, foreHasTrailers bool) error // post held content to backend
	proxyCopyTrailers(foreReq Request, proxyConfig *WebExchanProxyConfig) bool
	isVague() bool
	endVague() error
}

// backendRequest_ is the parent for backend[1-3]Request.
type backendRequest_ struct { // outgoing. needs building
	// Parent
	httpOut_ // outgoing http request
	// Assocs
	response backendResponse // the corresponding response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	unixTimes struct { // in seconds
		ifModifiedSince   int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		ifUnmodifiedSince int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	_backendRequest0 // all values in this struct must be zero by default!
}
type _backendRequest0 struct { // for fast reset, entirely
	indexes struct {
		host              uint8
		ifModifiedSince   uint8
		ifUnmodifiedSince uint8
		ifRange           uint8
	}
}

func (r *backendRequest_) onUse(httpVersion uint8) { // for non-zeros
	const asRequest = true
	r.httpOut_.onUse(httpVersion, asRequest)

	r.unixTimes.ifModifiedSince = -1   // not set
	r.unixTimes.ifUnmodifiedSince = -1 // not set
}
func (r *backendRequest_) onEnd() { // for zeros
	r._backendRequest0 = _backendRequest0{}

	r.httpOut_.onEnd()
}

func (r *backendRequest_) Response() backendResponse { return r.response }

func (r *backendRequest_) SetMethodURI(method string, uri string, hasContent bool) bool {
	return r.outMessage.(backendRequest).setMethodURI(ConstBytes(method), ConstBytes(uri), hasContent)
}
func (r *backendRequest_) setScheme(scheme []byte) bool { // HTTP/2 and HTTP/3 only. HTTP/1.x doesn't use this!
	// TODO: copy `:scheme $scheme` to r.fields
	return false
}
func (r *backendRequest_) control() []byte { return r.fields[0:r.controlEdge] } // TODO: maybe we need a struct type to represent pseudo headers?

func (r *backendRequest_) SetIfModifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, since)
}
func (r *backendRequest_) SetIfUnmodifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, since)
}

func (r *backendRequest_) beforeSend() {} // revising is not supported in backend side.
func (r *backendRequest_) doSend() error { // revising is not supported in backend side.
	return r.outMessage.sendChain()
}

func (r *backendRequest_) beforeEcho() {} // revising is not supported in backend side.
func (r *backendRequest_) doEcho() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	return r.outMessage.echoChain()
}
func (r *backendRequest_) endVague() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.outMessage.finalizeVague()
}

var ( // perfect hash table for request critical headers
	backendRequestCriticalHeaderTable = [12]struct {
		hash uint16
		name []byte
		fAdd func(*backendRequest_, []byte) (ok bool)
		fDel func(*backendRequest_) (deleted bool)
	}{ // connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade via
		0:  {hashContentLength, bytesContentLength, nil, nil}, // restricted
		1:  {hashConnection, bytesConnection, nil, nil},       // restricted
		2:  {hashIfRange, bytesIfRange, (*backendRequest_)._insertIfRange, (*backendRequest_)._removeIfRange},
		3:  {hashUpgrade, bytesUpgrade, nil, nil}, // restricted
		4:  {hashIfModifiedSince, bytesIfModifiedSince, (*backendRequest_)._insertIfModifiedSince, (*backendRequest_)._removeIfModifiedSince},
		5:  {hashIfUnmodifiedSince, bytesIfUnmodifiedSince, (*backendRequest_)._insertIfUnmodifiedSince, (*backendRequest_)._removeIfUnmodifiedSince},
		6:  {hashHost, bytesHost, (*backendRequest_)._insertHost, (*backendRequest_)._removeHost},
		7:  {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted
		8:  {hashContentType, bytesContentType, (*backendRequest_)._insertContentType, (*backendRequest_)._removeContentType},
		9:  {hashCookie, bytesCookie, nil, nil}, // restricted
		10: {hashDate, bytesDate, (*backendRequest_)._insertDate, (*backendRequest_)._removeDate},
		11: {hashVia, bytesVia, nil, nil}, // restricted
	}
	backendRequestCriticalHeaderFind = func(nameHash uint16) int { return (645048 / int(nameHash)) % len(backendRequestCriticalHeaderTable) }
)

func (r *backendRequest_) insertHeader(nameHash uint16, name []byte, value []byte) bool {
	h := &backendRequestCriticalHeaderTable[backendRequestCriticalHeaderFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is restricted to insert
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.outMessage.addHeader(name, value)
}
func (r *backendRequest_) _insertHost(host []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.host, bytesHost, host)
}
func (r *backendRequest_) _insertIfRange(ifRange []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.ifRange, bytesIfRange, ifRange)
}
func (r *backendRequest_) _insertIfModifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, bytesIfModifiedSince, since)
}
func (r *backendRequest_) _insertIfUnmodifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, bytesIfUnmodifiedSince, since)
}

func (r *backendRequest_) removeHeader(nameHash uint16, name []byte) bool {
	h := &backendRequestCriticalHeaderTable[backendRequestCriticalHeaderFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header is restricted to remove
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.outMessage.delHeader(name)
}
func (r *backendRequest_) _removeHost() (deleted bool) {
	return r._deleteSingleton(&r.indexes.host)
}
func (r *backendRequest_) _removeIfRange() (deleted bool) {
	return r._deleteSingleton(&r.indexes.ifRange)
}
func (r *backendRequest_) _removeIfModifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince)
}
func (r *backendRequest_) _removeIfUnmodifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince)
}

func (r *backendRequest_) proxyPassMessage(foreReq Request) error {
	return r._proxyPassMessage(foreReq)
}
func (r *backendRequest_) proxyCopyHeaders(foreReq Request, proxyConfig *WebExchanProxyConfig) bool {
	foreReq.proxyDelHopHeaders()

	// copy control (:method, :path, :authority, :scheme)
	uri := foreReq.UnsafeURI()
	if foreReq.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.
		uri = bytesAsterisk
	}
	if !r.outMessage.(backendRequest).setMethodURI(foreReq.UnsafeMethod(), uri, foreReq.HasContent()) {
		return false
	}
	if foreReq.IsAbsoluteForm() || len(proxyConfig.Hostname) != 0 || len(proxyConfig.Colonport) != 0 { // TODO: what about HTTP/2 and HTTP/3?
		foreReq.proxyUnsetHost()
		if foreReq.IsAbsoluteForm() {
			if !r.outMessage.addHeader(bytesHost, foreReq.UnsafeAuthority()) {
				return false
			}
		} else { // custom authority (hostname or colonport)
			var (
				hostname  []byte
				colonport []byte
			)
			if len(proxyConfig.Hostname) == 0 { // no custom hostname
				hostname = foreReq.UnsafeHostname()
			} else {
				hostname = proxyConfig.Hostname
			}
			if len(proxyConfig.Colonport) == 0 { // no custom colonport
				colonport = foreReq.UnsafeColonport()
			} else {
				colonport = proxyConfig.Colonport
			}
			if !r.outMessage.(backendRequest).proxySetAuthority(hostname, colonport) {
				return false
			}
		}
	}
	if r.httpVersion >= Version2 {
		var scheme []byte
		if r.stream.Conn().TLSMode() {
			scheme = bytesSchemeHTTPS
		} else {
			scheme = bytesSchemeHTTP
		}
		if !r.setScheme(scheme) {
			return false
		}
	} else {
		// we have no way to set scheme in HTTP/1.x unless we use absolute-form, which is a risk as some servers may not support it.
	}

	// copy selective forbidden headers (including cookie) from foreReq
	if foreReq.HasCookies() && !r.outMessage.(backendRequest).proxyCopyCookies(foreReq) {
		return false
	}
	if !r.outMessage.addHeader(bytesVia, proxyConfig.InboundViaName) { // an HTTP-to-HTTP gateway MUST send an appropriate Via header field in each inbound request message
		return false
	}
	if foreReq.AcceptTrailers() {
		// TODO: add te: trailers
		// TODO: add connection: te
	}

	// copy added headers
	for name, vValue := range proxyConfig.AddRequestHeaders {
		var value []byte
		if vValue.IsVariable() {
			value = vValue.BytesVar(foreReq)
		} else if v, ok := vValue.Bytes(); ok {
			value = v
		} else {
			// Invalid values are treated as empty
		}
		if !r.outMessage.addHeader(ConstBytes(name), value) {
			return false
		}
	}

	// copy remaining headers from foreReq
	if !foreReq.proxyWalkHeaders(func(header *pair, name []byte, value []byte) bool {
		if false { // TODO: some special headers should be copied directly?
			return r.outMessage.addHeader(name, value)
		} else {
			return r.outMessage.insertHeader(header.nameHash, name, value) // some headers are restricted
		}
	}) {
		return false
	}

	for _, name := range proxyConfig.DelRequestHeaders {
		r.outMessage.delHeader(name)
	}

	return true
}
func (r *backendRequest_) proxyCopyTrailers(foreReq Request, proxyConfig *WebExchanProxyConfig) bool {
	return r._proxyCopyTrailers(foreReq, proxyConfig)
}

// backendResponse is the backend-side http response.
type backendResponse interface { // for *backend[1-3]Response
	KeepAlive() int8
	HeadResult() int16
	BodyResult() int16
	Status() int16
	HasContent() bool
	ContentSize() int64
	HasTrailers() bool
	IsVague() bool
	examineTail() bool
	proxyTakeContent() any
	readContent() (data []byte, err error)
	proxyDelHopHeaders()
	proxyDelHopTrailers()
	proxyWalkHeaders(callback func(header *pair, name []byte, value []byte) bool) bool
	proxyWalkTrailers(callback func(header *pair, name []byte, value []byte) bool) bool
	recvHead()
	reuse()
}

// backendResponse_ is the parent for backend[1-3]Response.
type backendResponse_ struct { // incoming. needs parsing
	// Parent
	httpIn_ // incoming http response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backendResponse0 // all values in this struct must be zero by default!
}
type _backendResponse0 struct { // for fast reset, entirely
	status      int16    // 200, 302, 404, ...
	acceptBytes bool     // accept-ranges: bytes?
	hasAllow    bool     // has allow header?
	age         int32    // age seconds
	indexes     struct { // indexes of some selected singleton headers, for fast accessing
		etag         uint8   // etag header ->r.input
		expires      uint8   // expires header ->r.input
		lastModified uint8   // last-modified header ->r.input
		location     uint8   // location header ->r.input
		retryAfter   uint8   // retry-after header ->r.input
		server       uint8   // server header ->r.input
		_            [2]byte // padding
	}
	zones struct { // zones of some selected headers, for fast accessing
		allow  zone
		altSvc zone
		vary   zone
		_      [2]byte // padding
	}
	unixTimes struct { // parsed unix times in seconds
		expires      int64 // parsed unix time of expires
		lastModified int64 // parsed unix time of last-modified
	}
	cacheControl struct { // the cache-control info
		noCache         bool  // no-cache directive in cache-control
		noStore         bool  // no-store directive in cache-control
		noTransform     bool  // no-transform directive in cache-control
		public          bool  // public directive in cache-control
		private         bool  // private directive in cache-control
		mustRevalidate  bool  // must-revalidate directive in cache-control
		mustUnderstand  bool  // must-understand directive in cache-control
		proxyRevalidate bool  // proxy-revalidate directive in cache-control
		maxAge          int32 // max-age directive in cache-control
		sMaxage         int32 // s-maxage directive in cache-control
	}
}

func (r *backendResponse_) onUse(httpVersion uint8) { // for non-zeros
	const asResponse = true
	r.httpIn_.onUse(httpVersion, asResponse)
}
func (r *backendResponse_) onEnd() { // for zeros
	r._backendResponse0 = _backendResponse0{}

	r.httpIn_.onEnd()
}

func (r *backendResponse_) reuse() { // between 1xx and non-1xx responses
	httpVersion := r.httpVersion
	r.onEnd()
	r.onUse(httpVersion)
}

func (r *backendResponse_) Status() int16 { return r.status }

func (r *backendResponse_) examineHead() bool {
	for i := r.headers.from; i < r.headers.edge; i++ {
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	if DebugLevel() >= 3 {
		for i := 0; i < len(r.primes); i++ {
			prime := &r.primes[i]
			prime.show(r._placeOf(prime))
		}
		for i := 0; i < len(r.extras); i++ {
			extra := &r.extras[i]
			extra.show(r._placeOf(extra))
		}
	}

	// Basic checks against versions
	switch r.httpVersion {
	case Version1_0: // we don't support HTTP/1.0 in backend side!
		BugExitln("HTTP/1.0 must be denied priorly")
	case Version1_1:
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
	default: // HTTP/2 and HTTP/3
		r.keepAlive = 1 // default is keep-alive for HTTP/2 and HTTP/3
		// TODO: add checks here
	}

	if !r.determineContentMode() {
		// r.headResult is set.
		return false
	}
	if r.status < StatusOK && r.contentSize != -1 {
		r.headResult, r.failReason = StatusBadRequest, "content is not allowed in 1xx responses"
		return false
	}
	if r.contentSize > r.maxContentSize {
		r.headResult, r.failReason = StatusContentTooLarge, "content size exceeds backend's limit"
		return false
	}

	return true
}
func (r *backendResponse_) applyHeader(index uint8) bool {
	header := &r.primes[index]
	name := header.nameAt(r.input)
	if sh := &backendResponseSingletonHeaderTable[backendResponseSingletonHeaderFind(header.nameHash)]; sh.nameHash == header.nameHash && bytes.Equal(sh.name, name) {
		header.setSingleton()
		if !sh.parse { // unnecessary to parse generally
			header.setParsed()
			header.dataEdge = header.value.edge
		} else if !r._parseField(header, &sh.fdesc, r.input, true) { // fully
			r.headResult = StatusBadRequest
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &backendResponseImportantHeaderTable[backendResponseImportantHeaderFind(header.nameHash)]; mh.nameHash == header.nameHash && bytes.Equal(mh.name, name) {
		extraFrom := uint8(len(r.extras))
		if !r._splitField(header, &mh.fdesc, r.input) {
			r.headResult = StatusBadRequest
			return false
		}
		if header.isCommaValue() { // has sub headers, check them
			if extraEdge := uint8(len(r.extras)); !mh.check(r, r.extras, extraFrom, extraEdge) {
				// r.headResult is set.
				return false
			}
		} else if !mh.check(r, r.primes, index, index+1) { // no sub headers. check it
			// r.headResult is set.
			return false
		}
	} else {
		// All other headers are treated as list-based headers.
	}
	return true
}

var ( // perfect hash table for singleton response headers
	backendResponseSingletonHeaderTable = [12]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*backendResponse_, *pair, uint8) bool
	}{ // age content-length content-range content-type date etag expires last-modified location retry-after server set-cookie
		0:  {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*backendResponse_).checkDate},
		1:  {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*backendResponse_).checkContentLength},
		2:  {false, fdesc{hashAge, false, false, false, false, bytesAge}, (*backendResponse_).checkAge},
		3:  {false, fdesc{hashSetCookie, false, false, false, false, bytesSetCookie}, (*backendResponse_).checkSetCookie}, // `a=b; Path=/; HttpsOnly` is not parameters
		4:  {false, fdesc{hashLastModified, false, false, false, false, bytesLastModified}, (*backendResponse_).checkLastModified},
		5:  {false, fdesc{hashLocation, false, false, false, false, bytesLocation}, (*backendResponse_).checkLocation},
		6:  {false, fdesc{hashExpires, false, false, false, false, bytesExpires}, (*backendResponse_).checkExpires},
		7:  {false, fdesc{hashContentRange, false, false, false, false, bytesContentRange}, (*backendResponse_).checkContentRange},
		8:  {false, fdesc{hashETag, false, false, false, false, bytesETag}, (*backendResponse_).checkETag},
		9:  {false, fdesc{hashServer, false, false, false, true, bytesServer}, (*backendResponse_).checkServer},
		10: {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*backendResponse_).checkContentType},
		11: {false, fdesc{hashRetryAfter, false, false, false, false, bytesRetryAfter}, (*backendResponse_).checkRetryAfter},
	}
	backendResponseSingletonHeaderFind = func(nameHash uint16) int { return (889344 / int(nameHash)) % len(backendResponseSingletonHeaderTable) }
)

func (r *backendResponse_) checkAge(header *pair, index uint8) bool { // Age = delta-seconds
	if header.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty age"
		return false
	}
	// TODO: check and write to r.age
	return true
}
func (r *backendResponse_) checkETag(header *pair, index uint8) bool { // ETag = entity-tag
	// TODO: check
	r.indexes.etag = index
	return true
}
func (r *backendResponse_) checkExpires(header *pair, index uint8) bool { // Expires = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.expires, &r.unixTimes.expires)
}
func (r *backendResponse_) checkLastModified(header *pair, index uint8) bool { // Last-Modified = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.lastModified, &r.unixTimes.lastModified)
}
func (r *backendResponse_) checkLocation(header *pair, index uint8) bool { // Location = URI-reference
	// TODO: check
	r.indexes.location = index
	return true
}
func (r *backendResponse_) checkRetryAfter(header *pair, index uint8) bool { // Retry-After = HTTP-date / delay-seconds
	// TODO: check
	r.indexes.retryAfter = index
	return true
}
func (r *backendResponse_) checkServer(header *pair, index uint8) bool { // Server = product *( RWS ( product / comment ) )
	// TODO: check
	r.indexes.server = index
	return true
}
func (r *backendResponse_) checkSetCookie(header *pair, index uint8) bool { // Set-Cookie = set-cookie-string
	// set-cookie-string = cookie-pair *( ";" SP cookie-av )
	// cookie-pair = token "=" cookie-value
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	// cookie-av = expires-av / max-age-av / domain-av / path-av / secure-av / httponly-av / samesite-av / extension-av
	// expires-av = "Expires=" sane-cookie-date
	// max-age-av = "Max-Age=" non-zero-digit *DIGIT
	// domain-av = "Domain=" domain-value
	// path-av = "Path=" path-value
	// secure-av = "Secure"
	// httponly-av = "HttpOnly"
	// samesite-av = "SameSite=" samesite-value
	// extension-av = <any CHAR except CTLs or ";">
	return true
}

var ( // perfect hash table for important response headers
	backendResponseImportantHeaderTable = [17]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*backendResponse_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate
		0:  {fdesc{hashAcceptRanges, false, false, false, false, bytesAcceptRanges}, (*backendResponse_).checkAcceptRanges},
		1:  {fdesc{hashVia, false, false, false, true, bytesVia}, (*backendResponse_).checkVia},
		2:  {fdesc{hashWWWAuthenticate, false, false, false, false, bytesWWWAuthenticate}, (*backendResponse_).checkWWWAuthenticate},
		3:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*backendResponse_).checkConnection},
		4:  {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*backendResponse_).checkContentEncoding},
		5:  {fdesc{hashAllow, false, true, false, false, bytesAllow}, (*backendResponse_).checkAllow},
		6:  {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*backendResponse_).checkTransferEncoding}, // deliberately false
		7:  {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*backendResponse_).checkTrailer},
		8:  {fdesc{hashVary, false, false, false, false, bytesVary}, (*backendResponse_).checkVary},
		9:  {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*backendResponse_).checkUpgrade},
		10: {fdesc{hashProxyAuthenticate, false, false, false, false, bytesProxyAuthenticate}, (*backendResponse_).checkProxyAuthenticate},
		11: {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*backendResponse_).checkCacheControl},
		12: {fdesc{hashAltSvc, false, false, true, false, bytesAltSvc}, (*backendResponse_).checkAltSvc},
		13: {fdesc{hashCDNCacheControl, false, false, false, false, bytesCDNCacheControl}, (*backendResponse_).checkCDNCacheControl},
		14: {fdesc{hashCacheStatus, false, false, true, false, bytesCacheStatus}, (*backendResponse_).checkCacheStatus},
		15: {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*backendResponse_).checkAcceptEncoding},
		16: {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*backendResponse_).checkContentLanguage},
	}
	backendResponseImportantHeaderFind = func(nameHash uint16) int {
		return (72189325 / int(nameHash)) % len(backendResponseImportantHeaderTable)
	}
)

func (r *backendResponse_) checkAcceptRanges(pairs []pair, from uint8, edge uint8) bool { // Accept-Ranges = 1#range-unit
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "accept-ranges = 1#range-unit"
		return false
	}
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		bytesToLower(data) // range unit names are case-insensitive
		if bytes.Equal(data, bytesBytes) {
			r.acceptBytes = true
		} else {
			// Ignore
		}
	}
	return true
}
func (r *backendResponse_) checkAllow(pairs []pair, from uint8, edge uint8) bool { // Allow = #method
	r.hasAllow = true
	if r.zones.allow.isEmpty() {
		r.zones.allow.from = from
	}
	r.zones.allow.edge = edge
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *backendResponse_) checkAltSvc(pairs []pair, from uint8, edge uint8) bool { // Alt-Svc = clear / 1#alt-value
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "alt-svc = clear / 1#alt-value"
		return false
	}
	if r.zones.altSvc.isEmpty() {
		r.zones.altSvc.from = from
	}
	r.zones.altSvc.edge = edge
	for i := from; i < edge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *backendResponse_) checkCacheControl(pairs []pair, from uint8, edge uint8) bool { // Cache-Control = #cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkCacheStatus(pairs []pair, from uint8, edge uint8) bool { // ?
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkCDNCacheControl(pairs []pair, from uint8, edge uint8) bool { // ?
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkProxyAuthenticate(pairs []pair, from uint8, edge uint8) bool { // Proxy-Authenticate = #challenge
	// TODO; use r._checkChallenge
	return true
}
func (r *backendResponse_) checkWWWAuthenticate(pairs []pair, from uint8, edge uint8) bool { // WWW-Authenticate = #challenge
	// TODO; use r._checkChallenge
	return true
}
func (r *backendResponse_) _checkChallenge(pairs []pair, from uint8, edge uint8) bool { // challenge = auth-scheme [ 1*SP ( token68 / [ auth-param *( OWS "," OWS auth-param ) ] ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkTransferEncoding(pairs []pair, from uint8, edge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.status < StatusOK || r.status == StatusNoContent {
		r.headResult, r.failReason = StatusBadRequest, "transfer-encoding is not allowed in 1xx and 204 responses"
		return false
	}
	if r.status == StatusNotModified {
		// TODO
	}
	return r.httpIn_.checkTransferEncoding(pairs, from, edge)
}
func (r *backendResponse_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	if r.httpVersion >= Version2 {
		r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in http/2 and http/3"
		return false
	}
	// TODO: what about upgrade: websocket?
	r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in exchan mode"
	return false
}
func (r *backendResponse_) checkVary(pairs []pair, from uint8, edge uint8) bool { // Vary = #( "*" / field-name )
	if r.zones.vary.isEmpty() {
		r.zones.vary.from = from
	}
	r.zones.vary.edge = edge
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}

func (r *backendResponse_) unsafeDate() []byte {
	if r.iDate == 0 {
		return nil
	}
	return r.primes[r.iDate].valueAt(r.input)
}
func (r *backendResponse_) unsafeLastModified() []byte {
	if r.indexes.lastModified == 0 {
		return nil
	}
	return r.primes[r.indexes.lastModified].valueAt(r.input)
}

func (r *backendResponse_) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status < StatusOK || r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return r.contentSize >= 0 || r.IsVague()
}
func (r *backendResponse_) Content() string       { return string(r.unsafeContent()) }
func (r *backendResponse_) UnsafeContent() []byte { return r.unsafeContent() }

func (r *backendResponse_) examineTail() bool {
	for i := r.trailers.from; i < r.trailers.edge; i++ {
		if !r.applyTrailer(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *backendResponse_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

// backendSocket is the backend-side webSocket.
type backendSocket interface { // for *backend[1-3]Socket
	Read(dst []byte) (int, error)
	Write(src []byte) (int, error)
	Close() error
}

// backendSocket_ is the parent for backend[1-3]Socket.
type backendSocket_ struct { // incoming and outgoing
	// Parent
	httpSocket_
	// Assocs
	// Stream states (zeros)
	_backendSocket0 // all values in this struct must be zero by default!
}
type _backendSocket0 struct { // for fast reset, entirely
}

func (s *backendSocket_) onUse() {
	const asServer = false
	s.httpSocket_.onUse(asServer)
}
func (s *backendSocket_) onEnd() {
	s._backendSocket0 = _backendSocket0{}

	s.httpSocket_.onEnd()
}

func (s *backendSocket_) backendTodo() {
}

//////////////////////////////////////// HTTP protocol elements ////////////////////////////////////////

const ( // basic http constants
	// version codes
	Version1_0 = 0 // must be 0
	Version1_1 = 1
	Version2   = 2
	Version3   = 3

	// scheme codes
	SchemeHTTP  = 0 // must be 0
	SchemeHTTPS = 1

	// best known http method codes
	MethodGET     = 0x00000001
	MethodHEAD    = 0x00000002
	MethodPOST    = 0x00000004
	MethodPUT     = 0x00000008
	MethodDELETE  = 0x00000010
	MethodCONNECT = 0x00000020
	MethodOPTIONS = 0x00000040
	MethodTRACE   = 0x00000080

	// status codes
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

var httpVersionStrings = [...]string{
	Version1_0: stringHTTP1_0,
	Version1_1: stringHTTP1_1,
	Version2:   stringHTTP2,
	Version3:   stringHTTP3,
}

var httpVersionByteses = [...][]byte{
	Version1_0: bytesHTTP1_0,
	Version1_1: bytesHTTP1_1,
	Version2:   bytesHTTP2,
	Version3:   bytesHTTP3,
}

var httpSchemeStrings = [...]string{
	SchemeHTTP:  stringHTTP,
	SchemeHTTPS: stringHTTPS,
}

var httpSchemeByteses = [...][]byte{
	SchemeHTTP:  bytesHTTP,
	SchemeHTTPS: bytesHTTPS,
}

const ( // misc http types
	httpTargetOrigin    = 0 // must be 0
	httpTargetAbsolute  = 1 // scheme "://" hostname [ ":" port ] path-abempty [ "?" query ]
	httpTargetAuthority = 2 // hostname:port, /path/to/unix.sock
	httpTargetAsterisk  = 3 // *

	httpSectionControl  = 0 // must be 0
	httpSectionHeaders  = 1
	httpSectionContent  = 2
	httpSectionTrailers = 3

	httpCodingIdentity = 0 // must be 0
	httpCodingCompress = 1
	httpCodingDeflate  = 2 // this is in fact zlib format
	httpCodingGzip     = 3
	httpCodingBrotli   = 4
	httpCodingUnknown  = 5

	httpFormNotForm    = 0 // must be 0
	httpFormURLEncoded = 1 // application/x-www-form-urlencoded
	httpFormMultipart  = 2 // multipart/form-data

	httpContentTextNone  = 0 // must be 0
	httpContentTextInput = 1 // refers to r.input
	httpContentTextPool  = 2 // fetched from pool
	httpContentTextMake  = 3 // direct make
)

const ( // hashes of http fields. value is calculated by adding all ASCII values.
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
	hashMaxForwards        = 1243
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

var ( // byteses of http fields.
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
	bytesMaxForwards        = []byte("max-forwards")
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

const ( // hashes of misc http strings & byteses.
	hashBoundary = 868
	hashFilename = 833
	hashName     = 417
)

const ( // misc http strings.
	stringHTTP         = "http"
	stringHTTPS        = "https"
	stringHTTP1_0      = "HTTP/1.0"
	stringHTTP1_1      = "HTTP/1.1"
	stringHTTP2        = "HTTP/2"
	stringHTTP3        = "HTTP/3"
	stringColonport80  = ":80"
	stringColonport443 = ":443"
	stringSlash        = "/"
	stringAsterisk     = "*"
)

var ( // misc http byteses.
	bytesHTTP           = []byte(stringHTTP)
	bytesHTTPS          = []byte(stringHTTPS)
	bytesHTTP1_0        = []byte(stringHTTP1_0)
	bytesHTTP1_1        = []byte(stringHTTP1_1)
	bytesHTTP2          = []byte(stringHTTP2)
	bytesHTTP3          = []byte(stringHTTP3)
	bytesColonport80    = []byte(stringColonport80)
	bytesColonport443   = []byte(stringColonport443)
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

var httpTchar = [256]int8{ // tchar = ALPHA / DIGIT / "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~"
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
var httpPchar = [256]int8{ // pchar = ALPHA / DIGIT / "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / "-" / "." / ":" / ";" / "=" / "@" / "_" / "~" / pct-encoded. '/' is pchar to improve performance.
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
var httpKchar = [256]int8{ // cookie-octet = 0x21 / 0x23-0x2B / 0x2D-0x3A / 0x3C-0x5B / 0x5D-0x7E
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
var httpHchar = [256]int8{ // for hostname
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

// Range defines a range.
type Range struct { // 16 bytes
	From, Last int64 // [From:Last], inclusive
}

// defaultFdesc
var defaultFdesc = &fdesc{
	allowQuote: true,
	allowEmpty: false,
	allowParam: true,
	hasComment: false,
}

// fdesc describes an http field.
type fdesc struct {
	nameHash   uint16 // name hash
	allowQuote bool   // allow data quote or not
	allowEmpty bool   // allow empty data or not
	allowParam bool   // allow parameters or not
	hasComment bool   // has comment or not
	name       []byte // field name
}

// pair is used to hold queries, headers, cookies, forms, trailers, and params.
type pair struct { // 24 bytes
	nameHash uint16 // name hash, to support fast search. 0 means empty pair
	kind     int8   // see pair kinds
	nameSize uint8  // name ends at nameFrom+nameSize
	nameFrom int32  // name begins from
	value    span   // the value
	place    int8   // see pair places
	flags    byte   // fields only. see field flags
	params   zone   // fields only. refers to a zone of pairs
	dataEdge int32  // fields only. data ends at
}

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
	pairUnknown = iota
	pairQuery   // plain kv
	pairHeader  // field
	pairCookie  // plain kv
	pairForm    // plain kv
	pairTrailer // field
	pairParam   // parameter of fields, plain kv
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
	case pairQuery:
		kind = "query"
	case pairHeader:
		kind = "header"
	case pairCookie:
		kind = "cookie"
	case pairForm:
		kind = "form"
	case pairTrailer:
		kind = "trailer"
	case pairParam:
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
	Printf("{nameHash=%4d kind=%7s place=[%7s] flags=[%s] dataEdge=%d params=%v value=%v %s=%s}\n", p.nameHash, kind, plase, strings.Join(flags, ","), p.dataEdge, p.params, p.value, p.nameAt(place), p.valueAt(place))
}

// para is a name-value parameter in fields.
type para struct { // 16 bytes
	name, value span
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
	if DebugLevel() >= 2 {
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
	if DebugLevel() >= 2 {
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

// Upfile is a file uploaded by http client and used by http server.
type Upfile struct { // 48 bytes
	nameHash uint16 // hash of name, to support fast comparison
	flags    uint8  // see upfile flags
	errCode  int8   // error code
	nameSize uint8  // name size
	baseSize uint8  // base size
	typeSize uint8  // type size
	pathSize uint8  // path size
	nameFrom int32  // like: "avatar"
	baseFrom int32  // like: "michael.jpg"
	typeFrom int32  // like: "image/jpeg"
	pathFrom int32  // like: "/path/to/391384576"
	size     int64  // file size
	meta     string // cannot use []byte as it can cause memory leak if caller save file to another place
}

func (u *Upfile) nameEqualString(p []byte, x string) bool {
	if int(u.nameSize) != len(x) {
		return false
	}
	if u.metaSet() {
		return u.meta[u.nameFrom:u.nameFrom+int32(u.nameSize)] == x
	}
	return string(p[u.nameFrom:u.nameFrom+int32(u.nameSize)]) == x
}

const ( // upfile flags
	upfileFlagMetaSet = 0b10000000
	upfileFlagIsMoved = 0b01000000
)

func (u *Upfile) setMeta(p []byte) {
	if u.flags&upfileFlagMetaSet > 0 {
		return
	}
	u.flags |= upfileFlagMetaSet
	from := u.nameFrom
	if u.baseFrom < from {
		from = u.baseFrom
	}
	if u.pathFrom < from {
		from = u.pathFrom
	}
	if u.typeFrom < from {
		from = u.typeFrom
	}
	max, edge := u.typeFrom, u.typeFrom+int32(u.typeSize)
	if u.pathFrom > max {
		max = u.pathFrom
		edge = u.pathFrom + int32(u.pathSize)
	}
	if u.baseFrom > max {
		max = u.baseFrom
		edge = u.baseFrom + int32(u.baseSize)
	}
	if u.nameFrom > max {
		max = u.nameFrom
		edge = u.nameFrom + int32(u.nameSize)
	}
	u.meta = string(p[from:edge]) // dup to avoid memory leak
	u.nameFrom -= from
	u.baseFrom -= from
	u.typeFrom -= from
	u.pathFrom -= from
}
func (u *Upfile) metaSet() bool { return u.flags&upfileFlagMetaSet > 0 }
func (u *Upfile) setMoved()     { u.flags |= upfileFlagIsMoved }
func (u *Upfile) isMoved() bool { return u.flags&upfileFlagIsMoved > 0 }

const ( // upfile error codes
	upfileOK        = 0
	upfileError     = 1
	upfileCantWrite = 2
	upfileTooLarge  = 3
	upfilePartial   = 4
	upfileNoFile    = 5
)

var upfileErrors = [...]error{
	nil, // no error
	errors.New("general error"),
	errors.New("cannot write"),
	errors.New("too large"),
	errors.New("partial"),
	errors.New("no file"),
}

func (u *Upfile) IsOK() bool   { return u.errCode == 0 }
func (u *Upfile) Error() error { return upfileErrors[u.errCode] }

func (u *Upfile) Name() string { return u.meta[u.nameFrom : u.nameFrom+int32(u.nameSize)] }
func (u *Upfile) Base() string { return u.meta[u.baseFrom : u.baseFrom+int32(u.baseSize)] }
func (u *Upfile) Type() string { return u.meta[u.typeFrom : u.typeFrom+int32(u.typeSize)] }
func (u *Upfile) Path() string { return u.meta[u.pathFrom : u.pathFrom+int32(u.pathSize)] }
func (u *Upfile) Size() int64  { return u.size }

func (u *Upfile) MoveTo(path string) error {
	// TODO. Remember to mark as moved
	return nil
}

// Cookie is a "set-cookie" header that is sent to http client by http server.
type Cookie struct {
	name     string
	value    string
	expires  time.Time
	domain   string
	path     string
	sameSite string
	maxAge   int32
	secure   bool
	httpOnly bool
	invalid  bool
	quote    bool // if true, quote value with ""
	aSize    int8
	ageBuf   [10]byte
}

func (c *Cookie) Set(name string, value string) bool {
	// cookie-name = 1*cookie-octet
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	if name == "" {
		c.invalid = true
		return false
	}
	for i := 0; i < len(name); i++ {
		if b := name[i]; httpKchar[b] == 0 {
			c.invalid = true
			return false
		}
	}
	c.name = name
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	for i := 0; i < len(value); i++ {
		b := value[i]
		if httpKchar[b] == 1 {
			continue
		}
		if b == ' ' || b == ',' {
			c.quote = true
			continue
		}
		c.invalid = true
		return false
	}
	c.value = value
	return true
}

func (c *Cookie) SetDomain(domain string) bool {
	// TODO: check domain
	c.domain = domain
	return true
}
func (c *Cookie) SetPath(path string) bool {
	// path-value = *av-octet
	// av-octet = %x20-3A / %x3C-7E
	for i := 0; i < len(path); i++ {
		if b := path[i]; b < 0x20 || b > 0x7E || b == 0x3B {
			c.invalid = true
			return false
		}
	}
	c.path = path
	return true
}
func (c *Cookie) SetExpires(expires time.Time) bool {
	expires = expires.UTC()
	if expires.Year() < 1601 {
		c.invalid = true
		return false
	}
	c.expires = expires
	return true
}
func (c *Cookie) SetMaxAge(maxAge int32)  { c.maxAge = maxAge }
func (c *Cookie) SetSecure()              { c.secure = true }
func (c *Cookie) SetHttpOnly()            { c.httpOnly = true }
func (c *Cookie) SetSameSiteStrict()      { c.sameSite = "Strict" }
func (c *Cookie) SetSameSiteLax()         { c.sameSite = "Lax" }
func (c *Cookie) SetSameSiteNone()        { c.sameSite = "None" }
func (c *Cookie) SetSameSite(mode string) { c.sameSite = mode }

func (c *Cookie) size() int {
	// set-cookie: name=value; Expires=Sun, 06 Nov 1994 08:49:37 GMT; Max-Age=123; Domain=example.com; Path=/; Secure; HttpOnly; SameSite=Strict
	n := len(c.name) + 1 + len(c.value) // name=value
	if c.quote {
		n += 2 // ""
	}
	if !c.expires.IsZero() {
		n += len("; Expires=Sun, 06 Nov 1994 08:49:37 GMT")
	}
	if c.maxAge > 0 {
		m := i32ToDec(c.maxAge, c.ageBuf[:])
		c.aSize = int8(m)
		n += len("; Max-Age=") + m
	} else if c.maxAge < 0 {
		c.ageBuf[0] = '0'
		c.aSize = 1
		n += len("; Max-Age=0")
	}
	if c.domain != "" {
		n += len("; Domain=") + len(c.domain)
	}
	if c.path != "" {
		n += len("; Path=") + len(c.path)
	}
	if c.secure {
		n += len("; Secure")
	}
	if c.httpOnly {
		n += len("; HttpOnly")
	}
	if c.sameSite != "" {
		n += len("; SameSite=") + len(c.sameSite)
	}
	return n
}
func (c *Cookie) writeTo(dst []byte) int {
	i := copy(dst, c.name)
	dst[i] = '='
	i++
	if c.quote {
		dst[i] = '"'
		i++
		i += copy(dst[i:], c.value)
		dst[i] = '"'
		i++
	} else {
		i += copy(dst[i:], c.value)
	}
	if !c.expires.IsZero() {
		i += copy(dst[i:], "; Expires=")
		i += clockWriteHTTPDate(dst[i:], c.expires)
	}
	if c.maxAge != 0 {
		i += copy(dst[i:], "; Max-Age=")
		i += copy(dst[i:], c.ageBuf[0:c.aSize])
	}
	if c.domain != "" {
		i += copy(dst[i:], "; Domain=")
		i += copy(dst[i:], c.domain)
	}
	if c.path != "" {
		i += copy(dst[i:], "; Path=")
		i += copy(dst[i:], c.path)
	}
	if c.secure {
		i += copy(dst[i:], "; Secure")
	}
	if c.httpOnly {
		i += copy(dst[i:], "; HttpOnly")
	}
	if c.sameSite != "" {
		i += copy(dst[i:], "; SameSite=")
		i += copy(dst[i:], c.sameSite)
	}
	return i
}
