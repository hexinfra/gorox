// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access filter allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("accessFilter", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(accessFilter)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// accessFilter
type accessFilter struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *accessFilter) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *accessFilter) OnShutdown() {
	f.mesher.SubDone()
}

func (f *accessFilter) OnConfigure() {
	// TODO
}
func (f *accessFilter) OnPrepare() {
	// TODO
}

func (f *accessFilter) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
