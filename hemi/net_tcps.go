// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCPS (TCP/TLS/UDS) reverse proxy.

package hemi

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
)

func init() {
	RegisterTCPSDealet("tcpsProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(tcpsProxy)
		d.onCreate(name, stage, router)
		return d
	})
	RegisterBackend("tcpsBackend", func(name string, stage *Stage) Backend {
		b := new(TCPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// TCPSRouter
type TCPSRouter struct {
	// Mixins
	router_[*TCPSRouter, *tcpsGate, TCPSDealet, *tcpsCase]
}

func (r *TCPSRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, tcpsDealetCreators)
}
func (r *TCPSRouter) OnShutdown() {
	r.router_.onShutdown()
}

func (r *TCPSRouter) OnConfigure() {
	r.router_.onConfigure()
	// configure r here
	r.configureSubs()
}
func (r *TCPSRouter) OnPrepare() {
	r.router_.onPrepare()
	// prepare r here
	r.prepareSubs()
}

func (r *TCPSRouter) createCase(name string) *tcpsCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(tcpsCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *TCPSRouter) Serve() { // runner
	if r.IsUDS() {
		r._serveUDS()
	} else if r.IsTLS() {
		r._serveTLS()
	} else {
		r._serveTCP()
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealets) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if Debug() >= 2 {
		Printf("tcpsRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}
func (r *TCPSRouter) _serveUDS() {
	gate := new(tcpsGate)
	gate.init(0, r)
	if err := gate.Open(); err != nil {
		EnvExitln(err.Error())
	}
	r.AddGate(gate)
	r.IncSub(1)
	go gate.serveUDS()
}
func (r *TCPSRouter) _serveTLS() {
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub(1)
		go gate.serveTLS()
	}
}
func (r *TCPSRouter) _serveTCP() {
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub(1)
		go gate.serveTCP()
	}
}

func (r *TCPSRouter) dispatch(conn *TCPSConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// TCPSDealet
type TCPSDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *TCPSConn) (dealt bool)
}

// TCPSDealet_
type TCPSDealet_ struct {
	// Mixins
	Component_
	// States
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSRouter, TCPSDealet]
	// States
	matcher func(kase *tcpsCase, conn *TCPSConn, value []byte) bool
}

func (c *tcpsCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := tcpsCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *tcpsCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *tcpsCase) isMatch(conn *TCPSConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *tcpsCase) execute(conn *TCPSConn) (dealt bool) {
	for _, dealet := range c.dealets {
		if dealt := dealet.Deal(conn); dealt {
			return true
		}
	}
	return false
}

var tcpsCaseMatchers = map[string]func(kase *tcpsCase, conn *TCPSConn, value []byte) bool{
	"==": (*tcpsCase).equalMatch,
	"^=": (*tcpsCase).prefixMatch,
	"$=": (*tcpsCase).suffixMatch,
	"*=": (*tcpsCase).containMatch,
	"~=": (*tcpsCase).regexpMatch,
	"!=": (*tcpsCase).notEqualMatch,
	"!^": (*tcpsCase).notPrefixMatch,
	"!$": (*tcpsCase).notSuffixMatch,
	"!*": (*tcpsCase).notContainMatch,
	"!~": (*tcpsCase).notRegexpMatch,
}

func (c *tcpsCase) equalMatch(conn *TCPSConn, value []byte) bool { // value == patterns
	return c.case_._equalMatch(value)
}
func (c *tcpsCase) prefixMatch(conn *TCPSConn, value []byte) bool { // value ^= patterns
	return c.case_._prefixMatch(value)
}
func (c *tcpsCase) suffixMatch(conn *TCPSConn, value []byte) bool { // value $= patterns
	return c.case_._suffixMatch(value)
}
func (c *tcpsCase) containMatch(conn *TCPSConn, value []byte) bool { // value *= patterns
	return c.case_._containMatch(value)
}
func (c *tcpsCase) regexpMatch(conn *TCPSConn, value []byte) bool { // value ~= patterns
	return c.case_._regexpMatch(value)
}
func (c *tcpsCase) notEqualMatch(conn *TCPSConn, value []byte) bool { // value != patterns
	return c.case_._notEqualMatch(value)
}
func (c *tcpsCase) notPrefixMatch(conn *TCPSConn, value []byte) bool { // value !^ patterns
	return c.case_._notPrefixMatch(value)
}
func (c *tcpsCase) notSuffixMatch(conn *TCPSConn, value []byte) bool { // value !$ patterns
	return c.case_._notSuffixMatch(value)
}
func (c *tcpsCase) notContainMatch(conn *TCPSConn, value []byte) bool { // value !* patterns
	return c.case_._notContainMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool { // value !~ patterns
	return c.case_._notRegexpMatch(value)
}

// tcpsGate is an opening gate of TCPSRouter.
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	// States
	listener *net.TCPListener // the real gate. set after open
}

func (g *tcpsGate) init(id int32, router *TCPSRouter) {
	g.Gate_.Init(id, router)
}

func (g *tcpsGate) Open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option
		return system.SetReusePort(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.listener = listener.(*net.TCPListener)
	}
	return err
}
func (g *tcpsGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *tcpsGate) serveTCP() { // runner
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				continue
			}
			conn := getTCPSConn(connID, g, tcpConn, rawConn)
			if Debug() >= 1 {
				Printf("%+v\n", conn)
			}
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("tcpsGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}
func (g *tcpsGate) serveTLS() { // runner
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			conn := getTCPSConn(connID, g, tlsConn, nil)
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("tcpsGate=%d TLS done\n", g.id)
	}
	g.server.SubDone()
}
func (g *tcpsGate) serveUDS() { // runner
	// TODO
}

