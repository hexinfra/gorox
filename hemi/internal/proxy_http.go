// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP proxy handler and WebSocket proxy socklet implementation.

package internal

// httpProxy_ is the mixin for http1Proxy, http2Proxy, http3Proxy.
type httpProxy_ struct {
	// Mixins
	Handler_
	proxy_
	// Assocs
	app    *App   // the app to which the proxy belongs
	cacher Cacher // the cache server which is used by this proxy
	// States
	bufferClientContent bool        // buffer client content into TempFile?
	bufferServerContent bool        // buffer server content into TempFile?
	delRequestHeaders   [][]byte    // client request headers to delete
	addRequestHeaders   [][2][]byte // headers appended to client request
	delResponseHeaders  [][]byte    // server response headers to delete
	addResponseHeaders  [][2][]byte // headers appended to server response
}

func (h *httpProxy_) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.proxy_.init(stage)
	h.app = app
}

func (h *httpProxy_) configure(c Component) {
	h.proxy_.configure(c)
	if h.proxyMode == "forward" && !h.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}
	// withCache
	if v, ok := h.Find("withCache"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCache")
		}
	}
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	var headers []string
	// delRequestHeaders
	h.ConfigureStringList("delRequestHeaders", &headers, nil, []string{})
	for _, header := range headers {
		h.delRequestHeaders = append(h.delRequestHeaders, []byte(header))
	}
	// delResponseHeaders
	h.ConfigureStringList("delResponseHeaders", &headers, nil, []string{})
	for _, header := range headers {
		h.delResponseHeaders = append(h.delResponseHeaders, []byte(header))
	}
}
func (h *httpProxy_) prepare() {
}
func (h *httpProxy_) shutdown() {
}

func (h *httpProxy_) IsProxy() bool { return true }
func (h *httpProxy_) IsCache() bool { return h.cacher != nil }

// sockProxy_ is the mixin for sock1Proxy, sock2Proxy, sock3Proxy.
type sockProxy_ struct {
	// Mixins
	Socklet_
	proxy_
	// Assocs
	app *App // the app to which the proxy belongs
	// States
}

func (h *sockProxy_) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.proxy_.init(stage)
	h.app = app
}

func (h *sockProxy_) configure(c Component) {
	h.proxy_.configure(c)
	if h.proxyMode == "forward" && !h.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}
}
func (h *sockProxy_) prepare() {
}
func (h *sockProxy_) shutdown() {
}

func (h *sockProxy_) IsProxy() bool { return true }
