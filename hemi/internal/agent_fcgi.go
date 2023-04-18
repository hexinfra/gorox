// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI agent handlet relays requests to backend FCGI servers and cache responses.

// FCGI is mainly used by PHP applications. It doesn't support HTTP trailers.
// And we don't use request-side chunking due to the limitation of CGI/1.1 even
// though FCGI can do that through its framing protocol. Perhaps most FCGI
// applications don't implement this feature either.

// In response side, FCGI applications mostly use "unsized" output.

// To avoid ambiguity, the term "content" in FCGI specification is called "payload" in our implementation.

package internal

import (
	"bytes"
	"errors"
	"github.com/hexinfra/gorox/hemi/common/risky"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("fcgiAgent", func(name string, stage *Stage, app *App) Handlet {
		h := new(fcgiAgent)
		h.onCreate(name, stage, app)
		return h
	})
}

// fcgiAgent handlet
type fcgiAgent struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	app     *App         // the app to which the agent belongs
	backend *TCPSBackend // ...
	cacher  Cacher       // the cacher which is used by this agent
	// States
	bufferClientContent bool          // client content is buffered anyway?
	bufferServerContent bool          // server content is buffered anyway?
	keepConn            bool          // instructs FCGI server to keep conn?
	scriptFilename      []byte        // for SCRIPT_FILENAME
	addRequestHeaders   [][2][]byte   // headers appended to client request
	delRequestHeaders   [][]byte      // client request headers to delete
	addResponseHeaders  [][2][]byte   // headers appended to server response
	delResponseHeaders  [][]byte      // server response headers to delete
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max response content size allowed
	preferUnderscore    bool          // if header name "foo-bar" and "foo_bar" are both present, prefer "foo_bar" to "foo-bar"?
}

func (h *fcgiAgent) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *fcgiAgent) OnShutdown() {
	h.app.SubDone()
}

func (h *fcgiAgent) OnConfigure() {
	h.contentSaver_.onConfigure(h, TempDir()+"/fcgi/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				h.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for fcgiAgent\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for fcgiAgent")
	}
	// withCacher
	if v, ok := h.Find("withCacher"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCacher")
		}
	}
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// keepConn
	h.ConfigureBool("keepConn", &h.keepConn, false)
	// scriptFilename
	h.ConfigureBytes("scriptFilename", &h.scriptFilename, nil, nil)
	// sendTimeout
	h.ConfigureDuration("sendTimeout", &h.sendTimeout, func(value time.Duration) bool { return value >= 0 }, 60*time.Second)
	// recvTimeout
	h.ConfigureDuration("recvTimeout", &h.recvTimeout, func(value time.Duration) bool { return value >= 0 }, 60*time.Second)
	// maxContentSize
	h.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
	// preferUnderscore
	h.ConfigureBool("preferUnderscore", &h.preferUnderscore, false)
}
func (h *fcgiAgent) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *fcgiAgent) IsProxy() bool { return true }
func (h *fcgiAgent) IsCache() bool { return h.cacher != nil }

