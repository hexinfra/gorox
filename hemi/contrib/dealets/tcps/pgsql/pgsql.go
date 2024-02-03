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
	RegisterTCPSDealet("pgsqlProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(pgsqlProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (d *pgsqlProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *pgsqlProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *pgsqlProxy) OnConfigure() {
	// TODO
}
func (d *pgsqlProxy) OnPrepare() {
	// TODO
}

func (d *pgsqlProxy) Deal(conn *TCPSConn) (dealt bool) {
	return true
}
