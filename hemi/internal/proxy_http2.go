// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 proxy handler and WebSocket/2 proxy socklet implementation.

package internal

func init() {
	RegisterHandler("http2Proxy", func(name string, stage *Stage, app *App) Handler {
		h := new(http2Proxy)
		h.init(name, stage, app)
		return h
	})
	RegisterSocklet("sock2Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock2Proxy)
		s.init(name, stage, app)
		return s
	})
}

// http2Proxy handler passes requests to backend HTTP/2 servers and cache responses.
type http2Proxy struct {
	// Mixins
	httpProxy_
	// States
}

func (h *http2Proxy) init(name string, stage *Stage, app *App) {
	h.httpProxy_.init(name, stage, app)
}

func (h *http2Proxy) OnConfigure() {
	h.httpProxy_.onConfigure(h)
}
func (h *http2Proxy) OnPrepare() {
	h.httpProxy_.onPrepare()
}

func (h *http2Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http2Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO(diogin): Implementation
	return
}

// sock2Proxy socklet passes websockets to backend WebSocket/2 servers.
type sock2Proxy struct {
	// Mixins
	sockProxy_
	// States
}

func (s *sock2Proxy) init(name string, stage *Stage, app *App) {
	s.sockProxy_.init(name, stage, app)
}

func (s *sock2Proxy) OnConfigure() {
	s.sockProxy_.onConfigure(s)
}
func (s *sock2Proxy) OnPrepare() {
	s.sockProxy_.onPrepare()
}

func (s *sock2Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock2Proxy) Serve(req Request, sock Socket) { // currently reverse only
	// TODO(diogin): Implementation
	sock.Close()
}
