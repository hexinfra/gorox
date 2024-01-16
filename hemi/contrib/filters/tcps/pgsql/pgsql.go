// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// PgSQL proxy filter passes conns to backend PgSQL servers.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("pgsqlProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(pgsqlProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *pgsqlProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *pgsqlProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *pgsqlProxy) OnConfigure() {
	// TODO
}
func (f *pgsqlProxy) OnPrepare() {
	// TODO
}

func (f *pgsqlProxy) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (f *pgsqlProxy) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (f *pgsqlProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}
