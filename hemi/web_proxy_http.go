// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

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

// httpProxy handlet passes http requests to http backends and caches responses.
type httpProxy struct {
	// Parent
	Handlet_
	// Assocs
	stage   *Stage      // current stage
	webapp  *Webapp     // the webapp to which the proxy belongs
	backend HTTPBackend // the backend to pass to. can be *HTTP1Backend, *HTTP2Backend, or *HTTP3Backend
	cacher  Cacher      // the cacher which is used by this proxy
	// States
	WebExchanProxyConfig // embeded
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
				h.backend = backend.(HTTPBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for http proxy")
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
			h.AddRequestHeaders = addedHeaders
		} else {
			UseExitln("invalid addRequestHeaders")
		}
	}

	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.BufferClientContent, true)
	// hostname
	h.ConfigureBytes("hostname", &h.Hostname, nil, nil)
	// colonPort
	h.ConfigureBytes("colonPort", &h.ColonPort, nil, nil)
	// inboundViaName
	h.ConfigureBytes("inboundViaName", &h.InboundViaName, nil, bytesGorox)
	// delRequestHeaders
	h.ConfigureBytesList("delRequestHeaders", &h.DelRequestHeaders, nil, [][]byte{})
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.BufferServerContent, true)
	// outboundViaName
	h.ConfigureBytes("outboundViaName", &h.OutboundViaName, nil, nil)
	// addResponseHeaders
	h.ConfigureStringDict("addResponseHeaders", &h.AddResponseHeaders, nil, map[string]string{})
	// delResponseHeaders
	h.ConfigureBytesList("delResponseHeaders", &h.DelResponseHeaders, nil, [][]byte{})
}
func (h *httpProxy) OnPrepare() {
	// Currently nothing.
}

func (h *httpProxy) IsProxy() bool { return true }
func (h *httpProxy) IsCache() bool { return h.cacher != nil }

func (h *httpProxy) Handle(req Request, resp Response) (handled bool) {
	WebExchanReverseProxy(req, resp, h.backend, &h.WebExchanProxyConfig)
	return true
}

// WebExchanProxyConfig
type WebExchanProxyConfig struct {
	BufferClientContent bool
	Hostname            []byte
	ColonPort           []byte
	InboundViaName      []byte
	AppendPathPrefix    []byte
	AddRequestHeaders   map[string]Value
	DelRequestHeaders   [][]byte

	BufferServerContent bool
	OutboundViaName     []byte
	AddResponseHeaders  map[string]string
	DelResponseHeaders  [][]byte
}

func WebExchanReverseProxy(req Request, resp Response, backend HTTPBackend, cfg *WebExchanProxyConfig) {
	var content any
	hasContent := req.HasContent()
	if hasContent && cfg.BufferClientContent { // including size 0
		content = req.holdContent()
		if content == nil { // take failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	backStream, backErr := backend.FetchStream()
	if backErr != nil {
		resp.SendBadGateway(nil)
		return
	}
	defer backend.StoreStream(backStream)

	backReq := backStream.Request()
	if !backReq.proxyCopyHead(req, cfg) {
		backStream.markBroken()
		resp.SendBadGateway(nil)
		return
	}

	if !hasContent || cfg.BufferClientContent {
		hasTrailers := req.HasTrailers()
		backErr = backReq.proxyPost(content, hasTrailers) // nil (no content), []byte, tempFile
		if backErr == nil && hasTrailers {
			if !backReq.proxyCopyTail(req, cfg) {
				backStream.markBroken()
				backErr = httpOutTrailerFailed
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
			// Only HTTP/1 cares this. But the code is general between all HTTP versions.
			if backResp.KeepAlive() == 0 {
				backStream.httpConn().setPersistent(false)
			}
			break
		}
		// We got a 1xx
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
	if backHasContent && cfg.BufferServerContent { // including size 0
		backContent = backResp.holdContent()
		if backContent == nil { // take failed
			// backStream is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.proxyCopyHead(backResp, cfg) {
		backStream.markBroken()
		return
	}
	if !backHasContent || cfg.BufferServerContent {
		backHasTrailers := backResp.HasTrailers()
		if resp.proxyPost(backContent, backHasTrailers) != nil { // nil (no content), []byte, tempFile
			if backHasTrailers {
				backStream.markBroken()
			}
			return
		}
		if backHasTrailers && !resp.proxyCopyTail(backResp, cfg) {
			return
		}
	} else if err := resp.proxyPass(backResp); err != nil {
		backStream.markBroken()
		return
	}
}

// sockProxy socklet passes websockets to http backends.
type sockProxy struct {
	// Parent
	Socklet_
	// Assocs
	stage   *Stage      // current stage
	webapp  *Webapp     // the webapp to which the proxy belongs
	backend HTTPBackend // the backend to pass to. can be *HTTP1Backend, *HTTP2Backend, or *HTTP3Backend
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
				s.backend = backend.(HTTPBackend)
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

// WebSocketProxyConfig
type WebSocketProxyConfig struct {
	// TODO
}

func WebSocketReverseProxy(req Request, sock Socket, backend HTTPBackend, cfg *WebSocketProxyConfig) {
}
