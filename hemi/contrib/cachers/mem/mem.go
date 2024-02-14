// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Memory cacher implementation.

package mem

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterCacher("memCacher", func(name string, stage *Stage) Cacher {
		c := new(memCacher)
		c.onCreate(name, stage)
		return c
	})
}

// memCacher
type memCacher struct {
	// Mixins
	Cacher_
	// Assocs
	stage *Stage // current stage
	// States
}

func (c *memCacher) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
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
	c.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Printf("memCacher=%s done\n", c.Name())
	}
	c.stage.SubDone()
}

func (c *memCacher) Set(key []byte, wobject *Wobject) {
	// TODO
}
func (c *memCacher) Get(key []byte) (wobject *Wobject) {
	// TODO
	return
}
func (c *memCacher) Del(key []byte) bool {
	// TODO
	return false
}
