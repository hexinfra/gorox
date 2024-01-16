// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
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
	mesher_[*QUICMesher, *quicGate, QUICFilter, *quicCase]
}

func (m *QUICMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, quicFilterCreators)
}
func (m *QUICMesher) OnShutdown() {
	// We don't close(m.ShutChan) here.
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
	m.IncSub(len(m.filters) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // filters, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if Debug() >= 2 {
		Printf("quicMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
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

func (g *quicGate) execute(connection *QUICConnection) { // goroutine
	for _, kase := range g.mesher.cases {
		if !kase.isMatch(connection) {
			continue
		}
		if processed := kase.execute(connection); processed {
			break
		}
	}
	connection.Close()
	putQUICConnection(connection)
}

func (g *quicGate) justClose(quicConnection *quix.Connection) {
	quicConnection.Close()
	g.onConnectionClosed()
}
func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}

// quicCase
type quicCase struct {
	// Mixins
	case_[*QUICMesher, QUICFilter]
	// States
	matcher func(kase *quicCase, connection *QUICConnection, value []byte) bool
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

func (c *quicCase) isMatch(connection *QUICConnection) bool {
	if c.general {
		return true
	}
	value := connection.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, connection, value)
}

var quicCaseMatchers = map[string]func(kase *quicCase, connection *QUICConnection, value []byte) bool{
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

func (c *quicCase) equalMatch(connection *QUICConnection, value []byte) bool { // value == patterns
	return c.case_.equalMatch(value)
}
func (c *quicCase) prefixMatch(connection *QUICConnection, value []byte) bool { // value ^= patterns
	return c.case_.prefixMatch(value)
}
func (c *quicCase) suffixMatch(connection *QUICConnection, value []byte) bool { // value $= patterns
	return c.case_.suffixMatch(value)
}
func (c *quicCase) containMatch(connection *QUICConnection, value []byte) bool { // value *= patterns
	return c.case_.containMatch(value)
}
func (c *quicCase) regexpMatch(connection *QUICConnection, value []byte) bool { // value ~= patterns
	return c.case_.regexpMatch(value)
}
func (c *quicCase) notEqualMatch(connection *QUICConnection, value []byte) bool { // value != patterns
	return c.case_.notEqualMatch(value)
}
func (c *quicCase) notPrefixMatch(connection *QUICConnection, value []byte) bool { // value !^ patterns
	return c.case_.notPrefixMatch(value)
}
func (c *quicCase) notSuffixMatch(connection *QUICConnection, value []byte) bool { // value !$ patterns
	return c.case_.notSuffixMatch(value)
}
func (c *quicCase) notContainMatch(connection *QUICConnection, value []byte) bool { // value !* patterns
	return c.case_.notContainMatch(value)
}
func (c *quicCase) notRegexpMatch(connection *QUICConnection, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *quicCase) execute(connection *QUICConnection) (processed bool) {
	// TODO
	return false
}

// QUICFilter
type QUICFilter interface {
	// Imports
	Component
	// Methods
	Deal(connection *QUICConnection, stream *QUICStream) (next bool)
}

// QUICFilter_
type QUICFilter_ struct {
	// Mixins
	Component_
	// States
}

// poolQUICConnection
var poolQUICConnection sync.Pool

func getQUICConnection(id int64, stage *Stage, mesher *QUICMesher, gate *quicGate, quicConnection *quix.Connection) *QUICConnection {
	var connection *QUICConnection
	if x := poolQUICConnection.Get(); x == nil {
		connection = new(QUICConnection)
	} else {
		connection = x.(*QUICConnection)
	}
	connection.onGet(id, stage, mesher, gate, quicConnection)
	return connection
}
func putQUICConnection(connection *QUICConnection) {
	connection.onPut()
	poolQUICConnection.Put(connection)
}

// QUICConnection is the QUIC connection coming from QUICMesher.
type QUICConnection struct {
	// Connection states (stocks)
	// Connection states (controlled)
	// Connection states (non-zeros)
	id             int64
	stage          *Stage // current stage
	mesher         *QUICMesher
	gate           *quicGate
	quicConnection *quix.Connection
	// Connection states (zeros)
	quicConnection0
}
type quicConnection0 struct { // for fast reset, entirely
}

func (c *QUICConnection) onGet(id int64, stage *Stage, mesher *QUICMesher, gate *quicGate, quicConnection *quix.Connection) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.quicConnection = quicConnection
}
func (c *QUICConnection) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.quicConnection = nil
	c.quicConnection0 = quicConnection0{}
}

func (c *QUICConnection) Close() error {
	// TODO
	return nil
}

func (c *QUICConnection) unsafeVariable(code int16, name string) (value []byte) {
	return quicConnectionVariables[code](c)
}

// quicConnectionVariables
var quicConnectionVariables = [...]func(*QUICConnection) []byte{ // keep sync with varCodes in config.go
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
