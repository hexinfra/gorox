// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB proxy dealet passes conns to backend MongoDB servers.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPSDealet("mongoProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(mongoProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// mongoProxy
type mongoProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (d *mongoProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *mongoProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *mongoProxy) OnConfigure() {
	// TODO
}
func (d *mongoProxy) OnPrepare() {
	// TODO
}

func (d *mongoProxy) Deal(conn *TCPSConn) (dealt bool) {
	return true
}
