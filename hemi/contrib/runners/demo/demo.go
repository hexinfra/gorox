// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo runner.

package demo

import (
	"fmt"
	"time"

	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterRunner("demoRunner", func(name string, stage *Stage) Runner {
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
	r.MakeComp(name)
	r.stage = stage
}
func (r *demoRunner) OnShutdown() {
	close(r.Shut)
}

func (r *demoRunner) OnConfigure() {
	// TODO
}
func (r *demoRunner) OnPrepare() {
	// TODO
}

func (r *demoRunner) Run() { // goroutine
	r.Loop(time.Second, func(now time.Time) {
		fmt.Printf("i'm runner %s\n", r.Name())
	})
	if IsDebug(2) {
		Debugf("demoRunner=%s done\n", r.Name())
	}
	r.stage.SubDone()
}