func (g *tcpsGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
}

// poolTCPSConn
var poolTCPSConn sync.Pool

func getTCPSConn(id int64, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) *TCPSConn {
	var tcpsConn *TCPSConn
	if x := poolTCPSConn.Get(); x == nil {
		tcpsConn = new(TCPSConn)
	} else {
		tcpsConn = x.(*TCPSConn)
	}
	tcpsConn.onGet(id, gate, netConn, rawConn)
	return tcpsConn
}
func putTCPSConn(tcpsConn *TCPSConn) {
	tcpsConn.onPut()
	poolTCPSConn.Put(tcpsConn)
}

// TCPSConn is a TCP/TLS/UDS connection coming from TCPSRouter.
type TCPSConn struct {
	// Mixins
	ServerConn_
	// Conn states (stocks)
	stockInput  [8192]byte // for c.input
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn   net.Conn        // the connection (TCP/TLS/UDS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	region    Region          // a region-based memory pool
	input     []byte          // input buffer
	closeSema atomic.Int32
	// Conn states (zeros)
	tcpsConn0
}
type tcpsConn0 struct { // for fast reset, entirely
}

func (c *TCPSConn) onGet(id int64, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c.netConn = netConn
	c.rawConn = rawConn
	c.region.Init()
	c.input = c.stockInput[:]
	c.closeSema.Store(2)
}
func (c *TCPSConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.region.Free()
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
		c.input = nil
	}
	c.tcpsConn0 = tcpsConn0{}
	c.ServerConn_.OnPut()
}

func (c *TCPSConn) serve() { // runner
	router := c.Server().(*TCPSRouter)
	router.dispatch(c)
	c.closeConn()
	putTCPSConn(c)
}

func (c *TCPSConn) Recv() (p []byte, err error) { // p == nil means EOF
	// TODO: deadline
	n, err := c.netConn.Read(c.input)
	if n > 0 {
		p = c.input[:n]
	}
	if err != nil {
		c._checkClose()
	}
	return
}
func (c *TCPSConn) Send(p []byte) (err error) { // if p is nil, send EOF
	// TODO: deadline
	if p == nil {
		c.closeWrite()
		c._checkClose()
	} else {
		_, err = c.netConn.Write(p)
	}
	return
}
func (c *TCPSConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.closeConn()
	}
}

func (c *TCPSConn) closeWrite() {
	if router := c.Server(); router.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else if router.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TCPSConn) closeConn() {
	c.netConn.Close()
	c.gate.OnConnClosed()
}

func (c *TCPSConn) unsafeVariable(code int16, name string) (value []byte) {
	return tcpsConnVariables[code](c)
}

// tcpsConnVariables
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varCodes in config.go
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isUDS
	nil, // isTLS
	nil, // serverName
	nil, // nextProto
}

// tcpsProxy passes TCPS connections to another/backend TCPS server.
type tcpsProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage   *Stage      // current stage
	router  *TCPSRouter // the router to which the dealet belongs
	backend *TCPSBackend
	// States
}

func (d *tcpsProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpsProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *tcpsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				d.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpsProxy proxy")
	}
}
func (d *tcpsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpsProxy) Deal(conn *TCPSConn) (dealt bool) {
	// TODO
	return true
}

// TCPSBackend component.
type TCPSBackend struct {
	// Mixins
	Backend_[*tcpsNode]
	_streamHolder_
	_loadBalancer_
	// States
	health any // TODO
}

func (b *TCPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage, b.NewNode)
	b._loadBalancer_.init()
}

func (b *TCPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._streamHolder_.onConfigure(b, 1000)
	b._loadBalancer_.onConfigure(b)
}
func (b *TCPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._streamHolder_.onPrepare(b)
	b._loadBalancer_.onPrepare(len(b.nodes))
}

func (b *TCPSBackend) NewNode(id int32) *tcpsNode {
	node := new(tcpsNode)
	node.init(id, b)
	return node
}

func (b *TCPSBackend) FetchConn() (*TConn, error) {
	return b.nodes[b.getNext()].fetchConn()
}
func (b *TCPSBackend) StoreConn(tConn *TConn) {
	tConn.node.(*tcpsNode).storeConn(tConn)
}

