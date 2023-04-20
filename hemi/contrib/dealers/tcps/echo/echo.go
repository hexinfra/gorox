// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo dealers echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("echoDealer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		f := new(echoDealer)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// echoDealer
type echoDealer struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *echoDealer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *echoDealer) OnShutdown() {
	f.mesher.SubDone()
}

func (f *echoDealer) OnConfigure() {
	// TODO
}
func (f *echoDealer) OnPrepare() {
	// TODO
}

func (f *echoDealer) Process(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
