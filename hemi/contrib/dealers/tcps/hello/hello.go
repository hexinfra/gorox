// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello dealers print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("helloDealer", func(name string, stage *Stage, router *TCPSRouter) TCPSDealer {
		d := new(helloDealer)
		d.onCreate(name, stage, router)
		return d
	})
}

// helloDealer
type helloDealer struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (d *helloDealer) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *helloDealer) OnShutdown() {
	d.router.SubDone()
}

func (d *helloDealer) OnConfigure() {
	// TODO
}
func (d *helloDealer) OnPrepare() {
	// TODO
}

func (d *helloDealer) Deal(conn *TCPSConn) (next bool) {
	conn.Send(helloBytes)
	return false
}

var (
	helloBytes = []byte("hello, world!")
)
