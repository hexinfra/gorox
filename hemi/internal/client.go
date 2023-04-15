// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
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

// client is the interface for outgates and backends.
type client interface {
	Stage() *Stage
	TLSMode() bool
	WriteTimeout() time.Duration
	ReadTimeout() time.Duration
	AliveTimeout() time.Duration
	nextConnID() int64
}

// client_ is a mixin for outgates and backends.
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

func (c *client_) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}

func (c *client_) onConfigure() {
	// tlsMode
	c.ConfigureBool("tlsMode", &c.tlsMode, false)
	if c.tlsMode {
		c.tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
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

func (c *client_) OnShutdown() {
	close(c.Shut)
}

func (c *client_) Stage() *Stage               { return c.stage }
func (c *client_) TLSMode() bool               { return c.tlsMode }
func (c *client_) WriteTimeout() time.Duration { return c.writeTimeout }
func (c *client_) ReadTimeout() time.Duration  { return c.readTimeout }
func (c *client_) AliveTimeout() time.Duration { return c.aliveTimeout }

func (c *client_) nextConnID() int64 { return c.connID.Add(1) }

// outgate
type outgate interface {
	served() int64
}

// outgate_
type outgate_ struct {
	// Mixins
	client_
	// States
	nServed atomic.Int64
}

func (o *outgate_) onCreate(name string, stage *Stage) {
	o.client_.onCreate(name, stage)
}

func (o *outgate_) onConfigure() {
	o.client_.onConfigure()
}
func (o *outgate_) onPrepare() {
	o.client_.onPrepare()
}

func (o *outgate_) served() int64 { return o.nServed.Load() }
func (o *outgate_) incServed()    { o.nServed.Add(1) }

// backend is a group of stateless nodes.
type backend interface {
	Component
	client
	maintain() // goroutine
}

// backend_
type backend_[N node] struct {
	// Mixins
	client_
	// Assocs
	creator interface {
		createNode(id int32) N
	} // if Go's generic supports new(N) then this is not needed.
	nodes []N // nodes of this backend
	// States
}

func (b *backend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.client_.onCreate(name, stage)
	b.creator = creator
}

func (b *backend_[N]) onConfigure() {
	b.client_.onConfigure()
	// nodes
	v, ok := b.Find("nodes")
	if !ok {
		UseExitln("nodes is required for backends")
	}
	vNodes, ok := v.List()
	if !ok {
		UseExitln("nodes must be a list")
	}
	for id, elem := range vNodes {
		vNode, ok := elem.Dict()
		if !ok {
			UseExitln("node in nodes must be a dict")
		}
		node := b.creator.createNode(int32(id))
		// address
		vAddress, ok := vNode["address"]
		if !ok {
			UseExitln("address is required in node")
		}
		if address, ok := vAddress.String(); ok && address != "" {
			node.setAddress(address)
		}
		// weight
		vWeight, ok := vNode["weight"]
		if ok {
			if weight, ok := vWeight.Int32(); ok && weight > 0 {
				node.setWeight(weight)
			} else {
				UseExitln("bad weight in node")
			}
		} else {
			node.setWeight(1)
		}
		// keepConns
		vKeepConns, ok := vNode["keepConns"]
		if ok {
			if keepConns, ok := vKeepConns.Int32(); ok && keepConns > 0 {
				node.setKeepConns(keepConns)
			} else {
				UseExitln("bad keepConns in node")
			}
		} else {
			node.setKeepConns(10)
		}
		b.nodes = append(b.nodes, node)
	}
}
func (b *backend_[N]) onPrepare() {
	b.client_.onPrepare()
}

func (b *backend_[N]) maintain() { // goroutine
	shutNodes := make(chan struct{})
	for _, node := range b.nodes {
		b.IncSub(1) // one more node
		go node.maintain(shutNodes)
	}
	<-b.Shut         // waiting for backend shutdown signal
	close(shutNodes) // this will notify all nodes
	b.WaitSubs()     // nodes
	if IsDebug(2) {
		Debugf("backend=%s done\n", b.Name())
	}
	b.stage.SubDone()
}

// node is a member of backend. node must be stateless.
type node interface {
	setAddress(address string)
	setWeight(weight int32)
	setKeepConns(keepConns int32)
	maintain(shut chan struct{}) // goroutine
}

// node_ is a mixin for backend nodes.
type node_ struct {
	// Mixins
	subsWaiter_
	// States
	id        int32       // the node id
	address   string      // hostname:port
	weight    int32       // 1, 22, 333, ...
	keepConns int32       // max conns to keep alive
	down      atomic.Bool // TODO: false-sharing
	freeList  struct {    // free list of conns in this node
		sync.Mutex
		size int  // size of the list
		head conn // head element
		tail conn // tail element
	}
}

func (n *node_) init(id int32) {
	n.id = id
}

func (n *node_) setAddress(address string)    { n.address = address }
func (n *node_) setWeight(weight int32)       { n.weight = weight }
func (n *node_) setKeepConns(keepConns int32) { n.keepConns = keepConns }

func (n *node_) markDown()    { n.down.Store(true) }
func (n *node_) markUp()      { n.down.Store(false) }
func (n *node_) isDown() bool { return n.down.Load() }

func (n *node_) pullConn() conn {
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
	} else { // >= 1
		list.tail.setNext(conn)
		list.tail = conn
	}
	list.size++
}

func (n *node_) closeFree() int {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	for conn := list.head; conn != nil; conn = conn.getNext() {
		conn.closeConn()
	}
	size := list.size
	list.size = 0
	list.head, list.tail = nil, nil
	return size
}

var errNodeDown = errors.New("node is down")

// conn is the client conns.
type conn interface {
	getNext() conn
	setNext(next conn)
	isAlive() bool
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

func (c *conn_) getNext() conn     { return c.next }
func (c *conn_) setNext(next conn) { c.next = next }

func (c *conn_) isAlive() bool { return time.Now().Before(c.expire) }

// connection-oriented backend, supports TCPS and UNIX.
type WireBackend interface {
	backend
	streamHolder
	Dial() (SConn, error)
	FetchConn() (SConn, error)
	StoreConn(conn SConn)
}

// connection-oriented node, supports TCPS and UNIX.
type wireNode_ struct {
	// Mixins
	node_
	// Assocs
	backend WireBackend
}

func (n *wireNode_) init(id int32, backend WireBackend) {
	n.node_.init(id)
	n.backend = backend
}

// connection-oriented conn, supports TCPS and UNIX.
type SConn interface {
	conn
	SetWriteDeadline(deadline time.Time) error
	SetReadDeadline(deadline time.Time) error
	Write(p []byte) (n int, err error)
	Writev(vector *net.Buffers) (int64, error)
	Read(p []byte) (n int, err error)
	ReadFull(p []byte) (n int, err error)
	ReadAtLeast(p []byte, min int) (n int, err error)
	Close() error
	MakeTempName(p []byte, unixTime int64) (from int, edge int)
	IsBroken() bool
	MarkBroken()
}

// sConn_ is a mixin for TConn and XConn.
type sConn_ struct {
	// Mixins
	conn_
	// Conn states (non-zeros)
	maxStreams int32 // how many streams are allowed on this conn?
	// Conn states (zeros)
	counter     atomic.Int64 // used to make temp name
	usedStreams atomic.Int32 // how many streams has been used?
	writeBroken atomic.Bool  // write-side broken?
	readBroken  atomic.Bool  // read-side broken?
}

func (c *sConn_) onGet(id int64, client client, maxStreams int32) {
	c.conn_.onGet(id, client)
	c.maxStreams = maxStreams
}
func (c *sConn_) onPut() {
	c.conn_.onPut()
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
}

func (c *sConn_) MakeTempName(p []byte, unixTime int64) (from int, edge int) {
	return makeTempName(p, int64(c.client.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}
func (c *sConn_) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *sConn_) IsBroken() bool { return c.writeBroken.Load() || c.readBroken.Load() }
func (c *sConn_) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}

func (c *sConn_) markWriteBroken() { c.writeBroken.Store(true) }
func (c *sConn_) markReadBroken()  { c.readBroken.Store(true) }
