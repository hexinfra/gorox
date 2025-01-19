// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP reverse proxy implementation. See RFC 9110 and RFC 9111.

package hemi

import (
	"strings"
)

// Hcache component is the interface to storages of HTTP caching.
type Hcache interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Set(key []byte, hobject *Hobject)
	Get(key []byte) (hobject *Hobject)
	Del(key []byte) bool
}

// Hcache_ is the parent for all hcaches.
type Hcache_ struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (c *Hcache_) OnCreate(compName string, stage *Stage) {
	c.MakeComp(compName)
	c.stage = stage
}

func (c *Hcache_) Stage() *Stage { return c.stage }

func (c *Hcache_) todo() {
}

// Hobject represents an HTTP object in Hcache.
type Hobject struct {
	// TODO
	uri      []byte
	headers  any
	content  any
	trailers any
}

func (o *Hobject) todo() {
	// TODO
}

func init() {
	RegisterHandlet("httpProxy", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(httpProxy)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// httpProxy handlet passes http requests to http backends and caches responses.
type httpProxy struct {
	// Parent
	Handlet_
	// Assocs
	backend HTTPBackend // the *HTTP[1-3]Backend to pass to
	hcache  Hcache      // the hcache which is used by this proxy
	// States
	WebExchanProxyConfig // embeded
}

func (h *httpProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *httpProxy) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *httpProxy) OnConfigure() {
	// .toBackend
	if v, ok := h.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := h.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else {
				h.backend = backend.(HTTPBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for http proxy")
	}

	// .withHcache
	if v, ok := h.Find("withHcache"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if hcache := h.stage.Hcache(compName); hcache == nil {
				UseExitf("unknown hcache: '%s'\n", compName)
			} else {
				h.hcache = hcache
			}
		} else {
			UseExitln("invalid withHcache")
		}
	}

	// .addRequestHeaders
	if v, ok := h.Find("addRequestHeaders"); ok {
		addedHeaders := make(map[string]Value)
		if vHeaders, ok := v.Dict(); ok {
			for headerName, vHeaderValue := range vHeaders {
				if vHeaderValue.IsVariable() {
					name := vHeaderValue.name
					if p := strings.IndexByte(name, '_'); p != -1 {
						p++ // skip '_'
						vHeaderValue.name = name[:p] + strings.ReplaceAll(name[p:], "_", "-")
					}
				} else if _, ok := vHeaderValue.Bytes(); !ok {
					UseExitf("bad value in .addRequestHeaders")
				}
				addedHeaders[headerName] = vHeaderValue
			}
			h.AddRequestHeaders = addedHeaders
		} else {
			UseExitln("invalid addRequestHeaders")
		}
	}

	// .bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.BufferClientContent, true)
	// .hostname
	h.ConfigureBytes("hostname", &h.Hostname, nil, nil)
	// .colonport
	h.ConfigureBytes("colonport", &h.Colonport, nil, nil)
	// .inboundViaName
	h.ConfigureBytes("inboundViaName", &h.InboundViaName, nil, bytesGorox)
	// .delRequestHeaders
	h.ConfigureBytesList("delRequestHeaders", &h.DelRequestHeaders, nil, [][]byte{})
	// .bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.BufferServerContent, true)
	// .outboundViaName
	h.ConfigureBytes("outboundViaName", &h.OutboundViaName, nil, nil)
	// .addResponseHeaders
	h.ConfigureStringDict("addResponseHeaders", &h.AddResponseHeaders, nil, map[string]string{})
	// .delResponseHeaders
	h.ConfigureBytesList("delResponseHeaders", &h.DelResponseHeaders, nil, [][]byte{})
}
func (h *httpProxy) OnPrepare() {
	// Currently nothing.
}

func (h *httpProxy) IsProxy() bool { return true }
func (h *httpProxy) IsCache() bool { return h.hcache != nil }

func (h *httpProxy) Handle(req Request, resp Response) (handled bool) {
	WebExchanReverseProxy(req, resp, h.hcache, h.backend, &h.WebExchanProxyConfig)
	return true
}

// WebExchanProxyConfig
type WebExchanProxyConfig struct {
	// Inbound
	BufferClientContent bool
	Hostname            []byte // overrides client's hostname
	Colonport           []byte // overrides client's colonport
	InboundViaName      []byte
	AppendPathPrefix    []byte
	AddRequestHeaders   map[string]Value
	DelRequestHeaders   [][]byte
	// Outbound
	BufferServerContent bool
	OutboundViaName     []byte
	AddResponseHeaders  map[string]string
	DelResponseHeaders  [][]byte
}

// WebExchanReverseProxy
func WebExchanReverseProxy(foreReq Request, foreResp Response, hcache Hcache, backend HTTPBackend, proxyConfig *WebExchanProxyConfig) {
	var foreContent any // nil, []byte, tempFile
	foreHasContent := foreReq.HasContent()
	if foreHasContent && proxyConfig.BufferClientContent { // including size 0
		foreContent = foreReq.proxyTakeContent()
		if foreContent == nil { // take failed
			// foreStream was marked as broken
			foreResp.SetStatus(StatusBadRequest)
			foreResp.SendBytes(nil)
			return
		}
	}

	backStream, backErr := backend.FetchStream(foreReq)
	if backErr != nil {
		foreResp.SendBadGateway(nil)
		return
	}
	defer backend.StoreStream(backStream)

	backReq := backStream.Request()
	if !backReq.proxyCopyHeaders(foreReq, proxyConfig) {
		backStream.markBroken()
		foreResp.SendBadGateway(nil)
		return
	}

	if !foreHasContent || proxyConfig.BufferClientContent {
		foreHasTrailers := foreReq.HasTrailers()
		backErr = backReq.proxyPostMessage(foreContent, foreHasTrailers)
		if backErr == nil && foreHasTrailers {
			if !backReq.proxyCopyTrailers(foreReq, proxyConfig) {
				backStream.markBroken()
				backErr = httpOutTrailerFailed
			} else if backErr = backReq.endVague(); backErr != nil {
				backStream.markBroken()
			}
		} else if foreHasTrailers {
			backStream.markBroken()
		}
	} else if backErr = backReq.proxyPassMessage(foreReq); backErr != nil {
		backStream.markBroken()
	} else if backReq.isVague() { // must write the last chunk and trailers (if exist)
		if backErr = backReq.endVague(); backErr != nil {
			backStream.markBroken()
		}
	}
	if backErr != nil {
		foreResp.SendBadGateway(nil)
		return
	}

	backResp := backStream.Response()
	for { // until we found a non-1xx status (>= 200)
		backResp.recvHead()
		if backResp.HeadResult() != StatusOK || backResp.Status() == StatusSwitchingProtocols { // webSocket is not served in handlets.
			backStream.markBroken()
			if backResp.HeadResult() == StatusRequestTimeout {
				foreResp.SendGatewayTimeout(nil)
			} else {
				foreResp.SendBadGateway(nil)
			}
			return
		}
		if backResp.Status() >= StatusOK {
			if backResp.KeepAlive() == 0 { // connection close. only HTTP/1.x cares about this.
				backStream.(*backend1Stream).conn.persistent = false
			}
			break
		}
		// We got a 1xx response.
		if foreReq.VersionCode() == Version1_0 { // 1xx response is not supported by HTTP/1.0
			backStream.markBroken()
			foreResp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !foreResp.proxyPass1xx(backResp) {
			backStream.markBroken()
			return
		}
		backResp.reuse()
	}

	var backContent any // nil, []byte, tempFile
	backHasContent := false
	if !foreReq.IsHEAD() {
		backHasContent = backResp.HasContent()
	}
	if backHasContent && proxyConfig.BufferServerContent { // including size 0
		backContent = backResp.proxyTakeContent()
		if backContent == nil { // take failed
			// backStream was marked as broken
			foreResp.SendBadGateway(nil)
			return
		}
	}

	if !foreResp.proxyCopyHeaders(backResp, proxyConfig) {
		backStream.markBroken()
		return
	}
	if !backHasContent || proxyConfig.BufferServerContent {
		backHasTrailers := backResp.HasTrailers()
		if foreResp.proxyPostMessage(backContent, backHasTrailers) != nil {
			if backHasTrailers {
				backStream.markBroken()
			}
			return
		}
		if backHasTrailers && !foreResp.proxyCopyTrailers(backResp, proxyConfig) {
			return
		}
	} else if err := foreResp.proxyPassMessage(backResp); err != nil {
		backStream.markBroken()
		return
	}
}

func init() {
	RegisterSocklet("sockProxy", func(compName string, stage *Stage, webapp *Webapp) Socklet {
		s := new(sockProxy)
		s.onCreate(compName, stage, webapp)
		return s
	})
}

// sockProxy socklet passes webSockets to http backends.
type sockProxy struct {
	// Parent
	Socklet_
	// Assocs
	backend HTTPBackend // the *HTTP[1-3]Backend to pass to
	// States
	WebSocketProxyConfig // embeded
}

func (s *sockProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	s.Socklet_.OnCreate(compName, stage, webapp)
}
func (s *sockProxy) OnShutdown() {
	s.webapp.DecSub() // socklet
}

func (s *sockProxy) OnConfigure() {
	// .toBackend
	if v, ok := s.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := s.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else {
				s.backend = backend.(HTTPBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for webSocket proxy")
	}
}
func (s *sockProxy) OnPrepare() {
	// Currently nothing.
}

func (s *sockProxy) IsProxy() bool { return true }

func (s *sockProxy) Serve(req Request, sock Socket) {
	WebSocketReverseProxy(req, sock, s.backend, &s.WebSocketProxyConfig)
}

// WebSocketProxyConfig
type WebSocketProxyConfig struct {
	// TODO
}

// WebSocketReverseProxy
func WebSocketReverseProxy(foreReq Request, foreSock Socket, backend HTTPBackend, proxyConfig *WebSocketProxyConfig) {
	// TODO
	foreSock.Close()
}
