// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Hello socklets print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterSocklet("helloSocklet", func(compName string, stage *Stage, webapp *Webapp) Socklet {
		s := new(helloSocklet)
		s.onCreate(compName, stage, webapp)
		return s
	})
}

// helloSocklet
type helloSocklet struct {
	// Parent
	Socklet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
}

func (s *helloSocklet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	s.MakeComp(compName)
	s.stage = stage
	s.webapp = webapp
}
func (s *helloSocklet) OnShutdown() {
	s.webapp.DecSub() // socklet
}

func (s *helloSocklet) OnConfigure() {
	// TODO
}
func (s *helloSocklet) OnPrepare() {
	// TODO
}

func (s *helloSocklet) Serve(req Request, sock Socket) {
	sock.Write([]byte("hello, webSocket!"))
	sock.Close()
}
