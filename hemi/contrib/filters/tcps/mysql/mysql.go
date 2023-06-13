// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL proxy filter passes conns to backend MySQL servers.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("mysqlProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(mysqlProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// mysqlProxy
type mysqlProxy struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *mysqlProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *mysqlProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *mysqlProxy) OnConfigure() {
	// TODO
}
func (f *mysqlProxy) OnPrepare() {
	// TODO
}

func (f *mysqlProxy) OnDial() {
}
func (f *mysqlProxy) OnInput() (next bool) {
	return
}
func (f *mysqlProxy) OnOutput() (next bool) {
	return
}
func (f *mysqlProxy) OnClose() {
}
