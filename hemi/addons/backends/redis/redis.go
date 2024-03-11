// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis backend implementation.

package redis

import (
	"net"
	"sync"
	"syscall"

	. "github.com/hexinfra/gorox/hemi"

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
	// Parent
	Backend_[*redisNode]
}

func (b *RedisBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *RedisBackend) OnConfigure() {
}
func (b *RedisBackend) OnPrepare() {
}

func (b *RedisBackend) CreateNode(name string) Node {
	node := new(redisNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *RedisBackend) Dial() (*RedisConn, error) {
	return nil, nil
}

func (b *RedisBackend) FetchConn() (*RedisConn, error) {
	return nil, nil
}
func (b *RedisBackend) StoreConn(redisConn *RedisConn) {
}

// redisNode is a node in RedisBackend.
type redisNode struct {
	// Parent
	Node_
	// Assocs
}

func (n *redisNode) onCreate(name string, backend *RedisBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *redisNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *redisNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *redisNode) Maintain() { // runner
}

func (n *redisNode) dial() (*RedisConn, error) {
	return nil, nil
}

func (n *redisNode) fetchConn() (*RedisConn, error) {
	return nil, nil
}
func (n *redisNode) storeConn(redisConn *RedisConn) {
}

func (n *redisNode) closeConn(redisConn *RedisConn) { // TODO: use Node_.closeConn?
}

var poolRedisConn sync.Pool

func getRedisConn(id int64, node *redisNode, netConn net.Conn, rawConn syscall.RawConn) *RedisConn {
	var redisConn *RedisConn
	if x := poolRedisConn.Get(); x == nil {
		redisConn = new(RedisConn)
	} else {
		redisConn = x.(*RedisConn)
	}
	redisConn.onGet(id, node, netConn, rawConn)
	return redisConn
}
func putRedisConn(redisConn *RedisConn) {
	redisConn.onPut()
	poolRedisConn.Put(redisConn)
}

// RedisConn is a connection to redisNode.
type RedisConn struct {
	// Parent
	BackendConn_
	// Conn states (non-zeros)
	netConn net.Conn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *RedisConn) onGet(id int64, node *redisNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *RedisConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.BackendConn_.OnPut()
}

func (c *RedisConn) Close() error {
	netConn := c.netConn
	putRedisConn(c)
	return netConn.Close()
}
