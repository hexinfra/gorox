// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Mysql backend implementation.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/connectors/rdbms/mysql"
)

func init() {
	RegisterBackend("mysqlBackend", func(compName string, stage *Stage) Backend {
		b := new(MysqlBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// MysqlBackend is a group of mysql nodes.
type MysqlBackend struct {
	// Parent
	Backend_[*mysqlNode]
	// States
}

func (b *MysqlBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *MysqlBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	b.ConfigureNodes()
}
func (b *MysqlBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	b.PrepareNodes()
}

func (b *MysqlBackend) CreateNode(compName string) Node {
	node := new(mysqlNode)
	node.onCreate(compName, b.Stage(), b)
	b.AddNode(node)
	return node
}

// mysqlNode is a node in MysqlBackend.
type mysqlNode struct {
	// Parent
	Node_[*MysqlBackend]
}

func (n *mysqlNode) onCreate(compName string, stage *Stage, backend *MysqlBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *mysqlNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *mysqlNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *mysqlNode) Maintain() { // runner
}

// mysqlConn is a connection to mysqlNode.
type mysqlConn struct {
}
