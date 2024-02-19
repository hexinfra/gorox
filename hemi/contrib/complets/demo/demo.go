// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo complet.

package demo

import (
	"fmt"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterComplet("demoComplet", func(name string, stage *Stage) Complet {
		c := new(demoComplet)
		c.onCreate(name, stage)
		return c
	})
}

type demoComplet struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (c *demoComplet) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}
func (c *demoComplet) OnShutdown() {
	close(c.ShutChan) // notifies Run()
}

func (c *demoComplet) OnConfigure() {
	// TODO
}
func (c *demoComplet) OnPrepare() {
	// TODO
}

func (c *demoComplet) Run() { // runner
	c.Loop(time.Second, func(now time.Time) {
		fmt.Printf("i'm complet %s\n", c.Name())
	})
	if Debug() >= 2 {
		Printf("demoComplet=%s done\n", c.Name())
	}
	c.stage.SubDone()
}
