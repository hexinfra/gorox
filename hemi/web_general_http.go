// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP types. See RFC 9110.

package hemi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// httpHolder is the interface for _httpHolder_.
type httpHolder interface {
	// Imports
	contentSaver
	// Methods
	MaxMemoryContentSize() int32 // allowed to load into memory
}

// _httpHolder_ is a mixin.
type _httpHolder_ struct { // for httpNode_, httpServer_, and httpGate_
	// Mixins
	_contentSaver_ // so http messages can save their large contents in local file system.
	// States
	maxCumulativeStreamsPerConn int32 // max cumulative streams of one conn. 0 means infinite
	maxMemoryContentSize        int32 // max content size that can be loaded into memory directly
}

func (h *_httpHolder_) onConfigure(comp Component, defaultRecv time.Duration, defaultSend time.Duration, defaultDir string) {
	h._contentSaver_.onConfigure(comp, defaultRecv, defaultSend, defaultDir)

	// .maxCumulativeStreamsPerConn
	comp.ConfigureInt32("maxCumulativeStreamsPerConn", &h.maxCumulativeStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxCumulativeStreamsPerConn has an invalid value")
	}, 1000)

	// .maxMemoryContentSize
	comp.ConfigureInt32("maxMemoryContentSize", &h.maxMemoryContentSize, func(value int32) error {
		if value > 0 && value <= _1G { // DO NOT CHANGE THIS, otherwise integer overflow may occur
			return nil
		}
		return errors.New(".maxMemoryContentSize has an invalid value")
	}, _16M)
}
func (h *_httpHolder_) onPrepare(comp Component, perm os.FileMode) {
	h._contentSaver_.onPrepare(comp, perm)
}

func (h *_httpHolder_) MaxMemoryContentSize() int32 { return h.maxMemoryContentSize }

// httpConn
type httpConn interface {
	Holder() httpHolder
	ID() int64
	UDSMode() bool
	TLSMode() bool
	MakeTempName(dst []byte, unixTime int64) int
	remoteAddr() net.Addr
	markBroken()
	isBroken() bool
}

