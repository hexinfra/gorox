// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI proxy handlets pass requests to backend FCGI servers and cache responses.

package fcgi

import (
	. "github.com/hexinfra/gorox/hemi/internal"
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

	stage   *Stage
	app     *App
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher
	// States
	scriptFilename      string      // ...
	bufferClientContent bool        // ...
	bufferServerContent bool        // server content is buffered anyway?
	keepConn            bool        // instructs FCGI server to keep conn?
	addRequestHeaders   [][2][]byte // headers appended to client request
	delRequestHeaders   [][]byte    // client request headers to delete
	addResponseHeaders  [][2][]byte // headers appended to server response
	delResponseHeaders  [][]byte    // server response headers to delete
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
}
func (h *fcgiProxy) OnPrepare() {
}

func (h *fcgiProxy) IsProxy() bool { return true }
func (h *fcgiProxy) IsCache() bool { return h.cacher != nil }

func (h *fcgiProxy) Handle(req Request, resp Response) (next bool) {
	var (
		conn PConn
		err  error
	)
	if h.keepConn { // FCGI_KEEP_CONN=1
		conn, err = h.backend.FetchConn()
		if err != nil {
			resp.SendBadGateway(nil)
			return
		}
		defer h.backend.StoreConn(conn)
	} else { // FCGI_KEEP_CONN=0
		conn, err = h.backend.Dial()
		if err != nil {
			resp.SendBadGateway(nil)
			return
		}
		defer conn.Close()
	}

	stream := getFCGIStream(h, conn)
	defer putFCGIStream(stream)

	fReq, fResp := &stream.request, &stream.response
	_, _ = fReq, fResp

	if h.keepConn {
		// use fcgiBeginKeepConn
	} else {
		// use fcgiBeginDontKeep
	}

	// TODO(diogin): Implementation
	if h.scriptFilename == "" {
		// use absPath as SCRIPT_FILENAME
	} else {
		// use h.scriptFilename as SCRIPT_FILENAME
	}
	resp.Send("foobar")
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
	return nil
}
func (s *fcgiStream) setReadDeadline(deadline time.Time) error {
	return nil
}

func (s *fcgiStream) write(p []byte) (int, error) {
	return s.conn.Write(p)
}
func (s *fcgiStream) writev(vector *net.Buffers) (int64, error) {
	return s.conn.Writev(vector)
}
func (s *fcgiStream) read(p []byte) (int, error) {
	return s.conn.Read(p)
}
func (s *fcgiStream) readFull(p []byte) (int, error) {
	return s.conn.ReadFull(p)
}

func (s *fcgiStream) isBroken() bool {
	return false
}
func (s *fcgiStream) markBroken() {
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
	return false
}
func (r *fcgiRequest) addParam(name []byte, value []byte) bool {
	return false
}

func (r *fcgiRequest) setMaxSendTimeout(timeout time.Duration) {
	r.maxSendTimeout = timeout
}

func (r *fcgiRequest) sendBlob(content []byte) error {
	return nil
}
func (r *fcgiRequest) sendFile(content *os.File, info os.FileInfo) error {
	return nil
}
func (r *fcgiRequest) pushBlob(chunk []byte) error {
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
		p, err := req.ReadContent()
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
	return nil
}
func (r *fcgiRequest) syncHeaders() error {
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
	return nil
}

// poolFCGIParams
var poolFCGIParams sync.Pool

const fcgiMaxParams = 8 + 16384

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
	stockHeaders [32]fcgiHeader
	// States (controlled)
	header fcgiHeader // to overcome ...
	// States (non-zeros)
	input          []byte
	headers        []fcgiHeader
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
	receiving       int8
	contentReceived bool
	contentBlobKind int8
	maxContentSize  int64
	sizeReceived    int64
	indexes         struct {
		contentType uint8
		xPoweredBy  uint8
	}
}

func (r *fcgiResponse) onUse() {
	r.input = r.stockInput[:]
	r.headers = r.stockHeaders[0:1:cap(r.stockHeaders)] // use append(). r.headers[0] is skipped due to zero value of header indexes.
	r.contentSize = -1
	r.maxRecvTimeout = r.stream.proxy.maxRecvTimeout
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
}
func (r *fcgiResponse) _recvRecord() {
	// TODO: use getFCGIInput
}

func (r *fcgiResponse) applyHeader() bool {
	return false
}
func (r *fcgiResponse) walkHeaders() {
}

var ( // perfect hash table for response critical headers
	fcgiResponseCriticalHeaderNames = []byte("todo")
	fcgiResponseCriticalHeaderTable = [1]struct {
		hash  uint8
		from  uint8
		edge  uint8
		check func(*fcgiResponse, *fcgiHeader, uint8) bool
	}{
		0: {123, 0, 4, (*fcgiResponse).checkTransferEncoding},
	}
	fcgiResponseCriticalHeaderFind = func(hash uint8) int { return 0 }
)

func (r *fcgiResponse) checkTransferEncoding(header *fcgiHeader, index uint8) bool {
	return true
}
func (r *fcgiResponse) checkContentLength(header *fcgiHeader, index uint8) bool {
	return true
}

func (r *fcgiResponse) contentType() string {
	return ""
}

func (r *fcgiResponse) parseSetCookie() bool {
	// TODO
	return false
}

func (r *fcgiResponse) checkHead() bool {
	return false
}

func (r *fcgiResponse) cleanInput() {
}

func (r *fcgiResponse) setMaxRecvTimeout(timeout time.Duration) {
	r.maxRecvTimeout = timeout
}

func (r *fcgiResponse) hasContent() bool {
	return false
}
func (r *fcgiResponse) readContent(retain bool) (p []byte, err error) {
	return
}

func (r *fcgiResponse) holdContent() any {
	return nil
}
func (r *fcgiResponse) recvContent() any { // to []byte (for small content) or TempFile (for large content)
	return nil
}

func (r *fcgiResponse) addHeader(header *fcgiHeader) (edge uint8, ok bool) {
	return
}
func (r *fcgiResponse) getHeader(name string, hash uint8) (value []byte, ok bool) {
	return
}

func (r *fcgiResponse) newTempFile() {
}
func (r *fcgiResponse) beforeRead() error {
	return nil
}

// poolFCGIInput
var poolFCGIInput sync.Pool

const fcgiMaxRecord = 8 + 65535 + 255

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

// fcgiHeader is a header in fcgi response.
type fcgiHeader struct { // 8 bytes
	nameHash  uint8
	nameSize  uint8
	nameFrom  uint16
	valueFrom uint16
	valueEdge uint16
}

const ( // response header hashes
	fcgiHashContentLength    = 170
	fcgiHashContentType      = 234
	fcgiHashTransferEncoding = 217
)

var ( // response header bytes
	fcgiBytesContentLength    = []byte("content-length")
	fcgiBytesContentType      = []byte("content-type")
	fcgiBytesTransferEncoding = []byte("transfer-encoding")
)
