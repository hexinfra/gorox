// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis viewer editors.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSEditor("redisViewer", func(name string, stage *Stage, router *TCPSRouter) TCPSEditor {
		e := new(redisViewer)
		e.onCreate(name, stage, router)
		return e
	})
}

// redisViewer
type redisViewer struct {
	// Mixins
	TCPSEditor_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (e *redisViewer) onCreate(name string, stage *Stage, router *TCPSRouter) {
	e.MakeComp(name)
	e.stage = stage
	e.router = router
}
func (e *redisViewer) OnShutdown() {
	e.router.SubDone()
}

func (e *redisViewer) OnConfigure() {
	// TODO
}
func (e *redisViewer) OnPrepare() {
	// TODO
}

func (e *redisViewer) OnInput(conn *TCPSConn, kind int8) {
	// TODO
}
func (e *redisViewer) OnOutput(conn *TCPSConn, kind int8) {
	// TODO
}
