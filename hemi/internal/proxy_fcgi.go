// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI proxy handlets pass requests to backend FCGI servers and cache responses.

// FCGI (FastCGI) is mainly for PHP applications.

// FCGI uses a framing protocol like HTTP/2, so "content-length" is not required.
// It relies on framing protocol to decide when or where the content is finished.
// So, FCGI allows chunked content in HTTP, and we are not obliged to buffer content.

// But, FCGI does not support HTTP trailers.

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
	// Assocs
	stage   *Stage   // current stage
	app     *App     // the app to which the proxy belongs
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher   // the cache server which is used by this proxy
	// States
	scriptFilename      string      // ...
	bufferClientContent bool        // ...
	bufferServerContent bool        // server content is buffered anyway?
	keepConn            bool        // instructs FCGI server to keep conn?
	addRequestHeaders   [][2][]byte // headers appended to client request
	delRequestHeaders   [][]byte    // client request headers to delete
	addResponseHeaders  [][2][]byte // headers appended to server response
	delResponseHeaders  [][]byte    // server response headers to delete
	dialTimeout         time.Duration
	writeTimeout        time.Duration
	readTimeout         time.Duration
	maxSendTimeout      time.Duration
	maxRecvTimeout      time.Duration
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
	h.ConfigureString("scriptFilename", &h.scriptFilename, nil, "")
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// keepConn
	h.ConfigureBool("keepConn", &h.keepConn, false)
	// dialTimeout
	h.ConfigureDuration("dialTimeout", &h.dialTimeout, func(value time.Duration) bool { return value > time.Second }, 10*time.Second)
	// writeTimeout
	h.ConfigureDuration("writeTimeout", &h.writeTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
	// readTimeout
	h.ConfigureDuration("readTimeout", &h.readTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
}
func (h *fcgiProxy) OnPrepare() {
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
	if hasContent && h.bufferClientContent { // including size 0
		content = req.holdContent()
		if content == nil {
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.keepConn { // FCGI_KEEP_CONN=1
		fConn, fErr = h.backend.FetchConn()
		if fErr != nil {
			resp.SendBadGateway(nil)
			return
		}
		defer h.backend.StoreConn(fConn)
	} else { // FCGI_KEEP_CONN=0
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

	if h.scriptFilename == "" {
		// use absPath as SCRIPT_FILENAME
	} else {
		// use h.scriptFilename as SCRIPT_FILENAME
	}

	if !fReq.copyHead(req) {
		fStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if !hasContent || h.bufferClientContent {
		fErr = fReq.post(content)
		if fErr != nil {
			fStream.markBroken()
		}
	} else if fErr = fReq.sync(req); fErr != nil {
		fStream.markBroken()
	} else if fReq.contentSize == -2 {
		// write last chunk?
	}
	if fErr != nil {
		resp.SendBadGateway(nil)
		return
	}

	fResp := &fStream.response
	for { // until we found a non-1xx status (>= 200)
		fResp.recvHead()
		if fResp.headResult != StatusOK || fResp.status == StatusSwitchingProtocols {
			fStream.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		if fResp.status >= StatusOK {
			break
		}
		if req.VersionCode() == Version1_0 {
			fStream.markBroken()
			resp.SendBadGateway(nil)
			return
		}
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
	if !fHasContent || h.bufferServerContent {
		if resp.post(fContent, false) != nil {
			return
		}
	} else if err := resp.sync(fResp); err != nil {
		fStream.markBroken()
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

func (s *fcgiStream) isBroken() bool {
	return s.conn.IsBroken()
}
func (s *fcgiStream) markBroken() {
	s.conn.MarkBroken()
}

// fcgiRequest
type fcgiRequest struct {
	// Assocs
	stream   *fcgiStream
	response *fcgiResponse
	// States (buffers)
	stockParams [1536]byte
	// States (non-zeros)
	params         []byte
	contentSize    int64
	maxSendTimeout time.Duration
	// States (zeros)
	sendTime    time.Time   // the time when first send operation is performed
	vector      net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector [4][]byte   // for sending/pushing request. reset after stream. 96B
	fcgiRequest0
}
type fcgiRequest0 struct {
	paramsEdge    uint16 // edge of r.params. max size of r.params must be <= fcgiMaxParams.
	isSent        bool   // whether the request is sent
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
}

func (r *fcgiRequest) onUse() {
	r.params = r.stockParams[:]
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
	r.fixedVector = [4][]byte{}
	r.fcgiRequest0 = fcgiRequest0{}
}

func (r *fcgiRequest) copyHead(req Request) bool {
	// TODO
	return false
}
func (r *fcgiRequest) addParam(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *fcgiRequest) addSchemeParam(name []byte, value []byte) bool {
	// TODO
	return false
}

func (r *fcgiRequest) setMaxSendTimeout(timeout time.Duration) {
	r.maxSendTimeout = timeout
}

func (r *fcgiRequest) sendBlob(content []byte) error {
	// TODO
	return nil
}
func (r *fcgiRequest) sendFile(content *os.File, info os.FileInfo) error {
	// TODO
	return nil
}
func (r *fcgiRequest) pushBlob(chunk []byte) error {
	// TODO
	return nil
}

func (r *fcgiRequest) sync(req Request) error {
	pass := r.syncBytes
	if size := req.ContentSize(); size == -2 {
		pass = r.pushBlob
	} else { // size >= 0
		r.isSent = true
		r.contentSize = size
		if err := r.syncHeaders(); err != nil {
			return err
		}
	}
	for {
		p, err := req.readContent()
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
	return nil
}
func (r *fcgiRequest) syncBytes(p []byte) error {
	// TODO
	return nil
}
func (r *fcgiRequest) syncHeaders() error {
	// TODO
	if r.stream.proxy.keepConn {
		// use fcgiBeginKeepConn
	} else {
		// use fcgiBeginDontKeep
	}
	return nil
}
func (r *fcgiRequest) post(content any) error { // nil, []byte, *os.File
	if contentFile, ok := content.(*os.File); ok {
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		return r.sendFile(contentFile, fileInfo)
	} else if contentBlob, ok := content.([]byte); ok {
		return r.sendBlob(contentBlob)
	} else {
		return r.sendBlob(nil)
	}
}

func (r *fcgiRequest) growParams(size int) (from int, edge int, ok bool) {
	// TODO: use getFCGIParams
	return
}
func (r *fcgiRequest) beforeWrite() error {
	now := time.Now()
	if r.sendTime.IsZero() {
		r.sendTime = now
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.proxy.writeTimeout))
}

// poolFCGIParams
var poolFCGIParams sync.Pool

const fcgiMaxParams = 8 + 16384 // header + content

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

// fcgiResponse
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
	contentSize    int64
	maxRecvTimeout time.Duration
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
type fcgiResponse0 struct {
	pBack           int16 // element begins from. for parsing
	pFore           int16 // element spanning to. for parsing
	status          int16
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
	r.contentSize = -1
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
func (r *fcgiResponse) nextRecord() {
	// TODO: use getFCGIInput
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

var ( // perfect hash table for response critical headers
	fcgiResponseCriticalHeaderNames = []byte("content-length transfer-encoding")
	fcgiResponseCriticalHeaderTable = [1]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*fcgiResponse, *pair, uint8) bool
	}{
		0: {123, 0, 4, (*fcgiResponse).checkTransferEncoding},
	}
	fcgiResponseCriticalHeaderFind = func(hash uint16) int { return 0 }
)

func (r *fcgiResponse) checkTransferEncoding(header *pair, index uint8) bool {
	// TODO
	return true
}
func (r *fcgiResponse) checkContentLength(header *pair, index uint8) bool {
	// TODO
	return true
}

func (r *fcgiResponse) ContentSize() int64 { return r.contentSize }
func (r *fcgiResponse) UnsafeContentType() []byte {
	if r.indexes.contentType == 0 {
		return nil
	}
	return r.headers[r.indexes.contentType].valueAt(r.input)
}
func (r *fcgiResponse) unsafeDate() []byte {
	// TODO
	return nil
}
func (r *fcgiResponse) unsafeLastModified() []byte {
	// TODO
	return nil
}
func (r *fcgiResponse) delHopHeaders() {
	// TODO
}

func (r *fcgiResponse) parseSetCookie() bool {
	// TODO
	return false
}
func (r *fcgiResponse) hasSetCookies() bool {
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

func (r *fcgiResponse) SetMaxRecvTimeout(timeout time.Duration) {
	r.maxRecvTimeout = timeout
}

func (r *fcgiResponse) UnsafeContent() []byte {
	// TODO
	return nil
}
func (r *fcgiResponse) hasContent() bool {
	// TODO
	return false
}
func (r *fcgiResponse) readContent() (p []byte, err error) {
	// TODO
	return
}
func (r *fcgiResponse) holdContent() any {
	// TODO
	return nil
}
func (r *fcgiResponse) recvContent(retain bool) any { // to []byte (for small content) or TempFile (for large content)
	// TODO
	return nil
}

func (r *fcgiResponse) HasTrailers() bool               { return false } // fcgi doesn't support trailers
func (r *fcgiResponse) applyTrailer(trailer *pair) bool { return true }  // fcgi doesn't support trailers
func (r *fcgiResponse) walkTrailers(fn func(hash uint16, name []byte, value []byte) bool) bool {
	return true // fcgi doesn't support trailers
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
	// TODO
	return ""
}

func (r *fcgiResponse) newTempFile() {
	// TODO
}
func (r *fcgiResponse) beforeRead(toTime *time.Time) error {
	now := time.Now()
	if toTime.IsZero() {
		*toTime = now
	}
	return r.stream.setReadDeadline(now.Add(r.stream.proxy.readTimeout))
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
		1, // version
		fcgiTypeBeginRequest,
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length = 8
		0, 0, // padding length = 0 & reserved
		// content=8
		0, 1, 1, // role=responder, flags=keepConn
		0, 0, 0, 0, 0, // reserved
	}
	fcgiBeginDontKeep = []byte{ // 16 bytes
		// header=8
		1, // version
		fcgiTypeBeginRequest,
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length = 8
		0, 0, // padding length = 0 & reserved
		// content=8
		0, 1, 0, // role=responder, flags=dontKeep
		0, 0, 0, 0, 0, // reserved
	}
	fcgiEndParams = []byte{ // 8 bytes
		// header=8
		1, // version
		fcgiTypeParams,
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 0, // content length = 0
		0, 0, // padding length = 0 & reserved
		// content=0
	}
	fcgiEndStdin = []byte{ // 8 bytes
		// header=8
		1, // version
		fcgiTypeStdin,
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 0, // content length = 0
		0, 0, // padding length = 0 & reserved
		// content=0
	}
)

const ( // response record types
	fcgiTypeStdout     = 6
	fcgiTypeStderr     = 7
	fcgiTypeEndRequest = 3
)
