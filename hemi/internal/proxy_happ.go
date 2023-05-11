// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HAPP proxy implementation.

package internal

func init() {
	RegisterHandlet("happProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(happProxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// happProxy handlet passes requests to another/backend HAPP servers and cache responses.
type happProxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *happProxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *happProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *happProxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *happProxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *happProxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	// TODO
	return
}
