// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/TLS/UDS) router.

package hemi

import (
	"net"
	"regexp"
	"sync"
	"syscall"
	"time"
)

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
func (r *UDPXRouter) OnShutdown() {
	r.Server_.OnShutdown()
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
		//r.logger = NewLogger(r.accessLog.logFile, r.accessLog.rotate)
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

func (r *UDPXRouter) Log(str string) {
}
func (r *UDPXRouter) Logln(str string) {
}
func (r *UDPXRouter) Logf(str string) {
}

func (r *UDPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpxGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		if r.IsTLS() {
			go gate.serveTLS()
		} else if r.IsUDS() {
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

func (r *UDPXRouter) dispatch(conn *UDPXConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// udpxGate is an opening gate of UDPXRouter.
type udpxGate struct {
	// Parent
	Gate_
	// Assocs
	router *UDPXRouter
	// States
}

func (g *udpxGate) init(id int32, router *UDPXRouter) {
	g.Gate_.Init(id, router.MaxConnsPerGate())
	g.router = router
}

func (g *udpxGate) Server() Server  { return g.router }
func (g *udpxGate) Address() string { return g.router.Address() }
func (g *udpxGate) IsTLS() bool     { return g.router.IsTLS() }
func (g *udpxGate) IsUDS() bool     { return g.router.IsUDS() }

func (g *udpxGate) Open() error {
	// TODO
	return nil
}
func (g *udpxGate) _openUnix() error {
	// TODO
	return nil
}
func (g *udpxGate) _openInet() error {
	// TODO
	return nil
}
func (g *udpxGate) Shut() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpxGate) serveTLS() { // runner
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.router.DecSub() // gate
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
	g.OnConnClosed()
}

// poolUDPXConn
var poolUDPXConn sync.Pool

func getUDPXConn(id int64, gate *udpxGate, pktConn net.PacketConn, rawConn syscall.RawConn) *UDPXConn {
	var udpxConn *UDPXConn
	if x := poolUDPXConn.Get(); x == nil {
		udpxConn = new(UDPXConn)
	} else {
		udpxConn = x.(*UDPXConn)
	}
	udpxConn.onGet(id, gate, pktConn, rawConn)
	return udpxConn
}
func putUDPXConn(udpxConn *UDPXConn) {
	udpxConn.onPut()
	poolUDPXConn.Put(udpxConn)
}

// UDPXConn
type UDPXConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	router  *UDPXRouter
	gate    *udpxGate
	pktConn net.PacketConn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *UDPXConn) onGet(id int64, gate *udpxGate, pktConn net.PacketConn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id)
	c.router = gate.router
	c.gate = gate
	c.pktConn = pktConn
	c.rawConn = rawConn
}
func (c *UDPXConn) onPut() {
	c.pktConn = nil
	c.rawConn = nil
	c.router = nil
	c.gate = nil
	c.ServerConn_.OnPut()
}

func (c *UDPXConn) IsTLS() bool { return c.router.IsTLS() }
func (c *UDPXConn) IsUDS() bool { return c.router.IsUDS() }

func (c *UDPXConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.router.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *UDPXConn) serve() { // runner
	c.router.dispatch(c)
	c.closeConn()
	putUDPXConn(c)
}

func (c *UDPXConn) Close() error {
	pktConn := c.pktConn
	putUDPXConn(c)
	return pktConn.Close()
}

func (c *UDPXConn) closeConn() {
	// TODO: tls, uds?
	if c.router.IsTLS() {
	} else if c.router.IsUDS() {
	} else {
	}
	c.pktConn.Close()
}

func (c *UDPXConn) unsafeVariable(code int16, name string) (value []byte) {
	return udpxConnVariables[code](c)
}

// udpxConnVariables
var udpxConnVariables = [...]func(*UDPXConn) []byte{ // keep sync with varCodes
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isTLS
	nil, // isUDS
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
