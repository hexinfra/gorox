// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis proxy dealet passes connections to Redis backends.

package redis

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/builtin/backends/redis"
)

func init() {
	RegisterTCPXDealet("redisProxy", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(redisProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// redisProxy
type redisProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	router  *TCPXRouter
	backend *RedisBackend // the backend to pass to
	// States
}

func (d *redisProxy) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.TCPXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *redisProxy) OnShutdown() {
	d.router.DecDealet()
}

func (d *redisProxy) OnConfigure() {
	// TODO
}
func (d *redisProxy) OnPrepare() {
	// TODO
}

func (d *redisProxy) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}
