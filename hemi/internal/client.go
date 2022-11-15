// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General client implementation.

package internal

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// client is the interface for all outgates and backends.
type client interface {
	Stage() *Stage
	TLSMode() bool
	WriteTimeout() time.Duration
	ReadTimeout() time.Duration
	AliveTimeout() time.Duration
}

// client_ is a mixin shared by outgates and backends.
type client_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	tlsMode      bool          // use TLS?
	tlsConfig    *tls.Config   // TLS config if TLS is enabled
	dialTimeout  time.Duration // dial remote timeout
	writeTimeout time.Duration // write operation timeout
	readTimeout  time.Duration // read operation timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
}

func (c *client_) init(name string, stage *Stage) {
	c.CompInit(name)
	c.stage = stage
}

func (c *client_) onConfigure() {
	// tlsMode
	c.ConfigureBool("tlsMode", &c.tlsMode, false)
	if c.tlsMode {
		c.tlsConfig = new(tls.Config)
	}
	// dialTimeout
	c.ConfigureDuration("dialTimeout", &c.dialTimeout, func(value time.Duration) bool { return value > time.Second }, 10*time.Second)
	// writeTimeout
	c.ConfigureDuration("writeTimeout", &c.writeTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
	// readTimeout
	c.ConfigureDuration("readTimeout", &c.readTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
	// aliveTimeout
	c.ConfigureDuration("aliveTimeout", &c.aliveTimeout, func(value time.Duration) bool { return value > 0 }, 4*time.Second)
}
func (c *client_) onPrepare() {
}
func (c *client_) onShutdown() {
	c.SetShut()
}

func (c *client_) Stage() *Stage { return c.stage }
func (c *client_) TLSMode() bool { return c.tlsMode }

func (c *client_) WriteTimeout() time.Duration { return c.writeTimeout }
func (c *client_) ReadTimeout() time.Duration  { return c.readTimeout }
func (c *client_) AliveTimeout() time.Duration { return c.aliveTimeout }

func (c *client_) nextConnID() int64 {
	return c.connID.Add(1)
}

// backend is a group of nodes.
type backend interface {
	Component
	maintain() // goroutine
}

// node is a member of backend.
type node interface {
	maintain() // goroutine
}

// node_ is a mixin for backend nodes.
type node_ struct {
	// States
	id        int32       // the node id
	address   string      // hostname:port
	weight    int32       // 1, 22, 333, ...
	keepConns int32       // max conns to keep alive
	down      atomic.Bool // TODO: false-sharing
	freeList  struct {    // free list of conn in this node
		sync.Mutex
		size int32
		head conn
		tail conn
	}
}

func (n *node_) init(id int32) {
	n.id = id
}

func (n *node_) markDown()    { n.down.Store(true) }
func (n *node_) markUp()      { n.down.Store(false) }
func (n *node_) isDown() bool { return n.down.Load() }

func (n *node_) takeConn() conn {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()
	if list.size == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.getNext()
	conn.setNext(nil)
	list.size--
	return conn
}
func (n *node_) pushConn(conn conn) {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()
	if list.size == 0 {
		list.head = conn
		list.tail = conn
	} else {
		list.tail.setNext(conn)
		list.tail = conn
	}
	list.size++
}

var errNodeDown = errors.New("node is down")

// conn is the client conns.
type conn interface {
	isAlive() bool
	getNext() conn
	setNext(next conn)
	closeConn()
}

// conn_ is a mixin for client conns.
type conn_ struct {
	// Conn states (non-zeros)
	next   conn      // the link
	id     int64     // the conn id
	client client    // associated client
	expire time.Time // when the conn is considered expired
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

func (c *conn_) onGet(id int64, client client) {
	c.id = id
	c.client = client
	c.expire = time.Now().Add(client.AliveTimeout())
}
func (c *conn_) onPut() {
	c.client = nil
	c.expire = time.Time{}
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *conn_) isAlive() bool {
	return time.Now().Before(c.expire)
}

func (c *conn_) getNext() conn     { return c.next }
func (c *conn_) setNext(next conn) { c.next = next }

// connection-oriented backend, supports TCPS and Unix.
type PBackend interface {
	backend
	Dial() (PConn, error)
	FetchConn() (PConn, error)
	StoreConn(conn PConn)
}

// connection-oriented conn, supports TCPS and Unix.
type PConn interface {
	conn
	Write(p []byte) (n int, err error)
	Writev(vector *net.Buffers) (int64, error)
	Read(p []byte) (n int, err error)
	ReadFull(p []byte) (n int, err error)
	Close() error
}

// pConn_ is a trait for TConn and XConn.
type pConn_ struct {
	// Mixins
	conn_
	// Conn states (non-zeros)
	maxStreams int32 // how many streams are allowed on this conn?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	writeBroken atomic.Bool  // write-side broken?
	readBroken  atomic.Bool  // read-side broken?
}

func (c *pConn_) onGet(id int64, client client, maxStreams int32) {
	c.conn_.onGet(id, client)
	c.maxStreams = maxStreams
}
func (c *pConn_) onPut() {
	c.conn_.onPut()
	c.usedStreams.Store(0)
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
}

func (c *pConn_) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

func (c *pConn_) isBroken() bool {
	return c.writeBroken.Load() || c.readBroken.Load()
}
func (c *pConn_) markWriteBroken() { c.writeBroken.Store(true) }
func (c *pConn_) markReadBroken()  { c.readBroken.Store(true) }
