// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// PgSQL proxy dealet passes conns to backend PgSQL servers.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealet("pgsqlProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(pgsqlProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *pgsqlProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *pgsqlProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *pgsqlProxy) OnConfigure() {
	// TODO
}
func (d *pgsqlProxy) OnPrepare() {
	// TODO
}

func (d *pgsqlProxy) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (d *pgsqlProxy) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (d *pgsqlProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}
