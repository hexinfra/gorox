// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDPS (UDP/TLS/UDS) reverse proxy.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterUDPSDealet("udpsProxy", func(name string, stage *Stage, router *UDPSRouter) UDPSDealet {
		d := new(udpsProxy)
		d.onCreate(name, stage, router)
		return d
	})
	RegisterBackend("udpsBackend", func(name string, stage *Stage) Backend {
		b := new(UDPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// UDPSRouter
type UDPSRouter struct {
	// Mixins
	router_[*UDPSRouter, *udpsGate, UDPSDealet, *udpsCase]
}

func (r *UDPSRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, udpsDealetCreators)
}
func (r *UDPSRouter) OnShutdown() {
	r.router_.onShutdown()
}

func (r *UDPSRouter) OnConfigure() {
	r.router_.onConfigure()
	// TODO: configure r
	r.configureSubs()
}
func (r *UDPSRouter) OnPrepare() {
	r.router_.onPrepare()
	// TODO: prepare r
	r.prepareSubs()
}

func (r *UDPSRouter) createCase(name string) *udpsCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(udpsCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *UDPSRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpsGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub(1)
		if r.IsUDS() {
			go gate.serveUDS()
		} else if r.IsTLS() {
			go gate.serveTLS()
		} else {
			go gate.serveUDP()
		}
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealets) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if Debug() >= 2 {
		Printf("udpsRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

func (r *UDPSRouter) dispatch(conn *UDPSConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// UDPSDealet
type UDPSDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *UDPSConn) (dealt bool)
}

// UDPSDealet_
type UDPSDealet_ struct {
	// Mixins
	Component_
	// States
}

// udpsCase
type udpsCase struct {
	// Mixins
	case_[*UDPSRouter, UDPSDealet]
	// States
	matcher func(kase *udpsCase, conn *UDPSConn, value []byte) bool
}

func (c *udpsCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := udpsCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *udpsCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *udpsCase) isMatch(conn *UDPSConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *udpsCase) execute(conn *UDPSConn) (dealt bool) {
	for _, dealet := range c.dealets {
		if dealt := dealet.Deal(conn); dealt {
			return true
		}
	}
	return false
}

var udpsCaseMatchers = map[string]func(kase *udpsCase, conn *UDPSConn, value []byte) bool{
	"==": (*udpsCase).equalMatch,
	"^=": (*udpsCase).prefixMatch,
	"$=": (*udpsCase).suffixMatch,
	"*=": (*udpsCase).containMatch,
	"~=": (*udpsCase).regexpMatch,
	"!=": (*udpsCase).notEqualMatch,
	"!^": (*udpsCase).notPrefixMatch,
	"!$": (*udpsCase).notSuffixMatch,
	"!*": (*udpsCase).notContainMatch,
	"!~": (*udpsCase).notRegexpMatch,
}

func (c *udpsCase) equalMatch(conn *UDPSConn, value []byte) bool { // value == patterns
	return c.case_._equalMatch(value)
}
func (c *udpsCase) prefixMatch(conn *UDPSConn, value []byte) bool { // value ^= patterns
	return c.case_._prefixMatch(value)
}
func (c *udpsCase) suffixMatch(conn *UDPSConn, value []byte) bool { // value $= patterns
	return c.case_._suffixMatch(value)
}
func (c *udpsCase) containMatch(conn *UDPSConn, value []byte) bool { // value *= patterns
	return c.case_._containMatch(value)
}
func (c *udpsCase) regexpMatch(conn *UDPSConn, value []byte) bool { // value ~= patterns
	return c.case_._regexpMatch(value)
}
func (c *udpsCase) notEqualMatch(conn *UDPSConn, value []byte) bool { // value != patterns
	return c.case_._notEqualMatch(value)
}
func (c *udpsCase) notPrefixMatch(conn *UDPSConn, value []byte) bool { // value !^ patterns
	return c.case_._notPrefixMatch(value)
}
func (c *udpsCase) notSuffixMatch(conn *UDPSConn, value []byte) bool { // value !$ patterns
	return c.case_._notSuffixMatch(value)
}
func (c *udpsCase) notContainMatch(conn *UDPSConn, value []byte) bool { // value !* patterns
	return c.case_._notContainMatch(value)
}
func (c *udpsCase) notRegexpMatch(conn *UDPSConn, value []byte) bool { // value !~ patterns
	return c.case_._notRegexpMatch(value)
}

// udpsGate is an opening gate of UDPSRouter.
type udpsGate struct {
	// Mixins
	Gate_
	// Assocs
	// States
}

func (g *udpsGate) init(id int32, router *UDPSRouter) {
	g.Gate_.Init(id, router)
}

func (g *udpsGate) Open() error {
	// TODO
	return nil
}
func (g *udpsGate) Shut() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveUDP() { // runner
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.server.SubDone()
}
func (g *udpsGate) serveTLS() { // runner
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.server.SubDone()
}
func (g *udpsGate) serveUDS() { // runner
	// TODO
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
	g.OnConnClosed()
}

// poolUDPSConn
var poolUDPSConn sync.Pool

func getUDPSConn(id int64, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) *UDPSConn {
	var udpsConn *UDPSConn
	if x := poolUDPSConn.Get(); x == nil {
		udpsConn = new(UDPSConn)
	} else {
		udpsConn = x.(*UDPSConn)
	}
	udpsConn.onGet(id, gate, udpConn, rawConn)
	return udpsConn
}
func putUDPSConn(udpsConn *UDPSConn) {
	udpsConn.onPut()
	poolUDPSConn.Put(udpsConn)
}

// UDPSConn
type UDPSConn struct {
	// Mixins
	ServerConn_
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	udpConn *net.UDPConn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *UDPSConn) onGet(id int64, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c.udpConn = udpConn
	c.rawConn = rawConn
}
func (c *UDPSConn) onPut() {
	c.udpConn = nil
	c.rawConn = nil
	c.ServerConn_.OnPut()
}

func (c *UDPSConn) serve() { // runner
	router := c.Server().(*UDPSRouter)
	router.dispatch(c)
	c.closeConn()
	putUDPSConn(c)
}

func (c *UDPSConn) Close() error {
	udpConn := c.udpConn
	putUDPSConn(c)
	return udpConn.Close()
}

func (c *UDPSConn) closeConn() {
	// TODO: uds, tls?
	if router := c.Server(); router.IsUDS() {
	} else if router.IsTLS() {
	} else {
	}
	c.udpConn.Close()
}

func (c *UDPSConn) unsafeVariable(code int16, name string) (value []byte) {
	return udpsConnVariables[code](c)
}

// udpsConnVariables
var udpsConnVariables = [...]func(*UDPSConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}

// udpsProxy passes UDPS (UDP/TLS/UDS) conns to backend UDPS server.
type udpsProxy struct {
	// Mixins
	UDPSDealet_
	// Assocs
	stage   *Stage // current stage
	router  *UDPSRouter
	backend *UDPSBackend
	// States
}

func (d *udpsProxy) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpsProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *udpsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpsBackend, ok := backend.(*UDPSBackend); ok {
				d.backend = udpsBackend
			} else {
				UseExitf("incorrect backend '%s' for udpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpsProxy")
	}
}
func (d *udpsProxy) OnPrepare() {
}

func (d *udpsProxy) Deal(conn *UDPSConn) (dealt bool) {
	// TODO
	return true
}

// UDPSBackend component.
type UDPSBackend struct {
	// Mixins
	Backend_[*udpsNode]
	_loadBalancer_
	// States
	health any // TODO
}

func (b *UDPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage, b.NewNode)
	b._loadBalancer_.init()
}

func (b *UDPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._loadBalancer_.onConfigure(b)
}
func (b *UDPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._loadBalancer_.onPrepare(len(b.nodes))
}

func (b *UDPSBackend) NewNode(id int32) *udpsNode {
	node := new(udpsNode)
	node.init(id, b)
	return node
}

func (b *UDPSBackend) FetchConn() (*UConn, error) {
	return b.nodes[b.getNext()].fetchConn()
}
func (b *UDPSBackend) StoreConn(uConn *UConn) {
	uConn.node.(*udpsNode).storeConn(uConn)
}

func (b *UDPSBackend) Dial() (*UConn, error) {
	return b.nodes[b.getNext()].dial()
}

// udpsNode is a node in UDPSBackend.
type udpsNode struct {
	// Mixins
	Node_
	// Assocs
	// States
}

func (n *udpsNode) init(id int32, backend *UDPSBackend) {
	n.Node_.Init(id, backend)
}

func (n *udpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("udpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *udpsNode) fetchConn() (*UConn, error) {
	conn := n.pullConn()
	if conn != nil {
		uConn := conn.(*UConn)
		if uConn.isAlive() {
			return uConn, nil
		}
		n.closeConn(uConn)
	}
	return n.dial()
}
func (n *udpsNode) storeConn(uConn *UConn) {
	if uConn.isBroken() || n.isDown() || !uConn.isAlive() {
		n.closeConn(uConn)
	} else {
		n.pushConn(uConn)
	}
}

func (n *udpsNode) dial() (*UConn, error) {
	// TODO
	return nil, nil
}

func (n *udpsNode) closeConn(uConn *UConn) {
	uConn.Close()
	n.SubDone()
}

// poolUConn
var poolUConn sync.Pool

func getUConn(id int64, node *udpsNode, netConn net.PacketConn, rawConn syscall.RawConn) *UConn {
	var uConn *UConn
	if x := poolUConn.Get(); x == nil {
		uConn = new(UConn)
	} else {
		uConn = x.(*UConn)
	}
	uConn.onGet(id, node, netConn, rawConn)
	return uConn
}
func putUConn(uConn *UConn) {
	uConn.onPut()
	poolUConn.Put(uConn)
}

// UConn
type UConn struct {
	// Mixins
	BackendConn_
	// Conn states (non-zeros)
	netConn net.PacketConn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	broken atomic.Bool // is conn broken?
}

func (c *UConn) onGet(id int64, node *udpsNode, netConn net.PacketConn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *UConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.broken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *UConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *UConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *UConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.netConn.WriteTo(p, addr)
}
func (c *UConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) { return c.netConn.ReadFrom(p) }

func (c *UConn) isBroken() bool { return c.broken.Load() }
func (c *UConn) markBroken()    { c.broken.Store(true) }

func (c *UConn) Close() error {
	netConn := c.netConn
	putUConn(c)
	return netConn.Close()
}
