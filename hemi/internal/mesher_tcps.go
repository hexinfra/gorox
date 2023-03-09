// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS service mesher.

package internal

import (
	"context"
	"crypto/tls"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"net"
	"sync"
	"syscall"
)

// TCPSMesher
type TCPSMesher struct {
	// Mixins
	mesher_[*TCPSMesher, *tcpsGate, TCPSDealet, TCPSEditor, *tcpsCase]
}

func (m *TCPSMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, tcpsDealetCreators, tcpsEditorCreators)
}
func (m *TCPSMesher) OnShutdown() {
	// We don't close(m.Shut) here.
	for _, gate := range m.gates {
		gate.shutdown()
	}
}

func (m *TCPSMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs() // dealets, editors, cases
}
func (m *TCPSMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs() // dealets, editors, cases
}

func (m *TCPSMesher) createCase(name string) *tcpsCase {
	if m.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(tcpsCase)
	kase.onCreate(name, m)
	kase.setShell(kase)
	m.cases = append(m.cases, kase)
	return kase
}

func (m *TCPSMesher) serve() { // goroutine
	for id := int32(0); id < m.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(m, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		m.gates = append(m.gates, gate)
		m.IncSub(1)
		if m.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	m.WaitSubs() // gates
	m.IncSub(len(m.dealets) + len(m.editors) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealets, editors, cases
	// TODO: close access log file
	if IsDebug(2) {
		Debugf("tcpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

// TCPSDealet
type TCPSDealet interface {
	Component
	Deal(conn *TCPSConn) (next bool)
}

// TCPSDealet_
type TCPSDealet_ struct {
	// Mixins
	Component_
	// States
}

// TCPSEditor
type TCPSEditor interface {
	Component
	ider

	OnInput(conn *TCPSConn, kind int8)
	OnOutput(conn *TCPSConn, kind int8)
}

// TCPSEditor_
type TCPSEditor_ struct {
	Component_
	ider_
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSMesher, TCPSDealet, TCPSEditor]
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
	"*=": (*tcpsCase).wildcardMatch,
	"~=": (*tcpsCase).regexpMatch,
	"!=": (*tcpsCase).notEqualMatch,
	"!^": (*tcpsCase).notPrefixMatch,
	"!$": (*tcpsCase).notSuffixMatch,
	"!*": (*tcpsCase).notWildcardMatch,
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
func (c *tcpsCase) wildcardMatch(conn *TCPSConn, value []byte) bool { // value *= patterns
	return c.case_.wildcardMatch(value)
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
func (c *tcpsCase) notWildcardMatch(conn *TCPSConn, value []byte) bool { // value !* patterns
	return c.case_.notWildcardMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *tcpsCase) execute(conn *TCPSConn) (processed bool) {
	for _, editor := range c.editors {
		conn.hookEditor(editor)
	}
	for _, dealet := range c.dealets {
		if next := dealet.Deal(conn); !next {
			return true
		}
	}
	return false
}

// tcpsGate
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	mesher *TCPSMesher
	// States
	gate *net.TCPListener
}

func (g *tcpsGate) init(mesher *TCPSMesher, id int32) {
	g.Gate_.Init(mesher.stage, id, mesher.address, mesher.maxConnsPerGate)
	g.mesher = mesher
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
			tcpsConn := getTCPSConn(connID, g.stage, g.mesher, g, tcpConn, rawConn)
			if IsDebug(1) {
				Debugf("%+v\n", tcpsConn)
			}
			go tcpsConn.serve() // tcpsConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("tcpsGate=%d TCP done\n", g.id)
	}
	g.mesher.SubDone()
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
			tlsConn := tls.Server(tcpConn, g.mesher.tlsConfig)
			// TODO: set deadline
			if err := tlsConn.Handshake(); err != nil {
				tlsConn.Close()
				continue
			}
			tcpsConn := getTCPSConn(connID, g.stage, g.mesher, g, tlsConn, nil)
			go tcpsConn.serve() // tcpsConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("tcpsGate=%d TLS done\n", g.id)
	}
	g.mesher.SubDone()
}

func (g *tcpsGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}
func (g *tcpsGate) onConnectionClosed() {
	g.DecConns()
	g.SubDone()
}

// poolTCPSConn
var poolTCPSConn sync.Pool

func getTCPSConn(id int64, stage *Stage, mesher *TCPSMesher, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) *TCPSConn {
	var conn *TCPSConn
	if x := poolTCPSConn.Get(); x == nil {
		conn = new(TCPSConn)
	} else {
		conn = x.(*TCPSConn)
	}
	conn.onGet(id, stage, mesher, gate, netConn, rawConn)
	return conn
}
func putTCPSConn(conn *TCPSConn) {
	conn.onPut()
	poolTCPSConn.Put(conn)
}

// TCPSConn
type TCPSConn struct {
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage // current stage
	mesher  *TCPSMesher
	gate    *tcpsGate
	netConn net.Conn
	rawConn syscall.RawConn
	region  Region
	// Conn states (zeros)
	tcpsConn0
}
type tcpsConn0 struct {
	editors  [32]uint8
	nEditors int8
}

func (c *TCPSConn) onGet(id int64, stage *Stage, mesher *TCPSMesher, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.region.Init()
}
func (c *TCPSConn) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.netConn = nil
	c.rawConn = nil
	c.region.Free()
	c.tcpsConn0 = tcpsConn0{}
}

func (c *TCPSConn) serve() { // goroutine
	for _, kase := range c.mesher.cases {
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

func (c *TCPSConn) Read(p []byte) (n int, err error) {
	// TODO
	if c.nEditors > 0 {
	} else {
	}
	return c.netConn.Read(p)
}
func (c *TCPSConn) Write(p []byte) (n int, err error) {
	// TODO
	if c.nEditors > 0 {
	} else {
	}
	return c.netConn.Write(p)
}

func (c *TCPSConn) Close() error {
	netConn := c.netConn
	putTCPSConn(c)
	return netConn.Close()
}

func (c *TCPSConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnectionClosed()
}

func (c *TCPSConn) unsafeVariable(index int16) []byte {
	return tcpsConnVariables[index](c)
}

// tcpsConnVariables
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varCodes in config.go
	nil, // srcHost
	nil, // srcPort
	nil, // transport
	nil, // serverName
	nil, // nextProto
}
