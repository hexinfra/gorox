// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Mysql proxy dealet passes connections to Mysql backends.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/builtin/backends/mysql"
)

func init() {
	RegisterTCPXDealet("mysqlProxy", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(mysqlProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// mysqlProxy
type mysqlProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	router  *TCPXRouter
	backend *MysqlBackend // the backend to pass to
	// States
}

func (d *mysqlProxy) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.TCPXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *mysqlProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *mysqlProxy) OnConfigure() {
	// TODO
}
func (d *mysqlProxy) OnPrepare() {
	// TODO
}

func (d *mysqlProxy) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}
