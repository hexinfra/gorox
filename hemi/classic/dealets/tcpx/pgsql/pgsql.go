// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pgsql proxy dealet passes connections to Pgsql backends.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/classic/backends/pgsql"
)

func init() {
	RegisterTCPXDealet("pgsqlProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(pgsqlProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *TCPXRouter
	backend *PgsqlBackend // the backend to pass to
	// States
}

func (d *pgsqlProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *pgsqlProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *pgsqlProxy) OnConfigure() {
	// TODO
}
func (d *pgsqlProxy) OnPrepare() {
	// TODO
}

func (d *pgsqlProxy) Deal(conn *TCPXConn) (dealt bool) {
	return true
}
