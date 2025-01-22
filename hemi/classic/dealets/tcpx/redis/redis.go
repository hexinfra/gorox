// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis proxy dealet passes connections to Redis backends.

package redis

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/connectors/redis"
)

func init() {
	RegisterTCPXDealet("redisProxy", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(redisProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// redisProxy
type redisProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	router  *TCPXRouter
	backend *RedisBackend // the backend to pass to
	// States
}

func (d *redisProxy) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.TCPXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *redisProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *redisProxy) OnConfigure() {
	// TODO
}
func (d *redisProxy) OnPrepare() {
	// TODO
}

func (d *redisProxy) DealWith(conn *TCPXConn) (dealt bool) {
	return true
}

func init() {
	RegisterBackend("redisBackend", func(compName string, stage *Stage) Backend {
		b := new(RedisBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// RedisBackend is a group of redis nodes.
type RedisBackend struct {
	// Parent
	Backend_[*redisNode]
	// States
}

func (b *RedisBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *RedisBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	b.ConfigureNodes()
}
func (b *RedisBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	b.PrepareNodes()
}

func (b *RedisBackend) CreateNode(compName string) Node {
	node := new(redisNode)
	node.onCreate(compName, b.Stage(), b)
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
	Node_[*RedisBackend]
	// States
	idleTimeout time.Duration // conn idle timeout
	maxLifetime time.Duration // conn's max lifetime
}

func (n *redisNode) onCreate(compName string, stage *Stage, backend *RedisBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *redisNode) OnConfigure() {
	n.Node_.OnConfigure()

	// .idleTimeout
	n.ConfigureDuration("idleTimeout", &n.idleTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".idleTimeout has an invalid value")
	}, 2*time.Second)

	// .maxLifetime
	n.ConfigureDuration("maxLifetime", &n.maxLifetime, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxLifetime has an invalid value")
	}, 1*time.Minute)
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
	id         int64      // the conn id
	node       *redisNode // the node to which the connection belongs
	expireTime time.Time  // when the conn is considered expired
	netConn    net.Conn   // *net.TCPConn, *net.UnixConn
	rawConn    syscall.RawConn
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastWrite time.Time    // deadline of last write operation
	lastRead  time.Time    // deadline of last read operation
}

var poolRedisConn sync.Pool

func getRedisConn(id int64, node *redisNode, netConn net.Conn, rawConn syscall.RawConn) *RedisConn {
	var conn *RedisConn
	if x := poolRedisConn.Get(); x == nil {
		conn = new(RedisConn)
	} else {
		conn = x.(*RedisConn)
	}
	conn.onGet(id, node, netConn, rawConn)
	return conn
}
func putRedisConn(conn *RedisConn) {
	conn.onPut()
	poolRedisConn.Put(conn)
}

func (c *RedisConn) onGet(id int64, node *redisNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.node = node
	c.expireTime = time.Now().Add(node.idleTimeout)
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
