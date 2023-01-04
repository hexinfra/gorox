// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL viewer editors.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSEditor("mysqlViewer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSEditor {
		e := new(mysqlViewer)
		e.onCreate(name, stage, mesher)
		return e
	})
}

// mysqlViewer
type mysqlViewer struct {
	// Mixins
	TCPSEditor_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (e *mysqlViewer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	e.CompInit(name)
	e.stage = stage
	e.mesher = mesher
}
func (e *mysqlViewer) OnShutdown() {
	e.mesher.SubDone()
}

func (e *mysqlViewer) OnConfigure() {
}
func (e *mysqlViewer) OnPrepare() {
}

func (e *mysqlViewer) OnInput(conn *TCPSConn, kind int8) {
}
func (e *mysqlViewer) OnOutput(conn *TCPSConn, kind int8) {
}
