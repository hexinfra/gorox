// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo addon.

package demo

import (
	"fmt"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterAddon("demoAddon", func(name string, stage *Stage) Addon {
		a := new(demoAddon)
		a.onCreate(name, stage)
		return a
	})
}

type demoAddon struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (a *demoAddon) onCreate(name string, stage *Stage) {
	a.MakeComp(name)
	a.stage = stage
}
func (a *demoAddon) OnShutdown() {
	close(a.ShutChan) // notifies Run()
}

func (a *demoAddon) OnConfigure() {
	// TODO
}
func (a *demoAddon) OnPrepare() {
	// TODO
}

func (a *demoAddon) Run() { // runner
	a.Loop(time.Second, func(now time.Time) {
		fmt.Printf("i'm addon %s\n", a.Name())
	})
	if Debug() >= 2 {
		Printf("demoAddon=%s done\n", a.Name())
	}
	a.stage.SubDone()
}
