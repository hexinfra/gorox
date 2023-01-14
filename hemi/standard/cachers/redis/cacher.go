// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis cacher implementation.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
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
	// Mixins
	Cacher_
	// Assocs
	stage *Stage
	// States
	nodes []string
}

func (c *redisCacher) onCreate(name string, stage *Stage) {
	c.CompInit(name)
	c.stage = stage
}
func (c *redisCacher) OnShutdown() {
	close(c.Shut)
}

func (c *redisCacher) OnConfigure() {
}
func (c *redisCacher) OnPrepare() {
}

func (c *redisCacher) Maintain() { // goroutine
	Loop(time.Second, c.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("redisCacher=%s done\n", c.Name())
	}
	c.stage.SubDone()
}

func (c *redisCacher) Set(key []byte, hobject *Hobject) {
}
func (c *redisCacher) Get(key []byte) (hobject *Hobject) {
	return
}
func (c *redisCacher) Del(key []byte) bool {
	return false
}
