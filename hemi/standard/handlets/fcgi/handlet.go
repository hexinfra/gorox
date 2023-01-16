// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI proxy handlets pass requests to backend FCGI servers and cache responses.

package fcgi

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"net"
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

	exchan := getFCGIExchan(conn)
	defer putFCGIExchan(exchan)

	fReq, fResp := &exchan.request, &exchan.response
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

// poolFCGIExchan
var poolFCGIExchan sync.Pool

func getFCGIExchan(conn PConn) *fcgiExchan {
	var exchan *fcgiExchan
	if x := poolFCGIExchan.Get(); x == nil {
		exchan = new(fcgiExchan)
	} else {
		exchan = x.(*fcgiExchan)
	}
	exchan.onUse(conn)
	return exchan
}
func putFCGIExchan(exchan *fcgiExchan) {
	exchan.onEnd()
	poolFCGIExchan.Put(exchan)
}

// fcgiExchan
type fcgiExchan struct {
	// Assocs
	request  fcgiRequest  // the fcgi request
	response fcgiResponse // the fcgi response
	// Exchan states (buffers)
	stockStack [64]byte
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	conn PConn
}

func (s *fcgiExchan) onUse(conn PConn) {
	s.conn = conn
	s.request.onUse(conn)
	s.response.onUse(conn)
}
func (s *fcgiExchan) onEnd() {
	s.request.onEnd()
	s.response.onEnd()
	s.conn = nil
}

func (s *fcgiExchan) smallStack() []byte {
	return s.stockStack[:]
}

func (s *fcgiExchan) setWriteDeadline(deadline time.Time) error {
	return nil
}
func (s *fcgiExchan) setReadDeadline(deadline time.Time) error {
	return nil
}

func (s *fcgiExchan) write(p []byte) (int, error) {
	return s.conn.Write(p)
}
func (s *fcgiExchan) read(p []byte) (int, error) {
	return s.conn.Read(p)
}

// fcgiRequest
type fcgiRequest struct {
	// Assocs
	conn     PConn
	exchan   *fcgiExchan
	response *fcgiResponse
	// States (buffers)
	stockParams [1536]byte
	// States (non-zeros)
	params         []byte
	maxSendTimeout time.Duration
	// States (zeros)
	sendTime    time.Time
	vector      net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector [4][]byte   // for sending/pushing message. reset after stream. 96B
	fcgiRequest0
}
type fcgiRequest0 struct {
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

func (r *fcgiRequest) sync(req Request) error {
	return nil
}
func (r *fcgiRequest) pass(content any) error { // nil, []byte, *os.File
	return nil
}

// fcgiResponse
type fcgiResponse struct {
	// Assocs
	conn   PConn
	exchan *fcgiExchan
	// States (buffers)
	stockInput [2048]byte
	stockArray [1024]byte
	// States (non-zeros)
	input []byte
	array []byte
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

func (r *fcgiResponse) setMaxRecvTimeout(timeout time.Duration) {
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

func (r *fcgiResponse) beforeRead() error {
	return nil
}
