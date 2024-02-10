// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General components and elements for net, rpc, and web.

package hemi

import (
	"crypto/tls"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Server component.
type Server interface {
	// Imports
	Component
	// Methods
	Serve() // runner
	Stage() *Stage
	IsUDS() bool
	IsAbstract() bool
	IsTLS() bool
	ColonPort() string
	ColonPortBytes() []byte
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// Server_ is the mixin for all servers.
type Server_[G Gate] struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	gates []G    // a server may has many gates
	// States
	address         string        // hostname:port, /path/to/unix.sock
	colonPort       string        // like: ":9876"
	colonPortBytes  []byte        // like: []byte(":9876")
	udsMode         bool          // address is a unix domain socket?
	abstract        bool          // use abstract unix domain socket? only effective under linux
	tlsMode         bool          // tls mode?
	tlsConfig       *tls.Config   // set if is tls mode
	readTimeout     time.Duration // read() timeout
	writeTimeout    time.Duration // write() timeout
	numGates        int32         // number of gates
	maxConnsPerGate int32         // max concurrent connections allowed per gate
}

func (s *Server_[G]) OnCreate(name string, stage *Stage) { // exported
	s.MakeComp(name)
	s.stage = stage
}

func (s *Server_[G]) OnConfigure() {
	// address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok {
			if p := strings.IndexByte(address, ':'); p != -1 && p != len(address)-1 {
				s.address = address
				s.colonPort = address[p:]
				s.colonPortBytes = []byte(s.colonPort)
			} else if _, err := os.Stat(address); err == nil {
				s.address = address
				s.udsMode = true
			} else {
				UseExitln("bad address: " + address)
			}
		} else {
			UseExitln("address should be of string type")
		}
	} else {
		UseExitln("address is required for servers")
	}

	// abstract
	s.ConfigureBool("abstract", &s.abstract, false)

	// tlsMode
	s.ConfigureBool("tlsMode", &s.tlsMode, false)
	if s.tlsMode {
		s.tlsConfig = new(tls.Config)
	}

	// readTimeout
	s.ConfigureDuration("readTimeout", &s.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 60*time.Second)

	// writeTimeout
	s.ConfigureDuration("writeTimeout", &s.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 60*time.Second)

	// numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".numGates has an invalid value")
	}, s.stage.NumCPU())

	// maxConnsPerGate
	s.ConfigureInt32("maxConnsPerGate", &s.maxConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConnsPerGate has an invalid value")
	}, 100000)
}
func (s *Server_[G]) OnPrepare() {
	// Currently nothing.
}

func (s *Server_[G]) ShutGates() {
	// Notify gates. We don't use close(s.ShutChan) here.
	for _, gate := range s.gates {
		gate.Shut()
	}
}
func (s *Server_[G]) AppendGate(gate G) { s.gates = append(s.gates, gate) }

func (s *Server_[G]) Stage() *Stage               { return s.stage }
func (s *Server_[G]) Address() string             { return s.address }
func (s *Server_[G]) ColonPort() string           { return s.colonPort }
func (s *Server_[G]) ColonPortBytes() []byte      { return s.colonPortBytes }
func (s *Server_[G]) IsUDS() bool                 { return s.udsMode }
func (s *Server_[G]) IsAbstract() bool            { return s.abstract }
func (s *Server_[G]) IsTLS() bool                 { return s.tlsMode }
func (s *Server_[G]) ReadTimeout() time.Duration  { return s.readTimeout }
func (s *Server_[G]) WriteTimeout() time.Duration { return s.writeTimeout }
func (s *Server_[G]) NumGates() int32             { return s.numGates }
func (s *Server_[G]) MaxConnsPerGate() int32      { return s.maxConnsPerGate }

// Gate is the interface for all gates. Gates are not components.
type Gate interface {
	// Methods
	ID() int32
	IsShut() bool
	Open() error
	Shut() error
	OnConnClosed()
}

// Gate_ is the mixin for all gates.
type Gate_ struct {
	// Mixins
	subsWaiter_ // for conns
	// Assocs
	stage *Stage // current stage
	// States
	id       int32 // gate id
	udsMode  bool
	abstract bool
	tlsMode  bool
	address  string       // listening address
	isShut   atomic.Bool  // is gate shut?
	maxConns int32        // max concurrent conns allowed
	numConns atomic.Int32 // TODO: false sharing
}

func (g *Gate_) Init(stage *Stage, id int32, udsMode bool, abstract bool, tlsMode bool, address string, maxConns int32) {
	g.stage = stage
	g.id = id
	g.udsMode = udsMode
	g.abstract = abstract
	g.tlsMode = tlsMode
	g.address = address
	g.isShut.Store(false)
	g.maxConns = maxConns
	g.numConns.Store(0)
}

func (g *Gate_) Stage() *Stage    { return g.stage }
func (g *Gate_) ID() int32        { return g.id }
func (g *Gate_) Address() string  { return g.address }
func (g *Gate_) IsUDS() bool      { return g.udsMode }
func (g *Gate_) IsAbstract() bool { return g.abstract }
func (g *Gate_) IsTLS() bool      { return g.tlsMode }

func (g *Gate_) MarkShut()    { g.isShut.Store(true) }
func (g *Gate_) IsShut() bool { return g.isShut.Load() }

func (g *Gate_) DecConns() int32  { return g.numConns.Add(-1) }
func (g *Gate_) ReachLimit() bool { return g.numConns.Add(1) > g.maxConns }

func (g *Gate_) OnConnClosed() {
	g.DecConns()
	g.SubDone()
}

