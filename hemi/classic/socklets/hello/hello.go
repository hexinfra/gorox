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
	// States
}

func (s *helloSocklet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	s.Socklet_.OnCreate(compName, stage, webapp)
}
func (s *helloSocklet) OnShutdown() {
	s.Webapp().DecSub() // socklet
}

func (s *helloSocklet) OnConfigure() {
	// TODO
}
func (s *helloSocklet) OnPrepare() {
	// TODO
}

func (s *helloSocklet) Serve(req ServerRequest, sock ServerSocket) {
	sock.Write([]byte("hello, webSocket!"))
	sock.Close()
}
