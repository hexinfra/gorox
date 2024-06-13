// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPS (UDP/TLS/UDS) router, reverse proxy, and backend.

package hemi

import (
	"net"
	"regexp"
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
	// Parent
	Server_[*udpsGate]
	// Assocs
	dealets compDict[UDPSDealet] // defined dealets. indexed by name
	cases   compList[*udpsCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *logcfg // ...
	logger    *logger // router access logger
}

func (r *UDPSRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[UDPSDealet])
}
func (r *UDPSRouter) OnShutdown() {
	r.Server_.OnShutdown()
}

func (r *UDPSRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO

	// sub components
	r.dealets.walk(UDPSDealet.OnConfigure)
	r.cases.walk((*udpsCase).OnConfigure)
}
func (r *UDPSRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = newLogger(r.accessLog.logFile, r.accessLog.rotate)
	}

	// sub components
	r.dealets.walk(UDPSDealet.OnPrepare)
	r.cases.walk((*udpsCase).OnPrepare)
}

func (r *UDPSRouter) createDealet(sign string, name string) UDPSDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := udpsDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
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
func (r *UDPSRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *UDPSRouter) Log(str string) {
}
func (r *UDPSRouter) Logln(str string) {
}
func (r *UDPSRouter) Logf(str string) {
}

func (r *UDPSRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpsGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub()
		if r.IsTLS() {
			go gate.serveTLS()
		} else if r.IsUDS() {
			go gate.serveUDS()
		} else {
			go gate.serveUDP()
		}
	}
	r.WaitSubs() // gates

	r.SubsAddn(len(r.dealets) + len(r.cases))
	r.cases.walk((*udpsCase).OnShutdown)
	r.dealets.walk(UDPSDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("udpsRouter=%s done\n", r.Name())
	}
	r.stage.DecSub()
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

// udpsGate is an opening gate of UDPSRouter.
type udpsGate struct {
	// Parent
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
func (g *udpsGate) _openUnix() error {
	// TODO
	return nil
}
func (g *udpsGate) _openInet() error {
	// TODO
	return nil
}
func (g *udpsGate) Shut() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveTLS() { // runner
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.server.DecSub()
}
func (g *udpsGate) serveUDS() { // runner
	// TODO
}
func (g *udpsGate) serveUDP() { // runner
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.server.DecSub()
}

func (g *udpsGate) justClose(pktConn net.PacketConn) {
	pktConn.Close()
	g.OnConnClosed()
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
	// Parent
	Component_
	// States
}

// udpsCase
type udpsCase struct {
	// Parent
	Component_
	// Assocs
	router  *UDPSRouter
	dealets []UDPSDealet
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
	matcher  func(kase *udpsCase, conn *UDPSConn, value []byte) bool
}

func (c *udpsCase) onCreate(name string, router *UDPSRouter) {
	c.MakeComp(name)
	c.router = router
}
func (c *udpsCase) OnShutdown() {
	c.router.DecSub()
}

func (c *udpsCase) OnConfigure() {
	if c.info == nil {
		c.general = true
		return
	}
	cond := c.info.(caseCond)
	c.varCode = cond.varCode
	c.varName = cond.varName
	isRegexp := cond.compare == "~=" || cond.compare == "!~"
	for _, pattern := range cond.patterns {
		if pattern == "" {
			UseExitln("empty case cond pattern")
		}
		if !isRegexp {
			c.patterns = append(c.patterns, []byte(pattern))
		} else if exp, err := regexp.Compile(pattern); err == nil {
			c.regexps = append(c.regexps, exp)
		} else {
			UseExitln(err.Error())
		}
	}
	if matcher, ok := udpsCaseMatchers[cond.compare]; ok {
		c.matcher = matcher
	} else {
		UseExitln("unknown compare in case condition")
	}
}
func (c *udpsCase) OnPrepare() {
}

func (c *udpsCase) addDealet(dealet UDPSDealet) { c.dealets = append(c.dealets, dealet) }

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
	return equalMatch(value, c.patterns)
}
func (c *udpsCase) prefixMatch(conn *UDPSConn, value []byte) bool { // value ^= patterns
	return prefixMatch(value, c.patterns)
}
func (c *udpsCase) suffixMatch(conn *UDPSConn, value []byte) bool { // value $= patterns
	return suffixMatch(value, c.patterns)
}
func (c *udpsCase) containMatch(conn *UDPSConn, value []byte) bool { // value *= patterns
	return containMatch(value, c.patterns)
}
func (c *udpsCase) regexpMatch(conn *UDPSConn, value []byte) bool { // value ~= patterns
	return regexpMatch(value, c.regexps)
}
func (c *udpsCase) notEqualMatch(conn *UDPSConn, value []byte) bool { // value != patterns
	return notEqualMatch(value, c.patterns)
}
func (c *udpsCase) notPrefixMatch(conn *UDPSConn, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, c.patterns)
}
func (c *udpsCase) notSuffixMatch(conn *UDPSConn, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, c.patterns)
}
func (c *udpsCase) notContainMatch(conn *UDPSConn, value []byte) bool { // value !* patterns
	return notContainMatch(value, c.patterns)
}
func (c *udpsCase) notRegexpMatch(conn *UDPSConn, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, c.regexps)
}

