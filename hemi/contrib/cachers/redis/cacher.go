// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis cacher implementation.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterCacher("redisCacher", func(name string, stage *Stage) Cacher {
		c := new(redisCacher)
		c.init(name, stage)
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

func (c *redisCacher) init(name string, stage *Stage) {
	c.SetName(name)
	c.stage = stage
}

func (c *redisCacher) OnConfigure() {
}
func (c *redisCacher) OnPrepare() {
}
func (c *redisCacher) OnShutdown() {
}

func (c *redisCacher) Maintain() { // blocking
}

func (c *redisCacher) Set(key []byte, value *Centry) {
}
func (c *redisCacher) Get(key []byte) (value *Centry) {
	return
}
