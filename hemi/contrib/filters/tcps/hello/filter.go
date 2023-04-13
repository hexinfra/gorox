// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello filters print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("helloFilter", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(helloFilter)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// helloFilter
type helloFilter struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *helloFilter) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *helloFilter) OnShutdown() {
	f.mesher.SubDone()
}

func (f *helloFilter) OnConfigure() {
}
func (f *helloFilter) OnPrepare() {
}

func (f *helloFilter) Deal(conn *TCPSConn) (next bool) {
	conn.Write([]byte("hello, world"))
	conn.Close()
	return false
}
