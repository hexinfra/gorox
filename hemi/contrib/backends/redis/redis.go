// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis backend implementation.

package redis

import (
	"sync"

	. "github.com/hexinfra/gorox/hemi/internal"

	_ "github.com/hexinfra/gorox/hemi/common/drivers/redis"
)

func init() {
	RegisterBackend("redisBackend", func(name string, stage *Stage) Backend {
		b := new(RedisBackend)
		b.onCreate(name, stage)
		return b
	})
}

// RedisBackend is a group of redis nodes.
type RedisBackend struct {
	// Mixins
	Backend_[*redisNode]
}

func (b *RedisBackend) onCreate(name string, stage *Stage) {
	//b.Backend_.OnCreate(name, stage, b)
}

func (b *RedisBackend) OnConfigure() {
}
func (b *RedisBackend) OnPrepare() {
}

// redisNode is a node in RedisBackend.
type redisNode struct {
	// Mixins
	Node_
	// Assocs
	backend *RedisBackend
}

func (n *redisNode) init(id int32, backend *RedisBackend) {
	//n.Node.init(id)
	n.backend = backend
}

func (n *redisNode) Maintain() { // runner
}

func (n *redisNode) dial() (*redisConn, error) {
	return nil, nil
}

func (n *redisNode) fetchConn() (*redisConn, error) {
	return nil, nil
}
func (n *redisNode) storeConn(rConn *redisConn) {
}

func (n *redisNode) closeConn(rConn *redisConn) {
}

var poolRedisConn sync.Pool

func getRedisConn() {
}
func putRedisConn() {
}

// redisConn is a connection to redisNode.
type redisConn struct {
	// Mixins
	Conn_
	// Conn states (non-zeros)
	node *redisNode
	// Conn states (zeros)
}

func (c *redisConn) onGet() {
}
func (c *redisConn) onPut() {
}
