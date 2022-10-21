// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello handlers print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("helloHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(helloHandler)
		h.init(name, stage, app)
		return h
	})
}

// helloHandler
type helloHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
	Type string
	text string
}

func (h *helloHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app
}

func (h *helloHandler) OnConfigure() {
	// type
	h.ConfigureString("type", &h.Type, func(value string) bool { return value != "" }, "text/plain; charset=utf-8")
	// text
	h.ConfigureString("text", &h.text, func(value string) bool { return value != "" }, "hello, world!")
}
func (h *helloHandler) OnPrepare() {
}
func (h *helloHandler) OnShutdown() {
}

func (h *helloHandler) Handle(req Request, resp Response) (next bool) {
	resp.SetContentType(h.Type)
	resp.Send(h.text)
	return
}
