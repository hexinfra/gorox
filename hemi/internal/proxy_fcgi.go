// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI proxy handlet passes requests to backend FCGI servers and cache responses.

// FCGI is mainly used by PHP applications. It doesn't support HTTP trailers.
// And we don't use request-side chunking due to the limitation of CGI/1.1 even
// though FCGI can do that through its framing protocol. Perhaps most FCGI
// applications don't implement this feature either.

// In response side, FCGI applications mostly use "unsized" output.

// To avoid ambiguity, the term "content" in FCGI specification is called "payload" in our implementation.

package internal

import (
	"errors"
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
	stage   *Stage       // current stage
	app     *App         // the app to which the proxy belongs
	backend *TCPSBackend // the fcgi backend to pass to
	cacher  Cacher       // the cacher which is used by this proxy
	// States
	bufferClientContent bool          // client content is buffered anyway?
	bufferServerContent bool          // server content is buffered anyway?
	keepConn            bool          // instructs FCGI server to keep conn?
	preferUnderscore    bool          // if header name "foo-bar" and "foo_bar" are both present, prefer "foo_bar" to "foo-bar"?
	scriptFilename      []byte        // for SCRIPT_FILENAME
	indexFile           []byte        // for indexFile
	addRequestHeaders   [][2][]byte   // headers appended to client request
	delRequestHeaders   [][]byte      // client request headers to delete
	addResponseHeaders  [][2][]byte   // headers appended to server response
	delResponseHeaders  [][]byte      // server response headers to delete
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max response content size allowed
}

func (h *fcgiProxy) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
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
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				h.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for fcgiProxy, must be TCPSBackend\n", name)
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

	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// keepConn
	h.ConfigureBool("keepConn", &h.keepConn, false)
	// preferUnderscore
	h.ConfigureBool("preferUnderscore", &h.preferUnderscore, false)
	// scriptFilename
	h.ConfigureBytes("scriptFilename", &h.scriptFilename, nil, nil)

	// indexFile
	h.ConfigureBytes("indexFile", &h.indexFile, func(value []byte) error {
		if len(value) > 0 {
			return nil
		}
		return errors.New(".indexFile is an invalid value")
	}, []byte("index.php"))

	// sendTimeout
	h.ConfigureDuration("sendTimeout", &h.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout is an invalid value")
	}, 60*time.Second)

	// recvTimeout
	h.ConfigureDuration("recvTimeout", &h.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout is an invalid value")
	}, 60*time.Second)

	// maxContentSize
	h.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize is an invalid value")
	}, _1T)
}
func (h *fcgiProxy) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *fcgiProxy) IsProxy() bool { return true }
func (h *fcgiProxy) IsCache() bool { return h.cacher != nil }

func (h *fcgiProxy) Handle(req Request, resp Response) (next bool) { // reverse only
	var (
		content  any
		fConn    *TConn
		fErr     error
		fContent any
	)

	hasContent := req.HasContent()
	if hasContent && (h.bufferClientContent || req.IsUnsized()) { // including size 0
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
	if !fReq.copyHeadFrom(req, h.scriptFilename, h.indexFile) {
		fStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if hasContent && !h.bufferClientContent && !req.IsUnsized() {
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

	if !resp.copyHeadFrom(fResp, nil) { // viaName = nil
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
