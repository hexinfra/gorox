// Copyright (c) 2020-2023 Sun Lei <valentine0401@163.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package sunlei

import (
	"github.com/hexinfra/gorox/hemi/contrib/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterAppInit("sunlei", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("sunleiHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(sunleiHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// sunleiHandlet
type sunleiHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *sunleiHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *sunleiHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *sunleiHandlet) OnConfigure() {
}
func (h *sunleiHandlet) OnPrepare() {
	m := simple.New()

	h.UseMapper(h, m)
}

func (h *sunleiHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *sunleiHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}

func (h *sunleiHandlet) GET_(req Request, resp Response) {
	resp.Send("sunlei's index page")
}
func (h *sunleiHandlet) GET_send(req Request, resp Response) {
	resp.Send("utf-8中文字符串")
}
func (h *sunleiHandlet) GET_echo(req Request, resp Response) {
	resp.Echo("a")
	resp.Echo("b")
}
