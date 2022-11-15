// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
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
		f.init(name, stage, mesher)
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

func (f *redisViewer) init(name string, stage *Stage, mesher *TCPSMesher) {
	f.InitComp(name)
	f.stage = stage
	f.mesher = mesher
}

func (f *redisViewer) OnConfigure() {
}
func (f *redisViewer) OnPrepare() {
}
func (f *redisViewer) OnShutdown() {
	f.mesher.SubDone()
}

func (f *redisViewer) OnInput(conn *TCPSConn, kind int8) {
}
func (f *redisViewer) OnOutput(conn *TCPSConn, kind int8) {
}
