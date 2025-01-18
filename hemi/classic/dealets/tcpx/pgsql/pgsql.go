// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pgsql proxy dealet passes connections to Pgsql backends.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/connectors/rdbms/pgsql"
)

func init() {
	RegisterTCPXDealet("pgsqlProxy", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(pgsqlProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// pgsqlProxy
type pgsqlProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	router  *TCPXRouter
	backend *PgsqlBackend // the backend to pass to
	// States
}

func (d *pgsqlProxy) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.TCPXDealet_.OnCreate(compName, stage)
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

func (d *pgsqlProxy) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}

func init() {
	RegisterBackend("pgsqlBackend", func(compName string, stage *Stage) Backend {
		b := new(PgsqlBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// PgsqlBackend is a group of pgsql nodes.
type PgsqlBackend struct {
	// Parent
	Backend_[*pgsqlNode]
	// States
}

func (b *PgsqlBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
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

func (b *PgsqlBackend) CreateNode(compName string) Node {
	node := new(pgsqlNode)
	node.onCreate(compName, b.Stage(), b)
	b.AddNode(node)
	return node
}

// pgsqlNode is a node in PgsqlBackend.
type pgsqlNode struct {
	// Parent
	Node_[*PgsqlBackend]
}

func (n *pgsqlNode) onCreate(compName string, stage *Stage, backend *PgsqlBackend) {
	n.Node_.OnCreate(compName, stage, backend)
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
