// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo uniture.

package demo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
)

func init() {
	RegisterUniture("demo", func(name string, stage *Stage) Uniture {
		u := new(demoUniture)
		u.onCreate(name, stage)
		return u
	})
}

type demoUniture struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	// States
}

func (u *demoUniture) onCreate(name string, stage *Stage) {
	u.SetUp(name)
	u.stage = stage
}
func (u *demoUniture) OnShutdown() {
	close(u.Shut)
}

func (u *demoUniture) OnConfigure() {
}
func (u *demoUniture) OnPrepare() {
}

func (u *demoUniture) Run() { // goroutine
	Loop(time.Second, u.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("demoUniture=%s done\n", u.Name())
	}
	u.stage.SubDone()
}
