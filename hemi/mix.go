// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General components and elements for net, rpc, and web.

package hemi

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// agent collects shared methods between Server and Backend.
type agent interface {
	// Methods
	Stage() *Stage
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// _agent_ is a mixin for Server_ and Backend_.
type _agent_ struct {
	// Assocs
	stage *Stage // current stage
	// States
	readTimeout  time.Duration // read() timeout
	writeTimeout time.Duration // write() timeout
}

func (a *_agent_) onCreate(name string, stage *Stage) {
	a.stage = stage
}

func (a *_agent_) onConfigure(shell Component, readTimeout time.Duration, writeTimeout time.Duration) {
	// readTimeout
	shell.ConfigureDuration("readTimeout", &a.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, readTimeout)

	// writeTimeout
	shell.ConfigureDuration("writeTimeout", &a.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, writeTimeout)
}
func (a *_agent_) onPrepare() {
	// Currently nothing.
}

func (a *_agent_) Stage() *Stage               { return a.stage }
func (a *_agent_) ReadTimeout() time.Duration  { return a.readTimeout }
func (a *_agent_) WriteTimeout() time.Duration { return a.writeTimeout }

// Server component. A Server is a group of gates.
type Server interface {
	// Imports
	Component
	agent
	// Methods
	Serve() // runner
	Address() string
	ColonPort() string
	ColonPortBytes() []byte
	IsUDS() bool
	IsTLS() bool
	TLSConfig() *tls.Config
	MaxConnsPerGate() int32
}

// Server_ is the parent for all servers.
type Server_[G Gate] struct {
	// Parent
	Component_
	// Mixins
	_agent_
	// Assocs
	gates []G // a server has many gates
	// States
	address         string      // hostname:port, /path/to/unix.sock
	colonPort       string      // like: ":9876"
	colonPortBytes  []byte      // like: []byte(":9876")
	udsMode         bool        // is address a unix domain socket?
	tlsMode         bool        // use tls to secure the transport?
	tlsConfig       *tls.Config // set if tls mode is true
	maxConnsPerGate int32       // max concurrent connections allowed per gate
	numGates        int32       // number of gates
}

func (s *Server_[G]) OnCreate(name string, stage *Stage) { // exported
	s.MakeComp(name)
	s._agent_.onCreate(name, stage)
}
func (s *Server_[G]) OnShutdown() {
	// We don't use close(s.ShutChan) to notify gates.
	for _, gate := range s.gates {
		gate.Shut() // this causes gate to return immediately
	}
}

func (s *Server_[G]) OnConfigure() {
	s._agent_.onConfigure(s, 60*time.Second, 60*time.Second)

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
	s._agent_.onPrepare()
}

func (s *Server_[G]) AddGate(gate G) { s.gates = append(s.gates, gate) }

func (s *Server_[G]) Address() string        { return s.address }
func (s *Server_[G]) ColonPort() string      { return s.colonPort }
func (s *Server_[G]) ColonPortBytes() []byte { return s.colonPortBytes }
func (s *Server_[G]) IsUDS() bool            { return s.udsMode }
func (s *Server_[G]) IsTLS() bool            { return s.tlsMode }
func (s *Server_[G]) TLSConfig() *tls.Config { return s.tlsConfig }
func (s *Server_[G]) MaxConnsPerGate() int32 { return s.maxConnsPerGate }

func (s *Server_[G]) NumGates() int32 { return s.numGates }

// Backend component. A Backend is a group of nodes.
type Backend interface {
	// Imports
	Component
	agent
	// Methods
	Maintain() // runner
	DialTimeout() time.Duration
	AliveTimeout() time.Duration
	nextConnID() int64
}

// Backend_ is the parent for backends.
type Backend_[N Node] struct {
	// Parent
	Component_
	// Mixins
	_agent_
	// Assocs
	nodes   []N // nodes of this backend
	newNode func(id int32) N
	// States
	dialTimeout  time.Duration // dial remote timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
}

func (b *Backend_[N]) OnCreate(name string, stage *Stage, newNode func(id int32) N) {
	b.MakeComp(name)
	b._agent_.onCreate(name, stage)
	b.newNode = newNode
}
func (b *Backend_[N]) OnShutdown() {
	close(b.ShutChan) // notifies run() or Maintain()
}

func (b *Backend_[N]) OnConfigure() {
	b._agent_.onConfigure(b, 30*time.Second, 30*time.Second)

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

		node := b.newNode(int32(id))

		// address
		if vAddress, ok := vNode["address"]; !ok {
			UseExitln("address is required in node")
		} else if address, ok := vAddress.String(); ok && address != "" {
			node.setAddress(address)
		} else {
			UseExitln("bad address in node")
		}

		// tlsMode
		if vIsTLS, ok := vNode["tlsMode"]; ok {
			if tlsMode, ok := vIsTLS.Bool(); ok && tlsMode {
				node.setTLS()
			}
		}

		// weight
		if vWeight, ok := vNode["weight"]; !ok {
			node.setWeight(1)
		} else if weight, ok := vWeight.Int32(); ok && weight > 0 {
			node.setWeight(weight)
		} else {
			UseExitln("bad weight in node")
		}

		// keepConns
		if vKeepConns, ok := vNode["keepConns"]; !ok {
			node.setKeepConns(10)
		} else if keepConns, ok := vKeepConns.Int32(); ok && keepConns > 0 {
			node.setKeepConns(keepConns)
		} else {
			UseExitln("bad keepConns in node")
		}

		b.nodes = append(b.nodes, node)
	}

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
	b._agent_.onPrepare()
}

