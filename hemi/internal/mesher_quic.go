// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC service mesher.

package internal

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/quix"
	"os"
	"sync"
)

// QUICMesher
type QUICMesher struct {
	// Mixins
	mesher_[*QUICMesher, *quicGate, QUICRunner, QUICFilter, *quicCase]
}

func (m *QUICMesher) init(name string, stage *Stage) {
	m.mesher_.init(name, stage, quicRunnerCreators, quicFilterCreators)
}

func (m *QUICMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs() // runners, filters, cases
}
func (m *QUICMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs() // runners, filters, cases
}

func (m *QUICMesher) OnShutdown() {
	// We don't use m.Shutdown() here.
	for _, gate := range m.gates {
		gate.shutdown()
	}
}

func (m *QUICMesher) createCase(name string) *quicCase {
	if m.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quicCase)
	kase.init(name, m)
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
	m.IncSub(len(m.runners) + len(m.filters) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // runners, filters, cases
	m.logger.Writer().(*os.File).Close()
	if Debug(2) {
		fmt.Printf("quicMesher=%s done\n", m.Name())
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
	gate *quix.Gate
}

func (g *quicGate) init(mesher *QUICMesher, id int32) {
	g.Gate_.Init(mesher.stage, id, mesher.address, mesher.maxConnsPerGate)
	g.mesher = mesher
}

func (g *quicGate) open() error {
	// TODO
	return nil
}
func (g *quicGate) shutdown() error {
	g.Gate_.Shutdown()
	return g.gate.Close()
}

func (g *quicGate) serve() { // goroutine
	// TODO
	g.mesher.SubDone()
}

func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}
func (g *quicGate) justClose(quicConn *quix.Conn) {
	quicConn.Close()
	g.onConnectionClosed()
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

// QUICConn
type QUICConn struct {
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	stage    *Stage // current stage
	mesher   *QUICMesher
	gate     *quicGate
	quicConn *quix.Conn
	// Conn states (zeros)
	filters [32]uint8
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
	c.filters = [32]uint8{}
}

func (c *QUICConn) serve() { // goroutine
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

// quicStreamVariables
var quicStreamVariables = [...]func(*QUICStream) []byte{ // keep sync with varCodes in config.go
	// TODO
}

// QUICRunner
type QUICRunner interface {
	Component
	Process(conn *QUICConn, stream *QUICStream) (next bool)
}

// QUICRunner_
type QUICRunner_ struct {
	Component_
}

// QUICFilter
type QUICFilter interface {
	Component
	ider
	OnInput(conn *QUICConn, data []byte) (next bool)
}

// QUICFilter_
type QUICFilter_ struct {
	Component_
	ider_
}

// quicCase
type quicCase struct {
	// Mixins
	case_[*QUICMesher, QUICRunner, QUICFilter]
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
	"*=": (*quicCase).wildcardMatch,
	"~=": (*quicCase).regexpMatch,
	"!=": (*quicCase).notEqualMatch,
	"!^": (*quicCase).notPrefixMatch,
	"!$": (*quicCase).notSuffixMatch,
	"!*": (*quicCase).notWildcardMatch,
	"!~": (*quicCase).notRegexpMatch,
}

func (c *quicCase) equalMatch(conn *QUICConn, value []byte) bool {
	return c.case_.equalMatch(value)
}
func (c *quicCase) prefixMatch(conn *QUICConn, value []byte) bool {
	return c.case_.prefixMatch(value)
}
func (c *quicCase) suffixMatch(conn *QUICConn, value []byte) bool {
	return c.case_.suffixMatch(value)
}
func (c *quicCase) wildcardMatch(conn *QUICConn, value []byte) bool {
	return c.case_.wildcardMatch(value)
}
func (c *quicCase) regexpMatch(conn *QUICConn, value []byte) bool {
	return c.case_.regexpMatch(value)
}
func (c *quicCase) notEqualMatch(conn *QUICConn, value []byte) bool {
	return c.case_.notEqualMatch(value)
}
func (c *quicCase) notPrefixMatch(conn *QUICConn, value []byte) bool {
	return c.case_.notPrefixMatch(value)
}
func (c *quicCase) notSuffixMatch(conn *QUICConn, value []byte) bool {
	return c.case_.notSuffixMatch(value)
}
func (c *quicCase) notWildcardMatch(conn *QUICConn, value []byte) bool {
	return c.case_.notWildcardMatch(value)
}
func (c *quicCase) notRegexpMatch(conn *QUICConn, value []byte) bool {
	return c.case_.notRegexpMatch(value)
}

func (c *quicCase) execute(conn *QUICConn) (processed bool) {
	// TODO
	return false
}