// ServerConn_
type ServerConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id    int64
	stage *Stage
	// Conn states (zeros)
}

func (c *ServerConn_) onGet(id int64, stage *Stage) {
	c.id = id
	c.stage = stage
}
func (c *ServerConn_) onPut() {
	c.stage = nil
}

// Backend is a group of nodes.
type Backend interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Stage() *Stage
	WriteTimeout() time.Duration
	ReadTimeout() time.Duration
	AliveTimeout() time.Duration
	nextConnID() int64
}

// Backend_ is the mixin for backends.
type Backend_[N Node] struct {
	// Mixins
	Component_
	// Assocs
	stage   *Stage // current stage
	nodes   []N    // nodes of this backend
	creator interface {
		createNode(id int32) N
	} // if Go's generic supports new(N) then this is not needed.
	// States
	dialTimeout  time.Duration // dial remote timeout
	writeTimeout time.Duration // write operation timeout
	readTimeout  time.Duration // read operation timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
}

func (b *Backend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.MakeComp(name)
	b.stage = stage
	b.creator = creator
}
func (b *Backend_[N]) OnShutdown() {
	close(b.ShutChan) // notifies run() or Maintain()
}

func (b *Backend_[N]) onConfigure() {
	// dialTimeout
	b.ConfigureDuration("dialTimeout", &b.dialTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

	// writeTimeout
	b.ConfigureDuration("writeTimeout", &b.writeTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 30*time.Second)

	// readTimeout
	b.ConfigureDuration("readTimeout", &b.readTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 30*time.Second)

	// aliveTimeout
	b.ConfigureDuration("aliveTimeout", &b.aliveTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 5*time.Second)

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

		// tlsMode
		vIsTLS, ok := vNode["tlsMode"]
		if ok {
			if tlsMode, ok := vIsTLS.Bool(); ok && tlsMode {
				node.setIsTLS()
			}
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
	// Currently nothing.
}

func (b *Backend_[N]) Maintain() { // runner
	for _, node := range b.nodes {
		b.IncSub(1)
		go node.Maintain()
	}
	<-b.ShutChan

	// Backend was told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		node.shutdown()
	}
	b.WaitSubs() // nodes
	if Debug() >= 2 {
		Printf("backend=%s done\n", b.Name())
	}
	b.stage.SubDone()
}

func (b *Backend_[N]) Stage() *Stage               { return b.stage }
func (b *Backend_[N]) WriteTimeout() time.Duration { return b.writeTimeout }
func (b *Backend_[N]) ReadTimeout() time.Duration  { return b.readTimeout }
func (b *Backend_[N]) AliveTimeout() time.Duration { return b.aliveTimeout }

func (b *Backend_[N]) nextConnID() int64 { return b.connID.Add(1) }

// Node is a member of backend. Nodes are not components.
type Node interface {
	// Methods
	setAddress(address string)
	setIsTLS()
	setWeight(weight int32)
	setKeepConns(keepConns int32)
	Maintain() // runner
	shutdown()
}

// Node_ is the mixin for backend nodes.
type Node_ struct {
	// Mixins
	subsWaiter_ // usually for conns
	shutdownable_
	// States
	id        int32       // the node id
	udsMode   bool        // uds or not
	tlsMode   bool        // tls or not
	tlsConfig *tls.Config // TLS config if TLS is enabled
	address   string      // hostname:port, /path/to/unix.sock
	weight    int32       // 1, 22, 333, ...
	keepConns int32       // max conns to keep alive
	down      atomic.Bool // TODO: false-sharing
	freeList  struct {    // free list of conns in this node
		sync.Mutex
		head BackendConn // head element
		tail BackendConn // tail element
		qnty int         // size of the list
	}
}

func (n *Node_) init(id int32) {
	n.shutdownable_.init()
	n.id = id
}

func (n *Node_) setAddress(address string) {
	n.address = address
	if _, err := os.Stat(address); err == nil {
		n.udsMode = true
	}
}
func (n *Node_) setIsTLS() {
	n.tlsMode = true
	n.tlsConfig = new(tls.Config)
}
func (n *Node_) setWeight(weight int32)       { n.weight = weight }
func (n *Node_) setKeepConns(keepConns int32) { n.keepConns = keepConns }

func (n *Node_) markDown()    { n.down.Store(true) }
func (n *Node_) markUp()      { n.down.Store(false) }
func (n *Node_) isDown() bool { return n.down.Load() }

func (n *Node_) pullConn() BackendConn {
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
func (n *Node_) pushConn(conn BackendConn) {
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
	close(n.ShutChan) // notifies Maintain()
}

var errNodeDown = errors.New("node is down")

// BackendConn is the backend conns.
type BackendConn interface {
	// Methods
	getNext() BackendConn
	setNext(next BackendConn)
	isAlive() bool
	closeConn()
}

// BackendConn_ is the mixin for backend conns.
type BackendConn_ struct {
	// Conn states (non-zeros)
	next    BackendConn // the linked-list
	id      int64       // the conn id
	udsMode bool        // uds or not
	tlsMode bool        // tls or not
	expire  time.Time   // when the conn is considered expired
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

func (c *BackendConn_) onGet(id int64, udsMode bool, tlsMode bool, expire time.Time) {
	c.id = id
	c.udsMode = udsMode
	c.tlsMode = tlsMode
	c.expire = expire
}
func (c *BackendConn_) onPut() {
	c.expire = time.Time{}
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *BackendConn_) getNext() BackendConn     { return c.next }
func (c *BackendConn_) setNext(next BackendConn) { c.next = next }

func (c *BackendConn_) isAlive() bool { return time.Now().Before(c.expire) }