func (h *fcgiAgent) Handle(req Request, resp Response) (next bool) { // reverse only
	var (
		content  any
		fConn    *TConn
		fErr     error
		fContent any
	)

	hasContent := req.HasContent()
	if hasContent && (h.bufferClientContent || req.isUnsized()) { // including size 0
		content = req.takeContent()
		if content == nil { // take failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.keepConn {
		if fConn, fErr = h.backend.FetchConn(); fErr == nil {
			defer h.backend.StoreConn(fConn)
		}
	} else {
		if fConn, fErr = h.backend.Dial(); fErr == nil {
			defer fConn.Close()
		}
	}
	if fErr != nil {
		resp.SendBadGateway(nil)
		return
	}

	fStream := getFCGIStream(h, fConn)
	defer putFCGIStream(fStream)

	fReq := &fStream.request
	if !fReq.copyHeadFrom(req, h.scriptFilename) {
		fStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if hasContent && !h.bufferClientContent && !req.isUnsized() {
		fErr = fReq.pass(req)
	} else { // nil, []byte, tempFile
		fErr = fReq.post(content)
	}
	if fErr != nil {
		fStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}

	fResp := &fStream.response
	for { // until we found a non-1xx status (>= 200)
		fResp.recvHead()
		if fResp.headResult != StatusOK || fResp.status == StatusSwitchingProtocols { // websocket is not served in handlets.
			fStream.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		if fResp.status >= StatusOK {
			break
		}
		// We got 1xx
		if req.VersionCode() == Version1_0 {
			fStream.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.pass1xx(fResp) {
			fStream.markBroken()
			return
		}
		fResp.onEnd()
		fResp.onUse()
	}

	fHasContent := false
	if req.MethodCode() != MethodHEAD {
		fHasContent = fResp.hasContent()
	}
	if fHasContent && h.bufferServerContent { // including size 0
		fContent = fResp.takeContent()
		if fContent == nil { // take failed
			// fStream is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHeadFrom(fResp, nil) {
		fStream.markBroken()
		return
	}
	if fHasContent && !h.bufferServerContent {
		if err := resp.pass(fResp); err != nil {
			fStream.markBroken()
			return
		}
	} else if err := resp.post(fContent, false); err != nil { // false means no trailers
		return
	}

	return
}

// poolFCGIStream
var poolFCGIStream sync.Pool

func getFCGIStream(agent *fcgiAgent, conn *TConn) *fcgiStream {
	var stream *fcgiStream
	if x := poolFCGIStream.Get(); x == nil {
		stream = new(fcgiStream)
		req, resp := &stream.request, &stream.response
		req.stream = stream
		req.response = resp
		resp.stream = stream
	} else {
		stream = x.(*fcgiStream)
	}
	stream.onUse(agent, conn)
	return stream
}
func putFCGIStream(stream *fcgiStream) {
	stream.onEnd()
	poolFCGIStream.Put(stream)
}

// fcgiStream
type fcgiStream struct {
	// Assocs
	request  fcgiRequest  // the fcgi request
	response fcgiResponse // the fcgi response
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Stream states (controlled)
	// Stream states (non-zeros)
	agent  *fcgiAgent // associated agent
	conn   *TConn     // associated conn
	region Region     // a region-based memory pool
	// Stream states (zeros)
}

func (s *fcgiStream) onUse(agent *fcgiAgent, conn *TConn) {
	s.agent = agent
	s.conn = conn
	s.region.Init()
	s.request.onUse()
	s.response.onUse()
}
func (s *fcgiStream) onEnd() {
	s.request.onEnd()
	s.response.onEnd()
	s.region.Free()
	s.conn = nil
	s.agent = nil
}

func (s *fcgiStream) buffer256() []byte          { return s.stockBuffer[:] }
func (s *fcgiStream) unsafeMake(size int) []byte { return s.region.Make(size) }

func (s *fcgiStream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.MakeTempName(p, unixTime)
}

func (s *fcgiStream) setWriteDeadline(deadline time.Time) error {
	return s.conn.SetWriteDeadline(deadline)
}
func (s *fcgiStream) setReadDeadline(deadline time.Time) error {
	return s.conn.SetReadDeadline(deadline)
}

func (s *fcgiStream) write(p []byte) (int, error)                { return s.conn.Write(p) }
func (s *fcgiStream) writev(vector *net.Buffers) (int64, error)  { return s.conn.Writev(vector) }
func (s *fcgiStream) read(p []byte) (int, error)                 { return s.conn.Read(p) }
func (s *fcgiStream) readAtLeast(p []byte, min int) (int, error) { return s.conn.ReadAtLeast(p, min) }

func (s *fcgiStream) isBroken() bool { return s.conn.IsBroken() }
func (s *fcgiStream) markBroken()    { s.conn.MarkBroken() }

// poolFCGIParams
var poolFCGIParams sync.Pool

const fcgiMaxParams = _16K // max effective params

func getFCGIParams() []byte {
	if x := poolFCGIParams.Get(); x == nil {
		return make([]byte, fcgiMaxParams)
	} else {
		return x.([]byte)
	}
}
func putFCGIParams(params []byte) {
	if cap(params) != fcgiMaxParams {
		BugExitln("fcgi: bad params")
	}
	poolFCGIParams.Put(params)
}

// fcgiRequest
type fcgiRequest struct { // outgoing. needs building
	// Assocs
	stream   *fcgiStream
	response *fcgiResponse
	// States (stocks)
	stockParams [_2K]byte // for r.params
	// States (controlled)
	paramsHeader [8]byte // used by params record
	stdinHeader  [8]byte // used by stdin record
	// States (non-zeros)
	params      []byte        // place the payload of exactly one FCGI_PARAMS record. [<r.stockParams>/16K]
	sendTimeout time.Duration // timeout to send the whole request
	// States (zeros)
	sendTime     time.Time   // the time when first send operation is performed
	vector       net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector  [7][]byte   // for sending request. reset after stream. 120B
	fcgiRequest0             // all values must be zero by default in this struct!
}
type fcgiRequest0 struct { // for fast reset, entirely
	paramsEdge    uint16 // edge of r.params. max size of r.params must be <= fcgiMaxParams.
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
}

func (r *fcgiRequest) onUse() {
	copy(r.paramsHeader[:], fcgiEmptyParams) // payloadLen (r.paramsHeader[4:6]) needs modification on using
	copy(r.stdinHeader[:], fcgiEmptyStdin)   // payloadLen (r.stdinHeader[4:6]) needs modification for every stdin record on using
	r.params = r.stockParams[:]
	r.sendTimeout = r.stream.agent.sendTimeout
}
func (r *fcgiRequest) onEnd() {
	if cap(r.params) != cap(r.stockParams) {
		putFCGIParams(r.params)
		r.params = nil
	}
	r.sendTime = time.Time{}
	r.vector = nil
	r.fixedVector = [7][]byte{}

	r.fcgiRequest0 = fcgiRequest0{}
}

func (r *fcgiRequest) copyHeadFrom(req Request, scriptFilename []byte) bool {
	var value []byte

	// Add meta params
	if !r._addMetaParam(fcgiBytesGatewayInterface, fcgiBytesCGI1_1) { // GATEWAY_INTERFACE
		return false
	}
	if !r._addMetaParam(fcgiBytesServerSoftware, bytesGorox) { // SERVER_SOFTWARE
		return false
	}
	if !r._addMetaParam(fcgiBytesServerProtocol, req.UnsafeVersion()) { // SERVER_PROTOCOL
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestMethod, req.UnsafeMethod()) { // REQUEST_METHOD
		return false
	}
	if len(scriptFilename) == 0 {
		value = req.unsafeAbsPath()
	} else {
		value = scriptFilename
	}
	if !r._addMetaParam(fcgiBytesScriptFilename, value) { // SCRIPT_FILENAME
		return false
	}
	if !r._addMetaParam(fcgiBytesScriptName, req.UnsafePath()) { // SCRIPT_NAME
		return false
	}
	if value = req.UnsafeQueryString(); len(value) > 1 {
		value = value[1:] // excluding '?'
	} else {
		value = nil
	}
	if !r._addMetaParam(fcgiBytesQueryString, value) { // QUERY_STRING
		return false
	}
	if value = req.UnsafeContentLength(); value != nil && !r._addMetaParam(fcgiBytesContentLength, value) { // CONTENT_LENGTH
		return false
	}
	if value = req.UnsafeContentType(); value != nil && !r._addMetaParam(fcgiBytesContentType, value) { // CONTENT_TYPE
		return false
	}
	if req.IsHTTPS() && !r._addMetaParam(fcgiBytesHTTPS, fcgiBytesON) { // HTTPS
		return false
	}

	// Add http params
	if !req.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r._addHTTPParam(header, name, value)
	}) {
		return false
	}

	r.paramsHeader[4], r.paramsHeader[5] = byte(r.paramsEdge>>8), byte(r.paramsEdge)

	return true
}
func (r *fcgiRequest) _addMetaParam(name []byte, value []byte) bool { // like: REQUEST_METHOD
	return r._addParam(name, value, false)
}
func (r *fcgiRequest) _addHTTPParam(header *pair, name []byte, value []byte) bool { // like: HTTP_USER_AGENT
	if !header.isUnderscore() || !r.stream.agent.preferUnderscore {
		return r._addParam(name, value, true)
	}
	// TODO: got a "foo_bar" and user prefer it. avoid name conflicts with header which is like "foo-bar"
	return true
}
func (r *fcgiRequest) _addParam(name []byte, value []byte, http bool) bool { // into r.params
	nameLen, valueLen := len(name), len(value)
	if http {
		nameLen += len(fcgiBytesHTTP_)
	}
	paramSize := 1 + 1 + nameLen + valueLen
	if nameLen > 127 {
		paramSize += 3
	}
	if valueLen > 127 {
		paramSize += 3
	}
	from, edge, ok := r._growParams(paramSize)
	if !ok {
		return false
	}

	if nameLen > 127 {
		r.params[from] = byte(nameLen>>24) | 0x80
		r.params[from+1] = byte(nameLen >> 16)
		r.params[from+2] = byte(nameLen >> 8)
		from += 3
	}
	r.params[from] = byte(nameLen)
	from++
	if valueLen > 127 {
		r.params[from] = byte(valueLen>>24) | 0x80
		r.params[from+1] = byte(valueLen >> 16)
		r.params[from+2] = byte(valueLen >> 8)
		from += 3
	}
	r.params[from] = byte(valueLen)
	from++

	if http { // TODO: improve performance
		from += copy(r.params[from:], fcgiBytesHTTP_)
		last := from + copy(r.params[from:], name)
		for i := from; i < last; i++ {
			if b := r.params[i]; b >= 'a' && b <= 'z' {
				r.params[i] = b - 0x20 // to upper
			} else if b == '-' {
				r.params[i] = '_'
			}
		}
		from = last
	} else {
		from += copy(r.params[from:], name)
	}
	from += copy(r.params[from:], value)
	if from != edge {
		BugExitln("fcgi: from != edge")
	}
	return true
}
func (r *fcgiRequest) _growParams(size int) (from int, edge int, ok bool) { // to place more params into r.params[from:edge]
	if size <= 0 || size > _16K { // size allowed: (0, 16K]
		BugExitln("invalid size in growParams")
	}
	from = int(r.paramsEdge)
	last := r.paramsEdge + uint16(size)
	if last > _16K || last < r.paramsEdge {
		// Overflow
		return
	}
	if last > uint16(cap(r.params)) { // last <= _16K
		params := getFCGIParams()
		copy(params, r.params[:r.paramsEdge])
		r.params = params
	}
	r.paramsEdge = last
	edge, ok = int(r.paramsEdge), true
	return
}

func (r *fcgiRequest) pass(req Request) error { // only for sized (>0) content. unsized content must use post()
	r.vector = r.fixedVector[0:4]
	if r.stream.agent.keepConn {
		r.vector[0] = fcgiBeginKeepConn
	} else {
		r.vector[0] = fcgiBeginDontKeep
	}
	r.vector[1] = r.paramsHeader[:]
	r.vector[2] = r.params[:r.paramsEdge] // effective params
	r.vector[3] = fcgiEmptyParams
	if err := r._writeVector(); err != nil {
		return err
	}
	for {
		stdin, err := req.readContent()
		if len(stdin) > 0 {
			size := len(stdin)
			r.stdinHeader[4], r.stdinHeader[5] = byte(size>>8), byte(size)
			if err == io.EOF { // EOF is immediate, write with emptyStdin
				r.vector = r.fixedVector[0:3]
				r.vector[0] = r.stdinHeader[:]
				r.vector[1] = stdin
				r.vector[2] = fcgiEmptyStdin
				return r._writeVector()
			}
			// EOF is not immediate, err must be nil.
			r.vector = r.fixedVector[0:2]
			r.vector[0] = r.stdinHeader[:]
			r.vector[1] = stdin
			if e := r._writeVector(); e != nil {
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
	return r._writeBytes(fcgiEmptyStdin)
}
func (r *fcgiRequest) post(content any) error { // nil, []byte, *os.File. for bufferClientContent or unsized Request content
	if contentText, ok := content.([]byte); ok { // text, <= 64K1
		return r.sendText(contentText)
	} else if contentFile, ok := content.(*os.File); ok { // file
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		return r.sendFile(contentFile, fileInfo)
	} else { // nil means no content.
		return r.sendText(nil)
	}
}

func (r *fcgiRequest) sendText(content []byte) error { // content <= 64K1
	size := len(content)
	if size == 0 { // beginRequest + (params + emptyParams) + emptyStdin
		r.vector = r.fixedVector[0:5]
	} else { // beginRequest + (params + emptyParams) + (stdin + emptyStdin)
		r.vector = r.fixedVector[0:7]
	}
	if r.stream.agent.keepConn {
		r.vector[0] = fcgiBeginKeepConn
	} else {
		r.vector[0] = fcgiBeginDontKeep
	}
	r.vector[1] = r.paramsHeader[:]
	r.vector[2] = r.params[:r.paramsEdge]
	r.vector[3] = fcgiEmptyParams
	if size == 0 {
		r.vector[4] = fcgiEmptyStdin
	} else {
		r.stdinHeader[4], r.stdinHeader[5] = byte(size>>8), byte(size)
		r.vector[4] = r.stdinHeader[:]
		r.vector[5] = content
		r.vector[6] = fcgiEmptyStdin
	}
	return r._writeVector()
}
func (r *fcgiRequest) sendFile(content *os.File, info os.FileInfo) error {
	// TODO: use a buffer, for content, read to buffer, write buffer
	// TODO: beginRequest + (params + emptyParams) + (stdin * N + emptyStdin)
	return nil
}

func (r *fcgiRequest) _writeBytes(p []byte) error {
	if r.stream.isBroken() {
		return fcgiWriteBroken
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
func (r *fcgiRequest) _writeVector() error {
	if r.stream.isBroken() {
		return fcgiWriteBroken
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
func (r *fcgiRequest) _beforeWrite() error {
	now := time.Now()
	if r.sendTime.IsZero() {
		r.sendTime = now
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.agent.backend.WriteTimeout()))
}
func (r *fcgiRequest) _slowCheck(err error) error {
	if err == nil && r._tooSlow() {
		err = fcgiWriteTooSlow
	}
	if err != nil {
		r.stream.markBroken()
	}
	return err
}
func (r *fcgiRequest) _tooSlow() bool {
	return r.sendTimeout > 0 && time.Now().Sub(r.sendTime) >= r.sendTimeout
}

var ( // fcgi request errors
	fcgiWriteTooSlow = errors.New("fcgi write too slow")
	fcgiWriteBroken  = errors.New("fcgi write broken")
)

// poolFCGIRecords
var poolFCGIRecords sync.Pool

const fcgiMaxRecords = fcgiHeaderSize + _64K1 + fcgiMaxPadding // max record = header + max payload + max padding

func getFCGIRecords() []byte {
	if x := poolFCGIRecords.Get(); x == nil {
		return make([]byte, fcgiMaxRecords)
	} else {
		return x.([]byte)
	}
}
func putFCGIRecords(records []byte) {
	if cap(records) != fcgiMaxRecords {
		BugExitln("fcgi: bad records")
	}
	poolFCGIRecords.Put(records)
}

// fcgiResponse must implements webIn and response interface.
type fcgiResponse struct { // incoming. needs parsing
	// Assocs
	stream *fcgiStream
	// States (stocks)
	stockRecords [8456]byte // for r.records. fcgiHeaderSize + 8K + fcgiMaxPadding. good for PHP
	stockInput   [_2K]byte  // for r.input
	stockPrimes  [48]pair   // for r.primes
	stockExtras  [16]pair   // for r.extras
	// States (controlled)
	header pair // to overcome the limitation of Go's escape analysis when receiving headers
	// States (non-zeros)
	records        []byte        // bytes of incoming fcgi records. [<r.stockRecords>/fcgiMaxRecords]
	input          []byte        // bytes of incoming response headers. [<r.stockInput>/4K/16K]
	primes         []pair        // prime fcgi response headers
	extras         []pair        // extra fcgi response headers
	recvTimeout    time.Duration // timeout to recv the whole response content
	maxContentSize int64         // max content size allowed for current response
	status         int16         // 200, 302, 404, ...
	headResult     int16         // result of receiving response head. values are same as http status for convenience
	bodyResult     int16         // result of receiving response body. values are same as http status for convenience
	// States (zeros)
	failReason    string    // the reason of headResult or bodyResult
	recvTime      time.Time // the time when receiving response
	bodyTime      time.Time // the time when first body read operation is performed on this stream
	contentText   []byte    // if loadable, the received and loaded content of current response is at r.contentText[:r.receivedSize]
	contentFile   *os.File  // used by r.takeContent(), if content is tempFile. will be closed on stream ends
	fcgiResponse0           // all values must be zero by default in this struct!
}
type fcgiResponse0 struct { // for fast reset, entirely
	recordsFrom     int32    // from position of current records
	recordsEdge     int32    // edge position of current records
	stdoutFrom      int32    // if stdout's payload is too large to be appended to r.input, use this to note current from position
	stdoutEdge      int32    // see above, to note current edge position
	pBack           int32    // element begins from. for parsing header elements
	pFore           int32    // element spanning to. for parsing header elements
	head            span     // for debugging
	imme            span     // immediate bytes in r.input that belongs to content, not headers
	hasExtra        [8]bool  // see kindXXX for indexes
	inputEdge       int32    // edge position of r.input
	receiving       int8     // currently receiving. see httpSectionXXX
	contentTextKind int8     // kind of current r.contentText. see webContentTextXXX
	receivedSize    int64    // bytes of currently received content
	indexes         struct { // indexes of some selected singleton headers, for fast accessing
		contentType  uint8
		date         uint8
		etag         uint8
		expires      uint8
		lastModified uint8
		location     uint8
		xPoweredBy   uint8
		_            byte // padding
	}
	zones struct { // zones of some selected headers, for fast accessing
		allow           zone
		contentLanguage zone
		_               [4]byte // padding
	}
	unixTimes struct { // parsed unix times
		date         int64 // parsed unix time of date
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

func (r *fcgiResponse) onUse() {
	r.records = r.stockRecords[:]
	r.input = r.stockInput[:]
	r.primes = r.stockPrimes[0:1:cap(r.stockPrimes)] // use append(). r.primes[0] is skipped due to zero value of header indexes.
	r.extras = r.stockExtras[0:0:cap(r.stockExtras)] // use append()
	r.recvTimeout = r.stream.agent.recvTimeout
	r.maxContentSize = r.stream.agent.maxContentSize
	r.status = StatusOK
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *fcgiResponse) onEnd() {
	if cap(r.records) != cap(r.stockRecords) {
		putFCGIRecords(r.records)
		r.records = nil
	}
	if cap(r.input) != cap(r.stockInput) {
		PutNK(r.input)
		r.input = nil
	}
	if cap(r.primes) != cap(r.stockPrimes) {
		putPairs(r.primes)
		r.primes = nil
	}
	if cap(r.extras) != cap(r.stockExtras) {
		putPairs(r.extras)
		r.extras = nil
	}

	r.failReason = ""
	r.recvTime = time.Time{}
	r.bodyTime = time.Time{}

	if r.contentTextKind == webContentTextPool {
		PutNK(r.contentText)
	}
	r.contentText = nil // other content text kinds are only references, just reset.

	if r.contentFile != nil {
		r.contentFile.Close()
		if IsDebug(2) {
			Debugln("contentFile is left as is, not removed!")
		} else if err := os.Remove(r.contentFile.Name()); err != nil {
			// TODO: log?
		}
		r.contentFile = nil
	}

	r.fcgiResponse0 = fcgiResponse0{}
}

func (r *fcgiResponse) recvHead() {
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead() { // r.input must be empty because we don't use pipelining in requests.
		// r.headResult is set.
		return
	}
	if !r.recvHeaders() || !r.examineHead() { // there is no control in FCGI, or, control is included in headers
		// r.headResult is set.
		return
	}
	r.cleanInput()
}
func (r *fcgiResponse) growHead() bool { // we need more head data to be appended to r.input from r.records
	// Is r.input already full?
	if inputSize := int32(cap(r.input)); r.inputEdge == inputSize { // r.inputEdge reached end, so yes
		if inputSize == _16K { // max r.input size is 16K, we cannot use a larger input anymore
			r.headResult = StatusRequestHeaderFieldsTooLarge
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
	// r.input is not full. Are there any existing stdout data in r.records?
	if r.stdoutFrom == r.stdoutEdge { // no, we must receive a new non-empty stdout record
		if from, edge, err := r._recvStdout(); err == nil {
			r.stdoutFrom, r.stdoutEdge = from, edge
		} else { // unexpected error or EOF
			r.headResult = -1
			return false
		}
	}
	// There are some existing stdout data in r.records now, copy them to r.input
	freeSize := int32(cap(r.input)) - r.inputEdge
	haveSize := r.stdoutEdge - r.stdoutFrom
	copy(r.input[r.inputEdge:], r.records[r.stdoutFrom:r.stdoutEdge])
	if freeSize < haveSize { // too much data
		r.inputEdge += freeSize
		r.stdoutFrom += freeSize
	} else { // freeSize >= haveSize, take all stdout data
		r.inputEdge += haveSize
		r.stdoutFrom, r.stdoutEdge = 0, 0
	}
	return true
}
func (r *fcgiResponse) recvHeaders() bool { // 1*( field-name ":" OWS field-value OWS CRLF ) CRLF
	// generic-response = 1*header-field NL [ response-body ]
	// header-field    = CGI-field | generic-field
	// CGI-field       = Content-Type | Location | Status
	// Content-Type    = "Content-Type:" media-type NL
	// Status          = "Status:" status-code SP reason-phrase NL
	// generic-field   = field-name ":" [ field-value ] NL
	// field-name      = token
	// field-value     = *( field-content | LWSP )
	// field-content   = *( token | separator | quoted-string )
	header := &r.header
	header.zero()
	header.kind = kindHeader
	header.place = placeInput // all received headers are in r.input
	// r.pFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.pFore]; b == '\r' {
			// Skip '\r'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
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

		// field-name = token
		// token = 1*tchar

		r.pBack = r.pFore // now r.pBack is at header-field
		for {
			b := r.input[r.pFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.pFore] = b
			} else if t == 3 { // '_'
				// For fcgi, do nothing
			} else if b == ':' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			header.hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
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
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.input[r.pFore] == ' ' || r.input[r.pFore] == '\t' {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		// field-value = *( field-content | LWSP )
		r.pBack = r.pFore // now r.pBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.pFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
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
		if !r.addPrime(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
			return false
		}
		// r.pFore is now at the next header or end of headers.
		header.hash, header.flags = 0, 0 // reset for next header
	}
	r.receiving = httpSectionContent
	// Skip end of headers
	r.pFore++
	// Now the head is received, and r.pFore is at the beginning of content (if exists).
	r.head.set(0, r.pFore)

	return true
}
func (r *fcgiResponse) addPrime(prime *pair) bool {
	if len(r.primes) == cap(r.primes) { // full
		if cap(r.primes) != cap(r.stockPrimes) { // too many primes
			r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many primes"
			return false
		}
		r.primes = getPairs()
		r.primes = append(r.primes, r.stockPrimes[:]...)
	}
	r.primes = append(r.primes, *prime)
	return true
}

func (r *fcgiResponse) Status() int16 { return r.status }

func (r *fcgiResponse) ContentSize() int64 { return -2 }   // fcgi is unsized by default. we believe in framing
func (r *fcgiResponse) isUnsized() bool    { return true } // fcgi is unsized by default. we believe in framing

func (r *fcgiResponse) examineHead() bool {
	for i := 1; i < len(r.primes); i++ { // r.primes[0] is not used
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	// content length is not known at this time, can't check.
	return true
}
func (r *fcgiResponse) applyHeader(index int) bool {
	header := &r.primes[index]
	headerName := header.nameAt(r.input)
	if sh := &fcgiResponseSingletonHeaderTable[fcgiResponseSingletonHeaderFind(header.hash)]; sh.hash == header.hash && bytes.Equal(sh.name, headerName) {
		header.setSingleton()
		if !sh.parse { // unnecessary to parse
			header.setParsed()
		} else if !r._parseHeader(header, &sh.desc, true) {
			// r.headResult is set.
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &fcgiResponseImportantHeaderTable[fcgiResponseImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, headerName) {
		extraFrom := len(r.extras)
		if !r._splitHeader(header, &mh.desc) {
			// r.headResult is set.
			return false
		}
		if header.isCommaValue() { // has sub headers, check them
			if !mh.check(r, r.extras, extraFrom, len(r.extras)) {
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

func (r *fcgiResponse) _parseHeader(header *pair, desc *desc, fully bool) bool { // data and params
	// TODO
	// use r._addExtra
	return true
}
func (r *fcgiResponse) _splitHeader(header *pair, desc *desc) bool {
	// TODO
	// use r._addExtra
	return true
}
func (r *fcgiResponse) _addExtra(extra *pair) bool {
	if len(r.extras) == cap(r.extras) { // full
		if cap(r.extras) != cap(r.stockExtras) { // too many extras
			r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many extras"
			return false
		}
		r.extras = getPairs()
		r.extras = append(r.extras, r.stockExtras[:]...)
	}
	r.extras = append(r.extras, *extra)
	r.hasExtra[extra.kind] = true
	return true
}

var ( // perfect hash table for response singleton headers
	fcgiResponseSingletonHeaderTable = [4]struct {
		parse bool // need general parse or not
		desc       // allowQuote, allowEmpty, allowParam, hasComment
		check func(*fcgiResponse, *pair, int) bool
	}{ // content-length content-type location status
		0: {false, desc{fcgiHashStatus, false, false, false, false, fcgiBytesStatus}, (*fcgiResponse).checkStatus},
		1: {false, desc{hashContentLength, false, false, false, false, bytesContentLength}, (*fcgiResponse).checkContentLength},
		2: {true, desc{hashContentType, false, false, true, false, bytesContentType}, (*fcgiResponse).checkContentType},
		3: {false, desc{hashLocation, false, false, false, false, bytesLocation}, (*fcgiResponse).checkLocation},
	}
	fcgiResponseSingletonHeaderFind = func(hash uint16) int { return (2704 / int(hash)) % 4 }
)

func (r *fcgiResponse) checkContentLength(header *pair, index int) bool {
	header.zero() // we don't believe the value provided by fcgi application. we believe fcgi framing
	return true
}
func (r *fcgiResponse) checkContentType(header *pair, index int) bool {
	if r.indexes.contentType == 0 && !header.dataEmpty() {
		r.indexes.contentType = uint8(index)
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-type"
	return false
}
func (r *fcgiResponse) checkStatus(header *pair, index int) bool {
	if value := header.valueAt(r.input); len(value) >= 3 {
		if status, ok := decToI64(value[0:3]); ok {
			r.status = int16(status)
			return true
		}
	}
	r.headResult, r.failReason = StatusBadRequest, "bad status"
	return false
}
func (r *fcgiResponse) checkLocation(header *pair, index int) bool {
	r.indexes.location = uint8(index)
	return true
}

var ( // perfect hash table for response important headers
	fcgiResponseImportantHeaderTable = [3]struct {
		desc  // allowQuote, allowEmpty, allowParam, hasComment
		check func(*fcgiResponse, []pair, int, int) bool
	}{ // connection transfer-encoding upgrade
		0: {desc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*fcgiResponse).checkTransferEncoding}, // deliberately false
		1: {desc{hashConnection, false, false, false, false, bytesConnection}, (*fcgiResponse).checkConnection},
		2: {desc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*fcgiResponse).checkUpgrade},
	}
	fcgiResponseImportantHeaderFind = func(hash uint16) int { return (1488 / int(hash)) % 3 }
)

func (r *fcgiResponse) checkConnection(pairs []pair, from int, edge int) bool { // Connection = #connection-option
	return r._delHeaders(pairs, from, edge)
}
func (r *fcgiResponse) checkTransferEncoding(pairs []pair, from int, edge int) bool { // Transfer-Encoding = #transfer-coding
	return r._delHeaders(pairs, from, edge)
}
func (r *fcgiResponse) checkUpgrade(pairs []pair, from int, edge int) bool { // Upgrade = #protocol
	return r._delHeaders(pairs, from, edge)
}
func (r *fcgiResponse) _delHeaders(pairs []pair, from int, edge int) bool {
	for i := from; i < edge; i++ {
		pairs[i].zero()
	}
	return true
}

func (r *fcgiResponse) cleanInput() {
	if r.hasContent() {
		r.imme.set(r.pFore, r.inputEdge)
		// We don't know the size of unsized content. Let content receiver to decide & clean r.input.
	} else if _, _, err := r._recvStdout(); err != io.EOF { // no content, receive endRequest
		r.headResult, r.failReason = StatusBadRequest, "bad endRequest"
	}
}

func (r *fcgiResponse) hasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return true
}
func (r *fcgiResponse) takeContent() any { // to tempFile since we don't know the size of unsized content
	switch content := r._recvContent().(type) {
	case tempFile: // [0, r.maxContentSize]
		r.contentFile = content.(*os.File)
		return r.contentFile
	case error: // i/o error or unexpected EOF
		// TODO: log err?
		if IsDebug(2) {
			Debugln(content.Error())
		}
	}
	r.stream.markBroken()
	return nil
}
func (r *fcgiResponse) _recvContent() any { // to tempFile
	contentFile, err := r._newTempFile()
	if err != nil {
		return err
	}
	var p []byte
	for {
		p, err = r.readContent()
		if len(p) > 0 {
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
	os.Remove(contentFile.Name())
	return err
}
func (r *fcgiResponse) readContent() (p []byte, err error) {
	if r.imme.notEmpty() {
		p, err = r.input[r.imme.from:r.imme.edge], nil
		r.imme.zero()
		return
	}
	if from, edge, err := r._recvStdout(); from != edge {
		return r.records[from:edge], nil
	} else {
		return nil, err
	}
}

func (r *fcgiResponse) addTrailer(trailer *pair) bool { return true }  // fcgi doesn't support trailers
func (r *fcgiResponse) applyTrailer(index uint8) bool { return true }  // fcgi doesn't support trailers
func (r *fcgiResponse) HasTrailers() bool             { return false } // fcgi doesn't support trailers

func (r *fcgiResponse) examineTail() bool { return true } // fcgi doesn't support trailers

func (r *fcgiResponse) arrayCopy(p []byte) bool { return true } // not used, but required by webIn interface

func (r *fcgiResponse) delHopHeaders() {} // for fcgi, nothing to delete
func (r *fcgiResponse) forHeaders(fn func(header *pair, name []byte, value []byte) bool) bool { // by Response.copyHeadFrom(). excluding sub headers
	for i := 1; i < len(r.primes); i++ { // r.primes[0] is not used
		if header := &r.primes[i]; header.hash != 0 && !header.isSubField() {
			if !fn(header, header.nameAt(r.input), header.valueAt(r.input)) {
				return false
			}
		}
	}
	return true
}

func (r *fcgiResponse) delHopTrailers() {} // fcgi doesn't support trailers
func (r *fcgiResponse) forTrailers(fn func(trailer *pair, name []byte, value []byte) bool) bool { // fcgi doesn't support trailers
	return true
}

func (r *fcgiResponse) saveContentFilesDir() string { return r.stream.agent.SaveContentFilesDir() }

func (r *fcgiResponse) _newTempFile() (tempFile, error) { // to save content to
	filesDir := r.saveContentFilesDir()
	pathSize := len(filesDir)
	filePath := r.stream.unsafeMake(pathSize + 19) // 19 bytes is enough for int64
	copy(filePath, filesDir)
	from, edge := r.stream.makeTempName(filePath[pathSize:], r.recvTime.Unix())
	pathSize += copy(filePath[pathSize:], filePath[pathSize+from:pathSize+edge])
	return os.OpenFile(risky.WeakString(filePath[:pathSize]), os.O_RDWR|os.O_CREATE, 0644)
}
func (r *fcgiResponse) _beforeRead(toTime *time.Time) error {
	now := time.Now()
	if toTime.IsZero() {
		*toTime = now
	}
	return r.stream.setReadDeadline(now.Add(r.stream.agent.backend.ReadTimeout()))
}
func (r *fcgiResponse) _tooSlow() bool {
	return r.recvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.recvTimeout
}

func (r *fcgiResponse) _recvStdout() (int32, int32, error) { // r.records[from:edge] is the stdout data.
	const (
		fcgiKindStdout     = 6 // [S] many (ends with an emptyStdout record)
		fcgiKindStderr     = 7 // [S] many (ends with an emptyStderr record)
		fcgiKindEndRequest = 3 // [D] only one
	)
recv:
	kind, from, edge, err := r._recvRecord()
	if err != nil {
		return 0, 0, err
	}
	if kind == fcgiKindStdout && edge > from { // fast path
		return from, edge, nil
	}
	if kind == fcgiKindStderr {
		if IsDebug(2) && edge > from {
			Debugf("fcgi stderr=%s\n", r.records[from:edge])
		}
		goto recv
	}
	switch kind {
	case fcgiKindStdout: // emptyStdout
		for { // receive until endRequest
			kind, from, edge, err = r._recvRecord()
			if kind == fcgiKindEndRequest {
				return 0, 0, io.EOF
			}
			// Only stderr records are allowed here.
			if kind != fcgiKindStderr {
				return 0, 0, fcgiReadBadRecord
			}
			// Must be stderr.
			if IsDebug(2) && edge > from {
				Debugf("fcgi stderr=%s\n", r.records[from:edge])
			}
		}
	case fcgiKindEndRequest:
		return 0, 0, io.EOF
	default: // unknown record
		return 0, 0, fcgiReadBadRecord
	}
}
func (r *fcgiResponse) _recvRecord() (kind byte, from int32, edge int32, err error) { // r.records[from:edge] is the record payload.
	remainSize := r.recordsEdge - r.recordsFrom
	// At least an fcgi header must be immediate
	if remainSize < fcgiHeaderSize {
		if n, e := r._growRecords(fcgiHeaderSize - int(remainSize)); e == nil {
			remainSize += int32(n)
		} else {
			err = e
			return
		}
	}
	// FCGI header is now immediate.
	payloadLen := int32(r.records[r.recordsFrom+4])<<8 + int32(r.records[r.recordsFrom+5])
	recordSize := fcgiHeaderSize + payloadLen + int32(r.records[r.recordsFrom+6]) // with padding
	// Is the whole record immediate?
	if recordSize > remainSize { // no, we need to make it immediate by reading the missing bytes
		// Shoud we switch to a larger r.records?
		if recordSize > int32(cap(r.records)) { // yes, because this record is too large
			records := getFCGIRecords()
			r._slideRecords(records)
			r.records = records
		}
		// Now r.records is large enough to place this record, we can read the missing bytes of this record
		if n, e := r._growRecords(int(recordSize - remainSize)); e == nil {
			remainSize += int32(n)
		} else {
			err = e
			return
		}
	}
	// Now recordSize <= remainSize, the record is immediate, so continue parsing it.
	kind = r.records[r.recordsFrom+1]
	from = r.recordsFrom + fcgiHeaderSize
	edge = from + payloadLen // payload edge, ignoring padding
	if IsDebug(2) {
		Debugf("_recvRecord: kind=%d from=%d edge=%d payload=[%s]\n", kind, from, edge, r.records[from:edge])
	}
	// Clean up positions for next call.
	if recordSize == remainSize { // all remain data are consumed, so reset positions
		r.recordsFrom, r.recordsEdge = 0, 0
	} else { // recordSize < remainSize, extra records exist, so mark it for next call
		r.recordsFrom += recordSize
	}
	return
}
func (r *fcgiResponse) _growRecords(size int) (int, error) { // r.records is large enough.
	// Should we slide to get enough space to grow?
	if size > cap(r.records)-int(r.recordsEdge) { // yes
		r._slideRecords(r.records)
	}
	// We now have enough space to grow.
	n, err := r.stream.readAtLeast(r.records[r.recordsEdge:], size)
	if err != nil {
		return 0, err
	}
	r.recordsEdge += int32(n)
	return n, nil
}
func (r *fcgiResponse) _slideRecords(records []byte) { // so we can get more space to grow
	if r.recordsFrom > 0 {
		copy(records, r.records[r.recordsFrom:r.recordsEdge])
		r.recordsEdge -= r.recordsFrom
		r.recordsFrom = 0
	}
}

var ( // fcgi response errors
	fcgiReadBadRecord = errors.New("fcgi: bad record")
)

//////////////////////////////////////// FCGI protocol elements.

// FCGI Record = FCGI Header(8) + payload + padding
// FCGI Header = version(1) + type(1) + requestId(2) + payloadLen(2) + paddingLen(1) + reserved(1)

const ( // fcgi constants
	fcgiHeaderSize = 8
	fcgiMaxPadding = 255
)

// Discrete records are standalone.
// Streamed records end with an empty record (payloadLen=0).

var ( // request records
	fcgiBeginKeepConn = []byte{ // 16 bytes
		1, 1, // version, FCGI_BEGIN_REQUEST
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1. same below
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 1, 0, 0, 0, 0, 0, // role=responder, flags=keepConn
	}
	fcgiBeginDontKeep = []byte{ // 16 bytes
		1, 1, // version, FCGI_BEGIN_REQUEST
		0, 1, // request id = 1
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 0, 0, 0, 0, 0, 0, // role=responder, flags=dontKeep
	}
	fcgiEmptyParams = []byte{ // 8 bytes
		1, 4, // version, FCGI_PARAMS
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	}
	fcgiEmptyStdin = []byte{ // 8 bytes
		1, 5, // version, FCGI_STDIN
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	}
)

var ( // request param names
	fcgiBytesAuthType         = []byte("AUTH_TYPE")
	fcgiBytesContentLength    = []byte("CONTENT_LENGTH")
	fcgiBytesContentType      = []byte("CONTENT_TYPE")
	fcgiBytesDocumentRoot     = []byte("DOCUMENT_ROOT")
	fcgiBytesDocumentURI      = []byte("DOCUMENT_URI")
	fcgiBytesGatewayInterface = []byte("GATEWAY_INTERFACE")
	fcgiBytesHTTP_            = []byte("HTTP_")
	fcgiBytesHTTPS            = []byte("HTTPS")
	fcgiBytesPathInfo         = []byte("PATH_INFO")
	fcgiBytesPathTranslated   = []byte("PATH_TRANSLATED")
	fcgiBytesQueryString      = []byte("QUERY_STRING")
	fcgiBytesRedirectStatus   = []byte("REDIRECT_STATUS")
	fcgiBytesRemoteAddr       = []byte("REMOTE_ADDR")
	fcgiBytesRemoteHost       = []byte("REMOTE_HOST")
	fcgiBytesRequestMethod    = []byte("REQUEST_METHOD")
	fcgiBytesRequestScheme    = []byte("REQUEST_SCHEME")
	fcgiBytesRequestURI       = []byte("REQUEST_URI")
	fcgiBytesScriptFilename   = []byte("SCRIPT_FILENAME")
	fcgiBytesScriptName       = []byte("SCRIPT_NAME")
	fcgiBytesServerAddr       = []byte("SERVER_ADDR")
	fcgiBytesServerName       = []byte("SERVER_NAME")
	fcgiBytesServerPort       = []byte("SERVER_PORT")
	fcgiBytesServerProtocol   = []byte("SERVER_PROTOCOL")
	fcgiBytesServerSoftware   = []byte("SERVER_SOFTWARE")
)

var ( // request param values
	fcgiBytesCGI1_1 = []byte("CGI/1.1")
	fcgiBytesON     = []byte("on")
)

const ( // response header hashes
	fcgiHashStatus = 676
)

var ( // response header names
	fcgiBytesStatus = []byte("status")
)
