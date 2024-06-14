// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// MySQL backend implementation.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/common/drivers/rdbms/mysql"
)

// MySQLBackend is a group of mysql nodes.
type MySQLBackend struct {
	// Parent
	Backend_[*mysqlNode]
}

// mysqlNode is a node in MySQLBackend.
type mysqlNode struct {
	// Parent
	Node_[*MySQLBackend]
	// Assocs
}

func (n *mysqlNode) Maintain() { // runner
}

// mysqlConn is a connection to mysqlNode.
type mysqlConn struct {
}
