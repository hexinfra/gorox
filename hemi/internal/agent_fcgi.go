// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI agent handlets pass requests to backend FastCGI servers and cache responses.

// FastCGI is mainly used by PHP applications. It does not support HTTP trailers and request-side chunking.

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
	scriptFilename      []byte        // ...
	bufferClientContent bool          // ...
	bufferServerContent bool          // server content is buffered anyway?
	keepConn            bool          // instructs FCGI server to keep conn?
	addRequestHeaders   [][2][]byte   // headers appended to client request
	delRequestHeaders   [][]byte      // client request headers to delete
	addResponseHeaders  [][2][]byte   // headers appended to server response
	delResponseHeaders  [][]byte      // server response headers to delete
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response
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
}
func (h *fcgiAgent) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *fcgiAgent) IsProxy() bool { return true }
func (h *fcgiAgent) IsCache() bool { return h.cacher != nil }

func (h *fcgiAgent) Handle(hReq Request, hResp Response) (next bool) {
	var (
		hContent any
		fConn    PConn
		fErr     error
		fContent any
	)

	hHasContent := hReq.HasContent()
	if hHasContent && (h.bufferClientContent || hReq.isChunked()) { // including size 0
		hContent = hReq.holdContent()
		if hContent == nil {
			hResp.SetStatus(StatusBadRequest)
			hResp.SendBytes(nil)
			return
		}
	}

	if h.keepConn {
		fConn, fErr = h.backend.FetchConn()
		if fErr != nil {
			hResp.SendBadGateway(nil)
			return
		}
		defer h.backend.StoreConn(fConn)
	} else {
		fConn, fErr = h.backend.Dial()
		if fErr != nil {
			hResp.SendBadGateway(nil)
			return
		}
		defer fConn.Close()
	}

	fStream := getFCGIStream(h, fConn)
	defer putFCGIStream(fStream)

	fReq := &fStream.request
	if !fReq.copyHead(hReq, h.scriptFilename) {
		fStream.markBroken()
		hResp.SendBadGateway(nil)
		return
	}
	if hHasContent && !h.bufferClientContent && !hReq.isChunked() {
		if fErr = fReq.sync(hReq); fErr != nil {
			fStream.markBroken()
		}
	} else if fErr = fReq.post(hContent); fErr != nil {
		fStream.markBroken()
	}
	if fErr != nil {
		hResp.SendBadGateway(nil)
		return
	}

	fResp := &fStream.response
	for { // until we found a non-1xx status (>= 200)
		fResp.recvHead()
		if fResp.headResult != StatusOK || fResp.status == StatusSwitchingProtocols { // websocket is not served in handlets.
			fStream.markBroken()
			hResp.SendBadGateway(nil)
			return
		}
		if fResp.status >= StatusOK {
			break
		}
		// We got 1xx
		if hReq.VersionCode() == Version1_0 {
			fStream.markBroken()
			hResp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !hResp.sync1xx(fResp) {
			fStream.markBroken()
			return
		}
		fResp.onEnd()
		fResp.onUse()
	}

	fHasContent := false
	if hReq.MethodCode() != MethodHEAD {
		fHasContent = fResp.hasContent()
	}
	if fHasContent && h.bufferServerContent { // including size 0
		fContent = fResp.holdContent()
		if fContent == nil {
			// fStream is marked as broken
			hResp.SendBadGateway(nil)
			return
		}
	}

	if !hResp.copyHead(fResp) {
		fStream.markBroken()
		return
	}
	if fHasContent && !h.bufferServerContent {
		if err := hResp.sync(fResp); err != nil {
			fStream.markBroken()
			return
		}
	} else if err := hResp.post(fContent, false); err != nil { // false means no trailers
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
	stockBuffer [256]byte
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
	// States (non-zeros)
	params      []byte        // place exactly one FCGI_PARAMS record
	contentSize int64         // -1: not set, -2: chunked encoding, >=0: size
	sendTimeout time.Duration // timeout to send the whole request
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
	copy(r.params, fcgiParamsHeader) // contentLen (r.params[4:6]) needs modification
	r.contentSize = -1               // not set
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
	if !r._addMetaParam(fcgiBytesScriptFilename, value) {
		return false
	}
	// TODO
	return true
}
func (r *fcgiRequest) _addMetaParam(name []byte, value []byte) bool {
	// TODO
	return false
}
func (r *fcgiRequest) _addHTTPParam(name []byte, value []byte) bool {
	// TODO
	return false
}

func (r *fcgiRequest) setSendTimeout(timeout time.Duration) { r.sendTimeout = timeout }

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
	if r.stream.agent.keepConn {
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
	return r.stream.setWriteDeadline(now.Add(r.stream.agent.backend.WriteTimeout()))
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
		BugExitln("fcgi: bad params")
	}
	poolFCGIParams.Put(params)
}

// fcgiResponse must implements response interface.
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
	records     []byte        // bytes of incoming fcgi records. [<r.stockRecords>/16K/fcgiMaxRecords]
	input       []byte        // bytes of incoming response headers. [<r.stockInput>/4K/16K]
	headers     []pair        // fcgi response headers
	recvTimeout time.Duration // timeout to recv the whole response
	headResult  int16         // result of receiving response head. values are same as http status for convenience
	// States (zeros)
	headReason    string    // the reason of head result
	recvTime      time.Time // the time when receiving response
	bodyTime      time.Time // the time when first body read operation is performed on this stream
	contentBlob   []byte    // if loadable, the received and loaded content of current response is at r.contentBlob[:r.sizeReceived]
	contentHeld   *os.File  // used by r.holdContent(), if content is TempFile. will be closed on stream ends
	fcgiResponse0           // all values must be zero by default in this struct!
}
type fcgiResponse0 struct { // for fast reset, entirely
	recordsFrom     int32    // from position of current records
	recordsEdge     int32    // edge position of current records
	inputEdge       int32    // edge position of current input
	head            text     // for debugging
	imme            text     // immediate bytes in r.records that is content
	pBack           int32    // element begins from. for parsing header elements
	pFore           int32    // element spanning to. for parsing header elements
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
	r.records = r.stockRecords[:]
	r.input = r.stockInput[:]
	r.headers = r.stockHeaders[0:1:cap(r.stockHeaders)] // use append(). r.headers[0] is skipped due to zero value of header indexes.
	r.recvTimeout = r.stream.agent.recvTimeout
	r.headResult = StatusOK
}
func (r *fcgiResponse) onEnd() {
	if cap(r.records) != cap(r.stockRecords) {
		if cap(r.records) == fcgiMaxRecords {
			putFCGIMaxRecords(r.records)
		} else {
			PutNK(r.records)
		}
		r.records = nil
	}
	if cap(r.input) != cap(r.stockInput) {
		PutNK(r.input)
		r.input = nil
	}
	if cap(r.headers) != cap(r.stockHeaders) {
		put255Pairs(r.headers)
		r.headers = nil
	}

	r.headReason = ""
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

func (r *fcgiResponse) _readStdout() (int32, int32, error) { // only for stdout records
	for {
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
func (r *fcgiResponse) _readEndRequest() (appStatus int32, err error) { // only for endRequest records
	for {
		kind, from, edge, err := r._recvRecord()
		if err != nil {
			return 0, err
		}
		if kind != fcgiTypeEndRequest {
			if IsDebug(2) {
				Debugf("fcgi type=%d data=%s\n", kind, r.records[from:edge])
			}
			continue
		}
		if edge-from != 8 { // contentLen of endRequest
			return 0, fcgiReadBadRecord
		}
		appStatus = int32(r.records[from+3])
		appStatus += int32(r.records[from+2]) << 8
		appStatus += int32(r.records[from+1]) << 16
		appStatus += int32(r.records[from]) << 24
		return appStatus, nil
	}
}
func (r *fcgiResponse) _recvRecord() (kind byte, from int32, edge int32, err error) {
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
	contentLen := int32(r.records[r.recordsFrom+4])<<8 + int32(r.records[r.recordsFrom+5])
	recordSize := fcgiHeaderSize + contentLen + int32(r.records[r.recordsFrom+6])
	// Is the whole record immediate?
	if recordSize > remainSize { // not immediate, we need to make it immediate by reading the missing bytes
		// Shoud we switch to a larger r.records?
		if recordSize > int32(cap(r.records)) { // yes, the record is too large
			var records []byte
			if recordSize > _16K {
				records = getFCGIMaxRecords()
			} else { // recordSize <= _16K
				records = Get16K()
			}
			r._slideRecords(records)
			if cap(r.records) != cap(r.stockRecords) {
				PutNK(r.records) // must be 16K
			}
			r.records = records
		}
		// Now r.records is large enough. we can read the missing bytes of this record
		if n, e := r._growRecords(int(recordSize - remainSize)); e == nil {
			remainSize += int32(n)
		} else {
			err = e
			return
		}
	}
	// Now recordSize <= remainSize, the record is immediate.
	kind = r.records[r.recordsFrom+1]
	from = r.recordsFrom + fcgiHeaderSize
	edge = from + contentLen
	if recordSize == remainSize { // all remain data are consumed, reset positions
		r.recordsFrom = 0
		r.recordsEdge = 0
	} else { // recordSize < remainSize, extra records exist, mark it for next read
		r.recordsFrom += recordSize
	}
	if contentLen == 0 && (kind == fcgiTypeStdout || kind == fcgiTypeStderr) {
		err = io.EOF
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
func (r *fcgiResponse) _slideRecords(records []byte) {
	if r.recordsFrom > 0 {
		copy(records, r.records[r.recordsFrom:r.recordsEdge])
		r.recordsEdge -= r.recordsFrom
		if r.imme.notEmpty() {
			r.imme.sub(r.recordsFrom)
		}
		r.recordsFrom = 0
	}
}
func (r *fcgiResponse) _switchRecords() { // for receiving response content
	if cap(r.records) == cap(r.stockRecords) { // was using stock. switch to 16K
		records := Get16K()
		if imme := r.imme; imme.notEmpty() {
			copy(records[imme.from:imme.edge], r.records[imme.from:imme.edge])
		}
		r.records = records
	}
}

func (r *fcgiResponse) recvHead() {
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead() || !r.recvHeaders() || !r.checkHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
}
func (r *fcgiResponse) growHead() bool { // we need more head bytes to be appended to r.input
	if r.inputEdge == _16K {
		return false
	}
	if from, edge, err := r._readStdout(); err == nil {
		size := edge - from
		want := r.inputEdge + size
		_ = want
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		r.headResult = StatusRequestTimeout
	} else { // i/o error
		r.headResult = -1
	}
	return false
}
func (r *fcgiResponse) recvHeaders() bool {
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
	header.setPlace(placeInput) // all received headers are in r.input
	// r.pFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.pFore]; b == '\r' {
			// Skip '\r'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
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
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
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
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
			return false
		}

		// Skip OWS before field-value (and OWS after field-value, if field-value is empty)
		for r.input[r.pFore] == ' ' || r.input[r.pFore] == '\t' {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
				return false
			}
		}

		// field-value = *( field-content | LWSP )
		r.pBack = r.pFore // now r.pBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.pFore]; httpVchar[b] == 1 {
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
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
		if !r.applyHeader(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
			return false
		}
		// r.pFore is now at the next header or end of headers.
	}
	r.receiving = httpSectionContent
	// Skip end of headers
	r.pFore++
	// Now the head is received, and r.pFore is at the beginning of content (if exists).
	r.head.set(0, r.pFore)

	return true
}

func (r *fcgiResponse) Status() int16 { return r.status }

func (r *fcgiResponse) applyHeader(header *pair) bool {
	headerName := header.nameAt(r.input)
	if h := &fcgiResponseMultipleHeaderTable[fcgiResponseMultipleHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(fcgiResponseMultipleHeaderNames[h.from:h.edge], headerName) {
		from := len(r.headers) + 1 // excluding original header. overflow doesn't matter
		if !r.addMultipleHeader(header) {
			// r.headResult is set.
			return false
		}
		if h.check != nil && !h.check(r, from, len(r.headers)) {
			// r.headResult is set.
			return false
		}
	} else { // single-value response header
		if !r.addHeader(header) {
			// r.headResult is set.
			return false
		}
		if h := &fcgiResponseCriticalHeaderTable[fcgiResponseCriticalHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(fcgiResponseCriticalHeaderNames[h.from:h.edge], headerName) {
			if h.check != nil && !h.check(r, header, len(r.headers)-1) {
				// r.headResult is set.
				return false
			}
		}
	}
	return true
}

var ( // perfect hash table for response multiple headers
	fcgiResponseMultipleHeaderNames = []byte("connection transfer-encoding upgrade")
	fcgiResponseMultipleHeaderTable = [3]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*fcgiResponse, int, int) bool
	}{
		0: {httpHashTransferEncoding, 11, 28, (*fcgiResponse).checkTransferEncoding},
		1: {httpHashConnection, 0, 10, (*fcgiResponse).checkConnection},
		2: {httpHashUpgrade, 29, 36, (*fcgiResponse).checkUpgrade},
	}
	fcgiResponseMultipleHeaderFind = func(hash uint16) int { return (1488 / int(hash)) % 3 }
)

func (r *fcgiResponse) checkConnection(from int, edge int) bool {
	return r.delHeaders(from, edge)
}
func (r *fcgiResponse) checkTransferEncoding(from int, edge int) bool {
	return r.delHeaders(from, edge)
}
func (r *fcgiResponse) checkUpgrade(from int, edge int) bool {
	return r.delHeaders(from, edge)
}
func (r *fcgiResponse) delHeaders(from int, edge int) bool {
	for i := from; i < edge; i++ {
		r.headers[i].zero()
	}
	return true
}

var ( // perfect hash table for response critical headers
	fcgiResponseCriticalHeaderNames = []byte("content-length content-type location status")
	fcgiResponseCriticalHeaderTable = [4]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*fcgiResponse, *pair, int) bool
	}{
		0: {fcgiHashStatus, 37, 43, (*fcgiResponse).checkStatus},
		1: {httpHashContentLength, 0, 14, (*fcgiResponse).checkContentLength},
		2: {httpHashContentType, 15, 27, (*fcgiResponse).checkContentType},
		3: {httpHashLocation, 28, 36, (*fcgiResponse).checkLocation},
	}
	fcgiResponseCriticalHeaderFind = func(hash uint16) int { return (2704 / int(hash)) % 4 }
)

