// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis proxy dealet passes conns to backend Redis servers.

package redis

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPSDealet("redisProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(redisProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// redisProxy
type redisProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (d *redisProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *redisProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *redisProxy) OnConfigure() {
	// TODO
}
func (d *redisProxy) OnPrepare() {
	// TODO
}

func (d *redisProxy) Deal(conn *TCPSConn) (dealt bool) {
	return true
}
