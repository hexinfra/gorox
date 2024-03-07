// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Mixins for net, rpc, and web.

package hemi

import (
	"crypto/tls"
	"errors"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Backend_ is the parent for backends.
type Backend_[N Node] struct {
	// Parent
	Component_
	// Assocs
	stage *Stage      // current stage
	nodes compList[N] // nodes of this backend
	// States
	dialTimeout  time.Duration // dial remote timeout
	readTimeout  time.Duration // read() timeout
	writeTimeout time.Duration // write() timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
}

func (b *Backend_[N]) OnCreate(name string, stage *Stage) {
	b.MakeComp(name)
	b.stage = stage
}
func (b *Backend_[N]) OnShutdown() {
	close(b.ShutChan) // notifies Maintain()
}

func (b *Backend_[N]) OnConfigure() {
	// readTimeout
	b.ConfigureDuration("readTimeout", &b.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 30*time.Second)

	// writeTimeout
	b.ConfigureDuration("writeTimeout", &b.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 30*time.Second)

	// dialTimeout
	b.ConfigureDuration("dialTimeout", &b.dialTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

	// aliveTimeout
	b.ConfigureDuration("aliveTimeout", &b.aliveTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 5*time.Second)
}
func (b *Backend_[N]) OnPrepare() {
}

func (b *Backend_[N]) Maintain() { // runner
	for _, node := range b.nodes {
		b.IncSub()
		go node.Maintain()
	}
	<-b.ShutChan

	// Backend was told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		node.OnShutdown()
	}
	b.WaitSubs() // nodes
	if Debug() >= 2 {
		Printf("backend=%s done\n", b.Name())
	}
	b.stage.DecSub()
}

func (b *Backend_[N]) AddNode(node N) {
	node.setShell(node)
	b.nodes = append(b.nodes, node)
}

func (b *Backend_[N]) Stage() *Stage               { return b.stage }
func (b *Backend_[N]) ReadTimeout() time.Duration  { return b.readTimeout }
func (b *Backend_[N]) WriteTimeout() time.Duration { return b.writeTimeout }

func (b *Backend_[N]) DialTimeout() time.Duration  { return b.dialTimeout }
func (b *Backend_[N]) AliveTimeout() time.Duration { return b.aliveTimeout }

func (b *Backend_[N]) nextConnID() int64 { return b.connID.Add(1) }

// Node_ is the parent for all backend nodes.
type Node_ struct {
	// Parent
	Component_
	// Assocs
	backend Backend
	// States
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

func (n *Node_) OnCreate(name string, backend Backend) {
	n.MakeComp(name)
	n.backend = backend
}
func (n *Node_) OnShutdown() {
	close(n.ShutChan) // notifies Maintain()
}

func (n *Node_) OnConfigure() {
	// address
	if v, ok := n.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if address[0] == '@' { // abstract uds
				n.udsMode = true
			} else if _, err := os.Stat(address); err == nil { // pathname uds
				n.udsMode = true
			}
			n.address = address
		} else {
			UseExitln("bad address in node")
		}
	} else {
		UseExitln("address is required in node")
	}

	// tlsMode
	n.ConfigureBool("tlsMode", &n.tlsMode, false)
	if n.tlsMode {
		n.tlsConfig = new(tls.Config)
	}

	// weight
	n.ConfigureInt32("weight", &n.weight, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad node weight")
	}, 1)

	// keepConns
	n.ConfigureInt32("keepConns", &n.keepConns, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad keepConns in node")
	}, 10)
}
func (n *Node_) OnPrepare() {
}

func (n *Node_) Backend() Backend { return n.backend }
func (n *Node_) IsUDS() bool      { return n.udsMode }
func (n *Node_) IsTLS() bool      { return n.tlsMode }

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
		conn.Close()
	}
	qnty := list.qnty
	list.qnty = 0
	list.head, list.tail = nil, nil

	return qnty
}

func (n *Node_) closeConn(conn BackendConn) {
	conn.Close()
	n.DecSub()
}

