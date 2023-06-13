// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB proxy filter passes conns to backend MongoDB servers.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("mongoProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(mongoProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// mongoProxy
type mongoProxy struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *mongoProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *mongoProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *mongoProxy) OnConfigure() {
	// TODO
}
func (f *mongoProxy) OnPrepare() {
	// TODO
}

func (f *mongoProxy) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (f *mongoProxy) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (f *mongoProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}
