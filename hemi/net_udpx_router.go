// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/UDS) router implementation. See RFC 768 and RFC 8085.

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
	// Mixins
	_udpxHolder_ // to carry configs used by gates
	// Assocs
	dealets compDict[UDPXDealet] // defined dealets. indexed by component name
	cases   []*udpxCase          // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *LogConfig // ...
	logger    *Logger    // router access logger
}

func (r *UDPXRouter) onCreate(compName string, stage *Stage) {
	r.Server_.OnCreate(compName, stage)
	r.dealets = make(compDict[UDPXDealet])
}

func (r *UDPXRouter) OnConfigure() {
	r.Server_.OnConfigure()
	r._udpxHolder_.onConfigure(r)

	// accessLog, TODO

	// sub components
	r.dealets.walk(UDPXDealet.OnConfigure)
	for _, kase := range r.cases {
		kase.OnConfigure()
	}
}
func (r *UDPXRouter) OnPrepare() {
	r.Server_.OnPrepare()
	r._udpxHolder_.onPrepare(r)

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog)
	}

	// sub components
	r.dealets.walk(UDPXDealet.OnPrepare)
	for _, kase := range r.cases {
		kase.OnPrepare()
	}
}

func (r *UDPXRouter) createDealet(compSign string, compName string) UDPXDealet {
	if _, ok := r.dealets[compName]; ok {
		UseExitln("conflicting dealet with a same component name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := udpxDealetCreators[compSign]
	if !ok {
		UseExitln("unknown dealet sign: " + compSign)
	}
	dealet := create(compName, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[compName] = dealet
	return dealet
}
func (r *UDPXRouter) createCase(compName string) *udpxCase {
	if r.hasCase(compName) {
		UseExitln("conflicting case with a same component name")
	}
	kase := new(udpxCase)
	kase.onCreate(compName, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *UDPXRouter) hasCase(compName string) bool {
	for _, kase := range r.cases {
		if kase.CompName() == compName {
			return true
		}
	}
	return false
}

func (r *UDPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpxGate)
		gate.onNew(r, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		go gate.Serve()
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	for _, kase := range r.cases {
		go kase.OnShutdown()
	}
	r.dealets.goWalk(UDPXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("udpxRouter=%s done\n", r.CompName())
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

func (r *UDPXRouter) udpxHolder() _udpxHolder_ { return r._udpxHolder_ }

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
	Gate_[*UDPXRouter]
	// Mixins
	_udpxHolder_
	// States
}

func (g *udpxGate) onNew(router *UDPXRouter, id int32) {
	g.Gate_.OnNew(router, id)
	g._udpxHolder_ = router.udpxHolder()
}

func (g *udpxGate) Open() error {
	// TODO
	return nil
}
func (g *udpxGate) Shut() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpxGate) Serve() { // runner
	if g.UDSMode() {
		g.serveUDS()
	} else {
		g.serveUDP()
	}
}

func (g *udpxGate) serveUDS() {
	// TODO
}
func (g *udpxGate) serveUDP() {
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.server.DecSub() // gate
}

func (g *udpxGate) justClose(pktConn net.PacketConn) {
	pktConn.Close()
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
	c.udpxConn_.onGet(id, gate.Stage(), pktConn, rawConn, gate.UDSMode())

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

func (c *UDPXConn) unsafeVariable(varCode int16, varName string) (varValue []byte) {
	return udpxConnVariables[varCode](c)
}

// udpxConnVariables
var udpxConnVariables = [...]func(*UDPXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // udsMode
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

func (c *udpxCase) onCreate(compName string, router *UDPXRouter) {
	c.MakeComp(compName)
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
		if dealt := dealet.DealWith(conn); dealt {
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
	DealWith(conn *UDPXConn) (dealt bool)
}

// UDPXDealet_
type UDPXDealet_ struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (d *UDPXDealet_) OnCreate(compName string, stage *Stage) {
	d.MakeComp(compName)
	d.stage = stage
}

func (d *UDPXDealet_) Stage() *Stage { return d.stage }