// BackendConn_ is the parent for backend conns.
type BackendConn_ struct {
	// Conn states (non-zeros)
	next    BackendConn // the linked-list
	id      int64       // the conn id
	backend Backend
	node    Node
	expire  time.Time // when the conn is considered expired
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

func (c *BackendConn_) OnGet(id int64, node Node) {
	c.id = id
	c.backend = node.Backend()
	c.node = node
	c.expire = time.Now().Add(c.backend.AliveTimeout())
}
func (c *BackendConn_) OnPut() {
	c.backend = nil
	c.node = nil
	c.expire = time.Time{}
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *BackendConn_) ID() int64        { return c.id }
func (c *BackendConn_) Backend() Backend { return c.backend }
func (c *BackendConn_) Node() Node       { return c.node }

func (c *BackendConn_) IsUDS() bool { return c.node.IsUDS() }
func (c *BackendConn_) IsTLS() bool { return c.node.IsTLS() }

func (c *BackendConn_) isAlive() bool { return time.Now().Before(c.expire) }

func (c *BackendConn_) getNext() BackendConn     { return c.next }
func (c *BackendConn_) setNext(next BackendConn) { c.next = next }

// Server_ is the parent for all servers.
type Server_[G Gate] struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	gates []G    // a server has many gates
	// States
	readTimeout     time.Duration // read() timeout
	writeTimeout    time.Duration // write() timeout
	address         string        // hostname:port, /path/to/unix.sock
	colonPort       string        // like: ":9876"
	colonPortBytes  []byte        // like: []byte(":9876")
	udsMode         bool          // is address a unix domain socket?
	tlsMode         bool          // use tls to secure the transport?
	tlsConfig       *tls.Config   // set if tls mode is true
	maxConnsPerGate int32         // max concurrent connections allowed per gate
	numGates        int32         // number of gates
}

func (s *Server_[G]) OnCreate(name string, stage *Stage) { // exported
	s.MakeComp(name)
	s.stage = stage
}
func (s *Server_[G]) OnShutdown() {
	// We don't use close(s.ShutChan) to notify gates.
	for _, gate := range s.gates {
		gate.Shut() // this causes gate to close and return immediately
	}
}

func (s *Server_[G]) OnConfigure() {
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

	// address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if p := strings.IndexByte(address, ':'); p == -1 {
				s.udsMode = true
			} else {
				s.colonPort = address[p:]
				s.colonPortBytes = []byte(s.colonPort)
			}
			s.address = address
		} else {
			UseExitln("address should be of string type")
		}
	} else {
		UseExitln(".address is required for servers")
	}

	// tlsMode
	s.ConfigureBool("tlsMode", &s.tlsMode, false)
	if s.tlsMode {
		s.tlsConfig = new(tls.Config)
	}

	// maxConnsPerGate
	s.ConfigureInt32("maxConnsPerGate", &s.maxConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConnsPerGate has an invalid value")
	}, 10000)

	// numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".numGates has an invalid value")
	}, s.stage.NumCPU())
}
func (s *Server_[G]) OnPrepare() {
	if s.udsMode { // unix domain socket does not support reuseaddr/reuseport.
		s.numGates = 1
	}
}

func (s *Server_[G]) AddGate(gate G) { s.gates = append(s.gates, gate) }

func (s *Server_[G]) Stage() *Stage               { return s.stage }
func (s *Server_[G]) ReadTimeout() time.Duration  { return s.readTimeout }
func (s *Server_[G]) WriteTimeout() time.Duration { return s.writeTimeout }

func (s *Server_[G]) Address() string        { return s.address }
func (s *Server_[G]) ColonPort() string      { return s.colonPort }
func (s *Server_[G]) ColonPortBytes() []byte { return s.colonPortBytes }
func (s *Server_[G]) IsUDS() bool            { return s.udsMode }
func (s *Server_[G]) IsTLS() bool            { return s.tlsMode }
func (s *Server_[G]) TLSConfig() *tls.Config { return s.tlsConfig }
func (s *Server_[G]) MaxConnsPerGate() int32 { return s.maxConnsPerGate }

func (s *Server_[G]) NumGates() int32 { return s.numGates }

// Gate_ is the parent for all gates.
type Gate_ struct {
	// Mixins
	_subsWaiter_ // for conns
	// Assocs
	server Server
	// States
	id       int32        // gate id
	shut     atomic.Bool  // is gate shut?
	numConns atomic.Int32 // TODO: false sharing
}

func (g *Gate_) Init(id int32, server Server) {
	g.server = server
	g.id = id
	g.shut.Store(false)
	g.numConns.Store(0)
}

func (g *Gate_) Server() Server  { return g.server }
func (g *Gate_) Address() string { return g.server.Address() }
func (g *Gate_) IsUDS() bool     { return g.server.IsUDS() }
func (g *Gate_) IsTLS() bool     { return g.server.IsTLS() }

func (g *Gate_) ID() int32        { return g.id }
func (g *Gate_) IsShut() bool     { return g.shut.Load() }
func (g *Gate_) MarkShut()        { g.shut.Store(true) }
func (g *Gate_) DecConns() int32  { return g.numConns.Add(-1) }
func (g *Gate_) ReachLimit() bool { return g.numConns.Add(1) > g.server.MaxConnsPerGate() }

