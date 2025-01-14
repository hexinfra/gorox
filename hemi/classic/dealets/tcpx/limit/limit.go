// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Limit dealet limit clients' visiting frequency.

package limit

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPXDealet("limitDealet", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(limitDealet)
		d.onCreate(compName, stage, router)
		return d
	})
}

// limitDealet
type limitDealet struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage  *Stage // current stage
	router *TCPXRouter
	// States
}

func (d *limitDealet) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(compName)
	d.stage = stage
	d.router = router
}
func (d *limitDealet) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *limitDealet) OnConfigure() {
	// TODO
}
func (d *limitDealet) OnPrepare() {
	// TODO
}

func (d *limitDealet) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}