func (b *TCPSBackend) Dial() (*TConn, error) {
	return b.nodes[b.getNext()].dial()
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Mixins
	Node_
	// Assocs
	// States
}

func (n *tcpsNode) init(id int32, backend *TCPSBackend) {
	n.Node_.Init(id, backend)
}

func (n *tcpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.IncSub(0 - size)
	}
	n.WaitSubs() // conns
	if Debug() >= 2 {
		Printf("tcpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *tcpsNode) fetchConn() (*TConn, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		tConn := conn.(*TConn)
		if tConn.isAlive() && !tConn.reachLimit() && !down {
			return tConn, nil
		}
		n.closeConn(tConn)
	}
	if down {
		return nil, errNodeDown
	}
	tConn, err := n.dial()
	if err == nil {
		n.IncSub(1)
	}
	return tConn, err
}
func (n *tcpsNode) storeConn(tConn *TConn) {
	if tConn.IsBroken() || n.isDown() || !tConn.isAlive() {
		if Debug() >= 2 {
			Printf("TConn[node=%d id=%d] closed\n", tConn.node.ID(), tConn.id)
		}
		n.closeConn(tConn)
	} else {
		if Debug() >= 2 {
			Printf("TConn[node=%d id=%d] pushed\n", tConn.node.ID(), tConn.id)
		}
		n.pushConn(tConn)
	}
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
	if Debug() >= 2 {
		Printf("tcpsNode=%d dial %s\n", n.id, n.address)
	}
	if n.IsUDS() {
		return n._dialUDS()
	} else if n.IsTLS() {
		return n._dialTLS()
	} else {
		return n._dialTCP()
	}
}
func (n *tcpsNode) _dialUDS() (*TConn, error) {
	// TODO
	return nil, nil
}
func (n *tcpsNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if Debug() >= 2 {
		Printf("tcpsNode=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	tlsConn := tls.Client(netConn, n.tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	return getTConn(connID, n, tlsConn, nil), nil
}
func (n *tcpsNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if Debug() >= 2 {
		Printf("tcpsNode=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

func (n *tcpsNode) closeConn(tConn *TConn) {
	tConn.Close()
	n.SubDone()
}

// poolTConn
var poolTConn sync.Pool

func getTConn(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var tConn *TConn
	if x := poolTConn.Get(); x == nil {
		tConn = new(TConn)
	} else {
		tConn = x.(*TConn)
	}
	tConn.onGet(id, node, netConn, rawConn)
	return tConn
}
func putTConn(tConn *TConn) {
	tConn.onPut()
	poolTConn.Put(tConn)
}

// TConn is a backend-side connection to tcpsNode.
type TConn struct {
	// Mixins
	BackendConn_
	// Conn states (non-zeros)
	netConn    net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn    syscall.RawConn // for syscall. only usable when netConn is TCP/UDS
	maxStreams int32           // how many streams are allowed on this conn?
	// Conn states (zeros)
	counter     atomic.Int64 // used to make temp name
	usedStreams atomic.Int32 // how many streams has been used?
	writeBroken atomic.Bool  // write-side broken?
	readBroken  atomic.Bool  // read-side broken?
}

func (c *TConn) onGet(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c.netConn = netConn
	c.rawConn = rawConn
	c.maxStreams = node.Backend().(*TCPSBackend).MaxStreamsPerConn()
}
func (c *TConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *TConn) TCPConn() *net.TCPConn  { return c.netConn.(*net.TCPConn) }
func (c *TConn) TLSConn() *tls.Conn     { return c.netConn.(*tls.Conn) }
func (c *TConn) UDSConn() *net.UnixConn { return c.netConn.(*net.UnixConn) }

func (c *TConn) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *TConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *TConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *TConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *TConn) Write(p []byte) (n int, err error)         { return c.netConn.Write(p) }
func (c *TConn) Writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(c.netConn) }
func (c *TConn) Read(p []byte) (n int, err error)          { return c.netConn.Read(p) }
func (c *TConn) ReadFull(p []byte) (n int, err error)      { return io.ReadFull(c.netConn, p) }
func (c *TConn) ReadAtLeast(p []byte, min int) (n int, err error) {
	return io.ReadAtLeast(c.netConn, p, min)
}

func (c *TConn) IsBroken() bool { return c.writeBroken.Load() || c.readBroken.Load() }
func (c *TConn) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}

func (c *TConn) markWriteBroken() { c.writeBroken.Store(true) }
func (c *TConn) markReadBroken()  { c.readBroken.Store(true) }

func (c *TConn) CloseWrite() error {
	if c.IsUDS() {
		return c.netConn.(*net.UnixConn).CloseWrite()
	} else if c.IsTLS() {
		return c.netConn.(*tls.Conn).CloseWrite()
	} else {
		return c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TConn) Close() error {
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}
