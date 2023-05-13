// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS router.

package internal

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/hexinfra/gorox/hemi/common/system"
)

// TCPSRouter
type TCPSRouter struct {
	// Mixins
	router_[*TCPSRouter, *tcpsGate, TCPSDealer, TCPSEditor, *tcpsCase]
}

func (r *TCPSRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, tcpsDealerCreators, tcpsEditorCreators)
}
func (r *TCPSRouter) OnShutdown() {
	// We don't close(r.Shut) here.
	for _, gate := range r.gates {
		gate.shutdown()
	}
}

func (r *TCPSRouter) OnConfigure() {
	r.router_.onConfigure()
	// TODO: configure r
	r.configureSubs()
}
func (r *TCPSRouter) OnPrepare() {
	r.router_.onPrepare()
	// TODO: prepare r
	r.prepareSubs()
}

func (r *TCPSRouter) createCase(name string) *tcpsCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(tcpsCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *TCPSRouter) serve() { // goroutine
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		r.IncSub(1)
		if r.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealers) + len(r.editors) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealers, editors, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if IsDebug(2) {
		Debugf("tcpsRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

// tcpsGate
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	router *TCPSRouter
	// States
	gate *net.TCPListener // the real gate. set after open
}

func (g *tcpsGate) init(router *TCPSRouter, id int32) {
	g.Gate_.Init(router.stage, id, router.address, router.maxConnsPerGate)
	g.router = router
}

func (g *tcpsGate) open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		return system.SetReusePort(rawConn)
	}
	gate, err := listenConfig.Listen(context.Background(), "tcp", g.address)
	if err == nil {
		g.gate = gate.(*net.TCPListener)
	}
	return err
}
func (g *tcpsGate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *tcpsGate) serveTCP() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				tcpConn.Close()
				continue
			}
			tcpsConn := getTCPSConn(connID, g.stage, g.router, g, tcpConn, rawConn)
			if IsDebug(1) {
				Debugf("%+v\n", tcpsConn)
			}
			go tcpsConn.execute() // tcpsConn is put to pool in execute()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("tcpsGate=%d TCP done\n", g.id)
	}
	g.router.SubDone()
}
func (g *tcpsGate) serveTLS() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.router.tlsConfig)
			// TODO: set deadline
			if err := tlsConn.Handshake(); err != nil {
				tlsConn.Close()
				continue
			}
			tcpsConn := getTCPSConn(connID, g.stage, g.router, g, tlsConn, nil)
			go tcpsConn.execute() // tcpsConn is put to pool in execute()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("tcpsGate=%d TLS done\n", g.id)
	}
	g.router.SubDone()
}

func (g *tcpsGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}
func (g *tcpsGate) onConnectionClosed() {
	g.DecConns()
	g.SubDone()
}

// TCPSDealer
type TCPSDealer interface {
	Component
	Deal(conn *TCPSConn) (next bool)
}

// TCPSDealer_
type TCPSDealer_ struct {
	// Mixins
	Component_
	// States
}

// TCPSEditor
type TCPSEditor interface {
	Component
	identifiable

	OnInput(conn *TCPSConn, kind int8)
	OnOutput(conn *TCPSConn, kind int8)
}

// TCPSEditor_
type TCPSEditor_ struct {
	// Mixins
	Component_
	identifiable_
	// States
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSRouter, TCPSDealer, TCPSEditor]
	// States
	matcher func(kase *tcpsCase, conn *TCPSConn, value []byte) bool
}

func (c *tcpsCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := tcpsCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *tcpsCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *tcpsCase) isMatch(conn *TCPSConn) bool {
	if c.general {
		return true
	}
	return c.matcher(c, conn, conn.unsafeVariable(c.varCode))
}

var tcpsCaseMatchers = map[string]func(kase *tcpsCase, conn *TCPSConn, value []byte) bool{
	"==": (*tcpsCase).equalMatch,
	"^=": (*tcpsCase).prefixMatch,
	"$=": (*tcpsCase).suffixMatch,
	"~=": (*tcpsCase).regexpMatch,
	"!=": (*tcpsCase).notEqualMatch,
	"!^": (*tcpsCase).notPrefixMatch,
	"!$": (*tcpsCase).notSuffixMatch,
	"!~": (*tcpsCase).notRegexpMatch,
}