func (b *Backend_[N]) Maintain() { // runner
	for _, node := range b.nodes {
		b.IncSub()
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
	b.stage.DecSub()
}

func (b *Backend_[N]) DialTimeout() time.Duration  { return b.dialTimeout }
func (b *Backend_[N]) AliveTimeout() time.Duration { return b.aliveTimeout }

func (b *Backend_[N]) nextConnID() int64 { return b.connID.Add(1) }

// Gate is the interface for all gates. Gates are not components.
type Gate interface {
	// Methods
	Server() Server
	Address() string
	IsUDS() bool
	IsTLS() bool
	ID() int32
	IsShut() bool
	Open() error
	Shut() error
	OnConnClosed()
}

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

// Node is a member of backend. Nodes are not components.
type Node interface {
	// Methods
	setAddress(address string)
	setTLS()
	setWeight(weight int32)
	setKeepConns(keepConns int32)
	Backend() Backend
	ID() int32
	IsUDS() bool
	IsTLS() bool
	Maintain() // runner
	shutdown()
}

// Node_ is the parent for all backend nodes.
type Node_ struct {
	// Mixins
	_subsWaiter_ // usually for conns
	_shutdownable_
	// Assocs
	backend Backend
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
		head backendConn // head element
		tail backendConn // tail element
		qnty int         // size of the list
	}
}

func (n *Node_) Init(id int32, backend Backend) {
	n._shutdownable_.init()
	n.backend = backend
	n.id = id
}

func (n *Node_) setAddress(address string) {
	if address[0] == '@' { // abstract uds
		n.udsMode = true
	} else if _, err := os.Stat(address); err == nil { // normal uds
		n.udsMode = true
	}
	n.address = address
}
func (n *Node_) setTLS() {
	n.tlsMode = true
	n.tlsConfig = new(tls.Config)
}
func (n *Node_) setWeight(weight int32)       { n.weight = weight }
func (n *Node_) setKeepConns(keepConns int32) { n.keepConns = keepConns }

func (n *Node_) Backend() Backend { return n.backend }
func (n *Node_) ID() int32        { return n.id }
func (n *Node_) IsUDS() bool      { return n.udsMode }
func (n *Node_) IsTLS() bool      { return n.tlsMode }

func (n *Node_) markDown()    { n.down.Store(true) }
func (n *Node_) markUp()      { n.down.Store(false) }
func (n *Node_) isDown() bool { return n.down.Load() }

func (n *Node_) shutdown() {
	close(n.ShutChan) // notifies Maintain()
}