func (r *fcgiResponse) checkContentLength(header *pair, index int) bool {
	r.headers[index].zero() // we don't believe the value provided by fcgi application. we believe fcgi framing
	return true
}
func (r *fcgiResponse) checkContentType(header *pair, index int) bool {
	r.indexes.contentType = uint8(index)
	return true
}
func (r *fcgiResponse) checkStatus(header *pair, index int) bool {
	// TODO
	return true
}
func (r *fcgiResponse) checkLocation(header *pair, index int) bool {
	// TODO
	return true
}

func (r *fcgiResponse) ContentSize() int64 { return -2 } // fcgi is chunked by default
func (r *fcgiResponse) unsafeContentType() []byte {
	if r.indexes.contentType == 0 {
		return nil
	}
	return r.headers[r.indexes.contentType].valueAt(r.input)
}

func (r *fcgiResponse) addMultipleHeader(header *pair) bool {
	// Add main header before sub headers.
	if !r.addHeader(header) {
		// r.headResult is set.
		return false
	}
	// TODO
	return true
}
func (r *fcgiResponse) addHeader(header *pair) bool {
	if len(r.headers) == cap(r.headers) {
		if cap(r.headers) == cap(r.stockHeaders) {
			r.headers = get255Pairs()
			r.headers = append(r.headers, r.stockHeaders[:]...)
		} else { // overflow
			r.headResult = StatusRequestHeaderFieldsTooLarge
			return false
		}
	}
	r.headers = append(r.headers, *header)
	return true
}
func (r *fcgiResponse) getHeader(name string, hash uint16) (value []byte, ok bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		for i := 0; i < len(r.headers); i++ {
			header := &r.headers[i]
			if header.hash != hash {
				continue
			}
			if header.nameEqualString(r.input, name) {
				return header.valueAt(r.input), true
			}
		}
	}
	return
}

