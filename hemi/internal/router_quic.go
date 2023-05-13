// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC router.

package internal

import (
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

// QUICRouter
type QUICRouter struct {
	// Mixins
	router_[*QUICRouter, *quicGate, QUICDealer, QUICEditor, *quicCase]
}

func (r *QUICRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, quicDealerCreators, quicEditorCreators)
}
func (r *QUICRouter) OnShutdown() {
	// We don't close(r.Shut) here.
	for _, gate := range r.gates {
		gate.shutdown()
	}
}

func (r *QUICRouter) OnConfigure() {
	r.router_.onConfigure()
	// TODO: configure r
	r.configureSubs()
}
func (r *QUICRouter) OnPrepare() {
	r.router_.onPrepare()
	// TODO: prepare r
	r.prepareSubs()
}

func (r *QUICRouter) createCase(name string) *quicCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quicCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *QUICRouter) serve() { // goroutine
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quicGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		r.IncSub(1)
		go gate.serve()
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealers) + len(r.editors) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealers, editors, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if IsDebug(2) {
		Debugf("quicRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

// quicGate
type quicGate struct {
	// Mixins
	Gate_
	// Assocs
	router *QUICRouter
	// States
	gate *quix.Gate // the real gate. set after open
}

func (g *quicGate) init(router *QUICRouter, id int32) {
	g.Gate_.Init(router.stage, id, router.address, router.maxConnsPerGate)
	g.router = router
}

func (g *quicGate) open() error {
	// TODO
	return nil
}
func (g *quicGate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *quicGate) serve() { // goroutine
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.router.SubDone()
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
	case_[*QUICRouter, QUICDealer, QUICEditor]
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

func getQUICConn(id int64, stage *Stage, router *QUICRouter, gate *quicGate, quicConn *quix.Conn) *QUICConn {
	var conn *QUICConn
	if x := poolQUICConn.Get(); x == nil {
		conn = new(QUICConn)
	} else {
		conn = x.(*QUICConn)
	}
	conn.onGet(id, stage, router, gate, quicConn)
	return conn
}
func putQUICConn(conn *QUICConn) {
	conn.onPut()
	poolQUICConn.Put(conn)
}

// QUICConn is the QUIC connection coming from QUICRouter.
type QUICConn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	stage    *Stage // current stage
	router   *QUICRouter
	gate     *quicGate
	quicConn *quix.Conn
	// Conn states (zeros)
	editors [32]uint8
}

func (c *QUICConn) onGet(id int64, stage *Stage, router *QUICRouter, gate *quicGate, quicConn *quix.Conn) {
	c.id = id
	c.stage = stage
	c.router = router
	c.gate = gate
	c.quicConn = quicConn
}
func (c *QUICConn) onPut() {
	c.stage = nil
	c.router = nil
	c.gate = nil
	c.quicConn = nil
	c.editors = [32]uint8{}
}

func (c *QUICConn) execute() { // goroutine
	for _, kase := range c.router.cases {
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
