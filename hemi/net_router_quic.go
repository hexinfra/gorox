// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC (UDP/UDS) reverse proxy.

package hemi

import (
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

// QUICRouter
type QUICRouter struct {
	// Mixins
	router_[*QUICRouter, *quicGate, QUICDealet, *quicCase]
}

func (r *QUICRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, quicDealetCreators)
}
func (r *QUICRouter) OnShutdown() {
	r.router_.onShutdown()
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

func (r *QUICRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quicGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub(1)
		go gate.serve()
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealets) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if Debug() >= 2 {
		Printf("quicRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

func (r *QUICRouter) dispatch(conn *QUICConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
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
	case_[*QUICRouter, QUICDealet]
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

// quicGate is an opening gate of QUICRouter.
type quicGate struct {
	// Mixins
	Gate_
	// Assocs
	// States
	listener *quix.Listener // the real gate. set after open
}

func (g *quicGate) init(id int32, router *QUICRouter) {
	g.Gate_.Init(id, router)
}

func (g *quicGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quicGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *quicGate) serve() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.server.SubDone()
}

func (g *quicGate) justClose(quixConn *quix.Conn) {
	quixConn.Close()
	g.onConnectionClosed()
}
func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}

// poolQUICConn
var poolQUICConn sync.Pool

func getQUICConn(id int64, gate *quicGate, quixConn *quix.Conn) *QUICConn {
	var quicConn *QUICConn
	if x := poolQUICConn.Get(); x == nil {
		quicConn = new(QUICConn)
	} else {
		quicConn = x.(*QUICConn)
	}
	quicConn.onGet(id, gate, quixConn)
	return quicConn
}
func putQUICConn(quicConn *QUICConn) {
	quicConn.onPut()
	poolQUICConn.Put(quicConn)
}

// QUICConn is a QUIC connection coming from QUICRouter.
type QUICConn struct {
	// Mixins
	ServerConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	quixConn *quix.Conn
	// Conn states (zeros)
	quicConn0
}
type quicConn0 struct { // for fast reset, entirely
}

func (c *QUICConn) onGet(id int64, gate *quicGate, quixConn *quix.Conn) {
	c.ServerConn_.OnGet(id, gate)
	c.quixConn = quixConn
}
func (c *QUICConn) onPut() {
	c.quixConn = nil
	c.quicConn0 = quicConn0{}
	c.ServerConn_.OnPut()
}

func (c *QUICConn) mesh() { // runner
	router := c.Server().(*QUICRouter)
	router.dispatch(c)
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