func (c *tcpsCase) equalMatch(conn *TCPSConn, value []byte) bool { // value == patterns
	return c.case_.equalMatch(value)
}
func (c *tcpsCase) prefixMatch(conn *TCPSConn, value []byte) bool { // value ^= patterns
	return c.case_.prefixMatch(value)
}
func (c *tcpsCase) suffixMatch(conn *TCPSConn, value []byte) bool { // value $= patterns
	return c.case_.suffixMatch(value)
}
func (c *tcpsCase) regexpMatch(conn *TCPSConn, value []byte) bool { // value ~= patterns
	return c.case_.regexpMatch(value)
}
func (c *tcpsCase) notEqualMatch(conn *TCPSConn, value []byte) bool { // value != patterns
	return c.case_.notEqualMatch(value)
}
func (c *tcpsCase) notPrefixMatch(conn *TCPSConn, value []byte) bool { // value !^ patterns
	return c.case_.notPrefixMatch(value)
}
func (c *tcpsCase) notSuffixMatch(conn *TCPSConn, value []byte) bool { // value !$ patterns
	return c.case_.notSuffixMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *tcpsCase) execute(conn *TCPSConn) (processed bool) {
	for _, editor := range c.editors {
		conn.hookEditor(editor)
	}
	for _, dealer := range c.dealers {
		if next := dealer.Deal(conn); !next {
			return true
		}
	}
	return false
}

// poolTCPSConn
var poolTCPSConn sync.Pool

func getTCPSConn(id int64, stage *Stage, router *TCPSRouter, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) *TCPSConn {
	var conn *TCPSConn
	if x := poolTCPSConn.Get(); x == nil {
		conn = new(TCPSConn)
	} else {
		conn = x.(*TCPSConn)
	}
	conn.onGet(id, stage, router, gate, netConn, rawConn)
	return conn
}
func putTCPSConn(conn *TCPSConn) {
	conn.onPut()
	poolTCPSConn.Put(conn)
}

// TCPSConn is the TCP/TLS connection coming from TCPSRouter.
type TCPSConn struct {
	// Conn states (stocks)
	stockInput [8192]byte // for c.input
	// Conn states (controlled)
	// Conn states (non-zeros)
	id        int64           // connection id
	stage     *Stage          // current stage
	router    *TCPSRouter     // from router
	gate      *tcpsGate       // from gate
	netConn   net.Conn        // the connection (TCP/TLS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	region    Region
	input     []byte // input buffer
	closeSema atomic.Int32
	// Conn states (zeros)
	tcpsConn0
}
type tcpsConn0 struct {
	editors  [32]uint8
	nEditors int8
}

func (c *TCPSConn) onGet(id int64, stage *Stage, router *TCPSRouter, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.router = router
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.region.Init()
	c.input = c.stockInput[:]
	c.closeSema.Store(2)
}
func (c *TCPSConn) onPut() {
	c.stage = nil
	c.router = nil
	c.gate = nil
	c.netConn = nil
	c.rawConn = nil
	c.region.Free()
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
		c.input = nil
	}
	c.tcpsConn0 = tcpsConn0{}
}

func (c *TCPSConn) execute() { // goroutine
	for _, kase := range c.router.cases {
		if !kase.isMatch(c) {
			continue
		}
		if processed := kase.execute(c); processed {
			break
		}
	}
	c.closeConn()
	putTCPSConn(c)
}

func (c *TCPSConn) hookEditor(editor TCPSEditor) {
	if c.nEditors == int8(len(c.editors)) {
		BugExitln("hook too many editors")
	}
	c.editors[c.nEditors] = editor.ID()
	c.nEditors++
}

func (c *TCPSConn) Recv() (p []byte, err error) { // p == nil means EOF
	// TODO: deadline
	n, err := c.netConn.Read(c.input)
	if n > 0 {
		p = c.input[:n]
		if c.nEditors > 0 { // TODO
		} else {
		}
	}
	if err != nil {
		c._checkClose()
	}
	return
}
func (c *TCPSConn) Send(p []byte) (err error) { // if p is nil, send EOF
	// TODO: deadline
	if p == nil {
		c.closeWrite()
		c._checkClose()
	} else {
		if c.nEditors > 0 { // TODO
		} else {
		}
		_, err = c.netConn.Write(p)
	}
	return
}
func (c *TCPSConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.closeConn()
	}
}

func (c *TCPSConn) closeWrite() {
	if c.router.TLSMode() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TCPSConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnectionClosed()
}

func (c *TCPSConn) unsafeVariable(index int16) []byte {
	return tcpsConnVariables[index](c)
}

// tcpsConnVariables
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varCodes in engine.go
	nil, // srcHost
	nil, // srcPort
	nil, // transport
	nil, // serverName
	nil, // nextProto
}
