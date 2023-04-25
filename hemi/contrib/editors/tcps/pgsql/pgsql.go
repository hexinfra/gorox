// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// PgSQL viewer editors.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSEditor("pgsqlViewer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSEditor {
		e := new(pgsqlViewer)
		e.onCreate(name, stage, mesher)
		return e
	})
}

// pgsqlViewer
type pgsqlViewer struct {
	// Mixins
	TCPSEditor_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (e *pgsqlViewer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	e.MakeComp(name)
	e.stage = stage
	e.mesher = mesher
}
func (e *pgsqlViewer) OnShutdown() {
	e.mesher.SubDone()
}

func (e *pgsqlViewer) OnConfigure() {
	// TODO
}
func (e *pgsqlViewer) OnPrepare() {
	// TODO
}

func (e *pgsqlViewer) OnInput(conn *TCPSConn, kind int8) {
	// TODO
}
func (e *pgsqlViewer) OnOutput(conn *TCPSConn, kind int8) {
	// TODO
}
