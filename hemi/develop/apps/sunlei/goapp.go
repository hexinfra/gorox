// Copyright (c) 2020-2024 Sun Lei <valentine0401@163.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package sunlei

import (
	"github.com/hexinfra/gorox/hemi/addons/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("sunlei", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("sunleiHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(sunleiHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// sunleiHandlet
type sunleiHandlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *sunleiHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *sunleiHandlet) OnShutdown() {
	h.webapp.DecSub()
}

func (h *sunleiHandlet) OnConfigure() {
}
func (h *sunleiHandlet) OnPrepare() {
	m := simple.New()

	h.UseMapper(h, m)
}

func (h *sunleiHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
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