func (g *Gate_) OnConnClosed() {
	g.DecConns()
	g.DecSub()
}

// ServerConn_ is the parent for server conns.
type ServerConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id     int64 // the conn id
	server Server
	gate   Gate
	// Conn states (zeros)
	lastRead  time.Time // deadline of last read operation
	lastWrite time.Time // deadline of last write operation
}

func (c *ServerConn_) OnGet(id int64, gate Gate) {
	c.id = id
	c.server = gate.Server()
	c.gate = gate
}
func (c *ServerConn_) OnPut() {
	c.server = nil
	c.gate = nil
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
}

func (c *ServerConn_) ID() int64      { return c.id }
func (c *ServerConn_) Server() Server { return c.server }
func (c *ServerConn_) Gate() Gate     { return c.gate }

func (c *ServerConn_) IsUDS() bool { return c.server.IsUDS() }
func (c *ServerConn_) IsTLS() bool { return c.server.IsTLS() }

// streamHolder
type streamHolder interface {
	MaxStreamsPerConn() int32
}

// _streamHolder_ is a mixin.
type _streamHolder_ struct {
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (s *_streamHolder_) onConfigure(shell Component, defaultMaxStreams int32) {
	// maxStreamsPerConn
	shell.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, defaultMaxStreams)
}
func (s *_streamHolder_) onPrepare(shell Component) {
}

func (s *_streamHolder_) MaxStreamsPerConn() int32 { return s.maxStreamsPerConn }

// contentSaver
type contentSaver interface {
	SaveContentFilesDir() string
}

// _contentSaver_ is a mixin.
type _contentSaver_ struct {
	// States
	saveContentFilesDir string
}

func (s *_contentSaver_) onConfigure(shell Component, defaultDir string) {
	// saveContentFilesDir
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) error {
		if value != "" && len(value) <= 232 {
			return nil
		}
		return errors.New(".saveContentFilesDir has an invalid value")
	}, defaultDir)
}
func (s *_contentSaver_) onPrepare(shell Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *_contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// _loadBalancer_ is a mixin.
type _loadBalancer_ struct {
	// States
	balancer  string       // roundRobin, ipHash, random, ...
	indexGet  func() int64 // ...
	nodeIndex atomic.Int64 // for roundRobin. won't overflow because it is so large!
	numNodes  int64        // num of nodes
}

func (b *_loadBalancer_) init() {
	b.nodeIndex.Store(-1)
}

func (b *_loadBalancer_) onConfigure(shell Component) {
	// balancer
	shell.ConfigureString("balancer", &b.balancer, func(value string) error {
		if value == "roundRobin" || value == "ipHash" || value == "random" {
			return nil
		}
		return errors.New(".balancer has an invalid value")
	}, "roundRobin")
}
func (b *_loadBalancer_) onPrepare(numNodes int) {
	switch b.balancer {
	case "roundRobin":
		b.indexGet = b.getNextByRoundRobin
	case "ipHash":
		b.indexGet = b.getNextByIPHash
	case "random":
		b.indexGet = b.getNextByRandom
	default:
		BugExitln("unknown balancer")
	}
	b.numNodes = int64(numNodes)
}

func (b *_loadBalancer_) getNext() int64 { return b.indexGet() }

func (b *_loadBalancer_) getNextByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *_loadBalancer_) getNextByIPHash() int64 {
	// TODO
	return 0
}
func (b *_loadBalancer_) getNextByRandom() int64 {
	return rand.Int63n(b.numNodes)
}

// _subsWaiter_ is a mixin.
type _subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *_subsWaiter_) IncSub()        { w.subs.Add(1) }
func (w *_subsWaiter_) SubsAddn(n int) { w.subs.Add(n) }
func (w *_subsWaiter_) WaitSubs()      { w.subs.Wait() }
func (w *_subsWaiter_) DecSub()        { w.subs.Done() }

// _shutdownable_ is a mixin.
type _shutdownable_ struct {
	ShutChan chan struct{} // used to notify target to shutdown
}

func (s *_shutdownable_) init() {
	s.ShutChan = make(chan struct{})
}

func (s *_shutdownable_) Loop(interval time.Duration, callback func(now time.Time)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ShutChan:
			return
		case now := <-ticker.C:
			callback(now)
		}
	}
}

// identifiable
type identifiable interface {
	ID() uint8
	setID(id uint8)
}

// _identifiable_ is a mixin.
type _identifiable_ struct {
	id uint8
}

func (i *_identifiable_) ID() uint8 { return i.id }

func (i *_identifiable_) setID(id uint8) { i.id = id }
