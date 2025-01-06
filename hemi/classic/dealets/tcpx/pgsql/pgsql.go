// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pgsql proxy dealet passes connections to Pgsql backends.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/drivers/rdbms/pgsql"
)

func init() {
	RegisterTCPXDealet("pgsqlProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(pgsqlProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *TCPXRouter
	backend *PgsqlBackend // the backend to pass to
	// States
}

func (d *pgsqlProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *pgsqlProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *pgsqlProxy) OnConfigure() {
	// TODO
}
func (d *pgsqlProxy) OnPrepare() {
	// TODO
}

func (d *pgsqlProxy) Deal(conn *TCPXConn) (dealt bool) {
	return true
}

func init() {
	RegisterBackend("pgsqlBackend", func(name string, stage *Stage) Backend {
		b := new(PgsqlBackend)
		b.onCreate(name, stage)
		return b
	})
}

// PgsqlBackend is a group of pgsql nodes.
type PgsqlBackend struct {
	// Parent
	Backend_[*pgsqlNode]
	// States
}

func (b *PgsqlBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *PgsqlBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *PgsqlBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *PgsqlBackend) CreateNode(name string) Node {
	node := new(pgsqlNode)
	node.onCreate(name, b.Stage(), b)
	b.AddNode(node)
	return node
}

// pgsqlNode is a node in PgsqlBackend.
type pgsqlNode struct {
	// Parent
	Node_[*PgsqlBackend]
}

func (n *pgsqlNode) onCreate(name string, stage *Stage, backend *PgsqlBackend) {
	n.Node_.OnCreate(name, stage, backend)
}

func (n *pgsqlNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *pgsqlNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *pgsqlNode) Maintain() { // runner
}

// pgsqlConn is a connection to pgsqlNode.
type pgsqlConn struct {
}
