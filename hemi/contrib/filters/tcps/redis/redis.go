// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis proxy filter passes conns to backend Redis servers.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("redisProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(redisProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// redisProxy
type redisProxy struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (f *redisProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *redisProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *redisProxy) OnConfigure() {
	// TODO
}
func (f *redisProxy) OnPrepare() {
	// TODO
}

func (f *redisProxy) Deal(conn *TCPSConn) (next bool) { // reverse only
	// TODO
	return false
}
