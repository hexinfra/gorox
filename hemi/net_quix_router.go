// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (UDP/UDS) router.

package hemi

import (
	"regexp"
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/library/quic"
)

// QUIXRouter
type QUIXRouter struct {
	// Parent
	Server_[*quixGate]
	// Assocs
	dealets compDict[QUIXDealet] // defined dealets. indexed by name
	cases   compList[*quixCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *LogConfig // ...
	logger    *Logger    // router access logger
}

func (r *QUIXRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[QUIXDealet])
}
func (r *QUIXRouter) OnShutdown() {
	r.Server_.OnShutdown()
}

func (r *QUIXRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// sub components
	r.dealets.walk(QUIXDealet.OnConfigure)
	r.cases.walk((*quixCase).OnConfigure)
}
func (r *QUIXRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog.logFile, r.accessLog.rotate)
	}

	// sub components
	r.dealets.walk(QUIXDealet.OnPrepare)
	r.cases.walk((*quixCase).OnPrepare)
}

func (r *QUIXRouter) createDealet(sign string, name string) QUIXDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := quixDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
}
func (r *QUIXRouter) createCase(name string) *quixCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quixCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *QUIXRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *QUIXRouter) Log(str string) {
}
func (r *QUIXRouter) Logln(str string) {
}
func (r *QUIXRouter) Logf(str string) {
}

func (r *QUIXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quixGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		go gate.serve()
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	r.cases.walk((*quixCase).OnShutdown)
	r.dealets.walk(QUIXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("quixRouter=%s done\n", r.Name())
	}
	r.stage.DecSub() // router
}

func (r *QUIXRouter) dispatch(conn *QUIXConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// quixGate is an opening gate of QUIXRouter.
type quixGate struct {
	// Parent
	Gate_
	// Assocs
	router *QUIXRouter
	// States
	listener *quic.Listener // the real gate. set after open
}

func (g *quixGate) init(id int32, router *QUIXRouter) {
	g.Gate_.Init(id, router.MaxConnsPerGate())
	g.router = router
}

func (g *quixGate) Server() Server  { return g.router }
func (g *quixGate) Address() string { return g.router.Address() }
func (g *quixGate) IsTLS() bool     { return g.router.IsTLS() }
func (g *quixGate) IsUDS() bool     { return g.router.IsUDS() }

func (g *quixGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quixGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *quixGate) serve() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.router.DecSub() // gate
}

func (g *quixGate) justClose(quicConn *quic.Conn) {
	quicConn.Close()
	g.OnConnClosed()
}

// poolQUIXConn
var poolQUIXConn sync.Pool

func getQUIXConn(id int64, gate *quixGate, quicConn *quic.Conn) *QUIXConn {
	var quixConn *QUIXConn
	if x := poolQUIXConn.Get(); x == nil {
		quixConn = new(QUIXConn)
	} else {
		quixConn = x.(*QUIXConn)
	}
	quixConn.onGet(id, gate, quicConn)
	return quixConn
}
func putQUIXConn(quixConn *QUIXConn) {
	quixConn.onPut()
	poolQUIXConn.Put(quixConn)
}

// QUIXConn is a QUIX connection coming from QUIXRouter.
type QUIXConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	router   *QUIXRouter
	gate     *quixGate
	quicConn *quic.Conn
	// Conn states (zeros)
}

func (c *QUIXConn) onGet(id int64, gate *quixGate, quicConn *quic.Conn) {
	c.ServerConn_.OnGet(id)
	c.router = gate.router
	c.gate = gate
	c.quicConn = quicConn
}
func (c *QUIXConn) onPut() {
	c.quicConn = nil
	c.gate = nil
	c.router = nil
	c.ServerConn_.OnPut()
}

func (c *QUIXConn) IsTLS() bool { return c.router.IsTLS() }
func (c *QUIXConn) IsUDS() bool { return c.router.IsUDS() }

func (c *QUIXConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.router.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *QUIXConn) serve() { // runner
	c.router.dispatch(c)
	c.closeConn()
	putQUIXConn(c)
}

func (c *QUIXConn) closeConn() error {
	// TODO
	return nil
}

func (c *QUIXConn) unsafeVariable(code int16, name string) (value []byte) {
	return quixConnVariables[code](c)
}

