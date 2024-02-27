// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP reverse proxy implementation.

package hemi

import (
	"strings"
)

func init() {
	RegisterHandlet("httpProxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(httpProxy)
		h.onCreate(name, stage, webapp)
		return h
	})
	RegisterSocklet("sockProxy", func(name string, stage *Stage, webapp *Webapp) Socklet {
		s := new(sockProxy)
		s.onCreate(name, stage, webapp)
		return s
	})
}

// httpProxy handlet passes web requests to backend HTTP servers and cache responses.
type httpProxy struct {
	// Parent
	Handlet_
	// Assocs
	stage   *Stage     // current stage
	webapp  *Webapp    // the webapp to which the proxy belongs
	backend WebBackend // the backend to pass to
	cacher  Cacher     // the cacher which is used by this proxy
	// States
	hostname            []byte            // hostname used in ":authority" and "host" header
	colonPort           []byte            // colonPort used in ":authority" and "host" header
	viaName             []byte            // ...
	bufferClientContent bool              // buffer client content into tempFile?
	bufferServerContent bool              // buffer server content into tempFile?
	addRequestHeaders   map[string]Value  // headers appended to backend request
	delRequestHeaders   [][]byte          // backend request headers to delete
	addResponseHeaders  map[string]string // headers appended to server response
	delResponseHeaders  [][]byte          // server response headers to delete
}

func (h *httpProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *httpProxy) OnShutdown() {
	h.webapp.DecSub()
}

func (h *httpProxy) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				h.backend = backend.(WebBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for web proxy")
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

	// addRequestHeaders
	if v, ok := h.Find("addRequestHeaders"); ok {
		addedHeaders := make(map[string]Value)
		if vHeaders, ok := v.Dict(); ok {
			for name, vValue := range vHeaders {
				if vValue.IsVariable() {
					name := vValue.name
					if p := strings.IndexByte(name, '_'); p != -1 {
						p++ // skip '_'
						vValue.name = name[:p] + strings.ReplaceAll(name[p:], "_", "-")
					}
				} else if _, ok := vValue.Bytes(); !ok {
					UseExitf("bad value in .addRequestHeaders")
				}
				addedHeaders[name] = vValue
			}
			h.addRequestHeaders = addedHeaders
		} else {
			UseExitln("invalid addRequestHeaders")
		}
	}

	// hostname
	h.ConfigureBytes("hostname", &h.hostname, nil, nil)
	// colonPort
	h.ConfigureBytes("colonPort", &h.colonPort, nil, nil)
	// viaName
	h.ConfigureBytes("viaName", &h.viaName, nil, bytesGorox)
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// delRequestHeaders
	h.ConfigureBytesList("delRequestHeaders", &h.delRequestHeaders, nil, [][]byte{})
	// addResponseHeaders
	h.ConfigureStringDict("addResponseHeaders", &h.addResponseHeaders, nil, map[string]string{})
	// delResponseHeaders
	h.ConfigureBytesList("delResponseHeaders", &h.delResponseHeaders, nil, [][]byte{})
}
func (h *httpProxy) OnPrepare() {
	// Currently nothing.
}

func (h *httpProxy) IsProxy() bool { return true }
func (h *httpProxy) IsCache() bool { return h.cacher != nil }

func (h *httpProxy) Handle(req Request, resp Response) (handled bool) {
	handled = true

	var content any
	hasContent := req.HasContent()
	if hasContent && h.bufferClientContent { // including size 0
		content = req.takeContent()
		if content == nil { // take failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	backConn, backErr := h.backend.FetchConn()
	if backErr != nil {
		if Debug() >= 1 {
			Println(backErr.Error())
		}
		resp.SendBadGateway(nil)
		return
	}
	defer h.backend.StoreConn(backConn)

	backStream := backConn.FetchStream()
	defer backConn.StoreStream(backStream)

	// TODO: use backStream.ReverseExchan()

	backReq := backStream.Request()
	if !backReq.proxyCopyHead(req, h.hostname, h.colonPort, h.viaName, h.addRequestHeaders, h.delRequestHeaders) {
		backStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}

	if !hasContent || h.bufferClientContent {
		hasTrailers := req.HasTrailers()
		backErr = backReq.proxyPost(content, hasTrailers) // nil (no content), []byte, tempFile
		if backErr == nil && hasTrailers {
			if !backReq.proxyCopyTail(req) {
				backStream.markBroken()
				backErr = webOutTrailerFailed
			} else if backErr = backReq.endVague(); backErr != nil {
				backStream.markBroken()
			}
		} else if hasTrailers {
			backStream.markBroken()
		}
	} else if backErr = backReq.proxyPass(req); backErr != nil {
		backStream.markBroken()
	} else if backReq.isVague() { // must write last chunk and trailers (if exist)
		if backErr = backReq.endVague(); backErr != nil {
			backStream.markBroken()
		}
	}
	if backErr != nil {
		resp.SendBadGateway(nil)
		return
	}

	backResp := backStream.Response()
	for { // until we found a non-1xx status (>= 200)
		backResp.recvHead()
		if backResp.HeadResult() != StatusOK || backResp.Status() == StatusSwitchingProtocols { // websocket is not served in handlets.
			backStream.markBroken()
			if backResp.HeadResult() == StatusRequestTimeout {
				resp.SendGatewayTimeout(nil)
			} else {
				resp.SendBadGateway(nil)
			}
			return
		}
		if backResp.Status() >= StatusOK {
			// Only HTTP/1 concerns this. But the code is general between all HTTP versions.
			if backResp.KeepAlive() == 0 {
				backConn.setKeepConn(false)
			}
			break
		}
		// We got 1xx
		if req.VersionCode() == Version1_0 {
			backStream.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.proxyPass1xx(backResp) {
			backStream.markBroken()
			return
		}
		backResp.reuse()
	}

	var backContent any
	backHasContent := false
	if req.MethodCode() != MethodHEAD {
		backHasContent = backResp.HasContent()
	}
	if backHasContent && h.bufferServerContent { // including size 0
		backContent = backResp.takeContent()
		if backContent == nil { // take failed
			// backStream is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.proxyCopyHead(backResp, nil) { // viaName = nil
		backStream.markBroken()
		return
	}
	if !backHasContent || h.bufferServerContent {
		bHasTrailers := backResp.HasTrailers()
		if resp.proxyPost(backContent, bHasTrailers) != nil { // nil (no content), []byte, tempFile
			if bHasTrailers {
				backStream.markBroken()
			}
			return
		}
		if bHasTrailers && !resp.proxyCopyTail(backResp) {
			return
		}
	} else if err := resp.proxyPass(backResp); err != nil {
		backStream.markBroken()
		return
	}

	return
}

// sockProxy socklet passes websockets to backend WebSocket servers.
type sockProxy struct {
	// Parent
	Socklet_
	// Assocs
	stage   *Stage  // current stage
	webapp  *Webapp // the webapp to which the proxy belongs
	backend WebBackend
	// States
}

func (s *sockProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	s.MakeComp(name)
	s.stage = stage
	s.webapp = webapp
}
func (s *sockProxy) OnShutdown() {
	s.webapp.DecSub()
}

func (s *sockProxy) OnConfigure() {
	// toBackend
	if v, ok := s.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := s.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				s.backend = backend.(WebBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for websocket proxy")
	}
}
func (s *sockProxy) OnPrepare() {
	// Currently nothing.
}

func (s *sockProxy) IsProxy() bool { return true }

func (s *sockProxy) Serve(req Request, sock Socket) {
	// TODO(diogin): Implementation
	sock.Close()
}
