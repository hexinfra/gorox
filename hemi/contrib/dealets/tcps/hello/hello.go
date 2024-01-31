// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello dealets print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealet("helloDealet", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(helloDealet)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// helloDealet
type helloDealet struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *helloDealet) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *helloDealet) OnShutdown() {
	d.mesher.SubDone()
}

func (d *helloDealet) OnConfigure() {
	// TODO
}
func (d *helloDealet) OnPrepare() {
	// TODO
}

func (d *helloDealet) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (d *helloDealet) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (d *helloDealet) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}

var (
	helloBytes = []byte("hello, world!")
)
