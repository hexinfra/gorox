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
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
)

// broker is the interface for Server and Backend.
type broker interface {
	// Imports
	Component
	// Methods
	Stage() *Stage
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// broker_ is the mixin for Server_ and Backend_.
type broker_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	// States
	readTimeout  time.Duration // read() timeout
	writeTimeout time.Duration // write() timeout
}

func (b *broker_) onCreate(name string, stage *Stage) {
	b.MakeComp(name)
	b.stage = stage
}

func (b *broker_) onConfigure(shell Component, readTimeout time.Duration, writeTimeout time.Duration) {
	// readTimeout
	shell.ConfigureDuration("readTimeout", &b.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, readTimeout)

	// writeTimeout
	shell.ConfigureDuration("writeTimeout", &b.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, writeTimeout)
}
func (b *broker_) onPrepare() {
	// Currently nothing.
}

func (b *broker_) Stage() *Stage               { return b.stage }
func (b *broker_) ReadTimeout() time.Duration  { return b.readTimeout }
func (b *broker_) WriteTimeout() time.Duration { return b.writeTimeout }

// Server component.
type Server interface {
	// Imports
	broker
	// Methods
	Serve() // runner
	IsUDS() bool
	IsTLS() bool
	ColonPort() string
	ColonPortBytes() []byte
}

// Server_ is the mixin for all servers.
type Server_[G Gate] struct {
	// Mixins
	broker_
	// Assocs
	gates []G // a server may has many gates
	// States
	address         string      // hostname:port, /path/to/unix.sock
	colonPort       string      // like: ":9876"
	colonPortBytes  []byte      // like: []byte(":9876")
	udsMode         bool        // address is a unix domain socket?
	tlsMode         bool        // tls mode?
	tlsConfig       *tls.Config // set if is tls mode
	numGates        int32       // number of gates
	maxConnsPerGate int32       // max concurrent connections allowed per gate
}

func (s *Server_[G]) OnCreate(name string, stage *Stage) { // exported
	s.broker_.onCreate(name, stage)
}
func (s *Server_[G]) OnShutdown() {
	// Notify gates. We don't use close(s.ShutChan) here.
	for _, gate := range s.gates {
		gate.Shut()
	}
}

func (s *Server_[G]) OnConfigure() {
	s.broker_.onConfigure(s, 60*time.Second, 60*time.Second)

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

	// tlsMode
	s.ConfigureBool("tlsMode", &s.tlsMode, false)
	if s.tlsMode {
		s.tlsConfig = new(tls.Config)
	}

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
	s.broker_.onPrepare()
}

func (s *Server_[G]) AddGate(gate G) { s.gates = append(s.gates, gate) }

func (s *Server_[G]) Address() string        { return s.address }
func (s *Server_[G]) ColonPort() string      { return s.colonPort }
func (s *Server_[G]) ColonPortBytes() []byte { return s.colonPortBytes }
func (s *Server_[G]) IsUDS() bool            { return s.udsMode }
func (s *Server_[G]) IsTLS() bool            { return s.tlsMode }
func (s *Server_[G]) NumGates() int32        { return s.numGates }
func (s *Server_[G]) MaxConnsPerGate() int32 { return s.maxConnsPerGate }

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
	tlsMode  bool
	address  string       // listening address
	isShut   atomic.Bool  // is gate shut?
	maxConns int32        // max concurrent conns allowed
	numConns atomic.Int32 // TODO: false sharing
}

func (g *Gate_) Init(stage *Stage, id int32, udsMode bool, tlsMode bool, address string, maxConns int32) {
	g.stage = stage
	g.id = id
	g.udsMode = udsMode
	g.tlsMode = tlsMode
	g.address = address
	g.isShut.Store(false)
	g.maxConns = maxConns
	g.numConns.Store(0)
}

func (g *Gate_) Stage() *Stage   { return g.stage }
func (g *Gate_) ID() int32       { return g.id }
func (g *Gate_) Address() string { return g.address }
func (g *Gate_) IsUDS() bool     { return g.udsMode }
func (g *Gate_) IsTLS() bool     { return g.tlsMode }

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
	id     int64
	server Server
	gate   Gate
	// Conn states (zeros)
}