func (r *fcgiResponse) delHopHeaders() {} // for fcgi, nothing to delete
func (r *fcgiResponse) forHeaders(fn func(hash uint16, name []byte, value []byte) bool) bool {
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

func (r *fcgiResponse) setRecvTimeout(timeout time.Duration) { r.recvTimeout = timeout }

func (r *fcgiResponse) hasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return true
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
func (r *fcgiResponse) growBody() {
}

func (r *fcgiResponse) HasTrailers() bool               { return false } // fcgi doesn't support trailers
func (r *fcgiResponse) applyTrailer(trailer *pair) bool { return true }  // fcgi doesn't support trailers

func (r *fcgiResponse) delHopTrailers() {} // fcgi doesn't support trailers
func (r *fcgiResponse) forTrailers(fn func(hash uint16, name []byte, value []byte) bool) bool { // fcgi doesn't support trailers
	return true
}

func (r *fcgiResponse) arrayCopy(p []byte) bool { return true } // not used, but required by response interface

func (r *fcgiResponse) saveContentFilesDir() string {
	return r.stream.agent.SaveContentFilesDir() // must ends with '/'
}

func (r *fcgiResponse) _newTempFile() (TempFile, error) { // to save content to
	filesDir := r.saveContentFilesDir() // must ends with '/'
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

var ( // fcgi response errors
	fcgiReadBadRecord = errors.New("fcgi: bad record")
)

// poolFCGIMaxRecords
var poolFCGIMaxRecords sync.Pool

const fcgiMaxRecords = fcgiHeaderSize + _64K1 + fcgiMaxPadding // max record = header + max content + max padding

func getFCGIMaxRecords() []byte {
	if x := poolFCGIMaxRecords.Get(); x == nil {
		return make([]byte, fcgiMaxRecords)
	} else {
		return x.([]byte)
	}
}
func putFCGIMaxRecords(maxRecords []byte) {
	if cap(maxRecords) != fcgiMaxRecords {
		BugExitln("fcgi: bad maxRecords")
	}
	poolFCGIMaxRecords.Put(maxRecords)
}

// FCGI protocol elements.

// FCGI Record = FCGI Header(8) + content + padding
// FCGI Header = version(1) + type(1) + requestId(2) + contentLen(2) + paddingLen(1) + reserved(1)

// Discrete records are standalone.
// Stream records end with an empty record (contentLen=0).

const ( // fcgi constants
	fcgiHeaderSize = 8
	fcgiMaxPadding = 255
)

const ( // request record types
	fcgiTypeBeginRequest = 1 // only one
	fcgiTypeParams       = 4 // only one in our implementation (plus one empty params record)
	fcgiTypeStdin        = 5 // many (ends with an empty stdin record)
)

var ( // fcgi params
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
	fcgiTypeStdout     = 6 // many
	fcgiTypeStderr     = 7 // many
	fcgiTypeEndRequest = 3 // only one
)

const ( // fcgi hashes
	fcgiHashStatus = 676
)
