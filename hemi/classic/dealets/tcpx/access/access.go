// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Access dealet allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPXDealet("accessDealet", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(accessDealet)
		d.onCreate(name, stage, router)
		return d
	})
}

// accessDealet
type accessDealet struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage  *Stage // current stage
	router *TCPXRouter
	// States
}

func (d *accessDealet) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *accessDealet) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *accessDealet) OnConfigure() {
	// TODO
}
func (d *accessDealet) OnPrepare() {
	// TODO
}

func (d *accessDealet) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}
