// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Memory HTTP cacher implementation.

package mem

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterCacher("memCacher", func(compName string, stage *Stage) Cacher {
		c := new(memCacher)
		c.onCreate(compName, stage)
		return c
	})
}

// memCacher
type memCacher struct {
	// Parent
	Cacher_
	// Assocs
	stage *Stage // current stage
	// States
}

func (c *memCacher) onCreate(compName string, stage *Stage) {
	c.MakeComp(compName)
	c.stage = stage
}
func (c *memCacher) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *memCacher) OnConfigure() {
	// TODO
}
func (c *memCacher) OnPrepare() {
	// TODO
}

func (c *memCacher) Maintain() { // runner
	c.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("memCacher=%s done\n", c.CompName())
	}
	c.stage.DecSub() // cacher
}

func (c *memCacher) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *memCacher) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *memCacher) Del(key []byte) bool {
	// TODO
	return false
}