// quixConnVariables
var quixConnVariables = [...]func(*QUIXConn) []byte{ // keep sync with varCodes
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isTLS
	nil, // isUDS
}

// QUIXStream
type QUIXStream struct {
	// Parent
	// Stream states (non-zeros)
	quicStream *quic.Stream
	// Stream states (zeros)
}

func (s *QUIXStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUIXStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// quixCase
type quixCase struct {
	// Parent
	Component_
	// Assocs
	router  *QUIXRouter
	dealets []QUIXDealet
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
	matcher  func(kase *quixCase, conn *QUIXConn, value []byte) bool
}

func (c *quixCase) onCreate(name string, router *QUIXRouter) {
	c.MakeComp(name)
	c.router = router
}
func (c *quixCase) OnShutdown() {
	c.router.DecSub() // case
}

func (c *quixCase) OnConfigure() {
	if c.info == nil {
		c.general = true
		return
	}
	cond := c.info.(caseCond)
	c.varCode = cond.varCode
	c.varName = cond.varName
	isRegexp := cond.compare == "~=" || cond.compare == "!~"
	for _, pattern := range cond.patterns {
		if pattern == "" {
			UseExitln("empty case cond pattern")
		}
		if !isRegexp {
			c.patterns = append(c.patterns, []byte(pattern))
		} else if exp, err := regexp.Compile(pattern); err == nil {
			c.regexps = append(c.regexps, exp)
		} else {
			UseExitln(err.Error())
		}
	}
	if matcher, ok := quixCaseMatchers[cond.compare]; ok {
		c.matcher = matcher
	} else {
		UseExitln("unknown compare in case condition")
	}
}
func (c *quixCase) OnPrepare() {
}

func (c *quixCase) addDealet(dealet QUIXDealet) { c.dealets = append(c.dealets, dealet) }

func (c *quixCase) isMatch(conn *QUIXConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *quixCase) execute(conn *QUIXConn) (dealt bool) {
	// TODO
	return false
}

var quixCaseMatchers = map[string]func(kase *quixCase, conn *QUIXConn, value []byte) bool{
	"==": (*quixCase).equalMatch,
	"^=": (*quixCase).prefixMatch,
	"$=": (*quixCase).suffixMatch,
	"*=": (*quixCase).containMatch,
	"~=": (*quixCase).regexpMatch,
	"!=": (*quixCase).notEqualMatch,
	"!^": (*quixCase).notPrefixMatch,
	"!$": (*quixCase).notSuffixMatch,
	"!*": (*quixCase).notContainMatch,
	"!~": (*quixCase).notRegexpMatch,
}

func (c *quixCase) equalMatch(conn *QUIXConn, value []byte) bool { // value == patterns
	return equalMatch(value, c.patterns)
}
func (c *quixCase) prefixMatch(conn *QUIXConn, value []byte) bool { // value ^= patterns
	return prefixMatch(value, c.patterns)
}
func (c *quixCase) suffixMatch(conn *QUIXConn, value []byte) bool { // value $= patterns
	return suffixMatch(value, c.patterns)
}
func (c *quixCase) containMatch(conn *QUIXConn, value []byte) bool { // value *= patterns
	return containMatch(value, c.patterns)
}
func (c *quixCase) regexpMatch(conn *QUIXConn, value []byte) bool { // value ~= patterns
	return regexpMatch(value, c.regexps)
}
func (c *quixCase) notEqualMatch(conn *QUIXConn, value []byte) bool { // value != patterns
	return notEqualMatch(value, c.patterns)
}
func (c *quixCase) notPrefixMatch(conn *QUIXConn, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, c.patterns)
}
func (c *quixCase) notSuffixMatch(conn *QUIXConn, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, c.patterns)
}
func (c *quixCase) notContainMatch(conn *QUIXConn, value []byte) bool { // value !* patterns
	return notContainMatch(value, c.patterns)
}
func (c *quixCase) notRegexpMatch(conn *QUIXConn, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, c.regexps)
}

// QUIXDealet
type QUIXDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *QUIXConn, stream *QUIXStream) (dealt bool)
}

// QUIXDealet_
type QUIXDealet_ struct {
	// Parent
	Component_
	// States
}
