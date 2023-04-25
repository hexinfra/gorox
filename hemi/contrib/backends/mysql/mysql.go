// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL backend implementation.

package mysql

import (
	. "github.com/hexinfra/gorox/hemi/internal"

	_ "github.com/hexinfra/gorox/hemi/common/drivers/rdbms/mysql"
)

// MySQLBackend is a group of mysql nodes.
type MySQLBackend struct {
	// Mixins
	Backend_[*mysqlNode]
}

// mysqlNode is a node in MySQLBackend.
type mysqlNode struct {
	// Mixins
	Node_
	// Assocs
	backend *MySQLBackend
}

func (n *mysqlNode) Maintain() { // goroutine
}

// mysqlConn is a connection to mysqlNode.
type mysqlConn struct {
}
