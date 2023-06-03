// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB viewer editors.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSEditor("mongoViewer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSEditor {
		e := new(mongoViewer)
		e.onCreate(name, stage, mesher)
		return e
	})
}

// mongoViewer
type mongoViewer struct {
	// Mixins
	TCPSEditor_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (e *mongoViewer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	e.MakeComp(name)
	e.stage = stage
	e.mesher = mesher
}
func (e *mongoViewer) OnShutdown() {
	e.mesher.SubDone()
}

func (e *mongoViewer) OnConfigure() {
	// TODO
}
func (e *mongoViewer) OnPrepare() {
	// TODO
}

func (e *mongoViewer) OnInput(conn *TCPSConn, kind int8) {
	// TODO
}
func (e *mongoViewer) OnOutput(conn *TCPSConn, kind int8) {
	// TODO
}
