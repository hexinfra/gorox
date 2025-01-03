// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis backend implementation.

package redis

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/drivers/redis"
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
	// States
	idleTimeout time.Duration // conn idle timeout
	lifetime    time.Duration // conn's lifetime
}

func (b *RedisBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *RedisBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// idleTimeout
	b.ConfigureDuration("idleTimeout", &b.idleTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".idleTimeout has an invalid value")
	}, 3*time.Second)

	// lifetime
	b.ConfigureDuration("lifetime", &b.lifetime, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".lifetime has an invalid value")
	}, 1*time.Minute)

	// sub components
	b.ConfigureNodes()
}
func (b *RedisBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *RedisBackend) CreateNode(name string) Node {
	node := new(redisNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *RedisBackend) Dial() (*RedisConn, error) {
	// TODO
	return nil, nil
}

func (b *RedisBackend) FetchConn() (*RedisConn, error) {
	// TODO
	return nil, nil
}
func (b *RedisBackend) StoreConn(redisConn *RedisConn) {
	// TODO
}

// redisNode is a node in RedisBackend.
type redisNode struct {
	// Parent
	Node_
	// Assocs
	backend *RedisBackend
}

func (n *redisNode) onCreate(name string, backend *RedisBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *redisNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *redisNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *redisNode) Maintain() { // runner
	// TODO
}

func (n *redisNode) dial() (*RedisConn, error) {
	// TODO
	return nil, nil
}

func (n *redisNode) fetchConn() (*RedisConn, error) {
	// TODO
	return nil, nil
}
func (n *redisNode) storeConn(redisConn *RedisConn) {
	// TODO
}

func (n *redisNode) closeConn(redisConn *RedisConn) {
	// TODO
}

// RedisConn is a connection to redisNode.
type RedisConn struct {
	// Conn states (non-zeros)
	id         int64 // the conn id
	node       *redisNode
	expireTime time.Time // when the conn is considered expired
	netConn    net.Conn  // *net.TCPConn, *net.UnixConn
	rawConn    syscall.RawConn
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastWrite time.Time    // deadline of last write operation
	lastRead  time.Time    // deadline of last read operation
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

func (c *RedisConn) onGet(id int64, node *redisNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.node = node
	c.expireTime = time.Now().Add(node.backend.idleTimeout)
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *RedisConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.expireTime = time.Time{}
	c.node = nil
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *RedisConn) Close() error {
	netConn := c.netConn
	putRedisConn(c)
	return netConn.Close()
}
