// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Memory Hcache implementation.

package memory

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHcache("memoryHcache", func(compName string, stage *Stage) Hcache {
		c := new(memoryHcache)
		c.onCreate(compName, stage)
		return c
	})
}

// memoryHcache caches hobjects in memory.
type memoryHcache struct {
	// Parent
	Hcache_
	// States
}

func (c *memoryHcache) onCreate(compName string, stage *Stage) {
	c.Hcache_.OnCreate(compName, stage)
}
func (c *memoryHcache) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *memoryHcache) OnConfigure() {
	// TODO
}
func (c *memoryHcache) OnPrepare() {
	// TODO
}

func (c *memoryHcache) Maintain() { // runner
	c.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("memoryHcache=%s done\n", c.CompName())
	}
	c.Stage().DecSub() // hcache
}

func (c *memoryHcache) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *memoryHcache) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *memoryHcache) Del(key []byte) bool {
	// TODO
	return false
}
