// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access dealer allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("accessDealer", func(name string, stage *Stage, router *TCPSRouter) TCPSDealer {
		d := new(accessDealer)
		d.onCreate(name, stage, router)
		return d
	})
}

// accessDealer
type accessDealer struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (d *accessDealer) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *accessDealer) OnShutdown() {
	d.router.SubDone()
}

func (d *accessDealer) OnConfigure() {
	// TODO
}
func (d *accessDealer) OnPrepare() {
	// TODO
}

func (d *accessDealer) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
