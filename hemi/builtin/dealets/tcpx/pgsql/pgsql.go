// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pgsql proxy dealet passes connections to Pgsql backends.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/builtin/backends/pgsql"
)

func init() {
	RegisterTCPXDealet("pgsqlProxy", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(pgsqlProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	router  *TCPXRouter
	backend *PgsqlBackend // the backend to pass to
	// States
}

func (d *pgsqlProxy) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.TCPXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *pgsqlProxy) OnShutdown() {
	d.router.DecDealet()
}

func (d *pgsqlProxy) OnConfigure() {
	// TODO
}
func (d *pgsqlProxy) OnPrepare() {
	// TODO
}

func (d *pgsqlProxy) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}
