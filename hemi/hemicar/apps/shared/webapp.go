// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package shared

import (
	"github.com/hexinfra/gorox/hemi/classic/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("shared", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("sharedHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(sharedHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// sharedHandlet
type sharedHandlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *sharedHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *sharedHandlet) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *sharedHandlet) OnConfigure() {
}
func (h *sharedHandlet) OnPrepare() {
	m := simple.New()

	h.UseMapper(h, m)
}

func (h *sharedHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *sharedHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
