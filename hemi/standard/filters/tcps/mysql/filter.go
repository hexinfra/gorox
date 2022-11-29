// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL viewer filters.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("mysqlViewer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(mysqlViewer)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// mysqlViewer
type mysqlViewer struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *mysqlViewer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.CompInit(name)
	f.stage = stage
	f.mesher = mesher
}

func (f *mysqlViewer) OnConfigure() {
}
func (f *mysqlViewer) OnPrepare() {
}

func (f *mysqlViewer) OnShutdown() {
	f.mesher.SubDone()
}

func (f *mysqlViewer) OnInput(conn *TCPSConn, kind int8) {
}
func (f *mysqlViewer) OnOutput(conn *TCPSConn, kind int8) {
}
