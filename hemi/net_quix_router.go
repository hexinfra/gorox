// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (QUIC over UDP/UDS) router implementation. See RFC 8999, RFC 9000, RFC 9001, and RFC 9002.

package hemi

import (
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
)

// QUIXRouter
type QUIXRouter struct {
	// Parent
	Server_[*quixGate]
	// Mixins
	_quixHolder_ // to carry configs used by gates
	// Assocs
	dealets compDict[QUIXDealet] // defined dealets. indexed by component name
	cases   []*quixCase          // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	maxConcurrentConnsPerGate int32      // max concurrent connections allowed per gate
	accessLog                 *LogConfig // ...
	logger                    *Logger    // router access logger
}

func (r *QUIXRouter) onCreate(compName string, stage *Stage) {
	r.Server_.OnCreate(compName, stage)
	r.dealets = make(compDict[QUIXDealet])
}

func (r *QUIXRouter) OnConfigure() {
	r.Server_.OnConfigure()
	r._quixHolder_.onConfigure(r)

	// .maxConcurrentConnsPerGate
	r.ConfigureInt32("maxConcurrentConnsPerGate", &r.maxConcurrentConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConcurrentConnsPerGate has an invalid value")
	}, 10000)

	// sub components
	r.dealets.walk(QUIXDealet.OnConfigure)
	for _, kase := range r.cases {
		kase.OnConfigure()
	}
}
func (r *QUIXRouter) OnPrepare() {
	r.Server_.OnPrepare()
	r._quixHolder_.onPrepare(r)

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog)
	}

	// sub components
	r.dealets.walk(QUIXDealet.OnPrepare)
	for _, kase := range r.cases {
		kase.OnPrepare()
	}
}

func (r *QUIXRouter) MaxConcurrentConnsPerGate() int32 { return r.maxConcurrentConnsPerGate }

