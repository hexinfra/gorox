// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// MongoDB backend implementation.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/drivers/mongo"
)

// MongoBackend is a group of mongo nodes.
type MongoBackend struct {
	// Parent
	Backend_[*mongoNode]
}

// mongoNode is a node in MongoBackend.
type mongoNode struct {
	// Parent
	Node_
	// Assocs
	backend *MongoBackend
}

func (n *mongoNode) onCreate(name string, backend *MongoBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
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
