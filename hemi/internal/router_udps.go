// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS service router.

package internal

import (
	"net"
	"sync"
	"syscall"
)

// UDPSRouter
type UDPSRouter struct {
	// Mixins
	router_[*UDPSRouter, *udpsGate, UDPSRunner, UDPSFilter, *udpsCase]
}

func (r *UDPSRouter) init(name string, stage *Stage) {
	r.router_.init(name, stage, udpsRunnerCreators, udpsFilterCreators)
}

func (r *UDPSRouter) OnConfigure() {
	r.router_.onConfigure()

	r.configureSubs()
}
func (r *UDPSRouter) OnPrepare() {
	r.router_.onPrepare()

	r.prepareSubs()
}
func (r *UDPSRouter) OnShutdown() {
	r.shutdownSubs()

	r.router_.onShutdown()
}

func (r *UDPSRouter) createCase(name string) *udpsCase {
	/*
		if r.Case(name) != nil {
			UseExitln("conflicting case with a same name")
		}
	*/
	kase := new(udpsCase)
	kase.init(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *UDPSRouter) serve() {
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpsGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		go gate.serve()
	}
	select {}
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

func (g *udpsGate) serve() {
	if g.router.tlsMode {
		g.serveTLS()
	} else {
		g.serveUDP()
	}
}
func (g *udpsGate) serveUDP() {
	// TODO
}
func (g *udpsGate) serveTLS() {
	// TODO
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
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

// UDPSConn
type UDPSConn struct {
	// Conn states (buffers)
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

func (c *UDPSConn) serve() {
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
var udpsConnVariables = [...]func(*UDPSConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}

// UDPSRunner
type UDPSRunner interface {
	Component
	Process(conn *UDPSConn) (next bool)
}

// UDPSRunner_
type UDPSRunner_ struct {
	Component_
}

// UDPSFilter
type UDPSFilter interface {
	Component
	ider
	OnInput(conn *UDPSConn, data []byte) (next bool)
}

// UDPSFilter_
type UDPSFilter_ struct {
	Component_
	ider_
}

// udpsCase
type udpsCase struct {
	// Mixins
	case_[*UDPSRouter, UDPSRunner, UDPSFilter]
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
	"*=": (*udpsCase).wildcardMatch,
	"~=": (*udpsCase).regexpMatch,
	"!=": (*udpsCase).notEqualMatch,
	"!^": (*udpsCase).notPrefixMatch,
	"!$": (*udpsCase).notSuffixMatch,
	"!*": (*udpsCase).notWildcardMatch,
	"!~": (*udpsCase).notRegexpMatch,
}

func (c *udpsCase) equalMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.equalMatch(value)
}
func (c *udpsCase) prefixMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.prefixMatch(value)
}
func (c *udpsCase) suffixMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.suffixMatch(value)
}
func (c *udpsCase) wildcardMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.wildcardMatch(value)
}
func (c *udpsCase) regexpMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.regexpMatch(value)
}
func (c *udpsCase) notEqualMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.notEqualMatch(value)
}
func (c *udpsCase) notPrefixMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.notPrefixMatch(value)
}
func (c *udpsCase) notSuffixMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.notSuffixMatch(value)
}
func (c *udpsCase) notWildcardMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.notWildcardMatch(value)
}
func (c *udpsCase) notRegexpMatch(conn *UDPSConn, value []byte) bool {
	return c.case_.notRegexpMatch(value)
}

func (c *udpsCase) execute(conn *UDPSConn) (processed bool) {
	// TODO
	return false
}
