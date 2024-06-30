// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pgsql backend implementation.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/drivers/rdbms/pgsql"
)

// PgsqlBackend is a group of pgsql nodes.
type PgsqlBackend struct {
	// Parent
	Backend_[*pgsqlNode]
}

// pgsqlNode is a node in PgsqlBackend.
type pgsqlNode struct {
	// Parent
	Node_
	// Assocs
	backend *PgsqlBackend
}

func (n *pgsqlNode) onCreate(name string, backend *PgsqlBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
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
