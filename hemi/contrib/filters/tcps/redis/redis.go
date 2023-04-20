// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis viewer filters.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("redisViewer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(redisViewer)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// redisViewer
type redisViewer struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *redisViewer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *redisViewer) OnShutdown() {
	f.mesher.SubDone()
}

func (f *redisViewer) OnConfigure() {
	// TODO
}
func (f *redisViewer) OnPrepare() {
	// TODO
}

func (f *redisViewer) OnInput(conn *TCPSConn, kind int8) {
	// TODO
}
func (f *redisViewer) OnOutput(conn *TCPSConn, kind int8) {
	// TODO
}
