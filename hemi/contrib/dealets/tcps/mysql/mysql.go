// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL proxy dealet passes conns to backend MySQL servers.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPSDealet("mysqlProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(mysqlProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// mysqlProxy
type mysqlProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (d *mysqlProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *mysqlProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *mysqlProxy) OnConfigure() {
	// TODO
}
func (d *mysqlProxy) OnPrepare() {
	// TODO
}

func (d *mysqlProxy) Deal(conn *TCPSConn) (dealt bool) {
	return true
}
