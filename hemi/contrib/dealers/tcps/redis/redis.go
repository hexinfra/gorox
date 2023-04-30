// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis proxy dealer passes conns to backend Redis servers.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("redisProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(redisProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// redisProxy
type redisProxy struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *redisProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *redisProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *redisProxy) OnConfigure() {
	// TODO
}
func (d *redisProxy) OnPrepare() {
	// TODO
}

func (d *redisProxy) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
