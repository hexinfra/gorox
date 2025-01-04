// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/UDS) router, reverse proxy, and backend implementation. See RFC 768 and RFC 8085.

package hemi

import (
	"net"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

//////////////////////////////////////// UDPX router implementation ////////////////////////////////////////

// UDPXRouter
type UDPXRouter struct {
	// Parent
	Server_[*udpxGate]
	// Assocs
	dealets compDict[UDPXDealet] // defined dealets. indexed by name
	cases   compList[*udpxCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *LogConfig // ...
	logger    *Logger    // router access logger
}

func (r *UDPXRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[UDPXDealet])
}

func (r *UDPXRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO

	// sub components
	r.dealets.walk(UDPXDealet.OnConfigure)
	r.cases.walk((*udpxCase).OnConfigure)
}
func (r *UDPXRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog)
	}

	// sub components
	r.dealets.walk(UDPXDealet.OnPrepare)
	r.cases.walk((*udpxCase).OnPrepare)
}

func (r *UDPXRouter) createDealet(sign string, name string) UDPXDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := udpxDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
}
func (r *UDPXRouter) createCase(name string) *udpxCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(udpxCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *UDPXRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *UDPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpxGate)
		gate.onNew(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		if r.IsUDS() {
			go gate.serveUDS()
		} else {
			go gate.serveUDP()
		}
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	r.cases.walk((*udpxCase).OnShutdown)
	r.dealets.walk(UDPXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("udpxRouter=%s done\n", r.Name())
	}
	r.stage.DecSub() // router
}

func (r *UDPXRouter) Log(str string) {
	if r.logger != nil {
		r.logger.Log(str)
	}
}
func (r *UDPXRouter) Logln(str string) {
	if r.logger != nil {
		r.logger.Logln(str)
	}
}
func (r *UDPXRouter) Logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Logf(format, args...)
	}
}

func (r *UDPXRouter) serveConn(conn *UDPXConn) { // runner
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
	putUDPXConn(conn)
}

// udpxGate is an opening gate of UDPXRouter.
type udpxGate struct {
	// Parent
	Gate_
	// Assocs
	router *UDPXRouter
	// States
}

func (g *udpxGate) onNew(id int32, router *UDPXRouter) {
	g.Gate_.OnNew(id)
	g.router = router
}

func (g *udpxGate) Server() Server  { return g.router }
func (g *udpxGate) Address() string { return g.router.Address() }
func (g *udpxGate) IsUDS() bool     { return g.router.IsUDS() }
func (g *udpxGate) IsTLS() bool     { return false } // is DTLS useful?

func (g *udpxGate) Open() error {
	// TODO
	return nil
}
func (g *udpxGate) Shut() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpxGate) serveUDS() { // runner
	// TODO
}
func (g *udpxGate) serveUDP() { // runner
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.router.DecSub() // gate
}

func (g *udpxGate) justClose(pktConn net.PacketConn) {
	pktConn.Close()
	g.DecConn()
}

// UDPXConn
type UDPXConn struct {
	// Parent
	udpxConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *udpxGate
	// Conn states (zeros)
}

var poolUDPXConn sync.Pool

func getUDPXConn(id int64, gate *udpxGate, pktConn net.PacketConn, rawConn syscall.RawConn) *UDPXConn {
	var conn *UDPXConn
	if x := poolUDPXConn.Get(); x == nil {
		conn = new(UDPXConn)
	} else {
		conn = x.(*UDPXConn)
	}
	conn.onGet(id, gate, pktConn, rawConn)
	return conn
}
func putUDPXConn(conn *UDPXConn) {
	conn.onPut()
	poolUDPXConn.Put(conn)
}

func (c *UDPXConn) onGet(id int64, gate *udpxGate, pktConn net.PacketConn, rawConn syscall.RawConn) {
	router := gate.router
	c.udpxConn_.onGet(id, router.Stage().ID(), pktConn, rawConn, router.IsUDS())

	c.gate = gate
}
func (c *UDPXConn) onPut() {
	c.gate = nil

	c.udpxConn_.onPut()
}

func (c *UDPXConn) Close() error {
	pktConn := c.pktConn
	putUDPXConn(c)
	return pktConn.Close()
}

