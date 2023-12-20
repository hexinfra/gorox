// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General Web proxy implementation.

package internal

import (
	"strings"
)

// exchanProxy_ is the mixin for http[1-3]Proxy and hwebProxy.
type exchanProxy_ struct {
	// Mixins
	Handlet_
	// Assocs
	stage   *Stage  // current stage
	webapp  *Webapp // the webapp to which the proxy belongs
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

func (h *exchanProxy_) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
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
	if h.isForward && !h.webapp.isDefault {
		UseExitln("forward proxy can be bound to default webapp only")
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
func (h *exchanProxy_) onPrepare() {
	// Currently nothing.
}

func (h *exchanProxy_) IsProxy() bool { return true }
func (h *exchanProxy_) IsCache() bool { return h.cacher != nil }

// Cacher component is the interface to storages of Web caching. See RFC 9111.
type Cacher interface {
	// Imports
	Component
	// Methods
	Maintain() // goroutine
	Set(key []byte, webject *Webject)
	Get(key []byte) (webject *Webject)
	Del(key []byte) bool
}

// Cacher_
type Cacher_ struct {
	// Mixins
	Component_
}

// Webject is a Web object in cacher
type Webject struct {
	// TODO
	uri      []byte
	headers  any
	content  any
	trailers any
}

// socketProxy_ is the mixin for sock[1-3]Proxy.
type socketProxy_ struct {
	// Mixins
	Socklet_
	// Assocs
	stage   *Stage  // current stage
	webapp  *Webapp // the webapp to which the proxy belongs
	backend Backend // if works as forward proxy, this is nil
	// States
	isForward bool // reverse if false
}

func (s *socketProxy_) onCreate(name string, stage *Stage, webapp *Webapp) {
	s.MakeComp(name)
	s.stage = stage
	s.webapp = webapp
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
	if s.isForward && !s.webapp.isDefault {
		UseExitln("forward proxy can be bound to default webapp only")
	}
}
func (s *socketProxy_) onPrepare() {
	// Currently nothing.
}

func (s *socketProxy_) IsProxy() bool { return true }
