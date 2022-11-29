// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo optware.

package demo

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
)

func init() {
	RegisterOptware("demo", func(name string, stage *Stage) Optware {
		o := new(demoOptware)
		o.onCreate(name, stage)
		return o
	})
}

type demoOptware struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	// States
}

func (o *demoOptware) onCreate(name string, stage *Stage) {
	o.CompInit(name)
	o.stage = stage
}

func (o *demoOptware) OnConfigure() {
}
func (o *demoOptware) OnPrepare() {
}

func (o *demoOptware) OnShutdown() {
	o.Shutdown()
}

func (o *demoOptware) Run() { // goroutine
	Loop(time.Second, o.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Printf("demoOptware=%s done\n", o.Name())
	}
	o.stage.SubDone()
}
