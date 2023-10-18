// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General Web proxy implementation.

package internal

// exchanProxy_ is the mixin for http[1-3]Proxy and hwebProxy.
type exchanProxy_ struct {
	// Mixins
	Handlet_
	// Assocs
	stage   *Stage  // current stage
	app     *App    // the app to which the proxy belongs
	backend Backend // if works as forward proxy, this is nil
	cacher  Cacher  // the cacher which is used by this proxy
	// States
	hostname            []byte            // hostname used in ":authority" and "host" header
	colonPort           []byte            // colonPort used in ":authority" and "host" header
	viaName             []byte            // ...
	isForward           bool              // reverse if false
	bufferClientContent bool              // buffer client content into tempFile?
	bufferServerContent bool              // buffer server content into tempFile?
	addRequestHeaders   map[string]Value  // headers appended to client request
	delRequestHeaders   [][]byte          // client request headers to delete
	addResponseHeaders  map[string]string // headers appended to server response
	delResponseHeaders  [][]byte          // server response headers to delete
}

func (h *exchanProxy_) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}

func (h *exchanProxy_) onConfigure() {
	// proxyMode
	if v, ok := h.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			h.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	} else {
		h.isForward = false
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

	// addRequestHeaders
	if v, ok := h.Find("addRequestHeaders"); ok {
		if headers, ok := v.Dict(); ok {
			h.addRequestHeaders = headers
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
func (h *exchanProxy_) onPrepare() {
	// Currently nothing.
}

func (h *exchanProxy_) IsProxy() bool { return true }
func (h *exchanProxy_) IsCache() bool { return h.cacher != nil }

// socketProxy_ is the mixin for sock[1-3]Proxy.
type socketProxy_ struct {
	// Mixins
	Socklet_
	// Assocs
	stage   *Stage  // current stage
	app     *App    // the app to which the proxy belongs
	backend Backend // if works as forward proxy, this is nil
	// States
	isForward bool // reverse if false
}

func (s *socketProxy_) onCreate(name string, stage *Stage, app *App) {
	s.MakeComp(name)
	s.stage = stage
	s.app = app
}

func (s *socketProxy_) onConfigure() {
	// proxyMode
	if v, ok := s.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			s.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	} else {
		s.isForward = false
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
func (s *socketProxy_) onPrepare() {
	// Currently nothing.
}

func (s *socketProxy_) IsProxy() bool { return true }
