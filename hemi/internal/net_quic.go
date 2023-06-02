// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC network mesher.

package internal

import (
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

// QUICMesher
type QUICMesher struct {
	// Mixins
	mesher_[*QUICMesher, *quicGate, QUICDealer, QUICEditor, *quicCase]
}

func (m *QUICMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, quicDealerCreators, quicEditorCreators)
}
func (m *QUICMesher) OnShutdown() {
	// We don't close(m.Shut) here.
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

func (m *QUICMesher) serve() { // goroutine
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
	m.IncSub(len(m.dealers) + len(m.editors) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealers, editors, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if IsDebug(2) {
		Printf("quicMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

// quicGate
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
	return nil
}
func (g *quicGate) shut() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *quicGate) serve() { // goroutine
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}

func (g *quicGate) justClose(quicConn *quix.Conn) {
	quicConn.Close()
	g.onConnectionClosed()
}
func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}

// QUICDealer
type QUICDealer interface {
	Component
	Deal(conn *QUICConn, stream *QUICStream) (next bool)
}

// QUICDealer_
type QUICDealer_ struct {
	// Mixins
	Component_
	// States
}

// QUICEditor
type QUICEditor interface {
	Component
	identifiable
	OnInput(conn *QUICConn, data []byte) (next bool)
}

// QUICEditor_
type QUICEditor_ struct {
	// Mixins
	Component_
	identifiable_
	// States
}

// quicCase
type quicCase struct {
	// Mixins
	case_[*QUICMesher, QUICDealer, QUICEditor]
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
	return c.matcher(c, conn, conn.unsafeVariable(c.varCode))
}

var quicCaseMatchers = map[string]func(kase *quicCase, conn *QUICConn, value []byte) bool{
	"==": (*quicCase).equalMatch,
	"^=": (*quicCase).prefixMatch,
	"$=": (*quicCase).suffixMatch,
	"~=": (*quicCase).regexpMatch,
	"!=": (*quicCase).notEqualMatch,
	"!^": (*quicCase).notPrefixMatch,
	"!$": (*quicCase).notSuffixMatch,
	"!~": (*quicCase).notRegexpMatch,
}

func (c *quicCase) equalMatch(conn *QUICConn, value []byte) bool { // value == patterns
	return c.case_.equalMatch(value)
}
func (c *quicCase) prefixMatch(conn *QUICConn, value []byte) bool { // value ^= patterns
	return c.case_.prefixMatch(value)
}
func (c *quicCase) suffixMatch(conn *QUICConn, value []byte) bool { // value $= patterns
	return c.case_.suffixMatch(value)
}
func (c *quicCase) regexpMatch(conn *QUICConn, value []byte) bool { // value ~= patterns
	return c.case_.regexpMatch(value)
}
func (c *quicCase) notEqualMatch(conn *QUICConn, value []byte) bool { // value != patterns
	return c.case_.notEqualMatch(value)
}
func (c *quicCase) notPrefixMatch(conn *QUICConn, value []byte) bool { // value !^ patterns
	return c.case_.notPrefixMatch(value)
}
func (c *quicCase) notSuffixMatch(conn *QUICConn, value []byte) bool { // value !$ patterns
	return c.case_.notSuffixMatch(value)
}
func (c *quicCase) notRegexpMatch(conn *QUICConn, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *quicCase) execute(conn *QUICConn) (processed bool) {
	// TODO
	return false
}

// poolQUICConn
var poolQUICConn sync.Pool

func getQUICConn(id int64, stage *Stage, mesher *QUICMesher, gate *quicGate, quicConn *quix.Conn) *QUICConn {
	var conn *QUICConn
	if x := poolQUICConn.Get(); x == nil {
		conn = new(QUICConn)
	} else {
		conn = x.(*QUICConn)
	}
	conn.onGet(id, stage, mesher, gate, quicConn)
	return conn
}
func putQUICConn(conn *QUICConn) {
	conn.onPut()
	poolQUICConn.Put(conn)
}

// QUICConn is the QUIC connection coming from QUICMesher.
type QUICConn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	stage    *Stage // current stage
	mesher   *QUICMesher
	gate     *quicGate
	quicConn *quix.Conn
	// Conn states (zeros)
	quicConn0
}
type quicConn0 struct {
	editors [32]uint8 // editor ids which will apply on this conn. indexed by editor order
}

func (c *QUICConn) onGet(id int64, stage *Stage, mesher *QUICMesher, gate *quicGate, quicConn *quix.Conn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.quicConn = quicConn
}
func (c *QUICConn) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.quicConn = nil
	c.quicConn0 = quicConn0{}
}

func (c *QUICConn) execute() { // goroutine
	for _, kase := range c.mesher.cases {
		if !kase.isMatch(c) {
			continue
		}
		if processed := kase.execute(c); processed {
			break
		}
	}
	c.Close()
	putQUICConn(c)
}

func (c *QUICConn) Close() error {
	// TODO
	return nil
}

func (c *QUICConn) unsafeVariable(index int16) []byte {
	return quicConnVariables[index](c)
}

// quicConnVariables
var quicConnVariables = [...]func(*QUICConn) []byte{ // keep sync with varCodes in engine.go
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

// quicStreamVariables
var quicStreamVariables = [...]func(*QUICStream) []byte{ // keep sync with varCodes in engine.go
	// TODO
}