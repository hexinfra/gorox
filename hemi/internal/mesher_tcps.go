// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS service mesher.

package internal

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"net"
	"sync"
	"syscall"
)

// TCPSMesher
type TCPSMesher struct {
	// Mixins
	mesher_[*TCPSMesher, *tcpsGate, TCPSRunner, TCPSFilter, *tcpsCase]
}

func (m *TCPSMesher) init(name string, stage *Stage) {
	m.mesher_.init(name, stage, tcpsRunnerCreators, tcpsFilterCreators)
}

func (m *TCPSMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs()
}
func (m *TCPSMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs()
}
func (m *TCPSMesher) OnShutdown() {
	for _, gate := range m.gates {
		gate.shutdown()
	}
	m.shutdownSubs()
	// TODO: shutdown m
	m.mesher_.onShutdown()
}

func (m *TCPSMesher) createCase(name string) *tcpsCase {
	if m.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(tcpsCase)
	kase.init(name, m)
	kase.setShell(kase)
	m.cases = append(m.cases, kase)
	m.IncSub(1)
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
	m.WaitSubs()
	if Debug(2) {
		fmt.Printf("tcpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

// tcpsGate
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	mesher *TCPSMesher
	// States
	listener *net.TCPListener
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
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.address)
	if err == nil {
		g.listener = listener.(*net.TCPListener)
	}
	return err
}
func (g *tcpsGate) shutdown() error {
	g.Gate_.Shutdown()
	return g.listener.Close()
}

func (g *tcpsGate) serveTCP() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			if g.IsShutdown() {
				break
			} else {
				continue
			}
		}
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				tcpConn.Close()
				continue
			}
			tcpsConn := getTCPSConn(connID, g.stage, g.mesher, g, tcpConn, rawConn)
			if Debug(1) {
				fmt.Printf("%+v\n", tcpsConn)
			}
			go tcpsConn.serve() // tcpsConn is put to pool in serve()
			connID++
		}
	}
	// TODO: waiting for all connections end. Use sync.Cond?
	g.mesher.SubDone()
}
func (g *tcpsGate) serveTLS() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			if g.IsShutdown() {
				break
			} else {
				continue
			}
		}
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
	// TODO: waiting for all connections end. Use sync.Cond?
	g.mesher.SubDone()
}

func (g *tcpsGate) onConnectionClosed() {
	g.DecConns()
}
func (g *tcpsGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
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
	arena   region
	// Conn states (zeros)
	tcpsConn0
}
type tcpsConn0 struct {
	filters  [32]uint8
	nFilters int8
}

func (c *TCPSConn) onGet(id int64, stage *Stage, mesher *TCPSMesher, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.arena.init()
}
func (c *TCPSConn) onPut() {
	c.stage = nil
	c.mesher = nil
	c.gate = nil
	c.netConn = nil
	c.rawConn = nil
	c.arena.free()
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

func (c *TCPSConn) hookFilter(filter TCPSFilter) {
	if c.nFilters == int8(len(c.filters)) {
		BugExitln("hook too many filters")
	}
	c.filters[c.nFilters] = filter.ID()
	c.nFilters++
}

func (c *TCPSConn) Read(p []byte) (n int, err error) {
	// TODO
	if c.nFilters > 0 {
	} else {
	}
	return c.netConn.Read(p)
}
func (c *TCPSConn) Write(p []byte) (n int, err error) {
	// TODO
	if c.nFilters > 0 {
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

// TCPSRunner
type TCPSRunner interface {
	Component
	Process(conn *TCPSConn) (next bool)
}

// TCPSRunner_
type TCPSRunner_ struct {
	// Mixins
	Component_
	// States
}

// TCPSFilter
type TCPSFilter interface {
	Component
	ider

	OnInput(conn *TCPSConn, kind int8)
	OnOutput(conn *TCPSConn, kind int8)
}

// TCPSFilter_
type TCPSFilter_ struct {
	Component_
	ider_
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSMesher, TCPSRunner, TCPSFilter]
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

func (c *tcpsCase) equalMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.equalMatch(value)
}
func (c *tcpsCase) prefixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.prefixMatch(value)
}
func (c *tcpsCase) suffixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.suffixMatch(value)
}
func (c *tcpsCase) wildcardMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.wildcardMatch(value)
}
func (c *tcpsCase) regexpMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.regexpMatch(value)
}
func (c *tcpsCase) notEqualMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notEqualMatch(value)
}
func (c *tcpsCase) notPrefixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notPrefixMatch(value)
}
func (c *tcpsCase) notSuffixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notSuffixMatch(value)
}
func (c *tcpsCase) notWildcardMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notWildcardMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notRegexpMatch(value)
}

func (c *tcpsCase) execute(conn *TCPSConn) (processed bool) {
	for _, filter := range c.filters {
		conn.hookFilter(filter)
	}
	for _, runner := range c.runners {
		if next := runner.Process(conn); !next {
			return true
		}
	}
	return false
}
