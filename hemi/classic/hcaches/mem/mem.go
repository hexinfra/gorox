// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Memory HTTP hcache implementation.

package mem

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHcache("memHcache", func(compName string, stage *Stage) Hcache {
		c := new(memHcache)
		c.onCreate(compName, stage)
		return c
	})
}

// memHcache
type memHcache struct {
	// Parent
	Hcache_
	// Assocs
	stage *Stage // current stage
	// States
}

func (c *memHcache) onCreate(compName string, stage *Stage) {
	c.MakeComp(compName)
	c.stage = stage
}
func (c *memHcache) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *memHcache) OnConfigure() {
	// TODO
}
func (c *memHcache) OnPrepare() {
	// TODO
}

func (c *memHcache) Maintain() { // runner
	c.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("memHcache=%s done\n", c.CompName())
	}
	c.stage.DecSub() // hcache
}

func (c *memHcache) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *memHcache) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *memHcache) Del(key []byte) bool {
	// TODO
	return false
}
