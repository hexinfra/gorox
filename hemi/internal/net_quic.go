// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC (UDP/UDS) network mesher.

package internal

import (
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

// QUICMesher
type QUICMesher struct {
	// Mixins
	mesher_[*QUICMesher, *quicGate, QUICDealet, *quicCase]
}

func (m *QUICMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, quicDealetCreators)
}
func (m *QUICMesher) OnShutdown() {
	// Notify gates. We don't close(m.ShutChan) here.
	for _, gate := range m.gates {
		gate.shut()
	}
}

func (m *QUICMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs()
}
func (m *QUICMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs()
}

func (m *QUICMesher) createCase(name string) *quicCase {
	if m.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quicCase)
	kase.onCreate(name, m)
	kase.setShell(kase)
	m.cases = append(m.cases, kase)
	return kase
}

func (m *QUICMesher) serve() { // runner
	for id := int32(0); id < m.numGates; id++ {
		gate := new(quicGate)
		gate.init(m, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		m.gates = append(m.gates, gate)
		m.IncSub(1)
		go gate.serve()
	}
	m.WaitSubs() // gates
	m.IncSub(len(m.dealets) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealets, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if Debug() >= 2 {
		Printf("quicMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

func (m *QUICMesher) dispatch(conn *QUICConn) {
	for _, kase := range m.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// quicGate is an opening gate of QUICMesher.
type quicGate struct {
	// Mixins
	Gate_
	// Assocs
	mesher *QUICMesher
	// States
	gate *quix.Gate // the real gate. set after open
}

func (g *quicGate) init(mesher *QUICMesher, id int32) {
	g.Gate_.Init(mesher.stage, id, mesher.address, mesher.maxConnsPerGate)
	g.mesher = mesher
}

func (g *quicGate) open() error {
	// TODO
	// set g.gate
	return nil
}
func (g *quicGate) shut() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *quicGate) serve() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}

func (g *quicGate) justClose(quixConn *quix.Conn) {
	quixConn.Close()
	g.onConnectionClosed()
}
func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}

// QUICDealet
type QUICDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *QUICConn, stream *QUICStream) (dealt bool)
}

// QUICDealet_
type QUICDealet_ struct {
	// Mixins
	Component_
	// States
}

// quicCase
type quicCase struct {
	// Mixins
	case_[*QUICMesher, QUICDealet]
	// States
	matcher func(kase *quicCase, conn *QUICConn, value []byte) bool
}

func (c *quicCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := quicCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *quicCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *quicCase) isMatch(conn *QUICConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *quicCase) execute(conn *QUICConn) (dealt bool) {
	// TODO
	return false
}

func (c *quicCase) equalMatch(conn *QUICConn, value []byte) bool { // value == patterns
	return c.case_._equalMatch(value)
}
func (c *quicCase) prefixMatch(conn *QUICConn, value []byte) bool { // value ^= patterns
	return c.case_._prefixMatch(value)
}
func (c *quicCase) suffixMatch(conn *QUICConn, value []byte) bool { // value $= patterns
	return c.case_._suffixMatch(value)
}
func (c *quicCase) containMatch(conn *QUICConn, value []byte) bool { // value *= patterns
	return c.case_._containMatch(value)
}
func (c *quicCase) regexpMatch(conn *QUICConn, value []byte) bool { // value ~= patterns
	return c.case_._regexpMatch(value)
}
func (c *quicCase) notEqualMatch(conn *QUICConn, value []byte) bool { // value != patterns
	return c.case_._notEqualMatch(value)
}
func (c *quicCase) notPrefixMatch(conn *QUICConn, value []byte) bool { // value !^ patterns
	return c.case_._notPrefixMatch(value)
}
func (c *quicCase) notSuffixMatch(conn *QUICConn, value []byte) bool { // value !$ patterns
	return c.case_._notSuffixMatch(value)
}
func (c *quicCase) notContainMatch(conn *QUICConn, value []byte) bool { // value !* patterns
	return c.case_._notContainMatch(value)
}
func (c *quicCase) notRegexpMatch(conn *QUICConn, value []byte) bool { // value !~ patterns
	return c.case_._notRegexpMatch(value)
}

var quicCaseMatchers = map[string]func(kase *quicCase, conn *QUICConn, value []byte) bool{
	"==": (*quicCase).equalMatch,
	"^=": (*quicCase).prefixMatch,
	"$=": (*quicCase).suffixMatch,
	"*=": (*quicCase).containMatch,
	"~=": (*quicCase).regexpMatch,
	"!=": (*quicCase).notEqualMatch,
	"!^": (*quicCase).notPrefixMatch,
	"!$": (*quicCase).notSuffixMatch,
	"!*": (*quicCase).notContainMatch,
	"!~": (*quicCase).notRegexpMatch,
}

// poolQUICConn
var poolQUICConn sync.Pool

func getQUICConn(id int64, stage *Stage, mesher *QUICMesher, gate *quicGate, quixConn *quix.Conn) *QUICConn {
	var conn *QUICConn
	if x := poolQUICConn.Get(); x == nil {
		conn = new(QUICConn)
	} else {
		conn = x.(*QUICConn)
	}
	conn.onGet(id, stage, mesher, gate, quixConn)
	return conn
}
func putQUICConn(conn *QUICConn) {
	conn.onPut()
	poolQUICConn.Put(conn)
}

// QUICConn is a QUIC connection coming from QUICMesher.
type QUICConn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	stage    *Stage // current stage
	mesher   *QUICMesher
	gate     *quicGate
	quixConn *quix.Conn
	// Conn states (zeros)
	quicConn0
}
type quicConn0 struct { // for fast reset, entirely
}

func (c *QUICConn) onGet(id int64, stage *Stage, mesher *QUICMesher, gate *quicGate, quixConn *quix.Conn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.quixConn = quixConn
}
func (c *QUICConn) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.quixConn = nil
	c.quicConn0 = quicConn0{}
}

func (c *QUICConn) mesh() { // runner
	c.mesher.dispatch(c)
	c.Close()
	putQUICConn(c)
}

func (c *QUICConn) Close() error {
	// TODO
	return nil
}

func (c *QUICConn) unsafeVariable(code int16, name string) (value []byte) {
	return quicConnVariables[code](c)
}

// quicConnVariables
var quicConnVariables = [...]func(*QUICConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}

// QUICStream
type QUICStream struct {
}

func (s *QUICStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUICStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

func (s *QUICStream) unsafeVariable(code int16, name string) (value []byte) {
	return quicStreamVariables[code](s)
}

// quicStreamVariables
var quicStreamVariables = [...]func(*QUICStream) []byte{ // keep sync with varCodes in config.go
	// TODO
}
