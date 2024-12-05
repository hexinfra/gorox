// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis proxy dealet passes connections to Redis backends.

package redis

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/classic/backends/redis"
)

func init() {
	RegisterTCPXDealet("redisProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(redisProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// redisProxy
type redisProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *TCPXRouter
	backend *RedisBackend // the backend to pass to
	// States
}

func (d *redisProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *redisProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *redisProxy) OnConfigure() {
	// TODO
}
func (d *redisProxy) OnPrepare() {
	// TODO
}

func (d *redisProxy) Deal(conn *TCPXConn) (dealt bool) {
	return true
}
