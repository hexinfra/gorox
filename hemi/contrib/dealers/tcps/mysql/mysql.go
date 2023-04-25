// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL relay dealer passes conns to backend MySQL servers.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("mysqlRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(mysqlRelay)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// mysqlRelay
type mysqlRelay struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *mysqlRelay) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *mysqlRelay) OnShutdown() {
	d.mesher.SubDone()
}

func (d *mysqlRelay) OnConfigure() {
	// TODO
}
func (d *mysqlRelay) OnPrepare() {
	// TODO
}

func (d *mysqlRelay) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
