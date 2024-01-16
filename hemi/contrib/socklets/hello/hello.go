// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello socklets print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterSocklet("helloSocklet", func(name string, stage *Stage, webapp *Webapp) Socklet {
		s := new(helloSocklet)
		s.onCreate(name, stage, webapp)
		return s
	})
}

// helloSocklet
type helloSocklet struct {
	// Mixins
	Socklet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (s *helloSocklet) onCreate(name string, stage *Stage, webapp *Webapp) {
	s.MakeComp(name)
	s.stage = stage
	s.webapp = webapp
}
func (s *helloSocklet) OnShutdown() {
	s.webapp.SubDone()
}

func (s *helloSocklet) OnConfigure() {
	// TODO
}
func (s *helloSocklet) OnPrepare() {
	// TODO
}

func (s *helloSocklet) Serve(req Request, sock Socket) {
	sock.Write([]byte("hello, websocket!"))
	sock.Close()
}
