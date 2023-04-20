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
	RegisterTCPSDealer("helloDealer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		f := new(helloDealer)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// helloDealer
type helloDealer struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *helloDealer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *helloDealer) OnShutdown() {
	f.mesher.SubDone()
}

func (f *helloDealer) OnConfigure() {
	// TODO
}
func (f *helloDealer) OnPrepare() {
	// TODO
}

func (f *helloDealer) Process(conn *TCPSConn) (next bool) {
	conn.Send(helloBytes)
	return false
}

var (
	helloBytes = []byte("hello, world!")
)
