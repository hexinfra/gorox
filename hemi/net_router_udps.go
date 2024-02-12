// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDPS (UDP/TLS/UDS) reverse proxy.

package hemi

import (
	"net"
	"sync"
	"syscall"
	"time"
)

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
	g.isShut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveUDP() { // runner
	// TODO
	for !g.isShut.Load() {
		time.Sleep(time.Second)
	}
	g.server.SubDone()
}
func (g *udpsGate) serveTLS() { // runner
	// TODO
	for !g.isShut.Load() {
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

func (c *UDPSConn) mesh() { // runner
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
