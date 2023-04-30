// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS service mesher.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterUDPSDealer("udpsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealer {
		d := new(udpsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// UDPSMesher
type UDPSMesher struct {
	// Mixins
	mesher_[*UDPSMesher, *udpsGate, UDPSDealer, UDPSEditor, *udpsCase]
}

func (m *UDPSMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, udpsDealerCreators, udpsEditorCreators)
}
func (m *UDPSMesher) OnShutdown() {
	// We don't close(m.Shut) here.
	for _, gate := range m.gates {
		gate.shutdown()
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
	m.IncSub(len(m.dealers) + len(m.editors) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealers, editors, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if IsDebug(2) {
		Debugf("udpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
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
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}
func (g *udpsGate) serveTLS() { // goroutine
	// TODO
	for !g.shut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
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
	case_[*UDPSMesher, UDPSDealer, UDPSEditor]
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

// UDPSConn needs redesign, maybe datagram?
type UDPSConn struct {
	// Conn states (stocks)
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

func (c *UDPSConn) execute() { // goroutine
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
var udpsConnVariables = [...]func(*UDPSConn) []byte{ // keep sync with varCodes in engine.go
	// TODO
}

// udpsProxy passes UDP/DTLS connections to backend UDP/DTLS server.
type udpsProxy struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage   *Stage
	mesher  *UDPSMesher
	backend *UDPSBackend
	// States
}

func (d *udpsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *udpsProxy) OnShutdown() {
	d.mesher.SubDone()
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

func (d *udpsProxy) Deal(conn *UDPSConn) (next bool) { // reverse only
	// TODO
	return
}
