// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL viewer editors.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSEditor("mysqlViewer", func(name string, stage *Stage, router *TCPSRouter) TCPSEditor {
		e := new(mysqlViewer)
		e.onCreate(name, stage, router)
		return e
	})
}

// mysqlViewer
type mysqlViewer struct {
	// Mixins
	TCPSEditor_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (e *mysqlViewer) onCreate(name string, stage *Stage, router *TCPSRouter) {
	e.MakeComp(name)
	e.stage = stage
	e.router = router
}
func (e *mysqlViewer) OnShutdown() {
	e.router.SubDone()
}

func (e *mysqlViewer) OnConfigure() {
	// TODO
}
func (e *mysqlViewer) OnPrepare() {
	// TODO
}

func (e *mysqlViewer) OnInput(conn *TCPSConn, kind int8) {
	// TODO
}
func (e *mysqlViewer) OnOutput(conn *TCPSConn, kind int8) {
	// TODO
}
