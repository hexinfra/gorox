// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Hello dealets print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterTCPSDealet("helloDealet", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(helloDealet)
		d.onCreate(name, stage, router)
		return d
	})
}

// helloDealet
type helloDealet struct {
	// Parent
	TCPSDealet_
	// Assocs
	stage  *Stage // current stage
	router *TCPSRouter
	// States
}

func (d *helloDealet) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *helloDealet) OnShutdown() {
	d.router.DecSub()
}

func (d *helloDealet) OnConfigure() {
	// TODO
}
func (d *helloDealet) OnPrepare() {
	// TODO
}

func (d *helloDealet) Deal(conn *TCPSConn) (dealt bool) {
	conn.Send(helloBytes)
	return true
}

var (
	helloBytes = []byte("hello, world!")
)
