// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI proxy handlets pass requests to backend FCGI servers and cache responses.

// FCGI (FastCGI) is mainly for PHP applications. It does not support chunking and trailers.

package internal

import (
	"io"
	"net"
	"os"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("fcgiProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(fcgiProxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// fcgiProxy handlet
type fcgiProxy struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage   // current stage
	app     *App     // the app to which the proxy belongs
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher   // the cache server which is used by this proxy
	// States
	scriptFilename      []byte        // ...
	bufferClientContent bool          // ...
	bufferServerContent bool          // server content is buffered anyway?
	keepConn            bool          // instructs FCGI server to keep conn?
	addRequestHeaders   [][2][]byte   // headers appended to client request
	delRequestHeaders   [][]byte      // client request headers to delete
	addResponseHeaders  [][2][]byte   // headers appended to server response
	delResponseHeaders  [][]byte      // server response headers to delete
	maxSendTimeout      time.Duration // max timeout to send request
	maxRecvTimeout      time.Duration // max timeout to recv response
}

func (h *fcgiProxy) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *fcgiProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *fcgiProxy) OnConfigure() {
	h.contentSaver_.onConfigure(h, TempDir()+"/fcgi/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if pBackend, ok := backend.(PBackend); ok {
				h.backend = pBackend
			} else {
				UseExitf("incorrect backend '%s' for fcgiProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for fcgiProxy")
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
	// maxSendTimeout
	h.ConfigureDuration("maxSendTimeout", &h.maxSendTimeout, func(value time.Duration) bool { return value >= 0 }, 0)
	// maxRecvTimeout
	h.ConfigureDuration("maxRecvTimeout", &h.maxRecvTimeout, func(value time.Duration) bool { return value >= 0 }, 0)
}
func (h *fcgiProxy) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *fcgiProxy) IsProxy() bool { return true }
func (h *fcgiProxy) IsCache() bool { return h.cacher != nil }

