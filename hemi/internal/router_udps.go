// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS router.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// UDPSRouter
type UDPSRouter struct {
	// Mixins
	router_[*UDPSRouter, *udpsGate, UDPSDealer, UDPSEditor, *udpsCase]
}

func (r *UDPSRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, udpsDealerCreators, udpsEditorCreators)
}
func (r *UDPSRouter) OnShutdown() {
	// We don't close(r.Shut) here.
	for _, gate := range r.gates {
		gate.shutdown()
	}
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

func (r *UDPSRouter) serve() { // goroutine
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpsGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		r.IncSub(1)
		if r.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveUDP()
		}
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealers) + len(r.editors) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealers, editors, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if IsDebug(2) {
		Debugf("udpsRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

// udpsGate
type udpsGate struct {
	// Mixins
	// Assocs
	stage  *Stage // current stage
	router *UDPSRouter
	// States
	id      int32
	address string
	shut    atomic.Bool
}

func (g *udpsGate) init(router *UDPSRouter, id int32) {
	g.stage = router.stage
	g.router = router
	g.id = id
	g.address = router.address
}

func (g *udpsGate) open() error {
	// TODO
	return nil
}
func (g *udpsGate) shutdown() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveUDP() { // goroutine
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.router.SubDone()
}
func (g *udpsGate) serveTLS() { // goroutine
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.router.SubDone()
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
}

// UDPSDealer
type UDPSDealer interface {
	Component
	Deal(conn *UDPSConn) (next bool)
}

// UDPSDealer_
type UDPSDealer_ struct {
	Component_
}

// UDPSEditor
type UDPSEditor interface {
	Component
	identifiable
	OnInput(conn *UDPSConn, data []byte) (next bool)
}

// UDPSEditor_
type UDPSEditor_ struct {
	Component_
	identifiable_
}

// udpsCase
type udpsCase struct {
	// Mixins
	case_[*UDPSRouter, UDPSDealer, UDPSEditor]
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
	return c.matcher(c, conn, conn.unsafeVariable(c.varCode))
}

var udpsCaseMatchers = map[string]func(kase *udpsCase, conn *UDPSConn, value []byte) bool{
	"==": (*udpsCase).equalMatch,
	"^=": (*udpsCase).prefixMatch,
	"$=": (*udpsCase).suffixMatch,
	"~=": (*udpsCase).regexpMatch,
	"!=": (*udpsCase).notEqualMatch,
	"!^": (*udpsCase).notPrefixMatch,
	"!$": (*udpsCase).notSuffixMatch,
	"!~": (*udpsCase).notRegexpMatch,
}

func (c *udpsCase) equalMatch(conn *UDPSConn, value []byte) bool { // value == patterns
	return c.case_.equalMatch(value)
}
func (c *udpsCase) prefixMatch(conn *UDPSConn, value []byte) bool { // value ^= patterns
	return c.case_.prefixMatch(value)
}
func (c *udpsCase) suffixMatch(conn *UDPSConn, value []byte) bool { // value $= patterns
	return c.case_.suffixMatch(value)
}
func (c *udpsCase) regexpMatch(conn *UDPSConn, value []byte) bool { // value ~= patterns
	return c.case_.regexpMatch(value)
}
func (c *udpsCase) notEqualMatch(conn *UDPSConn, value []byte) bool { // value != patterns
	return c.case_.notEqualMatch(value)
}
func (c *udpsCase) notPrefixMatch(conn *UDPSConn, value []byte) bool { // value !^ patterns
	return c.case_.notPrefixMatch(value)
}
func (c *udpsCase) notSuffixMatch(conn *UDPSConn, value []byte) bool { // value !$ patterns
	return c.case_.notSuffixMatch(value)
}
func (c *udpsCase) notRegexpMatch(conn *UDPSConn, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *udpsCase) execute(conn *UDPSConn) (processed bool) {
	// TODO
	return false
}

// poolUDPSConn
var poolUDPSConn sync.Pool

func getUDPSConn(id int64, stage *Stage, router *UDPSRouter, gate *udpsGate, netConn *net.UDPConn, rawConn syscall.RawConn) *UDPSConn {
	var conn *UDPSConn
	if x := poolUDPSConn.Get(); x == nil {
		conn = new(UDPSConn)
	} else {
		conn = x.(*UDPSConn)
	}
	conn.onGet(id, stage, router, gate, netConn, rawConn)
	return conn
}
func putUDPSConn(conn *UDPSConn) {
	conn.onPut()
	poolUDPSConn.Put(conn)
}

// UDPSConn needs redesign, maybe datagram?
type UDPSConn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage // current stage
	router  *UDPSRouter
	gate    *udpsGate
	netConn *net.UDPConn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *UDPSConn) onGet(id int64, stage *Stage, router *UDPSRouter, gate *udpsGate, netConn *net.UDPConn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.router = router
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *UDPSConn) onPut() {
	c.stage = nil
	c.router = nil
	c.gate = nil
	c.netConn = nil
	c.rawConn = nil
}

func (c *UDPSConn) execute() { // goroutine
	for _, kase := range c.router.cases {
		if !kase.isMatch(c) {
			continue
		}
		if processed := kase.execute(c); processed {
			break
		}
	}
	c.closeConn()
	putUDPSConn(c)
}

func (c *UDPSConn) Close() error {
	netConn := c.netConn
	putUDPSConn(c)
	return netConn.Close()
}

func (c *UDPSConn) closeConn() {
	c.netConn.Close()
}

func (c *UDPSConn) unsafeVariable(index int16) []byte {
	return udpsConnVariables[index](c)
}

// udpsConnVariables
var udpsConnVariables = [...]func(*UDPSConn) []byte{ // keep sync with varCodes in engine.go
	// TODO
}
