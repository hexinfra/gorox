// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
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
	// TODO
}
func (f *helloFilter) OnPrepare() {
	// TODO
}

func (f *helloFilter) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (f *helloFilter) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (f *helloFilter) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}

var (
	helloBytes = []byte("hello, world!")
)
