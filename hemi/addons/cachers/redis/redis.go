// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis cacher implementation.

package redis

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/addons/backends/redis"
)

func init() {
	RegisterCacher("redisCacher", func(name string, stage *Stage) Cacher {
		c := new(redisCacher)
		c.onCreate(name, stage)
		return c
	})
}

// redisCacher
type redisCacher struct {
	// Parent
	Cacher_
	// Assocs
	stage *Stage // current stage
	// States
	nodes []string
}

func (c *redisCacher) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}
func (c *redisCacher) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *redisCacher) OnConfigure() {
	// TODO
}
func (c *redisCacher) OnPrepare() {
	// TODO
}

func (c *redisCacher) Maintain() { // runner
	c.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if DbgLevel() >= 2 {
		Printf("redisCacher=%s done\n", c.Name())
	}
	c.stage.DecSub()
}

func (c *redisCacher) Set(key []byte, wobject *Wobject) {
	// TODO
}
func (c *redisCacher) Get(key []byte) (wobject *Wobject) {
	// TODO
	return
}
func (c *redisCacher) Del(key []byte) bool {
	// TODO
	return false
}
