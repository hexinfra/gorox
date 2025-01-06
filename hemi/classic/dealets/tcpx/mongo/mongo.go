// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// MongoDB proxy dealet passes connections to MongoDB backends.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/drivers/mongo"
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
	d.router.DecSub() // dealet
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

func init() {
	RegisterBackend("mongoBackend", func(name string, stage *Stage) Backend {
		b := new(MongoBackend)
		b.onCreate(name, stage)
		return b
	})
}

// MongoBackend is a group of mongo nodes.
type MongoBackend struct {
	// Parent
	Backend_[*mongoNode]
	// States
}

func (b *MongoBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *MongoBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *MongoBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *MongoBackend) CreateNode(name string) Node {
	node := new(mongoNode)
	node.onCreate(name, b.Stage(), b)
	b.AddNode(node)
	return node
}

// mongoNode is a node in MongoBackend.
type mongoNode struct {
	// Parent
	Node_[*MongoBackend]
}

func (n *mongoNode) onCreate(name string, stage *Stage, backend *MongoBackend) {
	n.Node_.OnCreate(name, stage, backend)
}

func (n *mongoNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *mongoNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *mongoNode) Maintain() { // runner
}

// mongoConn is a connection to mongoNode.
type mongoConn struct {
}
