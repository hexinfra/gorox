// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis relay dealer passes conns to backend Redis servers.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("redisRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(redisRelay)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// redisRelay
type redisRelay struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *redisRelay) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *redisRelay) OnShutdown() {
	d.mesher.SubDone()
}

func (d *redisRelay) OnConfigure() {
	// TODO
}
func (d *redisRelay) OnPrepare() {
	// TODO
}

func (d *redisRelay) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
