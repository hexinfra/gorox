// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI agent handlet passes requests to backend FastCGI servers and cache responses.

// FastCGI is mainly used by PHP applications. It doesn't support HTTP trailers.
// And we don't use request-side chunking due to the limitation of CGI/1.1 even
// though FastCGI can do that through its framing protocol. Perhaps most FastCGI
// applications don't implement this feature either.

// To avoid ambiguity, the term "content" in FastCGI specification is called "payload" in our implementation.

package internal

import (
	"bytes"
	"errors"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
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
	stage   *Stage   // current stage
	app     *App     // the app to which the agent belongs
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher   // the cache server which is used by this agent
	// States
	scriptFilename      []byte        // for SCRIPT_FILENAME
	bufferClientContent bool          // client content is buffered anyway?
	bufferServerContent bool          // server content is buffered anyway?
	keepConn            bool          // instructs FCGI server to keep conn?
	addRequestHeaders   [][2][]byte   // headers appended to client request
	delRequestHeaders   [][]byte      // client request headers to delete
	addResponseHeaders  [][2][]byte   // headers appended to server response
	delResponseHeaders  [][]byte      // server response headers to delete
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max content size allowed
	preferUnderscore    bool          // if header name "foo-bar" and "foo_bar" are both present, prefer "foo_bar" to "foo-bar"?
}

