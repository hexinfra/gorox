// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB proxy implementation.

package internal

func init() {
	RegisterHandlet("hwebProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(hwebProxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// hwebProxy handlet passes requests to another/backend HWEB servers and cache responses.
type hwebProxy struct {
	// Mixins
	exchanProxy_
	// States
}

func (h *hwebProxy) onCreate(name string, stage *Stage, app *App) {
	h.exchanProxy_.onCreate(name, stage, app)
}
func (h *hwebProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *hwebProxy) OnConfigure() {
	h.exchanProxy_.onConfigure()
}
func (h *hwebProxy) OnPrepare() {
	h.exchanProxy_.onPrepare()
}

func (h *hwebProxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	// hResp.onUse(Version3)
	return
}
