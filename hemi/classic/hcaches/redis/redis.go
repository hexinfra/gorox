// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis Hcache implementation.

package redis

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/drivers/redis"
)

func init() {
	RegisterHcache("redisHcache", func(compName string, stage *Stage) Hcache {
		c := new(redisHcache)
		c.onCreate(compName, stage)
		return c
	})
}

// redisHcache
type redisHcache struct {
	// Parent
	Hcache_
	// States
	nodes []string
}

func (c *redisHcache) onCreate(compName string, stage *Stage) {
	c.Hcache_.OnCreate(compName, stage)
}
func (c *redisHcache) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *redisHcache) OnConfigure() {
	// TODO
}
func (c *redisHcache) OnPrepare() {
	// TODO
}

func (c *redisHcache) Maintain() { // runner
	c.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("redisHcache=%s done\n", c.CompName())
	}
	c.Stage().DecSub() // hcache
}

func (c *redisHcache) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *redisHcache) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *redisHcache) Del(key []byte) bool {
	// TODO
	return false
}
