// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS service mesher.

package internal

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// UDPSMesher
type UDPSMesher struct {
	// Mixins
	mesher_[*UDPSMesher, *udpsGate, UDPSDealet, UDPSEditor, *udpsCase]
}

func (m *UDPSMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, udpsDealetCreators, udpsEditorCreators)
}
func (m *UDPSMesher) OnShutdown() {
	// We don't use m.Shutdown() here.
	for _, gate := range m.gates {
		gate.shutdown()
	}
}

func (m *UDPSMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs() // dealets, editors, cases
}
func (m *UDPSMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs() // dealets, editors, cases
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

func (m *UDPSMesher) serve() { // goroutine
	for id := int32(0); id < m.numGates; id++ {
		gate := new(udpsGate)
		gate.init(m, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		m.gates = append(m.gates, gate)
		m.IncSub(1)
		if m.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveUDP()
		}
	}
	m.WaitSubs() // gates
	m.IncSub(len(m.dealets) + len(m.editors) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealets, editors, cases
	// TODO: close access log file
	if Debug(2) {
		fmt.Printf("udpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

// UDPSDealet
type UDPSDealet interface {
	Component
	Deal(conn *UDPSConn) (next bool)
}

// UDPSDealet_
type UDPSDealet_ struct {
	Component_
}

// UDPSEditor
type UDPSEditor interface {
	Component
	ider
	OnInput(conn *UDPSConn, data []byte) (next bool)
}

// UDPSEditor_
type UDPSEditor_ struct {
	Component_
	ider_
}

// udpsCase
type udpsCase struct {
	// Mixins
	case_[*UDPSMesher, UDPSDealet, UDPSEditor]
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
	"*=": (*udpsCase).wildcardMatch,
	"~=": (*udpsCase).regexpMatch,
	"!=": (*udpsCase).notEqualMatch,
	"!^": (*udpsCase).notPrefixMatch,
	"!$": (*udpsCase).notSuffixMatch,
	"!*": (*udpsCase).notWildcardMatch,
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
func (c *udpsCase) wildcardMatch(conn *UDPSConn, value []byte) bool { // value *= patterns
	return c.case_.wildcardMatch(value)
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
func (c *udpsCase) notWildcardMatch(conn *UDPSConn, value []byte) bool { // value !* patterns
	return c.case_.notWildcardMatch(value)
}
func (c *udpsCase) notRegexpMatch(conn *UDPSConn, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *udpsCase) execute(conn *UDPSConn) (processed bool) {
	// TODO
	return false
}

// udpsGate
type udpsGate struct {
	// Mixins
	// Assocs
	stage  *Stage // current stage
	mesher *UDPSMesher
	// States
	id      int32
	address string
	shut    atomic.Bool
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
func (g *udpsGate) shutdown() error {
	g.shut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveUDP() { // goroutine
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}
func (g *udpsGate) serveTLS() { // goroutine
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
}

// poolUDPSConn
var poolUDPSConn sync.Pool

func getUDPSConn(id int64, stage *Stage, mesher *UDPSMesher, gate *udpsGate, netConn *net.UDPConn, rawConn syscall.RawConn) *UDPSConn {
	var conn *UDPSConn
	if x := poolUDPSConn.Get(); x == nil {
		conn = new(UDPSConn)
	} else {
		conn = x.(*UDPSConn)
	}
	conn.onGet(id, stage, mesher, gate, netConn, rawConn)
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
	mesher  *UDPSMesher
	gate    *udpsGate
	netConn *net.UDPConn
	rawConn syscall.RawConn
	// Conn states (zeros)
}

func (c *UDPSConn) onGet(id int64, stage *Stage, mesher *UDPSMesher, gate *udpsGate, netConn *net.UDPConn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *UDPSConn) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.netConn = nil
	c.rawConn = nil
}

func (c *UDPSConn) serve() { // goroutine
	for _, kase := range c.mesher.cases {
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
