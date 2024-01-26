// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
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

// qClient is the interface for QUICOutgate, QUDSOutgate, QUICBackend, and QUDSBackend.
type qClient interface {
	// Imports
	client
	streamHolder
	// Methods
}

// tClient is the interface for TCPSOutgate, TUDSOutgate, TCPSBackend, and TUDSBackend.
type tClient interface {
	// Imports
	client
	streamHolder
	// Methods
}

// uClient is the interface for UDPSOutgate, UUDSOutgate, UDPSBackend, and UUDSBackend.
type uClient interface {
	// Imports
	client
	// Methods
}

// wBackend is the interface for TCPSBackend and TUDSBackend.
type wBackend interface {
	WriteTimeout() time.Duration
	ReadTimeout() time.Duration
	Dial() (wConn, error)
	FetchConn() (wConn, error)
	StoreConn(conn wConn)
}

// wConn is the interface for TConn and XConn.
type wConn interface {
	SetWriteDeadline(deadline time.Time) error
	SetReadDeadline(deadline time.Time) error
	Write(p []byte) (n int, err error)
	Writev(vector *net.Buffers) (int64, error)
	Read(p []byte) (n int, err error)
	ReadFull(p []byte) (n int, err error)
	ReadAtLeast(p []byte, min int) (n int, err error)
	CloseWrite() error
	Close() error
	MakeTempName(p []byte, unixTime int64) int
	IsBroken() bool
	MarkBroken()
}

// client is the interface for outgates and backends.
type client interface {
	// Imports
	// Methods
	Stage() *Stage
	TLSMode() bool
	WriteTimeout() time.Duration
	ReadTimeout() time.Duration
	AliveTimeout() time.Duration
	nextConnID() int64
}

// client_ is the mixin for outgates and backends.
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
		c.tlsConfig = new(tls.Config)
	}

	// dialTimeout
	c.ConfigureDuration("dialTimeout", &c.dialTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

	// writeTimeout
	c.ConfigureDuration("writeTimeout", &c.writeTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 30*time.Second)

	// readTimeout
	c.ConfigureDuration("readTimeout", &c.readTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 30*time.Second)

	// aliveTimeout
	c.ConfigureDuration("aliveTimeout", &c.aliveTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 5*time.Second)
}
func (c *client_) onPrepare() {
	// Currently nothing.
}

func (c *client_) OnShutdown() {
	close(c.ShutChan)
}

func (c *client_) Stage() *Stage               { return c.stage }
func (c *client_) TLSMode() bool               { return c.tlsMode }
func (c *client_) WriteTimeout() time.Duration { return c.writeTimeout }
func (c *client_) ReadTimeout() time.Duration  { return c.readTimeout }
func (c *client_) AliveTimeout() time.Duration { return c.aliveTimeout }

func (c *client_) nextConnID() int64 { return c.connID.Add(1) }

// outgate is the interface for outgates.
type outgate interface {
	// Methods
	servedConns() int64
	servedStreams() int64
}

// outgate_ is the mixin for outgates.
type outgate_ struct {
	// Mixins
	client_
	// States
	nServedStreams atomic.Int64
	nServedExchans atomic.Int64
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

func (o *outgate_) servedStreams() int64 { return o.nServedStreams.Load() }
func (o *outgate_) incServedStreams()    { o.nServedStreams.Add(1) }

func (o *outgate_) servedExchans() int64 { return o.nServedExchans.Load() }
func (o *outgate_) incServedExchans()    { o.nServedExchans.Add(1) }

// Backend is a group of nodes.
type Backend interface {
	// Imports
	Component
	client
	// Methods
	Maintain() // goroutine
}

// Backend_ is the mixin for backends.
type Backend_[N Node] struct {
	// Mixins
	client_
	// Assocs
	creator interface {
		createNode(id int32) N
	} // if Go's generic supports new(N) then this is not needed.
	nodes []N // nodes of this backend
	// States
}

func (b *Backend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.client_.onCreate(name, stage)
	b.creator = creator
}

func (b *Backend_[N]) onConfigure() {
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
		if !ok {
			node.setWeight(1)
		} else if weight, ok := vWeight.Int32(); ok && weight > 0 {
			node.setWeight(weight)
		} else {
			UseExitln("bad weight in node")
		}

		// keepConns
		vKeepConns, ok := vNode["keepConns"]
		if !ok {
			node.setKeepConns(10)
		} else if keepConns, ok := vKeepConns.Int32(); ok && keepConns > 0 {
			node.setKeepConns(keepConns)
		} else {
			UseExitln("bad keepConns in node")
		}

		b.nodes = append(b.nodes, node)
	}
}
func (b *Backend_[N]) onPrepare() {
	b.client_.onPrepare()
}