func (h *fcgiAgent) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
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
			} else if pBackend, ok := backend.(PBackend); ok {
				h.backend = pBackend
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
	// scriptFilename
	h.ConfigureBytes("scriptFilename", &h.scriptFilename, nil, nil)
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// keepConn
	h.ConfigureBool("keepConn", &h.keepConn, false)
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

func (h *fcgiAgent) Handle(req Request, resp Response) (next bool) {
	var (
		content  any
		fConn    PConn
		fErr     error
		fContent any
	)

	hasContent := req.HasContent()
	if hasContent && (h.bufferClientContent || req.isUnsized()) { // including size 0
		content = req.holdContent()
		if content == nil { // hold failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.keepConn {
		fConn, fErr = h.backend.FetchConn()
		if fErr != nil {
			resp.SendBadGateway(nil)
			return
		}
		defer h.backend.StoreConn(fConn)
	} else {
		fConn, fErr = h.backend.Dial()
		if fErr != nil {
			resp.SendBadGateway(nil)
			return
		}
		defer fConn.Close()
	}

	fStream := getFCGIStream(h, fConn)
	defer putFCGIStream(fStream)

	fReq := &fStream.request
	if !fReq.copyHead(req, h.scriptFilename) {
		fStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if hasContent && !h.bufferClientContent && !req.isUnsized() {
		if fErr = fReq.pass(req); fErr != nil {
			fStream.markBroken()
		}
	} else if fErr = fReq.post(content); fErr != nil {
		fStream.markBroken()
	}
	if fErr != nil {
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
		if !resp.sync1xx(fResp) {
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
		fContent = fResp.holdContent()
		if fContent == nil { // hold failed
			// fStream is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHead(fResp) {
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

func getFCGIStream(agent *fcgiAgent, conn PConn) *fcgiStream {
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
	// Stream states (buffers)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Stream states (controlled)
	// Stream states (non-zeros)
	agent  *fcgiAgent // associated agent
	conn   PConn      // associated conn
	region Region     // a region-based memory pool
	// Stream states (zeros)
}

func (s *fcgiStream) onUse(agent *fcgiAgent, conn PConn) {
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

func (s *fcgiStream) smallBuffer() []byte        { return s.stockBuffer[:] }
func (s *fcgiStream) unsafeMake(size int) []byte { return s.region.Make(size) }

func (s *fcgiStream) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return s.conn.MakeTempName(p, stamp)
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

// fcgiRequest
type fcgiRequest struct { // outgoing. needs building
	// Assocs
	stream   *fcgiStream
	response *fcgiResponse
	// States (buffers)
	stockParams [_2K]byte // for r.params
	// States (controlled)
	paramsHeader [8]byte // used by params record
	stdinHeader  [8]byte // used by stdin record
	// States (non-zeros)
	params      []byte        // place the payload of exactly one FCGI_PARAMS record. [<r.stockParams>/16K]
	contentSize int64         // info of content. -1: not set, -2: unsized, >=0: size
	sendTimeout time.Duration // timeout to send the whole request
	// States (zeros)
	sendTime     time.Time   // the time when first send operation is performed
	vector       net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector  [5][]byte   // for sending request. reset after stream. 120B
	fcgiRequest0             // all values must be zero by default in this struct!
}
type fcgiRequest0 struct { // for fast reset, entirely
	paramsEdge    uint16 // edge of r.params. max size of r.params must be <= fcgiMaxParams.
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
}

func (r *fcgiRequest) onUse() {
	copy(r.paramsHeader[:], fcgiEmptyParams) // payloadLen (r.paramsHeader[4:6]) needs modification
	copy(r.stdinHeader[:], fcgiEmptyStdin)   // payloadLen (r.stdinHeader[4:6]) needs modification for every stdin record
	r.params = r.stockParams[:]
	r.contentSize = -1 // not set
	r.sendTimeout = r.stream.agent.sendTimeout
}
func (r *fcgiRequest) onEnd() {
	if cap(r.params) != cap(r.stockParams) {
		putFCGIParams(r.params)
		r.params = nil
	}
	r.sendTime = time.Time{}
	r.vector = nil
	r.fixedVector = [5][]byte{}
	r.fcgiRequest0 = fcgiRequest0{}
}

func (r *fcgiRequest) copyHead(req Request, scriptFilename []byte) bool {
	var value []byte
	if len(scriptFilename) == 0 {
		value = req.unsafeAbsPath()
	} else {
		value = scriptFilename
	}
	// Add meta params
	if !r._addMetaParam(fcgiBytesScriptFilename, value) { // SCRIPT_FILENAME
		return false
	}
	if !r._addMetaParam(fcgiBytesScriptName, req.UnsafePath()) { // SCRIPT_NAME
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
	// Finalize params
	r.paramsHeader[4] = byte(r.paramsEdge >> 8)
	r.paramsHeader[5] = byte(r.paramsEdge)
	return true
}
func (r *fcgiRequest) _addMetaParam(name []byte, value []byte) bool { // CONTENT_LENGTH and so on
	return r._addParam(nil, name, value, false)
}
func (r *fcgiRequest) _addHTTPParam(header *pair, name []byte, value []byte) bool { // HTTP_CONTENT_LENGTH and so on
	if !header.isUnderscore() || !r.stream.agent.preferUnderscore {
		return r._addParam(fcgiBytesHTTP_, name, value, true)
	}
	// TODO: got a "foo_bar" and user prefer it. avoid name conflicts with header which is like "foo-bar"
	return true
}
func (r *fcgiRequest) _addParam(prefix []byte, name []byte, value []byte, toUpper bool) bool {
	nameLen, valueLen := len(prefix)+len(name), len(value)
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
	if len(prefix) > 0 {
		from += copy(r.params[from:], prefix)
	}
	if toUpper {
		last := from + copy(r.params[from:], name)
		for i := from; i < last; i++ {
			if b := r.params[i]; b >= 'a' && b <= 'z' {
				r.params[i] = b - 0x20 // to upper
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
func (r *fcgiRequest) _growParams(size int) (from int, edge int, ok bool) {
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

func (r *fcgiRequest) pass(req Request) error { // only for sized (>0) content
	// TODO: timeout
	r.contentSize = req.ContentSize()
	r.vector = r.fixedVector[0:4]
	if r.stream.agent.keepConn {
		r.vector[0] = fcgiBeginKeepConn
	} else {
		r.vector[0] = fcgiBeginDontKeep
	}
	r.vector[1] = r.paramsHeader[:]
	r.vector[2] = r.params[:r.paramsEdge] // effective params
	r.vector[3] = fcgiEmptyParams
	if _, err := r.stream.writev(&r.vector); err != nil {
		return err
	}
	for {
		p, err := req.readContent()
		if len(p) > 0 {
			size := len(p)
			r.stdinHeader[4] = byte(size >> 8)
			r.stdinHeader[5] = byte(size)
			if err == io.EOF { // EOF is immediate, write with emptyStdin
				// TODO
				r.vector = r.fixedVector[0:3]
				r.vector[0] = r.stdinHeader[:]
				r.vector[1] = p
				r.vector[2] = fcgiEmptyStdin
				_, e := r.stream.writev(&r.vector)
				return e
			}
			// EOF is not immediate, err must be nil.
			r.vector = r.fixedVector[0:2]
			r.vector[0] = r.stdinHeader[:]
			r.vector[1] = p
			if _, e := r.stream.writev(&r.vector); e != nil {
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
	_, err := r.stream.write(fcgiEmptyStdin)
	return err
}
func (r *fcgiRequest) post(content any) error { // nil, []byte, *os.File. for bufferClientContent or unsized Request content
	if contentBlob, ok := content.([]byte); ok { // blob
		return r.sendBlob(contentBlob)
	} else if contentFile, ok := content.(*os.File); ok { // file
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		return r.sendFile(contentFile, fileInfo)
	} else { // nil means no content.
		// TODO: beginRequest + params + emptyParams + emptyStdin
		return nil
	}
}

func (r *fcgiRequest) sendBlob(content []byte) error {
	// TODO: beginRequest + params + emptyParams + stdin * N + emptyStdin
	return nil
}
func (r *fcgiRequest) sendFile(content *os.File, info os.FileInfo) error {
	// TODO: beginRequest + params + emptyParams + stdin * N + emptyStdin
	// TODO: use a buffer, for content, read to buffer, write buffer
	return nil
}

func (r *fcgiRequest) _beforeWrite() error {
	now := time.Now()
	if r.sendTime.IsZero() {
		r.sendTime = now
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.agent.backend.WriteTimeout()))
}

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

// fcgiResponse must implements httpIn and hResponse interface.
type fcgiResponse struct { // incoming. needs parsing
	// Assocs
	stream *fcgiStream
	// States (buffers)
	stockRecords [8192]byte // for r.records
	stockInput   [_2K]byte  // for r.input
	stockHeaders [64]pair   // for r.headers
	// States (controlled)
	header pair // to overcome the limitation of Go's escape analysis when receiving headers
	// States (non-zeros)
	records        []byte        // bytes of incoming fcgi records. [<r.stockRecords>/16K/fcgiMaxRecords]
	input          []byte        // bytes of incoming response headers. [<r.stockInput>/4K/16K]
	headers        []pair        // fcgi response headers
	recvTimeout    time.Duration // timeout to recv the whole response content
	maxContentSize int64         // max content size allowed for current response
	headResult     int16         // result of receiving response head. values are same as http status for convenience
	bodyResult     int16         // result of receiving response body. values are same as http status for convenience
	// States (zeros)
	failReason    string    // the reason of headResult or bodyResult
	recvTime      time.Time // the time when receiving response
	bodyTime      time.Time // the time when first body read operation is performed on this stream
	contentBlob   []byte    // if loadable, the received and loaded content of current response is at r.contentBlob[:r.receivedSize]
	contentHeld   *os.File  // used by r.holdContent(), if content is TempFile. will be closed on stream ends
	fcgiResponse0           // all values must be zero by default in this struct!
}
type fcgiResponse0 struct { // for fast reset, entirely
	recordsFrom     int32    // from position of current records
	recordsEdge     int32    // edge position of current records
	stdoutFrom      int32    // if stdout's payload is too large to be appended to r.input, use this to note current from position
	stdoutEdge      int32    // see above, to note current edge position
	inputEdge       int32    // edge position of r.input
	head            text     // for debugging
	imme            text     // immediate bytes in r.input that belongs to content
	pBack           int32    // element begins from. for parsing header elements
	pFore           int32    // element spanning to. for parsing header elements
	status          int16    // 200, 302, 404, ...
	receiving       int8     // currently receiving. see httpSectionXXX
	contentBlobKind int8     // kind of current r.contentBlob. see httpContentBlobXXX
	receivedSize    int64    // bytes of currently received content
	indexes         struct { // indexes of some selected headers, for fast accessing
		contentType  uint8
		xPoweredBy   uint8
		date         uint8
		lastModified uint8
		expires      uint8
		etag         uint8
		location     uint8
	}
}

func (r *fcgiResponse) onUse() {
	r.records = r.stockRecords[:]
	r.input = r.stockInput[:]
	r.headers = r.stockHeaders[0:1:cap(r.stockHeaders)] // use append(). r.headers[0] is skipped due to zero value of header indexes.
	r.recvTimeout = r.stream.agent.recvTimeout
	r.maxContentSize = r.stream.agent.maxContentSize
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *fcgiResponse) onEnd() {
	if cap(r.records) != cap(r.stockRecords) {
		if cap(r.records) == fcgiMaxRecords {
			putFCGIRecords(r.records)
		} else { // 16K
			PutNK(r.records)
		}
		r.records = nil
	}
	if cap(r.input) != cap(r.stockInput) {
		PutNK(r.input)
		r.input = nil
	}
	if cap(r.headers) != cap(r.stockHeaders) {
		putPairs(r.headers)
		r.headers = nil
	}

	r.failReason = ""
	r.recvTime = time.Time{}
	r.bodyTime = time.Time{}

	if r.contentBlobKind == httpContentBlobPool {
		PutNK(r.contentBlob)
	}
	r.contentBlob = nil // other blob kinds are only references, just reset.

	if r.contentHeld != nil {
		r.contentHeld.Close()
		if IsDebug(2) {
			Debugln("contentHeld is left as is!")
		} else if err := os.Remove(r.contentHeld.Name()); err != nil {
			// TODO: log?
		}
		r.contentHeld = nil
	}

	r.fcgiResponse0 = fcgiResponse0{}
}

func (r *fcgiResponse) recvHead() {
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead() || !r.recvHeaders() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
}
func (r *fcgiResponse) growHead() bool { // we need more head data to be appended to r.input
	// Is r.input full?
	if inputSize := int32(cap(r.input)); r.inputEdge == inputSize { // r.inputEdge reached end, so r.input is full
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
	// r.input is not full. Are there any existing stdout data?
	if r.stdoutFrom == r.stdoutEdge { // no existing stdout data, receive one stdout record
		from, edge, err := r._recvStdout()
		if err != nil || from == edge { // i/o error on unexpected EOF
			r.headResult = -1
			return false
		}
		r.stdoutFrom, r.stdoutEdge = from, edge
	}
	// There are some existing stdout data.
	spaceSize := int32(cap(r.input)) - r.inputEdge
	stdoutSize := r.stdoutEdge - r.stdoutFrom
	copy(r.input[r.inputEdge:], r.records[r.stdoutFrom:r.stdoutEdge]) // this is the cost. sucks!
	if spaceSize < stdoutSize {
		r.inputEdge += spaceSize
		r.stdoutFrom += spaceSize
	} else { // space >= stdoutSize, take all stdout data
		r.inputEdge += stdoutSize
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
		if !r.addHeader(header) {
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

func (r *fcgiResponse) Status() int16 { return r.status }

func (r *fcgiResponse) addHeader(header *pair) bool {
	if len(r.headers) == cap(r.headers) {
		if cap(r.headers) == cap(r.stockHeaders) {
			r.headers = getPairs()
			r.headers = append(r.headers, r.stockHeaders[:]...)
		} else { // overflow
			r.headResult = StatusRequestHeaderFieldsTooLarge
			return false
		}
	}
	r.headers = append(r.headers, *header)
	return true
}

func (r *fcgiResponse) applyHeader(header *pair, index int) bool {
	headerName := header.nameAt(r.input)
	if sh := &fcgiResponseSingletonHeaderTable[fcgiResponseSingletonHeaderFind(header.hash)]; sh.hash == header.hash && bytes.Equal(fcgiResponseSingletonHeaderNames[sh.from:sh.edge], headerName) {
		header.setSingleton()
		if sh.parse && !r._parseHeader(header, &sh.fdesc, true) {
			// r.headResult is set.
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &fcgiResponseImportantHeaderTable[fcgiResponseImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(fcgiResponseImportantHeaderNames[mh.from:mh.edge], headerName) {
		from := len(r.headers)
		if !r._splitHeader(header, &mh.fdesc) {
			// r.headResult is set.
			return false
		}
		if !mh.check(r, from, len(r.headers)) {
			// r.headResult is set.
			return false
		}
	}
	return true
}

var ( // perfect hash table for response singleton headers
	fcgiResponseSingletonHeaderNames = []byte("content-length content-type location status")
	fcgiResponseSingletonHeaderTable = [4]struct {
		fdesc
		parse bool // need general parse or not
		check func(*fcgiResponse, *pair, int) bool
	}{
		0: {fdesc{fcgiHashStatus, 37, 43, false, false, false, false}, false, (*fcgiResponse).checkStatus},
		1: {fdesc{hashContentLength, 0, 14, false, false, false, false}, false, (*fcgiResponse).checkContentLength},
		2: {fdesc{hashContentType, 15, 27, false, false, true, false}, true, (*fcgiResponse).checkContentType},
		3: {fdesc{hashLocation, 28, 36, false, false, false, false}, false, (*fcgiResponse).checkLocation},
	}
	fcgiResponseSingletonHeaderFind = func(hash uint16) int { return (2704 / int(hash)) % 4 }
)

func (r *fcgiResponse) checkContentLength(header *pair, index int) bool {
	header.zero() // we don't believe the value provided by fcgi application. we believe fcgi framing
	return true
}
func (r *fcgiResponse) checkContentType(header *pair, index int) bool {
	r.indexes.contentType = uint8(index)
	return true
}
func (r *fcgiResponse) checkStatus(header *pair, index int) bool {
	if status, ok := decToI64(header.valueAt(r.input)); ok {
		r.status = int16(status)
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad status"
	return false
}
func (r *fcgiResponse) checkLocation(header *pair, index int) bool {
	r.indexes.location = uint8(index)
	return true
}

var ( // perfect hash table for response important headers
	fcgiResponseImportantHeaderNames = []byte("connection transfer-encoding upgrade")
	fcgiResponseImportantHeaderTable = [3]struct {
		fdesc
		check func(*fcgiResponse, int, int) bool
	}{
		0: {fdesc{hashTransferEncoding, 11, 28, false, false, false, false}, (*fcgiResponse).checkTransferEncoding}, // deliberately false
		1: {fdesc{hashConnection, 0, 10, false, false, false, false}, (*fcgiResponse).checkConnection},
		2: {fdesc{hashUpgrade, 29, 36, false, false, false, false}, (*fcgiResponse).checkUpgrade},
	}
	fcgiResponseImportantHeaderFind = func(hash uint16) int { return (1488 / int(hash)) % 3 }
)

func (r *fcgiResponse) checkConnection(from int, edge int) bool {
	return r._delHeaders(from, edge)
}
func (r *fcgiResponse) checkTransferEncoding(from int, edge int) bool {
	return r._delHeaders(from, edge)
}
func (r *fcgiResponse) checkUpgrade(from int, edge int) bool {
	return r._delHeaders(from, edge)
}
func (r *fcgiResponse) _delHeaders(from int, edge int) bool {
	for i := from; i < edge; i++ {
		r.headers[i].zero()
	}
	return true
}

func (r *fcgiResponse) _parseHeader(header *pair, desc *fdesc, fully bool) bool {
	// TODO
	return false
}
func (r *fcgiResponse) _splitHeader(header *pair, desc *fdesc) bool {
	// TODO
	return true
}

func (r *fcgiResponse) delHopHeaders() {} // for fcgi, nothing to delete
func (r *fcgiResponse) forHeaders(fn func(header *pair, name []byte, value []byte) bool) bool {
	for i := 1; i < len(r.headers); i++ { // r.headers[0] is not used
		if header := &r.headers[i]; header.hash != 0 && !header.isSubField() { // skip sub headers, only collect main headers
			if !fn(header, header.nameAt(r.input), header.valueAt(r.input)) {
				return false
			}
		}
	}
	return true
}

func (r *fcgiResponse) examineHead() bool {
	for i, n := 0, len(r.headers); i < n; i++ {
		if !r.applyHeader(&r.headers[i], i) {
			// r.headResult is set.
			return false
		}
	}
	// content length is not known at this time, can't check.
	return true
}
func (r *fcgiResponse) cleanInput() {
	if !r.hasContent() {
		return
	}
	r.imme.set(r.pFore, r.inputEdge)
	// We don't know the size of unsized content. Let content receiver to decide & clean r.input.
}

func (r *fcgiResponse) ContentSize() int64 { return -2 } // fcgi is unsized by default. we believe in framing

func (r *fcgiResponse) isUnsized() bool { return true } // fcgi is unsized by default. we believe in framing

func (r *fcgiResponse) hasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return true
}
func (r *fcgiResponse) holdContent() any {
	switch content := r.recvContent().(type) {
	case TempFile: // [0, r.maxContentSize]
		r.contentHeld = content.(*os.File)
		return r.contentHeld
	case error: // i/o error or unexpected EOF
		// TODO: log err?
	}
	r.stream.markBroken()
	return nil
}
func (r *fcgiResponse) recvContent() any { // to TempFile
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
	return contentFile // the TempFile
badRead:
	contentFile.Close()
	os.Remove(contentFile.Name())
	return err
}
func (r *fcgiResponse) readContent() (p []byte, err error) { // data in stdout records
	// TODO
	if r.imme.notEmpty() {
		// copy ...
		r.imme.zero()
	}
	return
}

func (r *fcgiResponse) addTrailer(trailer *pair) bool   { return true }  // fcgi doesn't support trailers
func (r *fcgiResponse) applyTrailer(trailer *pair) bool { return true }  // fcgi doesn't support trailers
func (r *fcgiResponse) HasTrailers() bool               { return false } // fcgi doesn't support trailers
func (r *fcgiResponse) delHopTrailers()                 {}               // fcgi doesn't support trailers
func (r *fcgiResponse) forTrailers(fn func(trailer *pair, name []byte, value []byte) bool) bool { // fcgi doesn't support trailers
	return true
}

func (r *fcgiResponse) arrayCopy(p []byte) bool { return true } // not used, but required by httpIn interface

func (r *fcgiResponse) saveContentFilesDir() string { return r.stream.agent.SaveContentFilesDir() }

func (r *fcgiResponse) _newTempFile() (TempFile, error) { // to save content to
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

func (r *fcgiResponse) _recvStdout() (int32, int32, error) { // r.records[from:edge] is the stdout data.
	for { // only for stdout records
		kind, from, edge, err := r._recvRecord()
		if err != nil {
			return 0, 0, err
		}
		if kind == fcgiTypeStdout {
			return from, edge, nil
		}
		if kind != fcgiTypeStderr {
			return 0, 0, fcgiReadBadRecord
		}
		if IsDebug(2) && edge > from {
			Debugf("fcgi stderr=%s\n", r.records[from:edge])
		}
	}
}
func (r *fcgiResponse) _recvEndRequest() (appStatus int32, err error) { // after emptyStdout, endRequest is followed.
	for { // only for endRequest records
		kind, from, edge, err := r._recvRecord()
		if err != nil {
			return 0, err
		}
		if kind != fcgiTypeEndRequest {
			if IsDebug(2) {
				Debugf("fcgi unexpected type=%d payload=%s\n", kind, r.records[from:edge])
			}
			continue
		}
		if edge-from != 8 { // payloadLen of endRequest
			return 0, fcgiReadBadRecord
		}
		appStatus = int32(r.records[from+3])
		appStatus += int32(r.records[from+2]) << 8
		appStatus += int32(r.records[from+1]) << 16
		appStatus += int32(r.records[from]) << 24
		return appStatus, nil
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
	// FCGI header is immediate.
	payloadLen := int32(r.records[r.recordsFrom+4])<<8 + int32(r.records[r.recordsFrom+5])
	recordSize := fcgiHeaderSize + payloadLen + int32(r.records[r.recordsFrom+6]) // with padding
	// Is the whole record immediate?
	if recordSize > remainSize { // not immediate, we need to make it immediate by reading the missing bytes
		// Shoud we switch to a larger r.records?
		if recordSize > int32(cap(r.records)) { // yes, because this record is too large
			var records []byte
			if recordSize <= _16K {
				records = Get16K()
			} else { // recordSize > _16K
				records = getFCGIRecords()
			}
			r._slideRecords(records)
			if cap(r.records) != cap(r.stockRecords) {
				PutNK(r.records) // must be 16K
			}
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
	edge = from + payloadLen
	// Clean up positions.
	if recordSize == remainSize { // all remain data are consumed, reset positions
		r.recordsFrom, r.recordsEdge = 0, 0
	} else { // recordSize < remainSize, extra records exist, mark it for next recv
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
func (r *fcgiResponse) _slideRecords(records []byte) { // so we get space to grow
	if r.recordsFrom > 0 {
		copy(records, r.records[r.recordsFrom:r.recordsEdge])
		r.recordsEdge -= r.recordsFrom
		if r.imme.notEmpty() {
			r.imme.sub(r.recordsFrom)
		}
		r.recordsFrom = 0
	}
}

/*
func (r *fcgiResponse) _switchRecords() { // for better performance when receiving response content, we need a 16K or larger r.records
	if cap(r.records) == cap(r.stockRecords) { // was using stock. switch to 16K
		records := Get16K()
		if imme := r.imme; imme.notEmpty() {
			copy(records[imme.from:imme.edge], r.records[imme.from:imme.edge])
		}
		r.records = records
	} else {
		// Has been 16K or fcgiMaxRecords already, do nothing.
	}
}
*/

var ( // fcgi response errors
	fcgiReadBadRecord = errors.New("fcgi: bad record")
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

// FCGI protocol elements.

// FCGI Record = FCGI Header(8) + payload + padding
// FCGI Header = version(1) + type(1) + requestId(2) + payloadLen(2) + paddingLen(1) + reserved(1)

const ( // fcgi constants
	fcgiHeaderSize = 8
	fcgiMaxPadding = 255
)

// Discrete records are standalone.
// Streamed records end with an empty record (payloadLen=0).

const ( // request record types
	fcgiTypeBeginRequest = 1 // [D] only one
	fcgiTypeParams       = 4 // [S] only one in our implementation (ends with an emptyParams record)
	fcgiTypeStdin        = 5 // [S] many (ends with an emptyStdin record)
)

var ( // request records
	fcgiBeginKeepConn = []byte{ // 16 bytes
		1, fcgiTypeBeginRequest, // version, type
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1. same below
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 1, 0, 0, 0, 0, 0, // role=responder, flags=keepConn
	}
	fcgiBeginDontKeep = []byte{ // 16 bytes
		1, fcgiTypeBeginRequest, // version, type
		0, 1, // request id = 1
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 0, 0, 0, 0, 0, 0, // role=responder, flags=dontKeep
	}
	fcgiEmptyParams = []byte{ // 8 bytes
		1, fcgiTypeParams, // version, type
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	}
	fcgiEmptyStdin = []byte{ // 8 bytes
		1, fcgiTypeStdin, // version, type
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

const ( // response record types
	fcgiTypeStdout     = 6 // [S] many (ends with an emptyStdout record)
	fcgiTypeStderr     = 7 // [S] many (ends with an emptyStderr record)
	fcgiTypeEndRequest = 3 // [D] only one
)

const ( // response header hashes
	fcgiHashStatus = 676
)