func (r *QUIXRouter) createDealet(compSign string, compName string) QUIXDealet {
	if _, ok := r.dealets[compName]; ok {
		UseExitln("conflicting dealet with a same component name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := quixDealetCreators[compSign]
	if !ok {
		UseExitln("unknown dealet sign: " + compSign)
	}
	dealet := create(compName, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[compName] = dealet
	return dealet
}
func (r *QUIXRouter) createCase(compName string) *quixCase {
	if r.hasCase(compName) {
		UseExitln("conflicting case with a same component name")
	}
	kase := new(quixCase)
	kase.onCreate(compName, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *QUIXRouter) hasCase(compName string) bool {
	for _, kase := range r.cases {
		if kase.CompName() == compName {
			return true
		}
	}
	return false
}

func (r *QUIXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quixGate)
		gate.onNew(r, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSubGate()
		go gate.Serve()
	}
	r.WaitSubGates()

	r.IncSubs(len(r.dealets) + len(r.cases))
	for _, kase := range r.cases {
		go kase.OnShutdown()
	}
	r.dealets.goWalk(QUIXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("quixRouter=%s done\n", r.CompName())
	}
	r.stage.DecSub() // router
}

func (r *QUIXRouter) Log(str string) {
	if r.logger != nil {
		r.logger.Log(str)
	}
}
func (r *QUIXRouter) Logln(str string) {
	if r.logger != nil {
		r.logger.Logln(str)
	}
}
func (r *QUIXRouter) Logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Logf(format, args...)
	}
}

func (r *QUIXRouter) quixHolder() _quixHolder_ { return r._quixHolder_ }

func (r *QUIXRouter) serveConn(conn *QUIXConn) { // runner
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
	putQUIXConn(conn)
}

// quixGate is an opening gate of QUIXRouter.
type quixGate struct {
	// Parent
	Gate_[*QUIXRouter]
	// Mixins
	_quixHolder_
	// States
	maxConcurrentConns int32          // max concurrent conns allowed for this gate
	concurrentConns    atomic.Int32   // TODO: false sharing
	listener           *tcp2.Listener // the real gate. set after open
}

func (g *quixGate) onNew(router *QUIXRouter, id int32) {
	g.Gate_.OnNew(router, id)
	g._quixHolder_ = router.quixHolder()
	g.maxConcurrentConns = router.MaxConcurrentConnsPerGate()
	g.concurrentConns.Store(0)
}

func (g *quixGate) DecConcurrentConns() int32 { return g.concurrentConns.Add(-1) }
func (g *quixGate) IncConcurrentConns() int32 { return g.concurrentConns.Add(1) }
func (g *quixGate) ReachLimit(concurrentConns int32) bool {
	return concurrentConns > g.maxConcurrentConns
}

func (g *quixGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quixGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *quixGate) Serve() { // runner
	if g.UDSMode() {
		g.serveUDS()
	} else {
		g.serveTLS()
	}
}

func (g *quixGate) serveUDS() {
	// TODO
}
func (g *quixGate) serveTLS() {
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.server.DecSubGate()
}

func (g *quixGate) justClose(quicConn *tcp2.Conn) {
	quicConn.Close()
	g.DecSubConn()
}

// QUIXConn is a QUIX connection coming from QUIXRouter.
type QUIXConn struct {
	// Parent
	quixConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *quixGate
	// Conn states (zeros)
}

var poolQUIXConn sync.Pool

func getQUIXConn(id int64, gate *quixGate, quicConn *tcp2.Conn) *QUIXConn {
	var conn *QUIXConn
	if x := poolQUIXConn.Get(); x == nil {
		conn = new(QUIXConn)
	} else {
		conn = x.(*QUIXConn)
	}
	conn.onGet(id, gate, quicConn)
	return conn
}
func putQUIXConn(conn *QUIXConn) {
	conn.onPut()
	poolQUIXConn.Put(conn)
}

func (c *QUIXConn) onGet(id int64, gate *quixGate, quicConn *tcp2.Conn) {
	c.quixConn_.onGet(id, gate.Stage(), quicConn, gate.UDSMode(), gate.TLSMode(), gate.MaxCumulativeStreamsPerConn(), gate.MaxConcurrentStreamsPerConn())

	c.gate = gate
}
func (c *QUIXConn) onPut() {
	c.gate = nil

	c.quixConn_.onPut()
}

func (c *QUIXConn) closeConn() error {
	// TODO
	return nil
}

func (c *QUIXConn) unsafeVariable(varCode int16, varName string) (varValue []byte) {
	return quixConnVariables[varCode](c)
}

// quixConnVariables
var quixConnVariables = [...]func(*QUIXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // udsMode
	3: nil, // tlsMode
}

// QUIXStream
type QUIXStream struct {
	// Parent
	quixStream_
	// Assocs
	conn *QUIXConn
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolQUIXStream sync.Pool

func getQUIXStream(conn *QUIXConn, quicStream *tcp2.Stream) *QUIXStream {
	var stream *QUIXStream
	if x := poolQUIXStream.Get(); x == nil {
		stream = new(QUIXStream)
	} else {
		stream = x.(*QUIXStream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putQUIXStream(stream *QUIXStream) {
	stream.onEnd()
	poolQUIXStream.Put(stream)
}

func (s *QUIXStream) onUse(conn *QUIXConn, quicStream *tcp2.Stream) {
	s.quixStream_.onUse(quicStream)
	s.conn = conn
}
func (s *QUIXStream) onEnd() {
	s.conn = nil
	s.quixStream_.onEnd()
}

func (s *QUIXStream) Write(src []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUIXStream) Read(dst []byte) (n int, err error) {
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

func (c *quixCase) onCreate(compName string, router *QUIXRouter) {
	c.MakeComp(compName)
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
	DealWith(conn *QUIXConn, stream *QUIXStream) (dealt bool)
}

// QUIXDealet_
type QUIXDealet_ struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (d *QUIXDealet_) OnCreate(compName string, stage *Stage) {
	d.MakeComp(compName)
	d.stage = stage
}

func (d *QUIXDealet_) Stage() *Stage { return d.stage }