// httpConn_ is a parent.
type httpConn_ struct { // for http[1-3]Conn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id           int64         // the conn id
	stage        *Stage        // current stage, for convenience
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

func (c *httpConn_) onGet(id int64, holder holder) {
	c.id = id
	c.stage = holder.Stage()
	c.udsMode = holder.UDSMode()
	c.tlsMode = holder.TLSMode()
	c.readTimeout = holder.ReadTimeout()
	c.writeTimeout = holder.WriteTimeout()
}
func (c *httpConn_) onPut() {
	c.stage = nil
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
	return makeTempName(dst, c.stage.ID(), unixTime, c.id, c.counter.Add(1))
}

func (c *httpConn_) markBroken()    { c.broken.Store(true) }
func (c *httpConn_) isBroken() bool { return c.broken.Load() }

// httpStream
type httpStream interface {
	Holder() httpHolder
	ID() int64
	UDSMode() bool
	TLSMode() bool
	MakeTempName(dst []byte, unixTime int64) int
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

// httpStream_ is a parent.
type httpStream_[C httpConn] struct { // for http[1-3]Stream_
	// Assocs
	conn C // the http connection
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	region Region // a region-based memory pool
	// Stream states (zeros)
}

func (s *httpStream_[C]) onUse(conn C) {
	s.conn = conn
	s.region.Init()
}
func (s *httpStream_[C]) onEnd() {
	var null C // nil
	s.conn = null
	s.region.Free()
}

func (s *httpStream_[C]) Holder() httpHolder { return s.conn.Holder() }
func (s *httpStream_[C]) UDSMode() bool      { return s.conn.UDSMode() }
func (s *httpStream_[C]) TLSMode() bool      { return s.conn.TLSMode() }
func (s *httpStream_[C]) MakeTempName(dst []byte, unixTime int64) int {
	return s.conn.MakeTempName(dst, unixTime)
}
func (s *httpStream_[C]) remoteAddr() net.Addr { return s.conn.remoteAddr() }

func (s *httpStream_[C]) buffer256() []byte          { return s.stockBuffer[:] }
func (s *httpStream_[C]) unsafeMake(size int) []byte { return s.region.Make(size) }

// httpIn
type httpIn interface {
	ContentSize() int64
	IsVague() bool
	HasTrailers() bool

	readContent() (data []byte, err error)
	examineTail() bool
	proxyDelHopFieldLines(kind int8)
	proxyWalkTrailerLines(out httpOut, callback func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool) bool
}

// _httpIn_ is a mixin.
type _httpIn_ struct { // for serverRequest_ and backendResponse_. incoming, needs parsing
	// Assocs
	stream httpStream // *backend[1-3]Stream, *server[1-3]Stream
	in     httpIn     // *backend[1-3]Response, *server[1-3]Request
	// Stream states (stocks)
	stockInput  [1536]byte // for r.input
	stockArray  [768]byte  // for r.array
	stockPrimes [40]pair   // for r.primes. 960B
	stockExtras [30]pair   // for r.extras. 720B
	// Stream states (controlled)
	inputNext      int32    // HTTP/1.x request only. next request begins from r.input[r.inputNext]. exists because HTTP/1.1 supports pipelining
	inputEdge      int32    // edge position of current message head is at r.input[r.inputEdge]. placed here to make it compatible with HTTP/1.1 pipelining
	mainPair       pair     // to overcome the limitation of Go's escape analysis when receiving incoming pairs
	contentCodings [4]uint8 // known content-encoding flags, controlled by r.numContentCodings. see httpCodingXXX for values
	acceptCodings  [4]uint8 // known accept-encoding flags, controlled by r.numAcceptCodings. see httpCodingXXX for values
	// Stream states (non-zeros)
	input                []byte        // bytes of raw incoming message heads. [<r.stockInput>/4K/16K]
	array                []byte        // store parsed, dynamic incoming data. [<r.stockArray>/4K/16K/64K1/(make <= 1G)]
	primes               []pair        // hold prime queries, headerLines(main+subs), cookies, forms, and trailerLines(main+subs). [<r.stockPrimes>/max]
	extras               []pair        // hold extra queries, headerLines(main+subs), cookies, forms, trailerLines(main+subs), and params. [<r.stockExtras>/max]
	recvTimeout          time.Duration // timeout to recv the whole message content. zero means no timeout
	maxContentSize       int64         // max content size allowed for current message. if the content is vague, size will be calculated on receiving
	maxMemoryContentSize int32         // max content size allowed for loading the content into memory. some content types are not allowed to load into memory
	_                    int32         // padding
	contentSize          int64         // size info about incoming content. -2: vague content, -1: no content, >=0: content size
	httpVersion          uint8         // Version1_0, Version1_1, Version2, Version3
	asResponse           bool          // treat this incoming message as a response? i.e. backend response
	keepAlive            int8          // -1: no connection header field, 0: connection close, 1: connection keep-alive
	_                    byte          // padding
	headResult           int16         // result of receiving message head. values are as same as http status for convenience
	bodyResult           int16         // result of receiving message body. values are as same as http status for convenience
	// Stream states (zeros)
	failReason  string    // the fail reason of headResult or bodyResult
	bodyWindow  []byte    // a window used for receiving body. for HTTP/1.x, sizes must be same with r.input. [HTTP/1.x=<none>/16K, HTTP/2/3=<none>/4K/16K/64K1]
	bodyTime    time.Time // the time when first body read operation is performed on this stream
	contentText []byte    // if loadable, the received and loaded content of current message is at r.contentText[:r.receivedSize]. [<none>/r.input/4K/16K/64K1/(make)]
	contentFile *os.File  // used by r.proxyTakeContent(), if content is tempFile. will be closed on stream ends
	_httpIn0              // all values in this struct must be zero by default!
}
type _httpIn0 struct { // for fast reset, entirely
	elemBack          int32   // element begins from. for parsing elements in control & headerLines & content & trailerLines
	elemFore          int32   // element spanning to. for parsing elements in control & headerLines & content & trailerLines
	head              span    // head (control data + header section) of current message -> r.input. set after head is received. only for debugging
	imme              span    // HTTP/1.x only. immediate data after current message head is at r.input[r.imme.from:r.imme.edge]
	hasExtra          [8]bool // has extra pairs? see pairXXX for indexes
	dateTime          int64   // parsed unix time of the date header field
	arrayEdge         int32   // next usable position of r.array begins from r.array[r.arrayEdge]. used when writing r.array
	arrayKind         int8    // kind of current r.array. see arrayKindXXX
	receiving         int8    // what message section are we currently receiving? see httpSectionXXX
	headerLines       zone    // header lines ->r.primes
	hasRevisers       bool    // are there any incoming revisers hooked on this incoming message?
	upgradeSocket     bool    // upgrade: websocket?
	acceptGzip        bool    // does the peer accept gzip content coding? i.e. accept-encoding: gzip
	acceptBrotli      bool    // does the peer accept brotli content coding? i.e. accept-encoding: br
	numContentCodings int8    // num of content-encoding flags, controls r.contentCodings
	numAcceptCodings  int8    // num of accept-encoding flags, controls r.acceptCodings
	iContentLength    uint8   // index of content-length header line in r.primes
	iContentLocation  uint8   // index of content-location header line in r.primes
	iContentRange     uint8   // index of content-range header line in r.primes
	iContentType      uint8   // index of content-type header line in r.primes
	iDate             uint8   // index of date header line in r.primes
	_                 [3]byte // padding
	zAccept           zone    // zone of accept header lines in r.primes. may not be continuous
	zAcceptEncoding   zone    // zone of accept-encoding header lines in r.primes. may not be continuous
	zCacheControl     zone    // zone of cache-control header lines in r.primes. may not be continuous
	zConnection       zone    // zone of connection header lines in r.primes. may not be continuous
	zContentEncoding  zone    // zone of content-encoding header lines in r.primes. may not be continuous
	zContentLanguage  zone    // zone of content-language header lines in r.primes. may not be continuous
	zKeepAlive        zone    // zone of keep-alive header lines in r.primes. may not be continuous
	zProxyConnection  zone    // zone of proxy-connection header lines in r.primes. may not be continuous
	zTrailer          zone    // zone of trailer header lines in r.primes. may not be continuous
	zTransferEncoding zone    // zone of transfer-encoding header lines in r.primes. may not be continuous
	zUpgrade          zone    // zone of upgrade header lines in r.primes. may not be continuous
	zVia              zone    // zone of via header lines in r.primes. may not be continuous
	contentReceived   bool    // is the content received? true if the message has no content or the content is received, otherwise false
	contentTextKind   int8    // kind of current r.contentText if it is text. see httpContentTextXXX
	receivedSize      int64   // bytes of currently received content. used and calculated by both sized & vague content receiver when receiving
	chunkSize         int64   // left size of current chunk if the chunk is too large to receive in one call. HTTP/1.1 chunked only
	chunkBack         int32   // for parsing chunked elements. HTTP/1.1 chunked only
	chunkFore         int32   // for parsing chunked elements. HTTP/1.1 chunked only
	chunkEdge         int32   // edge position of the filled chunked data in r.bodyWindow. HTTP/1.1 chunked only
	transferChunked   bool    // transfer-encoding: chunked? HTTP/1.1 only
	overChunked       bool    // for HTTP/1.1 requests, if chunked receiver over received in r.bodyWindow, then r.bodyWindow will be used as r.input on ends
	trailerLines      zone    // trailer lines -> r.primes. set after trailer section is received and parsed
}

func (r *_httpIn_) onUse(httpVersion uint8, asResponse bool) { // for non-zeros
	if httpVersion >= Version2 || asResponse { // we don't use http/1.1 request pipelining in the backend side.
		r.input = r.stockInput[:]
	} else { // must be http/1.x server side.
		// HTTP/1.1 servers support request pipelining, so input related are not set here.
	}
	r.array = r.stockArray[:]
	r.primes = r.stockPrimes[0:1:cap(r.stockPrimes)] // use append(). r.primes[0] is skipped due to zero value of pair indexes.
	r.extras = r.stockExtras[0:0:cap(r.stockExtras)] // use append()
	httpHolder := r.stream.Holder()
	r.recvTimeout = httpHolder.RecvTimeout()
	r.maxContentSize = httpHolder.MaxContentSize()
	r.maxMemoryContentSize = httpHolder.MaxMemoryContentSize()
	r.contentSize = -1 // no content
	r.httpVersion = httpVersion
	r.asResponse = asResponse
	r.keepAlive = -1 // no connection header field
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *_httpIn_) onEnd() { // for zeros
	if r.httpVersion >= Version2 || r.asResponse { // as we don't pipeline outgoing requests, incoming responses are not pipelined too.
		if cap(r.input) != cap(r.stockInput) {
			PutNK(r.input)
		}
		r.input = nil
		r.inputNext, r.inputEdge = 0, 0
	} else { // must be http/1.x server side.
		// HTTP/1.1 servers support request pipelining, so input related are not reset here.
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

	if r.inputNext != 0 { // only happens in HTTP/1.1 server side request pipelining
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
			// TODO: log err?
		}
		r.contentFile = nil
	}

	r._httpIn0 = _httpIn0{}
}

func (r *_httpIn_) UnsafeMake(size int) []byte { return r.stream.unsafeMake(size) }
func (r *_httpIn_) RemoteAddr() net.Addr       { return r.stream.remoteAddr() }

func (r *_httpIn_) VersionCode() uint8    { return r.httpVersion }
func (r *_httpIn_) IsHTTP1() bool         { return r.httpVersion <= Version1_1 }
func (r *_httpIn_) IsHTTP1_0() bool       { return r.httpVersion == Version1_0 }
func (r *_httpIn_) IsHTTP1_1() bool       { return r.httpVersion == Version1_1 }
func (r *_httpIn_) IsHTTP2() bool         { return r.httpVersion == Version2 }
func (r *_httpIn_) IsHTTP3() bool         { return r.httpVersion == Version3 }
func (r *_httpIn_) Version() string       { return httpVersionStrings[r.httpVersion] }
func (r *_httpIn_) UnsafeVersion() []byte { return httpVersionByteses[r.httpVersion] }

func (r *_httpIn_) KeepAlive() bool   { return r.keepAlive == 1 } // -1 was excluded priorly. either 0 or 1 here
func (r *_httpIn_) HeadResult() int16 { return r.headResult }
func (r *_httpIn_) BodyResult() int16 { return r.bodyResult }

func (r *_httpIn_) addHeaderLine(headerLine *pair) bool { // as prime
	if edge, ok := r._addPrime(headerLine); ok {
		r.headerLines.edge = edge
		return true
	}
	r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many header lines"
	return false
}
func (r *_httpIn_) HasHeaders() bool {
	return r.hasPairs(r.headerLines, pairHeader)
}
func (r *_httpIn_) AllHeaderLines() (headerLines [][2]string) {
	return r.allPairs(r.headerLines, pairHeader)
}
func (r *_httpIn_) H(name string) string {
	value, _ := r.Header(name)
	return value
}
func (r *_httpIn_) Hstr(name string, defaultValue string) string {
	if value, ok := r.Header(name); ok {
		return value
	}
	return defaultValue
}
func (r *_httpIn_) Hint(name string, defaultValue int) int {
	if value, ok := r.Header(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *_httpIn_) Header(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.headerLines, pairHeader)
	return string(v), ok
}
func (r *_httpIn_) UnsafeHeader(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.headerLines, pairHeader)
}
func (r *_httpIn_) Headers(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.headerLines, pairHeader)
}
func (r *_httpIn_) HasHeader(name string) bool {
	_, ok := r.getPair(name, 0, r.headerLines, pairHeader)
	return ok
}
func (r *_httpIn_) DelHeader(name string) (deleted bool) {
	// TODO: add restrictions on what header fields are allowed to del?
	return r.delPair(name, 0, r.headerLines, pairHeader)
}
func (r *_httpIn_) delHeader(name []byte, nameHash uint16) {
	r.delPair(WeakString(name), nameHash, r.headerLines, pairHeader)
}
func (r *_httpIn_) AddHeader(name string, value string) bool { // as extra, by webapp
	// TODO: add restrictions on what header fields are allowed to add? should we check the value?
	// TODO: parse and check? setFlags?
	return r.addExtra(name, value, 0, pairHeader)
}

