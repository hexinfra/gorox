// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// MySQL proxy dealet passes connections to MySQL backends.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/backends/mysql"
)

func init() {
	RegisterTCPXDealet("mysqlProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(mysqlProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// mysqlProxy
type mysqlProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *TCPXRouter
	backend *MySQLBackend // the backend to pass to
	// States
}

func (d *mysqlProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
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

func (d *mysqlProxy) Deal(conn *TCPXConn) (dealt bool) {
	return true
}
