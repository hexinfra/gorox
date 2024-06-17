// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Echo dealets echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPXDealet("echoDealet", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(echoDealet)
		d.onCreate(name, stage, router)
		return d
	})
}

// echoDealet
type echoDealet struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage  *Stage // current stage
	router *TCPXRouter
	// States
}

func (d *echoDealet) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *echoDealet) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *echoDealet) OnConfigure() {
	// TODO
}
func (d *echoDealet) OnPrepare() {
	// TODO
}

func (d *echoDealet) Deal(conn *TCPXConn) (dealt bool) {
	return true
}
