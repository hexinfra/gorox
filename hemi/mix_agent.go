// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Agents for net, rpc, and web.

package hemi

import (
	"crypto/tls"
	"errors"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Backend_ is the parent for backends.
type Backend_[N Node] struct {
	// Parent
	Component_
	// Mixins
	// Assocs
	stage *Stage      // current stage
	nodes compList[N] // nodes of this backend
	// States
	dialTimeout  time.Duration // dial remote timeout
	readTimeout  time.Duration // read() timeout
	writeTimeout time.Duration // write() timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
	balancer     string        // roundRobin, ipHash, random, ...
	indexGet     func() int64  // ...
	nodeIndex    atomic.Int64  // for roundRobin. won't overflow because it is so large!
	numNodes     int64         // num of nodes
	healthCheck  any           // TODO
}

func (b *Backend_[N]) OnCreate(name string, stage *Stage) {
	b.MakeComp(name)
	b.stage = stage
	b.nodeIndex.Store(-1)
	b.healthCheck = nil // TODO
}
func (b *Backend_[N]) OnShutdown() {
	close(b.ShutChan) // notifies Maintain() which shutdown sub components
}

func (b *Backend_[N]) OnConfigure() {
	// dialTimeout
	b.ConfigureDuration("dialTimeout", &b.dialTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

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

	// aliveTimeout
	b.ConfigureDuration("aliveTimeout", &b.aliveTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 5*time.Second)

	// balancer
	b.ConfigureString("balancer", &b.balancer, func(value string) error {
		if value == "roundRobin" || value == "ipHash" || value == "random" {
			return nil
		}
		return errors.New(".balancer has an invalid value")
	}, "roundRobin")
}
func (b *Backend_[N]) OnPrepare() {
	switch b.balancer {
	case "roundRobin":
		b.indexGet = b.nextIndexByRoundRobin
	case "ipHash":
		b.indexGet = b.nextIndexByIPHash
	case "random":
		b.indexGet = b.nextIndexByRandom
	default:
		BugExitln("unknown balancer")
	}
	b.numNodes = int64(len(b.nodes))
}

func (b *Backend_[N]) ConfigureNodes() {
	b.nodes.walk(N.OnConfigure)
}
func (b *Backend_[N]) PrepareNodes() {
	b.nodes.walk(N.OnPrepare)
}

func (b *Backend_[N]) Maintain() { // runner
	for _, node := range b.nodes {
		b.IncSub()
		go node.Maintain()
	}
	<-b.ShutChan

	// Backend was told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		go node.OnShutdown()
	}
	b.WaitSubs() // nodes

	if DbgLevel() >= 2 {
		Printf("backend=%s done\n", b.Name())
	}
	b.stage.DecSub()
}

func (b *Backend_[N]) AddNode(node N) {
	node.setShell(node)
	b.nodes = append(b.nodes, node)
}

func (b *Backend_[N]) Stage() *Stage               { return b.stage }
func (b *Backend_[N]) DialTimeout() time.Duration  { return b.dialTimeout }
func (b *Backend_[N]) ReadTimeout() time.Duration  { return b.readTimeout }
func (b *Backend_[N]) WriteTimeout() time.Duration { return b.writeTimeout }
func (b *Backend_[N]) AliveTimeout() time.Duration { return b.aliveTimeout }

func (b *Backend_[N]) nextConnID() int64 { return b.connID.Add(1) }

func (b *Backend_[N]) nextIndex() int64 { return b.indexGet() }
func (b *Backend_[N]) nextIndexByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *Backend_[N]) nextIndexByIPHash() int64 {
	// TODO
	return 0
}
func (b *Backend_[N]) nextIndexByRandom() int64 {
	return rand.Int63n(b.numNodes)
}

// Node_ is the parent for all backend nodes.
type Node_ struct {
	// Parent
	Component_
	// Assocs
	backend Backend
	// States
	udsMode        bool        // uds or not
	tlsMode        bool        // tls or not
	tlsConfig      *tls.Config // TLS config if TLS is enabled
	address        string      // hostname:port, /path/to/unix.sock
	weight         int32       // 1, 22, 333, ...
	keepAliveConns int32       // max conns to keep alive
	down           atomic.Bool // TODO: false-sharing
	health         any         // TODO
}