// poolUDPSConn
var poolUDPSConn sync.Pool

func getUDPSConn(id int64, gate *udpsGate, pktConn net.PacketConn, rawConn syscall.RawConn) *UDPSConn {
	var udpsConn *UDPSConn
	if x := poolUDPSConn.Get(); x == nil {
		udpsConn = new(UDPSConn)
	} else {
		udpsConn = x.(*UDPSConn)
	}
	udpsConn.onGet(id, gate, pktConn, rawConn)
	return udpsConn
}
func putUDPSConn(udpsConn *UDPSConn) {
	udpsConn.onPut()
	poolUDPSConn.Put(udpsConn)
}

// UDPSConn
type UDPSConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	pktConn net.PacketConn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *UDPSConn) onGet(id int64, gate *udpsGate, pktConn net.PacketConn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c.pktConn = pktConn
	c.rawConn = rawConn
}
func (c *UDPSConn) onPut() {
	c.pktConn = nil
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
	pktConn := c.pktConn
	putUDPSConn(c)
	return pktConn.Close()
}

func (c *UDPSConn) closeConn() {
	// TODO: tls, uds?
	if router := c.Server(); router.IsTLS() {
	} else if router.IsUDS() {
	} else {
	}
	c.pktConn.Close()
}

func (c *UDPSConn) unsafeVariable(code int16, name string) (value []byte) {
	return udpsConnVariables[code](c)
}

// udpsConnVariables
var udpsConnVariables = [...]func(*UDPSConn) []byte{ // keep sync with varCodes
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isTLS
	nil, // isUDS
}

// udpsProxy passes UDPS connections to UDPS backends.
type udpsProxy struct {
	// Parent
	UDPSDealet_
	// Assocs
	stage   *Stage // current stage
	router  *UDPSRouter
	backend *UDPSBackend // the backend to pass to
	// States
}

func (d *udpsProxy) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpsProxy) OnShutdown() {
	d.router.DecSub()
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
	// Parent
	Backend_[*udpsNode]
	// Mixins
	// States
}

func (b *UDPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *UDPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *UDPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *UDPSBackend) CreateNode(name string) Node {
	node := new(udpsNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *UDPSBackend) Dial() (*UConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

// udpsNode is a node in UDPSBackend.
type udpsNode struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *udpsNode) onCreate(name string, backend *UDPSBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *udpsNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *udpsNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *udpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("udpsNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *udpsNode) dial() (*UConn, error) {
	// TODO. note: use n.IncSub()
	return nil, nil
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
	// Parent
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
