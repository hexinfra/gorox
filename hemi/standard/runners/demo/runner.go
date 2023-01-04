// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo runner.

package demo

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
)

func init() {
	RegisterRunner("demo", func(name string, stage *Stage) Runner {
		r := new(demoRunner)
		r.onCreate(name, stage)
		return r
	})
}

type demoRunner struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	// States
}

func (r *demoRunner) onCreate(name string, stage *Stage) {
	r.CompInit(name)
	r.stage = stage
}

func (r *demoRunner) OnConfigure() {
}
func (r *demoRunner) OnPrepare() {
}

func (r *demoRunner) OnShutdown() {
	r.Shutdown()
}

func (r *demoRunner) Run() { // goroutine
	Loop(time.Second, r.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Printf("demoRunner=%s done\n", r.Name())
	}
	r.stage.SubDone()
}