func (b *Backend_[N]) Maintain() { // goroutine
	for _, node := range b.nodes {
		b.IncSub(1)
		go node.Maintain()
	}
	<-b.ShutChan

	// Backend is told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		node.shutdown()
	}
	b.WaitSubs() // nodes
	if Debug() >= 2 {
		Printf("backend=%s done\n", b.Name())
	}
	b.stage.SubDone()
}

// Node is a member of backend. Nodes are not components.
type Node interface {
	// Methods
	setAddress(address string)
	setWeight(weight int32)
	setKeepConns(keepConns int32)
	Maintain() // goroutine
	shutdown()
}

// Node_ is the mixin for backend nodes.
type Node_ struct {
	// Mixins
	subsWaiter_ // usually for conns
	shutdownable_
	// States
	id        int32       // the node id
	address   string      // hostname:port, /path/to/unix.sock
	weight    int32       // 1, 22, 333, ...
	keepConns int32       // max conns to keep alive
	down      atomic.Bool // TODO: false-sharing
	freeList  struct {    // free list of conns in this node
		sync.Mutex
		head Conn // head element
		tail Conn // tail element
		qnty int  // size of the list
	}
}

func (n *Node_) init(id int32) {
	n.shutdownable_.init()
	n.id = id
}

func (n *Node_) setAddress(address string)    { n.address = address }
func (n *Node_) setWeight(weight int32)       { n.weight = weight }
func (n *Node_) setKeepConns(keepConns int32) { n.keepConns = keepConns }

func (n *Node_) markDown()    { n.down.Store(true) }
func (n *Node_) markUp()      { n.down.Store(false) }
func (n *Node_) isDown() bool { return n.down.Load() }

func (n *Node_) pullConn() Conn {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.getNext()
	conn.setNext(nil)
	list.qnty--
	return conn
}
func (n *Node_) pushConn(conn Conn) {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		list.head = conn
		list.tail = conn
	} else { // >= 1
		list.tail.setNext(conn)
		list.tail = conn
	}
	list.qnty++
}

func (n *Node_) closeFree() int {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	for conn := list.head; conn != nil; conn = conn.getNext() {
		conn.closeConn()
	}
	qnty := list.qnty
	list.qnty = 0
	list.head, list.tail = nil, nil
	return qnty
}

func (n *Node_) shutdown() {
	close(n.ShutChan)
}

var errNodeDown = errors.New("node is down")

// Conn is the client conns.
type Conn interface {
	// Methods
	getNext() Conn
	setNext(next Conn)
	isAlive() bool
	closeConn()
}

// Conn_ is the mixin for client conns.
type Conn_ struct {
	// Conn states (non-zeros)
	next   Conn      // the linked-list
	id     int64     // the conn id
	client client    // associated client
	expire time.Time // when the conn is considered expired
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

func (c *Conn_) onGet(id int64, client client) {
	c.id = id
	c.client = client
	c.expire = time.Now().Add(client.AliveTimeout())
}
func (c *Conn_) onPut() {
	c.client = nil
	c.expire = time.Time{}
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *Conn_) getNext() Conn     { return c.next }
func (c *Conn_) setNext(next Conn) { c.next = next }

func (c *Conn_) isAlive() bool { return time.Now().Before(c.expire) }
