// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// MP4 handlets provide pseudo-streaming support for MP4 files.

package favicon

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("mp4Handlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(mp4Handlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// mp4Handlet
type mp4Handlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
}

func (h *mp4Handlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *mp4Handlet) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *mp4Handlet) OnConfigure() {
	// TODO
}
func (h *mp4Handlet) OnPrepare() {
	// TODO
}

func (h *mp4Handlet) Handle(req Request, resp Response) (handled bool) {
	// TODO
	resp.SendBytes(nil)
	return true
}
