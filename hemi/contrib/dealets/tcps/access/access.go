// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Access dealet allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPSDealet("accessDealet", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(accessDealet)
		d.onCreate(name, stage, router)
		return d
	})
}

// accessDealet
type accessDealet struct {
	// Parent
	TCPSDealet_
	// Assocs
	stage  *Stage // current stage
	router *TCPSRouter
	// States
}

func (d *accessDealet) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *accessDealet) OnShutdown() {
	d.router.DecSub()
}

func (d *accessDealet) OnConfigure() {
	// TODO
}
func (d *accessDealet) OnPrepare() {
	// TODO
}

func (d *accessDealet) Deal(conn *TCPSConn) (dealt bool) {
	return true
}