func (r *_httpIn_) _splitFieldLine(field *pair, fdesc *fdesc, p []byte) bool { // split: #element => [ element ] *( OWS "," OWS [ element ] )
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
		if !r._parseFieldLine(&subField, fdesc, p, false) { // parse one sub field
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
func (r *_httpIn_) _parseFieldLine(field *pair, fdesc *fdesc, p []byte, fully bool) bool { // for field data and value params
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

func (r *_httpIn_) checkContentLength(headerLine *pair, lineIndex uint8) bool { // Content-Length = 1*DIGIT
	// RFC 9110 (section 8.6):
	// Likewise, a sender MUST NOT forward a message with a Content-Length
	// header field value that does not match the ABNF above, with one
	// exception: a recipient of a Content-Length header field value
	// consisting of the same decimal value repeated as a comma-separated
	// list (e.g, "Content-Length: 42, 42") MAY either reject the message as
	// invalid or replace that invalid field value with a single instance of
	// the decimal value, since this likely indicates that a duplicate was
	// generated or combined by an upstream message processor.
	if r.contentSize == -1 { // r.contentSize can only be -1 or >= 0 here. -2 is set after the header section is received if the content is vague
		if size, ok := decToI64(headerLine.valueAt(r.input)); ok {
			r.contentSize = size
			r.iContentLength = lineIndex
			return true
		}
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or duplicate content-length"
	return false
}
func (r *_httpIn_) checkContentLocation(headerLine *pair, lineIndex uint8) bool { // Content-Location = absolute-URI / partial-URI
	if r.iContentLocation == 0 && headerLine.value.notEmpty() {
		// TODO: check syntax
		r.iContentLocation = lineIndex
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or duplicate content-location"
	return false
}
func (r *_httpIn_) checkContentRange(headerLine *pair, lineIndex uint8) bool { // Content-Range = range-unit SP ( range-resp / unsatisfied-range )
	if r.iContentRange == 0 && headerLine.value.notEmpty() {
		// TODO: check syntax
		r.iContentRange = lineIndex
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or duplicate content-range"
	return false
}
func (r *_httpIn_) checkContentType(headerLine *pair, lineIndex uint8) bool { // Content-Type = media-type
	// media-type = type "/" subtype *( OWS ";" OWS parameter )
	// type = token
	// subtype = token
	// parameter = token "=" ( token / quoted-string )
	if r.iContentType == 0 && !headerLine.dataEmpty() {
		// TODO: check syntax
		r.iContentType = lineIndex
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or duplicate content-type"
	return false
}
func (r *_httpIn_) checkDate(headerLine *pair, lineIndex uint8) bool { // Date = HTTP-date
	return r._checkHTTPDate(headerLine, lineIndex, &r.iDate, &r.dateTime)
}
func (r *_httpIn_) _checkHTTPDate(headerLine *pair, lineIndex uint8, pIndex *uint8, toTime *int64) bool { // HTTP-date = day-name "," SP day SP month SP year SP hour ":" minute ":" second SP GMT
	if *pIndex == 0 {
		if httpDate, ok := clockParseHTTPDate(headerLine.valueAt(r.input)); ok {
			*pIndex = lineIndex
			*toTime = httpDate
			return true
		}
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or duplicate http-date"
	return false
}

func (r *_httpIn_) checkAccept(subLines []pair, subFrom uint8, subEdge uint8) bool { // Accept = #( media-range [ weight ] )
	if r.zAccept.isEmpty() {
		r.zAccept.from = subFrom
	}
	r.zAccept.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *_httpIn_) checkAcceptEncoding(subLines []pair, subFrom uint8, subEdge uint8) bool { // Accept-Encoding = #( codings [ weight ] )
	if r.zAcceptEncoding.isEmpty() {
		r.zAcceptEncoding.from = subFrom
	}
	r.zAcceptEncoding.edge = subEdge
	// codings = content-coding / "identity" / "*"
	// content-coding = token
	for i := subFrom; i < subEdge; i++ {
		if r.numAcceptCodings == int8(cap(r.acceptCodings)) {
			break // ignore too many codings
		}
		subLine := &subLines[i]
		if subLine.kind != pairHeader {
			continue
		}
		subData := subLine.dataAt(r.input)
		bytesToLower(subData)
		var coding uint8
		if bytes.Equal(subData, bytesGzip) {
			r.acceptGzip = true
			coding = httpCodingGzip
		} else if bytes.Equal(subData, bytesBrotli) {
			r.acceptBrotli = true
			coding = httpCodingBrotli
		} else if bytes.Equal(subData, bytesDeflate) {
			coding = httpCodingDeflate
		} else if bytes.Equal(subData, bytesCompress) {
			coding = httpCodingCompress
		} else if bytes.Equal(subData, bytesIdentity) {
			coding = httpCodingIdentity
		} else {
			coding = httpCodingUnknown
		}
		r.acceptCodings[r.numAcceptCodings] = coding
		r.numAcceptCodings++
	}
	return true
}
func (r *_httpIn_) checkConnection(subLines []pair, subFrom uint8, subEdge uint8) bool { // Connection = #connection-option
	if r.httpVersion >= Version2 {
		r.headResult, r.failReason = StatusBadRequest, "connection header field is not allowed in HTTP/2 and HTTP/3"
		return false
	}
	if r.zConnection.isEmpty() {
		r.zConnection.from = subFrom
	}
	r.zConnection.edge = subEdge
	// connection-option = token
	for i := subFrom; i < subEdge; i++ {
		subData := subLines[i].dataAt(r.input)
		bytesToLower(subData) // connection options are case-insensitive.
		if bytes.Equal(subData, bytesClose) {
			r.keepAlive = 0
		} else {
			// We don't support "keep-alive" connection option as it's not formal in HTTP/1.0.
		}
	}
	return true
}
func (r *_httpIn_) checkContentEncoding(subLines []pair, subFrom uint8, subEdge uint8) bool { // Content-Encoding = #content-coding
	if r.zContentEncoding.isEmpty() {
		r.zContentEncoding.from = subFrom
	}
	r.zContentEncoding.edge = subEdge
	// content-coding = token
	for i := subFrom; i < subEdge; i++ {
		if r.numContentCodings == int8(cap(r.contentCodings)) {
			r.headResult, r.failReason = StatusBadRequest, "too many content codings applied to content"
			return false
		}
		subData := subLines[i].dataAt(r.input)
		bytesToLower(subData)
		var coding uint8
		if bytes.Equal(subData, bytesGzip) {
			coding = httpCodingGzip
		} else if bytes.Equal(subData, bytesBrotli) {
			coding = httpCodingBrotli
		} else if bytes.Equal(subData, bytesDeflate) { // this is in fact zlib format
			coding = httpCodingDeflate // some non-conformant implementations send the "deflate" compressed data without the zlib wrapper :(
		} else if bytes.Equal(subData, bytesCompress) {
			coding = httpCodingCompress
		} else {
			coding = httpCodingUnknown
		}
		r.contentCodings[r.numContentCodings] = coding
		r.numContentCodings++
	}
	return true
}
func (r *_httpIn_) checkContentLanguage(subLines []pair, subFrom uint8, subEdge uint8) bool { // Content-Language = #language-tag
	if r.zContentLanguage.isEmpty() {
		r.zContentLanguage.from = subFrom
	}
	r.zContentLanguage.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *_httpIn_) checkKeepAlive(subLines []pair, subFrom uint8, subEdge uint8) bool { // Keep-Alive = #keepalive-param
	if r.zKeepAlive.isEmpty() {
		r.zKeepAlive.from = subFrom
	}
	r.zKeepAlive.edge = subEdge
	return true
}
func (r *_httpIn_) checkProxyConnection(subLines []pair, subFrom uint8, subEdge uint8) bool { // Proxy-Connection = #connection-option
	if r.zProxyConnection.isEmpty() {
		r.zProxyConnection.from = subFrom
	}
	r.zProxyConnection.edge = subEdge
	return true
}
func (r *_httpIn_) checkTrailer(subLines []pair, subFrom uint8, subEdge uint8) bool { // Trailer = #field-name
	if r.zTrailer.isEmpty() {
		r.zTrailer.from = subFrom
	}
	r.zTrailer.edge = subEdge
	// field-name = token
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *_httpIn_) checkTransferEncoding(subLines []pair, subFrom uint8, subEdge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.httpVersion != Version1_1 {
		r.headResult, r.failReason = StatusBadRequest, "transfer-encoding is only allowed in http/1.1"
		return false
	}
	if r.zTransferEncoding.isEmpty() {
		r.zTransferEncoding.from = subFrom
	}
	r.zTransferEncoding.edge = subEdge
	// transfer-coding = "chunked" / "compress" / "deflate" / "gzip"
	for i := subFrom; i < subEdge; i++ {
		subData := subLines[i].dataAt(r.input)
		bytesToLower(subData)
		if bytes.Equal(subData, bytesChunked) {
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
func (r *_httpIn_) checkVia(subLines []pair, subFrom uint8, subEdge uint8) bool { // Via = #( received-protocol RWS received-by [ RWS comment ] )
	if r.zVia.isEmpty() {
		r.zVia.from = subFrom
	}
	r.zVia.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}

func (r *_httpIn_) determineContentMode() bool {
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
	} else if r.httpVersion >= Version2 && r.contentSize == -1 { // no content-length header field
		// TODO: if there is no content, HTTP/2 and HTTP/3 should mark END_STREAM in fields frame. use this to decide!
		r.contentSize = -2 // if there is no content-length in HTTP/2 or HTTP/3, we treat it as vague
	}
	return true
}
func (r *_httpIn_) IsVague() bool { return r.contentSize == -2 }

func (r *_httpIn_) ContentIsEncoded() bool { return r.zContentEncoding.notEmpty() }
func (r *_httpIn_) ContentSize() int64     { return r.contentSize }
func (r *_httpIn_) ContentType() string    { return string(r.UnsafeContentType()) }
func (r *_httpIn_) UnsafeContentLength() []byte {
	if r.iContentLength == 0 {
		return nil
	}
	return r.primes[r.iContentLength].valueAt(r.input)
}
func (r *_httpIn_) UnsafeContentType() []byte {
	if r.iContentType == 0 {
		return nil
	}
	return r.primes[r.iContentType].dataAt(r.input)
}

func (r *_httpIn_) SetRecvTimeout(timeout time.Duration) { r.recvTimeout = timeout }
func (r *_httpIn_) unsafeContent() []byte { // load message content into memory
	r._loadContent()
	if r.stream.isBroken() {
		return nil
	}
	return r.contentText[0:r.receivedSize]
}
func (r *_httpIn_) _loadContent() { // into memory. [0, r.maxContentSize]
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
func (r *_httpIn_) _dropContent() { // if message content is not received, this will be called at last
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
func (r *_httpIn_) _recvContent(retain bool) any { // to []byte (for small content <= 64K1) or tempFile (for large content > 64K1, or content is vague)
	if r.contentSize > 0 && r.contentSize <= _64K1 { // (0, 64K1]. save to []byte. the whole content is small so must be received in one read timeout
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
			data, err = r.in.readContent()
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

func (r *_httpIn_) addTrailerLine(trailerLine *pair) bool { // as prime
	if edge, ok := r._addPrime(trailerLine); ok {
		r.trailerLines.edge = edge
		return true
	}
	r.bodyResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many trailer lines"
	return false
}
func (r *_httpIn_) HasTrailers() bool {
	return r.hasPairs(r.trailerLines, pairTrailer)
}
func (r *_httpIn_) AllTrailerLines() (trailerLines [][2]string) {
	return r.allPairs(r.trailerLines, pairTrailer)
}
func (r *_httpIn_) T(name string) string {
	value, _ := r.Trailer(name)
	return value
}
func (r *_httpIn_) Tstr(name string, defaultValue string) string {
	if value, ok := r.Trailer(name); ok {
		return value
	}
	return defaultValue
}
func (r *_httpIn_) Tint(name string, defaultValue int) int {
	if value, ok := r.Trailer(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *_httpIn_) Trailer(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.trailerLines, pairTrailer)
	return string(v), ok
}
func (r *_httpIn_) UnsafeTrailer(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.trailerLines, pairTrailer)
}
func (r *_httpIn_) Trailers(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.trailerLines, pairTrailer)
}
func (r *_httpIn_) HasTrailer(name string) bool {
	_, ok := r.getPair(name, 0, r.trailerLines, pairTrailer)
	return ok
}
func (r *_httpIn_) DelTrailer(name string) (deleted bool) {
	return r.delPair(name, 0, r.trailerLines, pairTrailer)
}
func (r *_httpIn_) delTrailer(name []byte, nameHash uint16) {
	r.delPair(WeakString(name), nameHash, r.trailerLines, pairTrailer)
}
func (r *_httpIn_) AddTrailer(name string, value string) bool { // as extra, by webapp
	// TODO: add restrictions on what trailer fields are allowed to add? should we check the value?
	// TODO: parse and check? setFlags?
	return r.addExtra(name, value, 0, pairTrailer)
}

func (r *_httpIn_) _addPrime(prime *pair) (edge uint8, ok bool) {
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
func (r *_httpIn_) _delPrime(i uint8) { r.primes[i].zero() }

func (r *_httpIn_) addExtra(name string, value string, nameHash uint16, extraKind int8) bool {
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
func (r *_httpIn_) _addExtra(extra *pair) bool {
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

func (r *_httpIn_) hasPairs(primes zone, extraKind int8) bool {
	return primes.notEmpty() || r.hasExtra[extraKind]
}
func (r *_httpIn_) allPairs(primes zone, extraKind int8) [][2]string {
	var pairs [][2]string
	if extraKind == pairHeader || extraKind == pairTrailer { // skip sub field lines, only collects values of main field lines
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
func (r *_httpIn_) getPair(name string, nameHash uint16, primes zone, extraKind int8) (value []byte, ok bool) {
	if name == "" {
		return
	}
	if nameHash == 0 {
		nameHash = stringHash(name)
	}
	if extraKind == pairHeader || extraKind == pairTrailer { // skip comma field lines, only collects data of field lines without comma
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash == nameHash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					if !prime.isParsed() && !r._splitFieldLine(prime, defaultFdesc, p) {
						continue
					}
					if !prime.isCommaValue() { // not a comma field, collect it
						return prime.dataAt(p), true
					}
				}
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && !extra.isCommaValue() { // not a comma field, collect it
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
func (r *_httpIn_) getPairs(name string, nameHash uint16, primes zone, extraKind int8) (values []string, ok bool) {
	if name == "" {
		return
	}
	if nameHash == 0 {
		nameHash = stringHash(name)
	}
	if extraKind == pairHeader || extraKind == pairTrailer { // skip comma field lines, only collects data of field lines without comma
		for i := primes.from; i < primes.edge; i++ {
			if prime := &r.primes[i]; prime.nameHash == nameHash {
				if p := r._placeOf(prime); prime.nameEqualString(p, name) {
					if !prime.isParsed() && !r._splitFieldLine(prime, defaultFdesc, p) {
						continue
					}
					if !prime.isCommaValue() { // not a comma field, collect it
						values = append(values, string(prime.dataAt(p)))
					}
				}
			}
		}
		if r.hasExtra[extraKind] {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.nameHash == nameHash && extra.kind == extraKind && !extra.isCommaValue() { // not a comma field, collect it
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
func (r *_httpIn_) delPair(name string, nameHash uint16, primes zone, extraKind int8) (deleted bool) {
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
func (r *_httpIn_) _placeOf(pair *pair) []byte {
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

func (r *_httpIn_) proxyTakeContent() any {
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

func (r *_httpIn_) proxyDelHopHeaderFields() {
	r._proxyDelHopFieldLines(r.headerLines, pairHeader)
}
func (r *_httpIn_) proxyDelHopTrailerFields() {
	r._proxyDelHopFieldLines(r.trailerLines, pairTrailer)
}
func (r *_httpIn_) _proxyDelHopFieldLines(fieldLines zone, extraKind int8) { // TODO: improve performance
	delField := r.delHeader
	if extraKind == pairTrailer {
		delField = r.delTrailer
	}
	// These fields should be removed anyway: proxy-connection, keep-alive, te, transfer-encoding, upgrade
	if r.zProxyConnection.notEmpty() {
		delField(bytesProxyConnection, hashProxyConnection)
	}
	if r.zKeepAlive.notEmpty() {
		delField(bytesKeepAlive, hashKeepAlive)
	}
	if r.zTransferEncoding.notEmpty() {
		delField(bytesTransferEncoding, hashTransferEncoding)
	}
	if r.zUpgrade.notEmpty() {
		delField(bytesUpgrade, hashUpgrade)
	}
	r.in.proxyDelHopFieldLines(extraKind) // don't pass delField as parameter, it causes delField escapes to heap

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
		for j := fieldLines.from; j < fieldLines.edge; j++ {
			if fieldLine := &r.primes[j]; fieldLine.nameHash == optionHash && fieldLine.nameEqualBytes(p, optionName) {
				fieldLine.zero()
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

func (r *_httpIn_) proxyWalkHeaderLines(out httpOut, callback func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool) bool { // excluding sub header lines
	return r._proxyWalkMainFields(r.headerLines, pairHeader, out, callback)
}
func (r *_httpIn_) proxyWalkTrailerLines(out httpOut, callback func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool) bool { // excluding sub trailer lines
	return r._proxyWalkMainFields(r.trailerLines, pairTrailer, out, callback)
}
func (r *_httpIn_) _proxyWalkMainFields(fieldLines zone, extraKind int8, out httpOut, callback func(out httpOut, fieldLine *pair, fieldName []byte, lineValue []byte) bool) bool {
	for i := fieldLines.from; i < fieldLines.edge; i++ {
		if fieldLine := &r.primes[i]; fieldLine.nameHash != 0 {
			p := r._placeOf(fieldLine)
			if !callback(out, fieldLine, fieldLine.nameAt(p), fieldLine.valueAt(p)) {
				return false
			}
		}
	}
	if r.hasExtra[extraKind] {
		for i := 0; i < len(r.extras); i++ {
			if field := &r.extras[i]; field.nameHash != 0 && field.kind == extraKind && !field.isSubField() {
				if !callback(out, field, field.nameAt(r.array), field.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *_httpIn_) arrayCopy(src []byte) bool { // callers don't guarantee the intended memory cost is limited
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
func (r *_httpIn_) arrayPush(b byte) { // callers must ensure the intended memory cost is limited
	r.array[r.arrayEdge] = b
	if r.arrayEdge++; r.arrayEdge == int32(cap(r.array)) {
		r._growArray(1)
	}
}
func (r *_httpIn_) _growArray(size int32) bool { // stock(<4K)->4K->16K->64K1->(128K->...->1G)
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

func (r *_httpIn_) saveContentFilesDir() string { return r.stream.Holder().SaveContentFilesDir() }

func (r *_httpIn_) _newTempFile(retain bool) (tempFile, error) { // to save content to
	if retain {
		filesDir := r.saveContentFilesDir()
		pathBuffer := r.UnsafeMake(len(filesDir) + 19) // 19 bytes is enough for an int64
		n := copy(pathBuffer, filesDir)
		n += r.stream.MakeTempName(pathBuffer[n:], time.Now().Unix())
		return os.OpenFile(WeakString(pathBuffer[:n]), os.O_RDWR|os.O_CREATE, 0644)
	} else { // since data is not used by upper caller, we don't need to actually write data to file.
		return fakeFile, nil
	}
}

func (r *_httpIn_) _isLongTime() bool { // reports whether the receiving of incoming content costs a long time
	return r.recvTimeout > 0 && time.Since(r.bodyTime) >= r.recvTimeout
}

var ( // _httpIn_ errors
	httpInBadChunk = errors.New("bad incoming http chunk")
	httpInLongTime = errors.New("http incoming costs a long time")
)

// httpOut
type httpOut interface {
	controlData() []byte
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

// _httpOut_ is a mixin.
type _httpOut_ struct { // for serverResponse_ and backendRequest_. outgoing, needs building
	// Assocs
	stream httpStream // *backend[1-3]Stream, *server[1-3]Stream
	out    httpOut    // *backend[1-3]Request, *server[1-3]Response
	// Stream states (stocks)
	stockFields [1536]byte // for r.fields
	// Stream states (controlled)
	edges [128]uint16 // edges of header fields or trailer fields in r.fields, but not used at the same time. controlled by r.numHeaderFields or r.numTrailerFields. edges[0] is not used!
	piece Piece       // for r.chain. used when sending content or echoing chunks
	chain Chain       // outgoing piece chain. used when sending content or echoing chunks
	// Stream states (non-zeros)
	fields           []byte        // bytes of the header fields or trailer fields which are not manipulated at the same time. [<r.stockFields>/4K/16K]
	sendTimeout      time.Duration // timeout to send the whole message. zero means no timeout
	contentSize      int64         // info of outgoing content. -1: not set, -2: vague, >=0: size
	httpVersion      uint8         // Version1_1, Version2, Version3
	asRequest        bool          // treat this outgoing message as request?
	numHeaderFields  uint8         // 1+num of added header fields, starts from 1 because edges[0] is not used
	numTrailerFields uint8         // 1+num of added trailer fields, starts from 1 because edges[0] is not used
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
	fieldsEdge    uint16 // edge of r.fields. max size of r.fields must be <= 16K. used by both header fields and trailer fields because they are not manipulated at the same time
	hasRevisers   bool   // are there any outgoing revisers hooked on this outgoing message?
	isSent        bool   // whether the message is sent
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
	iContentType  uint8  // position of content-type in r.edges
	iDate         uint8  // position of date in r.edges
}

func (r *_httpOut_) onUse(httpVersion uint8, asRequest bool) { // for non-zeros
	r.fields = r.stockFields[:]
	httpHolder := r.stream.Holder()
	r.sendTimeout = httpHolder.SendTimeout()
	r.contentSize = -1 // not set
	r.httpVersion = httpVersion
	r.asRequest = asRequest
	r.numHeaderFields, r.numTrailerFields = 1, 1 // r.edges[0] is not used
}
func (r *_httpOut_) onEnd() { // for zeros
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

func (r *_httpOut_) unsafeMake(size int) []byte { return r.stream.unsafeMake(size) }

func (r *_httpOut_) AddContentType(contentType string) bool {
	return r.AddHeaderBytes(bytesContentType, ConstBytes(contentType))
}
func (r *_httpOut_) AddContentTypeBytes(contentType []byte) bool {
	return r.AddHeaderBytes(bytesContentType, contentType)
}

func (r *_httpOut_) Header(name string) (value string, ok bool) {
	v, ok := r.out.header(ConstBytes(name))
	return string(v), ok
}
func (r *_httpOut_) HasHeader(name string) bool {
	return r.out.hasHeader(ConstBytes(name))
}
func (r *_httpOut_) AddHeader(name string, value string) bool {
	return r.AddHeaderBytes(ConstBytes(name), ConstBytes(value))
}
func (r *_httpOut_) AddHeaderBytes(name []byte, value []byte) bool {
	nameHash, valid, lowerName := r._nameCheck(name)
	if !valid {
		return false
	}
	for _, b := range value { // to prevent response splitting
		if b == '\r' || b == '\n' {
			return false
		}
	}
	return r.out.insertHeader(nameHash, lowerName, value) // some header fields (e.g. "connection") are restricted
}
func (r *_httpOut_) DelHeader(name string) bool {
	return r.DelHeaderBytes(ConstBytes(name))
}
func (r *_httpOut_) DelHeaderBytes(name []byte) bool {
	nameHash, valid, lowerName := r._nameCheck(name)
	if !valid {
		return false
	}
	return r.out.removeHeader(nameHash, lowerName)
}
func (r *_httpOut_) _nameCheck(fieldName []byte) (nameHash uint16, valid bool, lowerName []byte) { // TODO: improve performance
	nameSize := len(fieldName)
	if nameSize == 0 || nameSize > 255 {
		return 0, false, nil
	}
	allLower := true
	for i := 0; i < nameSize; i++ {
		if b := fieldName[i]; b >= 'a' && b <= 'z' || b == '-' {
			nameHash += uint16(b)
		} else {
			nameHash = 0
			allLower = false
			break
		}
	}
	if allLower {
		return nameHash, true, fieldName
	}
	nameBuffer := r.stream.buffer256() // enough for name
	for i := 0; i < nameSize; i++ {
		b := fieldName[i]
		if b >= 'A' && b <= 'Z' {
			b += 0x20 // to lower
		} else if !(b >= 'a' && b <= 'z' || b == '-') {
			return 0, false, nil
		}
		nameHash += uint16(b)
		nameBuffer[i] = b
	}
	return nameHash, true, nameBuffer[:nameSize]
}

func (r *_httpOut_) isVague() bool { return r.contentSize == -2 }
func (r *_httpOut_) IsSent() bool  { return r.isSent }

func (r *_httpOut_) _insertContentType(contentType []byte) (ok bool) {
	return r._appendSingleton(&r.iContentType, bytesContentType, contentType)
}
func (r *_httpOut_) _insertDate(date []byte) (ok bool) { // rarely used in backend request
	return r._appendSingleton(&r.iDate, bytesDate, date)
}
func (r *_httpOut_) _appendSingleton(pIndex *uint8, name []byte, value []byte) bool {
	if *pIndex > 0 || !r.out.addHeader(name, value) {
		return false
	}
	*pIndex = r.numHeaderFields - 1 // r.numHeaderFields begins from 1, so must minus one
	return true
}

func (r *_httpOut_) _removeContentType() (deleted bool) { return r._deleteSingleton(&r.iContentType) }
func (r *_httpOut_) _removeDate() (deleted bool)        { return r._deleteSingleton(&r.iDate) }
func (r *_httpOut_) _deleteSingleton(pIndex *uint8) bool {
	if *pIndex == 0 { // not exist
		return false
	}
	r.out.delHeaderAt(*pIndex)
	*pIndex = 0
	return true
}

func (r *_httpOut_) _setUnixTime(pUnixTime *int64, pIndex *uint8, unixTime int64) bool {
	if unixTime < 0 {
		return false
	}
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.out.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = unixTime
	return true
}
func (r *_httpOut_) _addUnixTime(pUnixTime *int64, pIndex *uint8, name []byte, httpDate []byte) bool {
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.out.delHeaderAt(*pIndex)
		*pIndex = 0
	} else { // >= 0 or -1
		*pUnixTime = -2
	}
	if !r.out.addHeader(name, httpDate) {
		return false
	}
	*pIndex = r.numHeaderFields - 1 // r.numHeaderFields begins from 1, so must minus one
	return true
}
func (r *_httpOut_) _delUnixTime(pUnixTime *int64, pIndex *uint8) bool {
	if *pUnixTime == -1 {
		return false
	}
	if *pUnixTime == -2 { // was set through general api, must delete it
		r.out.delHeaderAt(*pIndex)
		*pIndex = 0
	}
	*pUnixTime = -1
	return true
}

func (r *_httpOut_) pickOutRanges(contentRanges []Range, rangeType string) {
	r.contentRanges = contentRanges
	r.rangeType = rangeType
}

func (r *_httpOut_) SetSendTimeout(timeout time.Duration) { r.sendTimeout = timeout }

func (r *_httpOut_) Send(content string) error      { return r.sendText(ConstBytes(content)) }
func (r *_httpOut_) SendBytes(content []byte) error { return r.sendText(content) }
func (r *_httpOut_) SendFile(contentPath string) error {
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
func (r *_httpOut_) SendJSON(content any) error { // TODO: optimize performance
	r.AddContentTypeBytes(bytesTypeJSON)
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	return r.sendText(data)
}

func (r *_httpOut_) Echo(chunk string) error      { return r.echoText(ConstBytes(chunk)) }
func (r *_httpOut_) EchoBytes(chunk []byte) error { return r.echoText(chunk) }
func (r *_httpOut_) EchoFile(chunkPath string) error {
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
func (r *_httpOut_) AddTrailer(name string, value string) bool {
	return r.AddTrailerBytes(ConstBytes(name), ConstBytes(value))
}
func (r *_httpOut_) AddTrailerBytes(name []byte, value []byte) bool {
	if !r.isSent { // trailer fields must be added after header fields & content was sent, otherwise r.fields will be messed up
		return false
	}
	return r.out.addTrailer(name, value)
}
func (r *_httpOut_) Trailer(name string) (value string, ok bool) {
	v, ok := r.out.trailer(ConstBytes(name))
	return string(v), ok
}

func (r *_httpOut_) _proxyPassMessage(in httpIn) error {
	proxyPass := r.out.proxyPassBytes
	if in.IsVague() || r.hasRevisers { // if we need to revise, we always use vague no matter the original content is sized or vague
		proxyPass = r.EchoBytes
	} else { // in is sized and there are no revisers, use proxyPassBytes
		r.isSent = true
		r.contentSize = in.ContentSize()
		// TODO: find a way to reduce i/o syscalls if content is small?
		if err := r.out.proxyPassHeaders(); err != nil {
			return err
		}
	}
	for {
		data, err := in.readContent()
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
	if in.HasTrailers() {
		if !in.proxyWalkTrailerLines(r.out, func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool {
			return out.addTrailer(trailerName, lineValue) // added trailer fields will be written by upper code eventually.
		}) {
			return httpOutTrailerFailed
		}
	}
	return nil
}
func (r *_httpOut_) proxyPostMessage(content any, hasTrailers bool) error {
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
		return r.out.doSend()
	}
}

func (r *_httpOut_) sendText(content []byte) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content)) // initial size, may be changed by revisers
	return r.out.doSend()
}
func (r *_httpOut_) sendFile(content *os.File, info os.FileInfo, shut bool) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	r.piece.SetFile(content, info, shut)
	r.chain.PushTail(&r.piece)
	r.contentSize = info.Size() // initial size, may be changed by revisers
	return r.out.doSend()
}
func (r *_httpOut_) _beforeSend() error {
	if r.isSent {
		return httpOutAlreadySent
	}
	r.isSent = true
	if r.hasRevisers {
		r.out.beforeSend()
	}
	return nil
}

func (r *_httpOut_) echoText(chunk []byte) error {
	if err := r._beforeEcho(); err != nil {
		return err
	}
	if len(chunk) == 0 { // empty chunk is not actually sent, since it is used to indicate the end. pretend to succeed
		return nil
	}
	r.piece.SetText(chunk)
	defer r.piece.zero()
	return r.out.doEcho()
}
func (r *_httpOut_) echoFile(chunk *os.File, info os.FileInfo, shut bool) error {
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
	return r.out.doEcho()
}
func (r *_httpOut_) _beforeEcho() error {
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
		r.out.beforeEcho()
	}
	return r.out.echoHeaders()
}

func (r *_httpOut_) growHeaders(size int) (from int, edge int, ok bool) { // header fields and trailer fields are not manipulated at the same time
	if r.numHeaderFields == uint8(cap(r.edges)) { // too many header fields
		return
	}
	return r._growFields(size)
}
func (r *_httpOut_) growTrailers(size int) (from int, edge int, ok bool) { // header fields and trailer fields are not manipulated at the same time
	if r.numTrailerFields == uint8(cap(r.edges)) { // too many trailer fields
		return
	}
	return r._growFields(size)
}
func (r *_httpOut_) _growFields(size int) (from int, edge int, ok bool) { // used by growHeaders first and growTrailers later as they are not manipulated at the same time
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

func (r *_httpOut_) _longTimeCheck(err error) error {
	if err == nil && r._isLongTime() {
		err = httpOutLongTime
	}
	if err != nil {
		r.stream.markBroken()
	}
	return err
}
func (r *_httpOut_) _isLongTime() bool { // reports whether the sending of outgoing content costs a long time
	return r.sendTimeout > 0 && time.Since(r.sendTime) >= r.sendTimeout
}

var ( // _httpOut_ errors
	httpOutLongTime      = errors.New("http outgoing costs a long time")
	httpOutWriteBroken   = errors.New("write broken")
	httpOutUnknownStatus = errors.New("unknown status")
	httpOutAlreadySent   = errors.New("already sent")
	httpOutTooLarge      = errors.New("content too large")
	httpOutMixedContent  = errors.New("mixed content mode")
	httpOutTrailerFailed = errors.New("add trailer failed")
)

// httpSocket
type httpSocket interface {
	Read(dst []byte) (int, error)
	Write(src []byte) (int, error)
	Close() error
}

// _httpSocket_ is a mixin.
type _httpSocket_ struct { // for serverSocket_ and backendSocket_. incoming and outgoing
	// Assocs
	stream httpStream // *backend[1-3]Stream, *server[1-3]Stream
	socket httpSocket // *backend[1-3]Socket, *server[1-3]Socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	asServer bool // treat this socket as a server socket?
	// Stream states (zeros)
	_httpSocket0 // all values in this struct must be zero by default!
}
type _httpSocket0 struct { // for fast reset, entirely
}

func (s *_httpSocket_) onUse(asServer bool) { // for non-zeros
	s.asServer = asServer
}
func (s *_httpSocket_) onEnd() { // for zeros
	s._httpSocket0 = _httpSocket0{}
}

func (s *_httpSocket_) todo() {
}

var ( // _httpSocket_ errors
	httpSocketWriteBroken = errors.New("write broken")
)