func (n *Node_) pullConn() backendConn {
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
func (n *Node_) pushConn(conn backendConn) {
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

// backendConn is the linked-list item.
type backendConn interface {
	// Methods
	getNext() backendConn
	setNext(next backendConn)
	Close() error
}

// BackendConn_ is the parent for backend conns.
type BackendConn_ struct {
	// Conn states (non-zeros)
	next    backendConn // the linked-list
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

func (c *BackendConn_) getNext() backendConn     { return c.next }
func (c *BackendConn_) setNext(next backendConn) { c.next = next }

// Stream_
type Stream_ struct {
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	region Region // a region-based memory pool
	// Stream states (zeros)
}

func (s *Stream_) onUse() { // for non-zeros
	s.region.Init()
}
func (s *Stream_) onEnd() { // for zeros
	s.region.Free()
}

func (s *Stream_) buffer256() []byte          { return s.stockBuffer[:] }
func (s *Stream_) unsafeMake(size int) []byte { return s.region.Make(size) }

const ( // array kinds
	arrayKindStock = iota // refers to stock buffer. must be 0
	arrayKindPool         // got from sync.Pool
	arrayKindMake         // made from make([]byte)
)

func makeTempName(p []byte, stageID int64, connID int64, unixTime int64, counter int64) int {
	// TODO: improvement
	// stageID(8) | connID(16) | seconds(32) | counter(8)
	stageID &= 0x7f
	connID &= 0xffff
	unixTime &= 0xffffffff
	counter &= 0xff
	return i64ToDec(stageID<<56|connID<<40|unixTime<<8|counter, p)
}

// hostnameTo
type hostnameTo[T Component] struct {
	hostname []byte // "example.com" for exact map, ".example.com" for suffix map, "www.example." for prefix map
	target   T
}

// tempFile is used to temporarily save request/response content in local file system.
type tempFile interface {
	Name() string // used by os.Remove()
	Write(p []byte) (n int, err error)
	Seek(offset int64, whence int) (ret int64, err error)
	Close() error
}

// fakeFile
var fakeFile _fakeFile

// _fakeFile implements tempFile.
type _fakeFile struct{}

func (f _fakeFile) Name() string                           { return "" }
func (f _fakeFile) Write(p []byte) (n int, err error)      { return }
func (f _fakeFile) Seek(int64, int) (ret int64, err error) { return }
func (f _fakeFile) Close() error                           { return nil }

// Region
type Region struct { // 512B
	blocks [][]byte  // the blocks. [<stocks>/make]
	stocks [4][]byte // for blocks. 96B
	block0 [392]byte // for blocks[0]
}

func (r *Region) Init() {
	r.blocks = r.stocks[0:1:cap(r.stocks)]                    // block0 always at 0
	r.stocks[0] = r.block0[:]                                 // first block is always block0
	binary.BigEndian.PutUint16(r.block0[cap(r.block0)-2:], 0) // reset used size of block0
}
func (r *Region) Make(size int) []byte { // good for a lot of small buffers
	if size <= 0 {
		BugExitln("bad size")
	}
	block := r.blocks[len(r.blocks)-1]
	edge := cap(block)
	ceil := edge - 2
	used := int(binary.BigEndian.Uint16(block[ceil:edge]))
	want := used + size
	if want <= 0 {
		BugExitln("size too large")
	}
	if want <= ceil {
		binary.BigEndian.PutUint16(block[ceil:edge], uint16(want))
		return block[used:want]
	}
	ceil = _4K - 2
	if size > ceil {
		return make([]byte, size)
	}
	block = Get4K()
	binary.BigEndian.PutUint16(block[ceil:_4K], uint16(size))
	r.blocks = append(r.blocks, block)
	return block[0:size]
}
func (r *Region) Free() {
	for i := 1; i < len(r.blocks); i++ {
		PutNK(r.blocks[i])
		r.blocks[i] = nil
	}
	if cap(r.blocks) != cap(r.stocks) {
		r.stocks = [4][]byte{}
		r.blocks = nil
	}
}

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

var errNodeDown = errors.New("node is down")
