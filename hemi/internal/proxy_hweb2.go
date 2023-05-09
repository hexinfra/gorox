// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/2 proxy implementation.

package internal

func init() {
	RegisterHandlet("hweb2Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(hweb2Proxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// hweb2Proxy handlet passes requests to another/backend HWEB/2 servers and cache responses.
type hweb2Proxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *hweb2Proxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *hweb2Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *hweb2Proxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *hweb2Proxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *hweb2Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	return
}
