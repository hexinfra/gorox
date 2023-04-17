// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB proxy handlet and WebSocket/HWEB proxy socklet implementation.

package internal

func init() {
	RegisterHandlet("hwebProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(hwebProxy)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterSocklet("hsockProxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(hsockProxy)
		s.onCreate(name, stage, app)
		return s
	})
}

// hwebProxy handlet passes requests to another HWEB servers and cache responses.
type hwebProxy struct {
	// Mixins
	httpProxy_
	// States
}

func (h *hwebProxy) onCreate(name string, stage *Stage, app *App) {
	h.httpProxy_.onCreate(name, stage, app)
}
func (h *hwebProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *hwebProxy) OnConfigure() {
	h.httpProxy_.onConfigure()
}
func (h *hwebProxy) OnPrepare() {
	h.httpProxy_.onPrepare()
}

func (h *hwebProxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	return
}

// hsockProxy socklet passes websockets to another WebSocket/HWEB servers.
type hsockProxy struct {
	// Mixins
	sockProxy_
	// States
}

func (s *hsockProxy) onCreate(name string, stage *Stage, app *App) {
	s.sockProxy_.onCreate(name, stage, app)
}
func (s *hsockProxy) OnShutdown() {
	s.app.SubDone()
}

func (s *hsockProxy) OnConfigure() {
	s.sockProxy_.onConfigure()
}
func (s *hsockProxy) OnPrepare() {
	s.sockProxy_.onPrepare()
}

func (s *hsockProxy) Serve(req Request, sock Socket) { // forward or reverse
	// TODO(diogin): Implementation
	if s.isForward {
	} else {
	}
	sock.Close()
}