func (c *UDPXConn) unsafeVariable(code int16, name string) (value []byte) {
	return udpxConnVariables[code](c)
}

// udpxConnVariables
var udpxConnVariables = [...]func(*UDPXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // isUDS
}

// udpxCase
type udpxCase struct {
	// Parent
	Component_
	// Assocs
	router  *UDPXRouter
	dealets []UDPXDealet
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
	matcher  func(kase *udpxCase, conn *UDPXConn, value []byte) bool
}

func (c *udpxCase) onCreate(name string, router *UDPXRouter) {
	c.MakeComp(name)
	c.router = router
}
func (c *udpxCase) OnShutdown() {
	c.router.DecSub() // case
}

func (c *udpxCase) OnConfigure() {
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
	if matcher, ok := udpxCaseMatchers[cond.compare]; ok {
		c.matcher = matcher
	} else {
		UseExitln("unknown compare in case condition")
	}
}
func (c *udpxCase) OnPrepare() {
}

func (c *udpxCase) addDealet(dealet UDPXDealet) { c.dealets = append(c.dealets, dealet) }

func (c *udpxCase) isMatch(conn *UDPXConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *udpxCase) execute(conn *UDPXConn) (dealt bool) {
	for _, dealet := range c.dealets {
		if dealt := dealet.Deal(conn); dealt {
			return true
		}
	}
	return false
}

var udpxCaseMatchers = map[string]func(kase *udpxCase, conn *UDPXConn, value []byte) bool{
	"==": (*udpxCase).equalMatch,
	"^=": (*udpxCase).prefixMatch,
	"$=": (*udpxCase).suffixMatch,
	"*=": (*udpxCase).containMatch,
	"~=": (*udpxCase).regexpMatch,
	"!=": (*udpxCase).notEqualMatch,
	"!^": (*udpxCase).notPrefixMatch,
	"!$": (*udpxCase).notSuffixMatch,
	"!*": (*udpxCase).notContainMatch,
	"!~": (*udpxCase).notRegexpMatch,
}

func (c *udpxCase) equalMatch(conn *UDPXConn, value []byte) bool { // value == patterns
	return equalMatch(value, c.patterns)
}
func (c *udpxCase) prefixMatch(conn *UDPXConn, value []byte) bool { // value ^= patterns
	return prefixMatch(value, c.patterns)
}
func (c *udpxCase) suffixMatch(conn *UDPXConn, value []byte) bool { // value $= patterns
	return suffixMatch(value, c.patterns)
}
func (c *udpxCase) containMatch(conn *UDPXConn, value []byte) bool { // value *= patterns
	return containMatch(value, c.patterns)
}
func (c *udpxCase) regexpMatch(conn *UDPXConn, value []byte) bool { // value ~= patterns
	return regexpMatch(value, c.regexps)
}
func (c *udpxCase) notEqualMatch(conn *UDPXConn, value []byte) bool { // value != patterns
	return notEqualMatch(value, c.patterns)
}
func (c *udpxCase) notPrefixMatch(conn *UDPXConn, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, c.patterns)
}
func (c *udpxCase) notSuffixMatch(conn *UDPXConn, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, c.patterns)
}
func (c *udpxCase) notContainMatch(conn *UDPXConn, value []byte) bool { // value !* patterns
	return notContainMatch(value, c.patterns)
}
func (c *udpxCase) notRegexpMatch(conn *UDPXConn, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, c.regexps)
}

// UDPXDealet
type UDPXDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *UDPXConn) (dealt bool)
}

// UDPXDealet_
type UDPXDealet_ struct {
	// Parent
	Component_
	// States
}

//////////////////////////////////////// UDPX reverse proxy implementation ////////////////////////////////////////

