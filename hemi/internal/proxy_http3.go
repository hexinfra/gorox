// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 proxy handlet and WebSocket/3 proxy socklet implementation.

package internal

func init() {
	RegisterHandlet("http3Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(http3Proxy)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterSocklet("sock3Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock3Proxy)
		s.onCreate(name, stage, app)
		return s
	})
}

// http3Proxy handlet passes requests to backend HTTP/3 servers and cache responses.
type http3Proxy struct {
	// Mixins
	httpProxy_
	// States
}

func (h *http3Proxy) onCreate(name string, stage *Stage, app *App) {
	h.httpProxy_.onCreate(name, stage, app)
}

func (h *http3Proxy) OnConfigure() {
	h.httpProxy_.onConfigure(h)
}
func (h *http3Proxy) OnPrepare() {
	h.httpProxy_.onPrepare()
}

func (h *http3Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http3Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO(diogin): Implementation
	return
}

// sock3Proxy socklet relays websockets to backend WebSocket/3 servers.
type sock3Proxy struct {
	// Mixins
	sockProxy_
	// States
}

func (s *sock3Proxy) onCreate(name string, stage *Stage, app *App) {
	s.sockProxy_.onCreate(name, stage, app)
}

func (s *sock3Proxy) OnConfigure() {
	s.sockProxy_.onConfigure(s)
}
func (s *sock3Proxy) OnPrepare() {
	s.sockProxy_.onPrepare()
}

func (s *sock3Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock3Proxy) Serve(req Request, sock Socket) { // currently reverse only
	// TODO(diogin): Implementation
	sock.Close()
}
