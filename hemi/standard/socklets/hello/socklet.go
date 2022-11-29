// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello socklets print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterSocklet("helloSocklet", func(name string, stage *Stage, app *App) Socklet {
		s := new(helloSocklet)
		s.onCreate(name, stage, app)
		return s
	})
}

// helloSocklet
type helloSocklet struct {
	// Mixins
	Socklet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (s *helloSocklet) onCreate(name string, stage *Stage, app *App) {
	s.CompInit(name)
	s.stage = stage
	s.app = app
}

func (s *helloSocklet) OnConfigure() {
}
func (s *helloSocklet) OnPrepare() {
}

func (s *helloSocklet) OnShutdown() {
	s.app.SubDone()
}

func (s *helloSocklet) Serve(req Request, sock Socket) {
	sock.Write([]byte("hello, websocket!"))
	sock.Close()
}