func (h *fcgiProxy) Handle(req Request, resp Response) (next bool) {
	var (
		content  any
		fContent any
		fErr     error
		fConn    PConn
	)

	hasContent := req.HasContent()
	if hasContent && (h.bufferClientContent || req.isChunked()) { // including size 0
		content = req.holdContent()
		if content == nil {
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
	if hasContent && !h.bufferClientContent && !req.isChunked() {
		if fErr = fReq.sync(req); fErr != nil {
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
		if fContent == nil {
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
		if err := resp.sync(fResp); err != nil {
			fStream.markBroken()
			return
		}
	} else if err := resp.post(fContent, false); err != nil {
		return
	}

	return
}

// poolFCGIStream
var poolFCGIStream sync.Pool

func getFCGIStream(proxy *fcgiProxy, conn PConn) *fcgiStream {
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
	stream.onUse(proxy, conn)
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
	stockBuffer [256]byte
	// Stream states (controlled)
	// Stream states (non-zeros)
	proxy  *fcgiProxy // associated proxy
	conn   PConn      // associated conn
	region Region     // a region-based memory pool
	// Stream states (zeros)
}

func (s *fcgiStream) onUse(proxy *fcgiProxy, conn PConn) {
	s.proxy = proxy
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
	s.proxy = nil
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

func (s *fcgiStream) write(p []byte) (int, error)               { return s.conn.Write(p) }
func (s *fcgiStream) writev(vector *net.Buffers) (int64, error) { return s.conn.Writev(vector) }
func (s *fcgiStream) read(p []byte) (int, error)                { return s.conn.Read(p) }
func (s *fcgiStream) readFull(p []byte) (int, error)            { return s.conn.ReadFull(p) }

func (s *fcgiStream) isBroken() bool { return s.conn.IsBroken() }
func (s *fcgiStream) markBroken()    { s.conn.MarkBroken() }

// fcgiRequest
type fcgiRequest struct {
	// Assocs
	stream   *fcgiStream
	response *fcgiResponse
	// States (buffers)
	stockParams [2048]byte
	// States (non-zeros)
	params         []byte // place exactly one FCGI_PARAMS record
	contentSize    int64
	maxSendTimeout time.Duration // max timeout to send request
	// States (zeros)
	sendTime     time.Time   // the time when first send operation is performed
	vector       net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector  [5][]byte   // for sending request. reset after stream. 120B
	fcgiRequest0             // all values must be zero by default in this struct!
}
type fcgiRequest0 struct { // for fast reset, entirely
	paramsEdge    uint16 // edge of r.params. max size of r.params must be <= fcgiMaxParams.
	isSent        bool   // whether the request is sent
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
}

func (r *fcgiRequest) onUse() {
	r.params = r.stockParams[:]
	copy(r.params, fcgiParamsHeader)
	r.contentSize = -1 // not set
	r.maxSendTimeout = r.stream.proxy.maxSendTimeout
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
	if !r.addMetaParam(fcgiBytesScriptFilename, value) {
		return false
	}
	// TODO
	return true
}
func (r *fcgiRequest) addMetaParam(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *fcgiRequest) addHTTPParam(name []byte, value []byte) bool {
	// TODO
	return false
}

func (r *fcgiRequest) setMaxSendTimeout(timeout time.Duration) { r.maxSendTimeout = timeout }

func (r *fcgiRequest) sync(req Request) error { // only for counted (>0) content
	r.isSent = true
	r.contentSize = req.ContentSize()
	if err := r.syncParams(); err != nil {
		return err
	}
	for {
		p, err := req.readContent()
		if len(p) >= 0 {
			if e := r.syncBytes(p); e != nil {
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
	return nil
}
func (r *fcgiRequest) syncParams() error {
	// TODO: build the params record, sync beginRequest and params
	if r.stream.proxy.keepConn {
		// use fcgiBeginKeepConn
	} else {
		// use fcgiBeginDontKeep
	}
	return nil
}
func (r *fcgiRequest) syncBytes(p []byte) error {
	// TODO: create a stdin record, sync
	return nil
}
func (r *fcgiRequest) post(content any) error { // nil, []byte, *os.File. for bufferClientContent or chunked Request content
	if contentBlob, ok := content.([]byte); ok {
		return r.sendBlob(contentBlob)
	} else if contentFile, ok := content.(*os.File); ok {
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		return r.sendFile(contentFile, fileInfo)
	} else { // nil, means no content
		return r.sendBlob(nil)
	}
}

func (r *fcgiRequest) checkSend() error {
	if r.isSent {
		return httpAlreadySent
	}
	r.isSent = true
	return nil
}
func (r *fcgiRequest) sendBlob(content []byte) error { // with params
	if err := r.checkSend(); err != nil {
		return err
	}
	// TODO: beginRequest + params + stdin
	return nil
}
func (r *fcgiRequest) sendFile(content *os.File, info os.FileInfo) error { // with params
	if err := r.checkSend(); err != nil {
		return err
	}
	// TODO: beginRequest + params + stdin
	return nil
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
		copy(params, r.params[0:r.paramsEdge])
		r.params = params
	}
	r.paramsEdge = last
	edge, ok = int(r.paramsEdge), true
	return
}

func (r *fcgiRequest) _beforeWrite() error {
	now := time.Now()
	if r.sendTime.IsZero() {
		r.sendTime = now
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.proxy.backend.WriteTimeout()))
}

// poolFCGIParams
var poolFCGIParams sync.Pool

const fcgiMaxParams = _16K // header + content

func getFCGIParams() []byte {
	if x := poolFCGIParams.Get(); x == nil {
		return make([]byte, fcgiMaxParams)
	} else {
		return x.([]byte)
	}
}
func putFCGIParams(params []byte) {
	if cap(params) != fcgiMaxParams {
		BugExitln("bad fcgi params")
	}
	poolFCGIParams.Put(params)
}

// fcgiResponse must implements response interface.
type fcgiResponse struct {
	// Assocs
	stream *fcgiStream
	// States (buffers)
	stockInput   [2048]byte // for fcgi response headers
	stockHeaders [64]pair
	// States (controlled)
	header pair // to overcome ...
	// States (non-zeros)
	input          []byte
	headers        []pair
	contentSize    int64         // info of content. >=0: content-length, -1: no content, -2: chunked content
	maxRecvTimeout time.Duration // max timeout to recv response
	headResult     int16
	// States (zeros)
	headReason    string    // the reason of head result
	inputEdge     int32     // edge position of current response
	bodyWindow    []byte    // a window used for receiving content
	recvTime      time.Time // the time when receiving response
	bodyTime      time.Time // the time when first body read operation is performed on this stream
	contentBlob   []byte    // if loadable, the received and loaded content of current response is at r.contentBlob[:r.sizeReceived]
	contentHeld   *os.File  // used by r.holdContent(), if content is TempFile. will be closed on stream ends
	fcgiResponse0           // all values must be zero by default in this struct!
}
type fcgiResponse0 struct { // for fast reset, entirely
	pBack           int16    // element begins from. for parsing
	pFore           int16    // element spanning to. for parsing
	status          int16    // 200, 302, 404, ...
	receiving       int8     // currently receiving. see httpSectionXXX
	contentReceived bool     // is content received? if response has no content, it is true (received)
	contentBlobKind int8     // kind of current r.contentBlob. see httpContentBlobXXX
	maxContentSize  int64    // max content size allowed for current response
	sizeReceived    int64    // bytes of currently received content
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
	r.input = r.stockInput[:]
	r.headers = r.stockHeaders[0:1:cap(r.stockHeaders)] // use append(). r.headers[0] is skipped due to zero value of header indexes.
	r.contentSize = -1                                  // no content
	r.maxRecvTimeout = r.stream.proxy.maxRecvTimeout
	r.headResult = StatusOK
}
func (r *fcgiResponse) onEnd() {
	if cap(r.input) != cap(r.stockInput) {
		putFCGIInput(r.input)
		r.input, r.inputEdge = nil, 0
	}
	if cap(r.headers) != cap(r.stockHeaders) {
		// TODO: put
		r.headers = nil
	}

	r.headReason = ""

	if r.bodyWindow != nil {
		// TODO: put
		r.bodyWindow = nil
	}

	r.recvTime = time.Time{}
	r.bodyTime = time.Time{}

	if r.contentBlobKind == httpContentBlobPool {
		PutNK(r.contentBlob)
	}
	r.contentBlob = nil // other blob kinds are only references, just reset.

	if r.contentHeld != nil {
		r.contentHeld.Close()
		os.Remove(r.contentHeld.Name())
		r.contentHeld = nil
		if IsDebug(2) {
			Debugln("contentHeld is closed and removed!!")
		}
	}

	r.fcgiResponse0 = fcgiResponse0{}
}

func (r *fcgiResponse) recvHead() {
	// TODO
}
func (r *fcgiResponse) _nextRecord() {
	// TODO: use getFCGIInput if necessary
}

func (r *fcgiResponse) Status() int16 { return r.status }

func (r *fcgiResponse) applyHeader(header *pair) bool {
	// TODO
	return false
}
func (r *fcgiResponse) walkHeaders(fn func(hash uint16, name []byte, value []byte) bool) bool {
	// TODO
	return false
}

var ( // perfect hash table for response multiple headers
	fcgiResponseMultipleHeaderNames = []byte("connection transfer-encoding")
	fcgiResponseMultipleHeaderTable = [2]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*fcgiResponse, uint8, uint8) bool
	}{
		0: {httpHashConnection, 0, 4, (*fcgiResponse).checkConnection},
		1: {httpHashTransferEncoding, 0, 4, (*fcgiResponse).checkTransferEncoding},
	}
	fcgiResponseMultipleHeaderFind = func(hash uint16) int { return 0 }
)

func (r *fcgiResponse) checkConnection(from uint8, edge uint8) bool {
	for i := from; i < edge; i++ {
		r.headers[i].zero()
	}
	return true
}
func (r *fcgiResponse) checkTransferEncoding(from uint8, edge uint8) bool {
	for i := from; i < edge; i++ {
		r.headers[i].zero()
	}
	return true
}

var ( // perfect hash table for response critical headers
	fcgiResponseCriticalHeaderNames = []byte("content-length content-type status")
	fcgiResponseCriticalHeaderTable = [3]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*fcgiResponse, *pair, uint8) bool
	}{
		0: {httpHashContentLength, 0, 4, (*fcgiResponse).checkContentLength},
		1: {httpHashContentType, 0, 4, (*fcgiResponse).checkContentType},
		2: {fcgiHashStatus, 0, 4, (*fcgiResponse).checkStatus},
	}
	fcgiResponseCriticalHeaderFind = func(hash uint16) int { return 0 }
)

func (r *fcgiResponse) checkContentLength(header *pair, index uint8) bool {
	r.headers[index].zero() // we don't believe the value provided by fcgi application.
	return true
}
func (r *fcgiResponse) checkContentType(header *pair, index uint8) bool {
	r.indexes.contentType = index
	return true
}
func (r *fcgiResponse) checkStatus(header *pair, index uint8) bool {
	// TODO
	return true
}

func (r *fcgiResponse) ContentSize() int64 { return r.contentSize }
func (r *fcgiResponse) unsafeContentType() []byte {
	if r.indexes.contentType == 0 {
		return nil
	}
	return r.headers[r.indexes.contentType].valueAt(r.input)
}
func (r *fcgiResponse) delHopHeaders() {} // for fcgi, nothing to delete

func (r *fcgiResponse) parseSetCookie() bool {
	// TODO
	return false
}

func (r *fcgiResponse) checkHead() bool {
	// TODO
	return false
}

func (r *fcgiResponse) cleanInput() {
	// TODO
}

func (r *fcgiResponse) markChunked()                            { r.contentSize = -2 }
func (r *fcgiResponse) isChunked() bool                         { return r.contentSize == -2 }
func (r *fcgiResponse) setMaxRecvTimeout(timeout time.Duration) { r.maxRecvTimeout = timeout }

func (r *fcgiResponse) hasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return r.contentSize >= 0 || r.isChunked()
}
func (r *fcgiResponse) holdContent() any {
	// TODO
	return nil
}
func (r *fcgiResponse) recvContent() any { // to []byte (for small content) or TempFile (for large content)
	// TODO
	return nil
}
func (r *fcgiResponse) readContent() (p []byte, err error) {
	// TODO
	return
}

func (r *fcgiResponse) HasTrailers() bool               { return false } // fcgi doesn't support trailers
func (r *fcgiResponse) applyTrailer(trailer *pair) bool { return true }  // fcgi doesn't support trailers
func (r *fcgiResponse) walkTrailers(fn func(hash uint16, name []byte, value []byte) bool) bool { // fcgi doesn't support trailers
	return true
}
func (r *fcgiResponse) delHopTrailers() {} // fcgi doesn't support trailers

func (r *fcgiResponse) addHeader(header *pair) (edge uint8, ok bool) {
	// TODO
	return
}
func (r *fcgiResponse) getHeader(name string, hash uint16) (value []byte, ok bool) {
	// TODO
	return
}

func (r *fcgiResponse) arrayCopy(p []byte) bool { return true } // not used, but required by response interface

func (r *fcgiResponse) saveContentFilesDir() string {
	return r.stream.proxy.SaveContentFilesDir() // must ends with '/'
}

func (r *fcgiResponse) _newTempFile() (TempFile, error) {
	// TODO
	return nil, nil
}
func (r *fcgiResponse) _beforeRead(toTime *time.Time) error {
	now := time.Now()
	if toTime.IsZero() {
		*toTime = now
	}
	return r.stream.setReadDeadline(now.Add(r.stream.proxy.backend.ReadTimeout()))
}

// poolFCGIInput
var poolFCGIInput sync.Pool

const fcgiMaxRecord = 8 + 65535 + 255 // header + max content + max padding

func getFCGIInput() []byte {
	if x := poolFCGIInput.Get(); x == nil {
		return make([]byte, fcgiMaxRecord)
	} else {
		return x.([]byte)
	}
}
func putFCGIInput(input []byte) {
	if cap(input) != fcgiMaxRecord {
		BugExitln("bad fcgi input")
	}
	poolFCGIInput.Put(input)
}

// FCGI protocol elements.

// FCGI Record = FCGI Header(8) + content + padding
// FCGI Header = version(1) + type(1) + requestId(2) + contentLength(2) + paddingLength(1) + reserved(1)

// Discrete records are standalone.
// Stream records end with an empty record (contentLength=0).

const ( // request record types
	fcgiTypeBeginRequest = 1
	fcgiTypeParams       = 4
	fcgiTypeStdin        = 5
)

var ( // predefined request records
	fcgiBeginKeepConn = []byte{ // 16 bytes
		// header=8
		1, fcgiTypeBeginRequest, // version, type
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1. same below
		0, 8, // content length = 8
		0, 0, // padding length = 0, reserved = 0
		// content=8
		0, 1, 1, // role=responder, flags=keepConn
		0, 0, 0, 0, 0, // reserved
	}
	fcgiBeginDontKeep = []byte{ // 16 bytes
		// header=8
		1, fcgiTypeBeginRequest, // version, type
		0, 1, // request id = 1
		0, 8, // content length = 8
		0, 0, // padding length = 0, reserved = 0
		// content=8
		0, 1, 0, // role=responder, flags=dontKeep
		0, 0, 0, 0, 0, // reserved
	}
	fcgiParamsHeader = []byte{ // 8 bytes
		// header=8
		1, fcgiTypeParams, // version, type
		0, 1, // request id = 1
		0, 0, // content length = 0
		0, 0, // padding length = 0, reserved = 0
	}
	fcgiEndParams = []byte{ // 8 bytes
		// header=8
		1, fcgiTypeParams, // version, type
		0, 1, // request id = 1
		0, 0, // content length = 0
		0, 0, // padding length = 0, reserved = 0
		// content=0
	}
	fcgiEndStdin = []byte{ // 8 bytes
		// header=8
		1, fcgiTypeStdin, // version, type
		0, 1, // request id = 1
		0, 0, // content length = 0
		0, 0, // padding length = 0, reserved = 0
		// content=0
	}
)

const ( // response record types
	fcgiTypeStdout     = 6
	fcgiTypeStderr     = 7
	fcgiTypeEndRequest = 3
)

const ( // fcgi hashes
	fcgiHashStatus = 676
)

var ( // fcgi variables
	fcgiBytesAuthType         = []byte("AUTH_TYPE")
	fcgiBytesContentLength    = []byte("CONTENT_LENGTH")
	fcgiBytesContentType      = []byte("CONTENT_TYPE")
	fcgiBytesDocumentRoot     = []byte("DOCUMENT_ROOT")
	fcgiBytesDocumentURI      = []byte("DOCUMENT_URI")
	fcgiBytesGatewayInterface = []byte("GATEWAY_INTERFACE")
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