func (c *ServerConn_) onGet(id int64, server Server, gate Gate) {
	c.id = id
	c.server = server
	c.gate = gate
}
func (c *ServerConn_) onPut() {
	c.server = nil
	c.gate = nil
}

func (c *ServerConn_) IsUDS() bool { return c.server.IsUDS() }
func (c *ServerConn_) IsTLS() bool { return c.server.IsTLS() }

// Backend is a group of nodes.
type Backend interface {
	// Imports
	broker
	// Methods
	Maintain() // runner
	AliveTimeout() time.Duration
	nextConnID() int64
}

// Backend_ is the mixin for backends.
type Backend_[N Node] struct {
	// Mixins
	broker_
	// Assocs
	nodes   []N // nodes of this backend
	creator interface {
		CreateNode(id int32) N
	} // if Go's generic supports new(N) then this is not needed.
	// States
	dialTimeout  time.Duration // dial remote timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
}

func (b *Backend_[N]) OnCreate(name string, stage *Stage, creator interface{ CreateNode(id int32) N }) {
	b.broker_.onCreate(name, stage)
	b.creator = creator
}
func (b *Backend_[N]) OnShutdown() {
	close(b.ShutChan) // notifies run() or Maintain()
}

func (b *Backend_[N]) OnConfigure() {
	b.broker_.onConfigure(b, 30*time.Second, 30*time.Second)

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
		node := b.creator.CreateNode(int32(id))

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
				node.setTLS()
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
func (b *Backend_[N]) OnPrepare() {
	b.broker_.onPrepare()
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

func (b *Backend_[N]) AliveTimeout() time.Duration { return b.aliveTimeout }

func (b *Backend_[N]) nextConnID() int64 { return b.connID.Add(1) }

// Node is a member of backend. Nodes are not components.
type Node interface {
	// Methods
	setAddress(address string)
	setTLS()
	setWeight(weight int32)
	setKeepConns(keepConns int32)
	ID() int32
	IsUDS() bool
	IsTLS() bool
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
func (n *Node_) setTLS() {
	n.tlsMode = true
	n.tlsConfig = new(tls.Config)
}
func (n *Node_) setWeight(weight int32)       { n.weight = weight }
func (n *Node_) setKeepConns(keepConns int32) { n.keepConns = keepConns }

func (n *Node_) ID() int32   { return n.id }
func (n *Node_) IsUDS() bool { return n.udsMode }
func (n *Node_) IsTLS() bool { return n.tlsMode }

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
	backend Backend
	node    Node
	expire  time.Time // when the conn is considered expired
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

func (c *BackendConn_) onGet(id int64, backend Backend, node Node) {
	c.id = id
	c.backend = backend
	c.node = node
	c.expire = time.Now().Add(backend.AliveTimeout())
}
func (c *BackendConn_) onPut() {
	c.backend = nil
	c.node = nil
	c.expire = time.Time{}
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *BackendConn_) IsUDS() bool { return c.node.IsUDS() }
func (c *BackendConn_) IsTLS() bool { return c.node.IsTLS() }

func (c *BackendConn_) getNext() BackendConn     { return c.next }
func (c *BackendConn_) setNext(next BackendConn) { c.next = next }

func (c *BackendConn_) isAlive() bool { return time.Now().Before(c.expire) }

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

// contentSaver_ is a mixin.
type contentSaver_ struct {
	// States
	saveContentFilesDir string
}

func (s *contentSaver_) onConfigure(shell Component, defaultDir string) {
	// saveContentFilesDir
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) error {
		if value != "" && len(value) <= 232 {
			return nil
		}
		return errors.New(".saveContentFilesDir has an invalid value")
	}, defaultDir)
}
func (s *contentSaver_) onPrepare(shell Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// streamHolder
type streamHolder interface {
	MaxStreamsPerConn() int32
}

// streamHolder_ is a mixin.
type streamHolder_ struct {
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (s *streamHolder_) onConfigure(shell Component, defaultMaxStreams int32) {
	// maxStreamsPerConn
	shell.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, defaultMaxStreams)
}
func (s *streamHolder_) onPrepare(shell Component) {
}

func (s *streamHolder_) MaxStreamsPerConn() int32 { return s.maxStreamsPerConn }

// loadBalancer_ is a mixin.
type loadBalancer_ struct {
	// States
	balancer  string       // roundRobin, ipHash, random, ...
	indexGet  func() int64 // ...
	nodeIndex atomic.Int64 // for roundRobin. won't overflow because it is so large!
	numNodes  int64        // num of nodes
}

func (b *loadBalancer_) init() {
	b.nodeIndex.Store(-1)
}

func (b *loadBalancer_) onConfigure(shell Component) {
	// balancer
	shell.ConfigureString("balancer", &b.balancer, func(value string) error {
		if value == "roundRobin" || value == "ipHash" || value == "random" {
			return nil
		}
		return errors.New(".balancer has an invalid value")
	}, "roundRobin")
}
func (b *loadBalancer_) onPrepare(numNodes int) {
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

func (b *loadBalancer_) getNext() int64 { return b.indexGet() }

func (b *loadBalancer_) getNextByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *loadBalancer_) getNextByIPHash() int64 {
	// TODO
	return 0
}
func (b *loadBalancer_) getNextByRandom() int64 {
	return rand.Int63n(b.numNodes)
}

// subsWaiter_ is a mixin.
type subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *subsWaiter_) IncSub(n int) { w.subs.Add(n) }
func (w *subsWaiter_) WaitSubs()    { w.subs.Wait() }
func (w *subsWaiter_) SubDone()     { w.subs.Done() }

// shutdownable_ is a mixin.
type shutdownable_ struct {
	ShutChan chan struct{} // used to notify target to shutdown
}

func (s *shutdownable_) init() {
	s.ShutChan = make(chan struct{})
}

func (s *shutdownable_) Loop(interval time.Duration, callback func(now time.Time)) {
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

// identifiable_ is a mixin.
type identifiable_ struct {
	id uint8
}

func (i *identifiable_) ID() uint8 { return i.id }

func (i *identifiable_) setID(id uint8) { i.id = id }

// logcfg
type logcfg struct {
	logFile string
	rotate  string
	format  string
	bufSize int
}

// logger is logger for routers, webapps, and services.
type logger struct {
	file   *os.File
	queue  chan string
	buffer []byte
	size   int
	used   int
}

func newLogger(logFile string) (*logger, error) {
	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	l := new(logger)
	l.file = file
	l.queue = make(chan string)
	l.buffer = make([]byte, 1048576)
	l.size = len(l.buffer)
	l.used = 0
	go l.saver()
	return l, nil
}

func (l *logger) Log(v ...any) {
	if s := fmt.Sprint(v...); s != "" {
		l.queue <- s
	}
}
func (l *logger) Logln(v ...any) {
	if s := fmt.Sprintln(v...); s != "" {
		l.queue <- s
	}
}
func (l *logger) Logf(f string, v ...any) {
	if s := fmt.Sprintf(f, v...); s != "" {
		l.queue <- s
	}
}

func (l *logger) Close() { l.queue <- "" }

func (l *logger) saver() { // runner
	for {
		s := <-l.queue
		if s == "" {
			goto over
		}
		l.write(s)
	more:
		for {
			select {
			case s = <-l.queue:
				if s == "" {
					goto over
				}
				l.write(s)
			default:
				l.clear()
				break more
			}
		}
	}
over:
	l.clear()
	l.file.Close()
}
func (l *logger) write(s string) {
	n := len(s)
	if n >= l.size {
		l.clear()
		l.flush(risky.ConstBytes(s))
		return
	}
	w := copy(l.buffer[l.used:], s)
	l.used += w
	if l.used == l.size {
		l.clear()
		if n -= w; n > 0 {
			copy(l.buffer, s[w:])
			l.used = n
		}
	}
}
func (l *logger) clear() {
	if l.used > 0 {
		l.flush(l.buffer[:l.used])
		l.used = 0
	}
}
func (l *logger) flush(p []byte) { l.file.Write(p) }

var errNodeDown = errors.New("node is down")
