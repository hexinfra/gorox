// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Web (HTTP and WebSocket) reverse proxy implementation.

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

// stream is the backend-side http stream.
type stream interface { // for *backend[1-3]Stream
	Request() request
	Response() response
	Socket() socket

	isBroken() bool
	markBroken()
}

//////////////////////////////////////// HTTP exchan proxy implementation ////////////////////////////////////////

// request is the backend-side http request.
type request interface { // for *backend[1-3]Request
	setMethodURI(method []byte, uri []byte, hasContent bool) bool
	proxySetAuthority(hostname []byte, colonPort []byte) bool
	proxyCopyCookies(foreReq Request) bool // HTTP 1.x/2/3 have different requirements on "cookie" header
	proxyCopyHeaders(foreReq Request, proxyConfig *WebExchanProxyConfig) bool
	proxyPassMessage(foreReq Request) error                       // pass content to backend directly
	proxyPostMessage(foreContent any, foreHasTrailers bool) error // post held content to backend
	proxyCopyTrailers(foreReq Request, proxyConfig *WebExchanProxyConfig) bool
	isVague() bool
	endVague() error
}

// response is the backend-side http response.
type response interface { // for *backend[1-3]Response
	KeepAlive() int8
	HeadResult() int16
	BodyResult() int16
	Status() int16
	HasContent() bool
	ContentSize() int64
	HasTrailers() bool
	IsVague() bool
	examineTail() bool
	proxyTakeContent() any
	readContent() (p []byte, err error)
	proxyDelHopHeaders()
	proxyDelHopTrailers()
	forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool
	forTrailers(callback func(header *pair, name []byte, value []byte) bool) bool
	recvHead()
	reuse()
}

// httpProxy handlet passes http requests to http backends and caches responses.
type httpProxy struct {
	// Parent
	Handlet_
	// Assocs
	stage   *Stage     // current stage
	webapp  *Webapp    // the webapp to which the proxy belongs
	backend WebBackend // the *HTTP[1-3]Backend to pass to
	cacher  Cacher     // the cacher which is used by this proxy
	// States
	WebExchanProxyConfig // embeded
}

func (h *httpProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *httpProxy) OnShutdown() {
	h.webapp.DecSub() // handlet
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
	WebExchanReverseProxy(req, resp, h.cacher, h.backend, &h.WebExchanProxyConfig)
	return true
}

// WebExchanProxyConfig
type WebExchanProxyConfig struct {
	// Inbound
	BufferClientContent bool
	Hostname            []byte // overrides client's hostname
	ColonPort           []byte // overrides client's colonPort
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
func WebExchanReverseProxy(foreReq Request, foreResp Response, cacher Cacher, backend WebBackend, proxyConfig *WebExchanProxyConfig) {
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

	backStream, backErr := backend.FetchStream()
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
				backErr = webOutTrailerFailed
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
	if foreReq.MethodCode() != MethodHEAD {
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

// Cacher component is the interface to storages of HTTP caching.
type Cacher interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Set(key []byte, hobject *Hobject)
	Get(key []byte) (hobject *Hobject)
	Del(key []byte) bool
}

// Cacher_ is the parent for all cachers.
type Cacher_ struct {
	// Parent
	Component_
	// Assocs
	// States
}

func (c *Cacher_) todo() {
}

// Hobject represents an HTTP object in Cacher.
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

//////////////////////////////////////// HTTP socket proxy implementation ////////////////////////////////////////

// socket is the backend-side webSocket.
type socket interface { // for *backend[1-3]Socket
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

// sockProxy socklet passes webSockets to http backends.
type sockProxy struct {
	// Parent
	Socklet_
	// Assocs
	stage   *Stage     // current stage
	webapp  *Webapp    // the webapp to which the proxy belongs
	backend WebBackend // the *HTTP[1-3]Backend to pass to
	// States
	WebSocketProxyConfig // embeded
}

func (s *sockProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	s.MakeComp(name)
	s.stage = stage
	s.webapp = webapp
}
func (s *sockProxy) OnShutdown() {
	s.webapp.DecSub() // socklet
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
func WebSocketReverseProxy(req Request, sock Socket, backend WebBackend, proxyConfig *WebSocketProxyConfig) {
	// TODO
	sock.Close()
}
