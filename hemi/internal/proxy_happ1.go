// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP/1 proxy implementation.

package internal

func init() {
	RegisterHandlet("happ1Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(happ1Proxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// happ1Proxy handlet passes requests to another/backend HAPP/1 servers and cache responses.
type happ1Proxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *happ1Proxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *happ1Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *happ1Proxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *happ1Proxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *happ1Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	return
}
