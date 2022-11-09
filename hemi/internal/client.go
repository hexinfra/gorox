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
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
	AliveTimeout() time.Duration
}

// client_ is a mixin shared by outgate_ and backend_.
type client_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	tlsMode      bool          // use TLS?
	tlsConfig    *tls.Config   // TLS config if TLS is enabled
	dialTimeout  time.Duration // ...
	readTimeout  time.Duration // ...
	writeTimeout time.Duration // ...
	aliveTimeout time.Duration // ...
	connID       atomic.Int64
}

func (c *client_) init(name string, stage *Stage) {
	c.SetName(name)
	c.stage = stage
}

func (c *client_) onConfigure() {
	// tlsMode
	c.ConfigureBool("tlsMode", &c.tlsMode, false)
	if c.tlsMode {
		c.tlsConfig = new(tls.Config)
	}
	// dialTimeout
	c.ConfigureDuration("dialTimeout", &c.dialTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
	// readTimeout
	c.ConfigureDuration("readTimeout", &c.readTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
	// writeTimeout
	c.ConfigureDuration("writeTimeout", &c.writeTimeout, func(value time.Duration) bool { return value > time.Second }, 30*time.Second)
	// aliveTimeout
	c.ConfigureDuration("aliveTimeout", &c.aliveTimeout, func(value time.Duration) bool { return value > 0 }, 10*time.Second)
}
func (c *client_) onPrepare() {
}
func (c *client_) onShutdown() {
	c.SetShut()
}

func (c *client_) Stage() *Stage               { return c.stage }
func (c *client_) TLSMode() bool               { return c.tlsMode }
func (c *client_) ReadTimeout() time.Duration  { return c.readTimeout }
func (c *client_) WriteTimeout() time.Duration { return c.writeTimeout }
func (c *client_) AliveTimeout() time.Duration { return c.aliveTimeout }

func (c *client_) nextConnID() int64 {
	return c.connID.Add(1)
}

// outgate_ is a mixin for outgates.
type outgate_ struct {
	// Mixins
	client_
	// States
}

func (f *outgate_) init(name string, stage *Stage) {
	f.client_.init(name, stage)
	// other states
}

func (f *outgate_) onConfigure() {
	f.client_.onConfigure()
}
func (f *outgate_) onPrepare() {
	f.client_.onPrepare()
}
func (f *outgate_) onShutdown() {
	f.client_.onShutdown()
}

// backend is a group of nodes.
type backend interface {
	Component
	maintain() // goroutine
}

// backend_ is a mixin for backends.
type backend_ struct {
	// Mixins
	client_
	// States
	balancer  string       // roundRobin, ipHash, random, ...
	indexGet  func() int64 // ...
	nodeIndex atomic.Int64 // for roundRobin. won't overflow because it is so large!
	numNodes  int64        // ...
}

func (b *backend_) init(name string, stage *Stage) {
	b.client_.init(name, stage)
	b.nodeIndex.Store(-1)
}

func (b *backend_) onConfigure() {
	b.client_.onConfigure()
	// balancer
	b.ConfigureString("balancer", &b.balancer, func(value string) bool {
		return value == "roundRobin" || value == "ipHash" || value == "random"
	}, "roundRobin")
}
func (b *backend_) onPrepare(numNodes int) {
	b.client_.onPrepare()
	switch b.balancer {
	case "roundRobin":
		b.indexGet = b.getIndexByRoundRobin
	case "ipHash":
		b.indexGet = b.getIndexByIPHash
	case "random":
		b.indexGet = b.getIndexByRandom
	default:
		BugExitln("this should not happen")
	}
	b.numNodes = int64(numNodes)
}
func (b *backend_) onShutdown() {
	b.client_.onShutdown()
}

func (b *backend_) getIndex() int64 {
	return b.indexGet()
}

func (b *backend_) getIndexByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *backend_) getIndexByIPHash() int64 {
	// TODO
	return 0
}
func (b *backend_) getIndexByRandom() int64 {
	// TODO
	return 0
}

var errNodeDown = errors.New("node is down")

// node is a member of backend.
type node interface {
	// maybe detect?
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
	lastRead  time.Time // deadline of last read operation
	lastWrite time.Time // deadline of last write operation
}

func (c *conn_) onGet(id int64, client client) {
	c.id = id
	c.client = client
	c.expire = time.Now().Add(client.AliveTimeout())
}
func (c *conn_) onPut() {
	c.client = nil
	c.expire = time.Time{}
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
}

func (c *conn_) isAlive() bool { return time.Now().Before(c.expire) }

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
	Read(p []byte) (n int, err error)
	ReadFull(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Writev(vector *net.Buffers) (int64, error)
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
