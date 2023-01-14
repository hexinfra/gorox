// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo opture.

package demo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
)

func init() {
	RegisterOpture("demo", func(name string, stage *Stage) Opture {
		o := new(demoOpture)
		o.onCreate(name, stage)
		return o
	})
}

type demoOpture struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	// States
}

func (o *demoOpture) onCreate(name string, stage *Stage) {
	o.CompInit(name)
	o.stage = stage
}
func (o *demoOpture) OnShutdown() {
	close(o.Shut)
}

func (o *demoOpture) OnConfigure() {
}
func (o *demoOpture) OnPrepare() {
}

func (o *demoOpture) Run() { // goroutine
	Loop(time.Second, o.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("demoOpture=%s done\n", o.Name())
	}
	o.stage.SubDone()
}
