// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo filters echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("echoFilter", func(name string, stage *Stage, router *TCPSRouter) TCPSFilter {
		f := new(echoFilter)
		f.init(name, stage, router)
		return f
	})
}

// echoFilter
type echoFilter struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (f *echoFilter) init(name string, stage *Stage, router *TCPSRouter) {
	f.SetName(name)
	f.stage = stage
	f.router = router
}

func (f *echoFilter) OnConfigure() {
}
func (f *echoFilter) OnPrepare() {
}
func (f *echoFilter) OnShutdown() {
}

func (f *echoFilter) Process(conn *TCPSConn) (next bool) {
	p := make([]byte, 4096)
	for {
		n, err := conn.Read(p) // filters
		if err != nil {
			break
		}
		conn.Write(p[:n]) // editors
	}
	return
}
