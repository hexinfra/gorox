// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// PgSQL relay dealer passes conns to backend PgSQL servers.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("pgsqlRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(pgsqlRelay)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// pgsqlRelay
type pgsqlRelay struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *pgsqlRelay) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *pgsqlRelay) OnShutdown() {
	d.mesher.SubDone()
}

func (d *pgsqlRelay) OnConfigure() {
	// TODO
}
func (d *pgsqlRelay) OnPrepare() {
	// TODO
}

func (d *pgsqlRelay) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
