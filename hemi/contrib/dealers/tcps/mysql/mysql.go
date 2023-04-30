// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL proxy dealer passes conns to backend MySQL servers.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("mysqlProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(mysqlProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// mysqlProxy
type mysqlProxy struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *mysqlProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *mysqlProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *mysqlProxy) OnConfigure() {
	// TODO
}
func (d *mysqlProxy) OnPrepare() {
	// TODO
}

func (d *mysqlProxy) Deal(conn *TCPSConn) (next bool) { // reverse only
	// TODO
	return false
}
