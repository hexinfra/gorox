// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP reverse proxy (a.k.a. gateway) implementation. See RFC 9110 and RFC 9111.

package hemi

import (
	"strings"
)

func init() {
	RegisterHandlet("httpProxy", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(httpProxy)
		h.onCreate(compName, stage, webapp)
		return h
	})
	RegisterSocklet("sockProxy", func(compName string, stage *Stage, webapp *Webapp) Socklet {
		s := new(sockProxy)
		s.onCreate(compName, stage, webapp)
		return s
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
	HTTPProxyConfig // embeded
}

func (h *httpProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *httpProxy) OnShutdown() { h.webapp.DecHandlet() }

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
	if v, ok := h.Find("addResponseHeaders"); ok {
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
					UseExitf("bad value in .addResponseHeaders")
				}
				addedHeaders[headerName] = vHeaderValue
			}
			h.AddResponseHeaders = addedHeaders
		} else {
			UseExitln("invalid addResponseHeaders")
		}
	}
	// .delResponseHeaders
	h.ConfigureBytesList("delResponseHeaders", &h.DelResponseHeaders, nil, [][]byte{})
}
func (h *httpProxy) OnPrepare() {
	// Currently nothing.
}

func (h *httpProxy) IsProxy() bool { return true }            // works as a reverse proxy
func (h *httpProxy) IsCache() bool { return h.hcache != nil } // works as a proxy cache?

func (h *httpProxy) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	HTTPReverseProxy(req, resp, h.hcache, h.backend, &h.HTTPProxyConfig)
	return true
}

// HTTPProxyConfig
type HTTPProxyConfig struct {
	// Inbound, to origin servers
	BufferClientContent bool
	Hostname            []byte // overrides client provided hostname
	Colonport           []byte // overrides client provided colonport
	InboundViaName      []byte
	AppendPathPrefix    []byte
	AddRequestHeaders   map[string]Value
	DelRequestHeaders   [][]byte
	// Outbound, to user agents
	BufferServerContent bool
	OutboundViaName     []byte
	AddResponseHeaders  map[string]Value
	DelResponseHeaders  [][]byte
}

// HTTPReverseProxy
func HTTPReverseProxy(servReq ServerRequest, servResp ServerResponse, hcache Hcache, backend HTTPBackend, proxyConfig *HTTPProxyConfig) {
	var servContent any // nil, []byte, tempFile
	servHasContent := servReq.HasContent()
	if servHasContent && proxyConfig.BufferClientContent { // including size 0
		servContent = servReq.proxyTakeContent()
		if servContent == nil { // take failed
			// servStream was marked as broken
			servResp.SetStatus(StatusBadRequest)
			servResp.SendBytes(nil)
			return
		}
	}

	backStream, backErr := backend.AcquireStream(servReq)
	if backErr != nil {
		servResp.SendBadGateway(nil)
		return
	}
	defer backend.ReleaseStream(backStream)

	backReq := backStream.Request()
	if !backReq.proxyCopyHeaderLines(servReq, proxyConfig) {
		backStream.markBroken()
		servResp.SendBadGateway(nil)
		return
	}

	if !servHasContent || proxyConfig.BufferClientContent {
		servHasTrailers := servReq.HasTrailers()
		backErr = backReq.proxyPostMessage(servContent, servHasTrailers)
		if backErr == nil && servHasTrailers {
			if !backReq.proxyCopyTrailerLines(servReq, proxyConfig) {
				backStream.markBroken()
				backErr = httpOutTrailerFailed
			} else if backErr = backReq.endVague(); backErr != nil {
				backStream.markBroken()
			}
		} else if servHasTrailers {
			backStream.markBroken()
		}
	} else if backErr = backReq.proxyPassMessage(servReq); backErr != nil {
		backStream.markBroken()
	} else if backReq.isVague() { // must write the last chunk and trailer fields (if exist)
		if backErr = backReq.endVague(); backErr != nil {
			backStream.markBroken()
		}
	}
	if backErr != nil {
		servResp.SendBadGateway(nil)
		return
	}

	backResp := backStream.Response()
	for { // until we found a non-1xx status (>= 200)
		backResp.recvHead()
		if backResp.HeadResult() != StatusOK || backResp.Status() == StatusSwitchingProtocols { // webSocket is not served in handlets.
			backStream.markBroken()
			if backResp.HeadResult() == StatusRequestTimeout {
				servResp.SendGatewayTimeout(nil)
			} else {
				servResp.SendBadGateway(nil)
			}
			return
		}
		if backResp.Status() >= StatusOK {
			if !backResp.KeepAlive() { // connection close. only HTTP/1.x uses this. TODO: what if the connection is closed remotely?
				backStream.(*backend1Stream).conn.persistent = false // backend told us to not keep the connection alive
			}
			break
		}
		// We got a 1xx response.
		if servReq.VersionCode() == Version1_0 { // 1xx response is not supported by HTTP/1.0
			backStream.markBroken()
			servResp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !servResp.proxyPass1xx(backResp) {
			backStream.markBroken()
			return
		}
		backResp.reuse()
	}

	var backContent any // nil, []byte, tempFile
	backHasContent := false
	if !servReq.IsHEAD() {
		backHasContent = backResp.HasContent()
	}
	if backHasContent && proxyConfig.BufferServerContent { // including size 0
		backContent = backResp.proxyTakeContent()
		if backContent == nil { // take failed
			// backStream was marked as broken
			servResp.SendBadGateway(nil)
			return
		}
	}

	if !servResp.proxyCopyHeaderLines(backResp, proxyConfig) {
		backStream.markBroken()
		return
	}
	if !backHasContent || proxyConfig.BufferServerContent {
		backHasTrailers := backResp.HasTrailers()
		if servResp.proxyPostMessage(backContent, backHasTrailers) != nil {
			if backHasTrailers {
				backStream.markBroken()
			}
			return
		}
		if backHasTrailers {
			if !servResp.proxyCopyTrailerLines(backResp, proxyConfig) {
				return
			}
		}
	} else if err := servResp.proxyPassMessage(backResp); err != nil {
		backStream.markBroken()
		return
	}
}

// sockProxy socklet passes webSockets to http backends.
type sockProxy struct {
	// Parent
	Socklet_
	// Assocs
	backend HTTPBackend // the *HTTP[1-3]Backend to pass to
	// States
	SOCKProxyConfig // embeded
}

func (s *sockProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	s.Socklet_.OnCreate(compName, stage, webapp)
}
func (s *sockProxy) OnShutdown() { s.webapp.DecSocklet() }

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

func (s *sockProxy) IsProxy() bool { return true } // works as a reverse proxy

func (s *sockProxy) Serve(req ServerRequest, sock ServerSocket) {
	SOCKReverseProxy(req, sock, s.backend, &s.SOCKProxyConfig)
}

// SOCKProxyConfig
type SOCKProxyConfig struct {
	// TODO
}

// SOCKReverseProxy
func SOCKReverseProxy(servReq ServerRequest, servSock ServerSocket, backend HTTPBackend, proxyConfig *SOCKProxyConfig) {
	// TODO
	servSock.Close()
}
