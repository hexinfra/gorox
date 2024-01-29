// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB backend implementation.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi/internal"

	_ "github.com/hexinfra/gorox/hemi/common/drivers/mongo"
)

// MongoBackend is a group of mongo nodes.
type MongoBackend struct {
	// Mixins
	Backend_[*mongoNode]
}

// mongoNode is a node in MongoBackend.
type mongoNode struct {
	// Mixins
	Node_
	// Assocs
	backend *MongoBackend
}

func (n *mongoNode) Maintain() { // runner
}

// mongoConn is a connection to mongoNode.
type mongoConn struct {
}
