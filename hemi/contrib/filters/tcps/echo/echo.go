// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo filters echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"io"
)

func init() {
	RegisterTCPSFilter("echoFilter", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(echoFilter)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// echoFilter
type echoFilter struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *echoFilter) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *echoFilter) OnShutdown() {
	f.mesher.SubDone()
}

func (f *echoFilter) OnConfigure() {
	// TODO
}
func (f *echoFilter) OnPrepare() {
	// TODO
}

func (f *echoFilter) Process(conn *TCPSConn) (next bool) {
	// TODO: deadline?
	io.Copy(conn, conn)
	return false
}
