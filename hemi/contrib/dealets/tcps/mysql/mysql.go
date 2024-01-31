// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL proxy dealet passes conns to backend MySQL servers.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealet("mysqlProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(mysqlProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// mysqlProxy
type mysqlProxy struct {
	// Mixins
	TCPSDealet_
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

func (d *mysqlProxy) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (d *mysqlProxy) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (d *mysqlProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}
