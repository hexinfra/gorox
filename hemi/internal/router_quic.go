// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC service router.

package internal

import (
	"github.com/hexinfra/gorox/hemi/libraries/logger"
	"github.com/hexinfra/gorox/hemi/libraries/quix"
	"os"
	"path/filepath"
	"sync"
)

// QUICRouter
type QUICRouter struct {
	// Mixins
	router_[*QUICRouter, *quicGate, QUICRunner, QUICFilter, QUICEditor]
	// Assocs
	cases compList[*quicCase] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	logFile string
	logger  *logger.Logger
}

func (r *QUICRouter) init(name string, stage *Stage) {
	r.router_.init(name, stage)
	r.router_.setCreators(quicRunnerCreators, quicFilterCreators, quicEditorCreators)
}

func (r *QUICRouter) OnConfigure() {
	r.Configure()
	// logFile
	r.ConfigureString("logFile", &r.logFile, func(value string) bool { return value != "" }, LogsDir()+"/quic_"+r.name+".log")

	// sub components
	r.configureSubs()
	r.cases.walk((*quicCase).OnConfigure)
}
func (r *QUICRouter) OnPrepare() {
	r.Prepare()
	// logger
	if err := os.MkdirAll(filepath.Dir(r.logFile), 0755); err != nil {
		EnvExitln(err.Error())
	}

	// sub components
	r.prepareSubs()
	r.cases.walk((*quicCase).OnPrepare)
}
func (r *QUICRouter) OnShutdown() {
	r.cases.walk((*quicCase).OnShutdown)
	// sub components
	r.shutdownSubs()

	r.Shutdown()
}

func (r *QUICRouter) createCase(name string) *quicCase {
	/*
		if r.Case(name) != nil {
			UseExitln("conflicting case with a same name")
		}
	*/
	kase := new(quicCase)
	kase.init(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *QUICRouter) serve() {
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quicGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		go gate.serve()
	}
	select {}
}

func (r *QUICRouter) dispatch(conn *QUICConn) {
	/*
		for _, kase := range r.cases {
			if !kase.isMatch(conn) {
				continue
			}
			if processed := executeCase[*QUICConn, quicFilter](kase, conn); processed {
				return
			}
		}
	*/
	conn.Close()
	putQUICConn(conn)
}

// quicGate
type quicGate struct {
	// Mixins
	Gate_
	// Assocs
	router *QUICRouter
	// States
	gate *quix.Gate
}

func (g *quicGate) init(router *QUICRouter, id int32) {
	g.Init(router.stage, id, router.address, router.maxConnsPerGate)
	g.router = router
}

func (g *quicGate) open() error {
	// TODO
	return nil
}

func (g *quicGate) serve() {
	// TODO
}

func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}
func (g *quicGate) justClose(quicConn *quix.Conn) {
	quicConn.Close()
	g.onConnectionClosed()
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

// QUICEditor
type QUICEditor interface {
	Component
	ider
	OnOutput(conn *QUICConn, data []byte)
}

// QUICEditor_
type QUICEditor_ struct {
	Component_
	ider_
}

// quicCase
type quicCase struct {
	// Mixins
	case_[*QUICRouter, QUICRunner, QUICFilter, QUICEditor]
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

// QUICConn
type QUICConn struct {
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	stage    *Stage
	router   *QUICRouter
	gate     *quicGate
	quicConn *quix.Conn
	// Conn states (zeros)
	filters [32]uint8
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
	c.filters = [32]uint8{}
	c.editors = [32]uint8{}
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
