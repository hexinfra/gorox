// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// MongoDB proxy dealet passes connections to MongoDB backends.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/backends/mongo"
)

func init() {
	RegisterTCPXDealet("mongoProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(mongoProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// mongoProxy
type mongoProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *TCPXRouter
	backend *MongoBackend // the backend to pass to
	// States
}

func (d *mongoProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *mongoProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *mongoProxy) OnConfigure() {
	// TODO
}
func (d *mongoProxy) OnPrepare() {
	// TODO
}

func (d *mongoProxy) Deal(conn *TCPXConn) (dealt bool) {
	return true
}