func init() {
	RegisterUDPXDealet("udpxProxy", func(name string, stage *Stage, router *UDPXRouter) UDPXDealet {
		d := new(udpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// udpxProxy dealet passes UDPX connections to UDPX backends.
type udpxProxy struct {
	// Parent
	UDPXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *UDPXRouter  // the router to which the dealet belongs
	backend *UDPXBackend // the backend to pass to
	// States
	UDPXProxyConfig // embeded
}

func (d *udpxProxy) onCreate(name string, stage *Stage, router *UDPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpxProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *udpxProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpxBackend, ok := backend.(*UDPXBackend); ok {
				d.backend = udpxBackend
			} else {
				UseExitf("incorrect backend '%s' for udpxProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpxProxy")
	}
}
func (d *udpxProxy) OnPrepare() {
}

func (d *udpxProxy) Deal(conn *UDPXConn) (dealt bool) {
	UDPXReverseProxy(conn, d.backend, &d.UDPXProxyConfig)
	return true
}

// UDPXProxyConfig
type UDPXProxyConfig struct {
	// TODO
}

// UDPXReverseProxy
func UDPXReverseProxy(conn *UDPXConn, backend *UDPXBackend, proxyConfig *UDPXProxyConfig) {
	// TODO
}

//////////////////////////////////////// UDPX backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("udpxBackend", func(name string, stage *Stage) Backend {
		b := new(UDPXBackend)
		b.onCreate(name, stage)
		return b
	})
}

// UDPXBackend component.
type UDPXBackend struct {
	// Parent
	Backend_[*udpxNode]
	// States
}

func (b *UDPXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *UDPXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *UDPXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *UDPXBackend) CreateNode(name string) Node {
	node := new(udpxNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *UDPXBackend) Dial() (*UConn, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.dial()
}

// udpxNode is a node in UDPXBackend.
type udpxNode struct {
	// Parent
	Node_
	// Assocs
	backend *UDPXBackend
	// States
}

func (n *udpxNode) onCreate(name string, backend *UDPXBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *udpxNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *udpxNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *udpxNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("udpxNode=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *udpxNode) dial() (*UConn, error) {
	// TODO. note: use n.IncSub()?
	return nil, nil
}

// UConn
type UConn struct {
	// Parent
	udpxConn_
	// Conn states (non-zeros)
	node *udpxNode
	// Conn states (zeros)
}

var poolUConn sync.Pool

func getUConn(id int64, node *udpxNode, pktConn net.PacketConn, rawConn syscall.RawConn) *UConn {
	var conn *UConn
	if x := poolUConn.Get(); x == nil {
		conn = new(UConn)
	} else {
		conn = x.(*UConn)
	}
	conn.onGet(id, node, pktConn, rawConn)
	return conn
}
func putUConn(conn *UConn) {
	conn.onPut()
	poolUConn.Put(conn)
}

func (c *UConn) onGet(id int64, node *udpxNode, pktConn net.PacketConn, rawConn syscall.RawConn) {
	backend := node.backend
	c.udpxConn_.onGet(id, backend.Stage().ID(), pktConn, rawConn, node.IsUDS())

	c.node = node
}
func (c *UConn) onPut() {
	c.node = nil

	c.udpxConn_.onPut()
}

func (c *UConn) Close() error {
	// TODO: c.node.DecSub()?
	pktConn := c.pktConn
	putUConn(c)
	return pktConn.Close()
}

//////////////////////////////////////// UDPX in/out implementation ////////////////////////////////////////

// udpxConn
type udpxConn interface {
}

// udpxConn_
type udpxConn_ struct {
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stageID int32
	udsMode bool
	pktConn net.PacketConn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastRead  time.Time    // deadline of last read operation
	lastWrite time.Time    // deadline of last write operation
	broken    atomic.Bool
}

func (c *udpxConn_) onGet(id int64, stageID int32, pktConn net.PacketConn, rawConn syscall.RawConn, udsMode bool) {
	c.id = id
	c.stageID = stageID
	c.pktConn = pktConn
	c.rawConn = rawConn
	c.udsMode = udsMode
}
func (c *udpxConn_) onPut() {
	c.pktConn = nil
	c.rawConn = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.broken.Store(false)
}

func (c *udpxConn_) IsUDS() bool { return c.udsMode }

func (c *udpxConn_) MakeTempName(to []byte, unixTime int64) int {
	return makeTempName(to, c.stageID, c.id, unixTime, c.counter.Add(1))
}

func (c *udpxConn_) markBroken()    { c.broken.Store(true) }
func (c *udpxConn_) isBroken() bool { return c.broken.Load() }

func (c *udpxConn_) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.pktConn.WriteTo(p, addr)
}
func (c *udpxConn_) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	return c.pktConn.ReadFrom(p)
}
