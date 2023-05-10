// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP/2 proxy implementation.

package internal

func init() {
	RegisterHandlet("happ2Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(happ2Proxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// happ2Proxy handlet passes requests to another/backend HAPP/2 servers and cache responses.
type happ2Proxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *happ2Proxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *happ2Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *happ2Proxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *happ2Proxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *happ2Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	return
}
