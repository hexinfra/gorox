// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Memory cacher implementation.

package mem

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
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
	stage *Stage
	// States
}

func (c *memCacher) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}
func (c *memCacher) OnShutdown() {
	close(c.Shut)
}

func (c *memCacher) OnConfigure() {
}
func (c *memCacher) OnPrepare() {
}

func (c *memCacher) Maintain() { // goroutine
	Loop(time.Second, c.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("memCacher=%s done\n", c.Name())
	}
	c.stage.SubDone()
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
