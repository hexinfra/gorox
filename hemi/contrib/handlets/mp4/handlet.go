// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MP4 handlets provide pseudo-streaming support for MP4 files.

package favicon

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("mp4Handlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(mp4Handlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// mp4Handlet
type mp4Handlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *mp4Handlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *mp4Handlet) OnShutdown() {
	h.app.SubDone()
}

func (h *mp4Handlet) OnConfigure() {
}
func (h *mp4Handlet) OnPrepare() {
}

func (h *mp4Handlet) Handle(req Request, resp Response) (next bool) {
	// TODO
	resp.SendBytes(nil)
	return
}