func (n *Node_) OnCreate(name string, backend Backend) {
	n.MakeComp(name)
	n.backend = backend
	n.health = nil // TODO
}
func (n *Node_) OnShutdown() {
	close(n.ShutChan) // notifies Maintain() which close conns
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

	// keepAliveConns
	n.ConfigureInt32("keepAliveConns", &n.keepAliveConns, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad keepAliveConns in node")
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

func (n *Node_) closeConn(conn io.Closer) {
	conn.Close()
	n.DecSub()
}

// BackendConn_ is the parent for backend conns.
type BackendConn_ struct {
	// Conn states (non-zeros)
	id      int64 // the conn id
	backend Backend
	node    Node
	expire  time.Time // when the conn is considered expired
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastWrite time.Time    // deadline of last write operation
	lastRead  time.Time    // deadline of last read operation
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
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *BackendConn_) ID() int64        { return c.id }
func (c *BackendConn_) Backend() Backend { return c.backend }
func (c *BackendConn_) Node() Node       { return c.node }
func (c *BackendConn_) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *BackendConn_) IsUDS() bool { return c.node.IsUDS() }
func (c *BackendConn_) IsTLS() bool { return c.node.IsTLS() }

func (c *BackendConn_) isAlive() bool { return time.Now().Before(c.expire) }

// Server_ is the parent for all servers.
type Server_[G Gate] struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	gates []G    // a server has many gates
	// States
	readTimeout       time.Duration // read() timeout
	writeTimeout      time.Duration // write() timeout
	address           string        // hostname:port, /path/to/unix.sock
	colonPort         string        // like: ":9876"
	colonPortBytes    []byte        // []byte(colonPort)
	udsColonPort      string        // uds doesn't have a port. use this as port if server is listening at uds
	udsColonPortBytes []byte        // []byte(udsColonPort)
	udsMode           bool          // is address a unix domain socket?
	tlsMode           bool          // use tls to secure the transport?
	tlsConfig         *tls.Config   // set if tls mode is true
	maxConnsPerGate   int32         // max concurrent connections allowed per gate
	numGates          int32         // number of gates
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

	// udsColonPort
	s.ConfigureString("udsColonPort", &s.udsColonPort, nil, ":80")
	s.udsColonPortBytes = []byte(s.udsColonPort)

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

func (s *Server_[G]) Address() string { return s.address }
func (s *Server_[G]) ColonPort() string {
	if s.udsMode {
		return s.udsColonPort
	} else {
		return s.colonPort
	}
}
func (s *Server_[G]) ColonPortBytes() []byte {
	if s.udsMode {
		return s.udsColonPortBytes
	} else {
		return s.colonPortBytes
	}
}
func (s *Server_[G]) IsUDS() bool            { return s.udsMode }
func (s *Server_[G]) IsTLS() bool            { return s.tlsMode }
func (s *Server_[G]) TLSConfig() *tls.Config { return s.tlsConfig }
func (s *Server_[G]) MaxConnsPerGate() int32 { return s.maxConnsPerGate }

func (s *Server_[G]) NumGates() int32 { return s.numGates }

// Gate is the interface for all gates. Gates are not components.
type Gate interface {
	// Methods
	Server() Server
	Address() string
	ID() int32
	IsUDS() bool
	IsTLS() bool
	Open() error
	Shut() error
	IsShut() bool
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

// ServerConn_ is the parent for server conns.
type ServerConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id     int64 // the conn id
	server Server
	gate   Gate
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastRead  time.Time    // deadline of last read operation
	lastWrite time.Time    // deadline of last write operation
}

func (c *ServerConn_) OnGet(id int64, gate Gate) {
	c.id = id
	c.server = gate.Server()
	c.gate = gate
}
func (c *ServerConn_) OnPut() {
	c.server = nil
	c.gate = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
}

func (c *ServerConn_) ID() int64      { return c.id }
func (c *ServerConn_) Server() Server { return c.server }
func (c *ServerConn_) Gate() Gate     { return c.gate }
func (c *ServerConn_) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *ServerConn_) IsUDS() bool { return c.server.IsUDS() }
func (c *ServerConn_) IsTLS() bool { return c.server.IsTLS() }
