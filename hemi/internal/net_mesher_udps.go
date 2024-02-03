// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDPS (UDP/TLS/UDS) network mesher.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// UDPSMesher
type UDPSMesher struct {
	// Mixins
	mesher_[*UDPSMesher, *udpsGate, UDPSDealet, *udpsCase]
}

func (m *UDPSMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, udpsDealetCreators)
}
func (m *UDPSMesher) OnShutdown() {
	// Notify gates. We don't close(m.ShutChan) here.
	for _, gate := range m.gates {
		gate.shut()
	}
}

func (m *UDPSMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs()
}
func (m *UDPSMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs()
}

func (m *UDPSMesher) createCase(name string) *udpsCase {
	if m.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(udpsCase)
	kase.onCreate(name, m)
	kase.setShell(kase)
	m.cases = append(m.cases, kase)
	return kase
}

func (m *UDPSMesher) serve() { // runner
	for id := int32(0); id < m.numGates; id++ {
		gate := new(udpsGate)
		gate.init(m, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		m.gates = append(m.gates, gate)
		m.IncSub(1)
		if m.TLSMode() {
			go gate.serveTLS()
		} else {
			go gate.serveUDP()
		}
	}
	m.WaitSubs() // gates
	m.IncSub(len(m.dealets) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealets, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if Debug() >= 2 {
		Printf("udpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

func (m *UDPSMesher) dispatch(conn *UDPSConn) {
	for _, kase := range m.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// udpsGate is an opening gate of UDPSMesher.
type udpsGate struct {
	// Mixins
	// Assocs
	stage  *Stage // current stage
	mesher *UDPSMesher
	// States
	id      int32
	address string
	isShut  atomic.Bool
}

func (g *udpsGate) init(mesher *UDPSMesher, id int32) {
	g.stage = mesher.stage
	g.mesher = mesher
	g.id = id
	g.address = mesher.address
}

func (g *udpsGate) open() error {
	// TODO
	return nil
}
func (g *udpsGate) shut() error {
	g.isShut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveUDP() { // runner
	// TODO
	for !g.isShut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}
func (g *udpsGate) serveTLS() { // runner
	// TODO
	for !g.isShut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
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
	case_[*UDPSMesher, UDPSDealet]
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

// poolUDPSConn
var poolUDPSConn sync.Pool

func getUDPSConn(id int64, stage *Stage, mesher *UDPSMesher, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) *UDPSConn {
	var conn *UDPSConn
	if x := poolUDPSConn.Get(); x == nil {
		conn = new(UDPSConn)
	} else {
		conn = x.(*UDPSConn)
	}
	conn.onGet(id, stage, mesher, gate, udpConn, rawConn)
	return conn
}
func putUDPSConn(conn *UDPSConn) {
	conn.onPut()
	poolUDPSConn.Put(conn)
}

// UDPSConn
type UDPSConn struct {
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage // current stage
	mesher  *UDPSMesher
	gate    *udpsGate
	udpConn *net.UDPConn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *UDPSConn) onGet(id int64, stage *Stage, mesher *UDPSMesher, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.udpConn = udpConn
	c.rawConn = rawConn
}
func (c *UDPSConn) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.udpConn = nil
	c.rawConn = nil
}

func (c *UDPSConn) mesh() { // runner
	c.mesher.dispatch(c)
	c.closeConn()
	putUDPSConn(c)
}

func (c *UDPSConn) Close() error {
	udpConn := c.udpConn
	putUDPSConn(c)
	return udpConn.Close()
}

func (c *UDPSConn) closeConn() {
	c.udpConn.Close()
}

func (c *UDPSConn) unsafeVariable(code int16, name string) (value []byte) {
	return udpsConnVariables[code](c)
}

// udpsConnVariables
var udpsConnVariables = [...]func(*UDPSConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}
