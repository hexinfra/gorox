// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI proxy handlets pass requests to backend FCGI servers and cache responses.

// FCGI allows chunked content in HTTP, so we are not obliged to buffer content.

package internal

import (
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
	scriptFilename      string // ...
	bufferClientContent bool   // ...
	bufferServerContent bool   // server content is buffered anyway?
	keepConn            bool   // instructs FCGI server to keep conn?
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

	stream := getFCGIStream(conn)
	defer putFCGIStream(stream)

	feq, fesp := &stream.request, &stream.response
	_, _ = feq, fesp

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

func getFCGIStream(conn PConn) *fcgiStream {
	var stream *fcgiStream
	if x := poolFCGIStream.Get(); x == nil {
		stream = new(fcgiStream)
	} else {
		stream = x.(*fcgiStream)
	}
	stream.onUse(conn)
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
	stockStack [64]byte
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn PConn
}

func (s *fcgiStream) onUse(conn PConn) {
	s.conn = conn
	s.request.onUse(conn)
	s.response.onUse(conn)
}
func (s *fcgiStream) onEnd() {
	s.request.onEnd()
	s.response.onEnd()
	s.conn = nil
}

func (s *fcgiStream) smallStack() []byte {
	return s.stockStack[:]
}

func (s *fcgiStream) setWriteDeadline(deadline time.Time) error {
	return nil
}
func (s *fcgiStream) setReadDeadline(deadline time.Time) error {
	return nil
}

func (s *fcgiStream) write(p []byte) (int, error) {
	return s.conn.Write(p)
}
func (s *fcgiStream) read(p []byte) (int, error) {
	return s.conn.Read(p)
}

// fcgiRequest
type fcgiRequest struct {
	// Assocs
	conn     PConn
	stream   *fcgiStream
	response *fcgiResponse
	// States
}

func (r *fcgiRequest) onUse(conn PConn) {
	r.conn = conn
}
func (r *fcgiRequest) onEnd() {
	r.conn = nil
}

func (r *fcgiRequest) withHead(req Request) bool {
	return false
}

func (r *fcgiRequest) pass(req Request) error {
	return nil
}
func (r *fcgiRequest) post(content any) error { // nil, []byte, *os.File
	return nil
}

// fcgiResponse
type fcgiResponse struct {
	// Assocs
	conn   PConn
	stream *fcgiStream
	// States
	stockInput [2048]byte
	stockArray [1024]byte
	input      []byte
	array      []byte
}

func (r *fcgiResponse) onUse(conn PConn) {
	r.conn = conn
	r.input = r.stockInput[:]
	r.array = r.stockArray[:]
}
func (r *fcgiResponse) onEnd() {
	r.conn = nil
}

func (r *fcgiResponse) recvHead() {
}
func (r *fcgiResponse) addHeader() bool {
	return false
}
func (r *fcgiResponse) checkHead() bool {
	return false
}

func (r *fcgiResponse) walkHeaders() {
}

func (r *fcgiResponse) setMaxRecvSeconds(seconds int64) {
}

func (r *fcgiResponse) readContent() (from int, edge int, err error) {
	return
}

func (r *fcgiResponse) recvContent() any { // to []byte (for small content) or TempFile (for large content)
	return nil
}
func (r *fcgiResponse) holdContent() any {
	return nil
}

func (r *fcgiResponse) newTempFile() {
}

func (r *fcgiResponse) prepareRead() error {
	return nil
}

// fcgi protocol elements.

const (
	fcgiVersion   = 1 // fcgi protocol version
	fcgiHeadLen   = 8 // length of fcgi record head
	fcgiNullID    = 0 // request id for management records
	fcgiResponder = 1 // traditional cgi role
	fcgiComplete  = 0 // protocol status ok
)

const ( // record types
	typeBeginRequest    = 1  // by request
	typeAbortRequest    = 2  // by request
	typeEndRequest      = 3  // by response
	typeParams          = 4  // by request
	typeStdin           = 5  // by request
	typeStdout          = 6  // by response
	typeStderr          = 7  // by response
	typeData            = 8  // by request, NOT supported
	typeGetValues       = 9  // by request
	typeGetValuesResult = 10 // by response
	typeUnknownKind     = 11 // by response
	typeMax             = typeUnknownKind
)

var (
	fcgiBeginKeepConn = []byte{
		fcgiVersion,
		typeBeginRequest,
		0, 1, // request id. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length
		0, 0, // padding length & reserved
		0, fcgiResponder, // role
		1,             // flags=keepConn
		0, 0, 0, 0, 0, // reserved
	}
	fcgiBeginDontKeep = []byte{
		fcgiVersion,
		typeBeginRequest,
		0, 1, // request id. we don't support pipelining or multiplex, only one request at a time, so request id is always 1
		0, 8, // content length
		0, 0, // padding length & reserved
		0, fcgiResponder, // role
		0,             // flags=dontKeep
		0, 0, 0, 0, 0, // reserved
	}
)
