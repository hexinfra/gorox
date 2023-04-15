// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP proxy handlet and WebSocket proxy socklet implementation.

package internal

// httpProxy_ is the mixin for http[1-3]Proxy.
type httpProxy_ struct {
	// Mixins
	Handlet_
	// Assocs
	stage   *Stage  // current stage
	app     *App    // the app to which the proxy belongs
	cacher  Cacher  // the cacher which is used by this proxy
	backend backend // if works as forward proxy, this is nil
	// States
	hostname            []byte      // ...
	colonPort           []byte      // ...
	viaName             []byte      // ...
	isForward           bool        // reverse if false
	bufferClientContent bool        // buffer client content into tempFile?
	bufferServerContent bool        // buffer server content into tempFile?
	addRequestHeaders   [][2][]byte // headers appended to client request
	delRequestHeaders   [][]byte    // client request headers to delete
	addResponseHeaders  [][2][]byte // headers appended to server response
	delResponseHeaders  [][]byte    // server response headers to delete
}

func (h *httpProxy_) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}

func (h *httpProxy_) onConfigure() {
	// proxyMode
	if v, ok := h.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			h.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	}
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				h.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if !h.isForward {
		UseExitln("toBackend is required for reverse proxy")
	}
	if h.isForward && !h.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
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
	// delResponseHeaders
	h.ConfigureBytesList("delResponseHeaders", &h.delResponseHeaders, nil, [][]byte{})
}
func (h *httpProxy_) onPrepare() {
}

func (h *httpProxy_) IsProxy() bool { return true }
func (h *httpProxy_) IsCache() bool { return h.cacher != nil }

// sockProxy_ is the mixin for sock[1-3]Proxy.
type sockProxy_ struct {
	// Mixins
	Socklet_
	// Assocs
	stage   *Stage  // current stage
	app     *App    // the app to which the proxy belongs
	backend backend // if works as forward proxy, this is nil
	// States
	isForward bool // reverse if false
}

func (s *sockProxy_) onCreate(name string, stage *Stage, app *App) {
	s.MakeComp(name)
	s.stage = stage
	s.app = app
}

func (s *sockProxy_) onConfigure() {
	// proxyMode
	if v, ok := s.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			s.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	}
	// toBackend
	if v, ok := s.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := s.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				s.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if !s.isForward {
		UseExitln("toBackend is required for reverse proxy")
	}
	if s.isForward && !s.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}
}
func (s *sockProxy_) onPrepare() {
}

func (s *sockProxy_) IsProxy() bool { return true }