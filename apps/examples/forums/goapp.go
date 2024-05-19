// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A simple forums webapp.

package forums

import (
	"github.com/hexinfra/gorox/hemi/contrib/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("forums", func(webapp *Webapp) error {
		return nil
	})
	RegisterHandlet("forumsHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(forumsHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

type forumsHandlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *forumsHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *forumsHandlet) OnShutdown() {
	h.webapp.DecSub()
}

func (h *forumsHandlet) OnConfigure() {
}
func (h *forumsHandlet) OnPrepare() {
	m := simple.New()

	m.GET("/", h.index)

	h.UseMapper(h, m)
}

func (h *forumsHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *forumsHandlet) notFound(req Request, resp Response) {
	resp.Send("oops, target not found!")
}

func (h *forumsHandlet) index(req Request, resp Response) {
	resp.Send("forums index")
}
