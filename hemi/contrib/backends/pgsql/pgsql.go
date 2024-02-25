// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// PgSQL backend implementation.

package pgsql

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/common/drivers/rdbms/pgsql"
)

// PgSQLBackend is a group of pgsql nodes.
type PgSQLBackend struct {
	// Parent
	Backend_[*pgsqlNode]
}

// pgsqlNode is a node in PgSQLBackend.
type pgsqlNode struct {
	// Parent
	Node_
	// Assocs
	backend *PgSQLBackend
}

func (n *pgsqlNode) Maintain() { // runner
}

// pgsqlConn is a connection to pgsqlNode.
type pgsqlConn struct {
}
