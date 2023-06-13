// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS network mesher.

package internal

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
)

// TCPSMesher
type TCPSMesher struct {
	// Mixins
	mesher_[*TCPSMesher, *tcpsGate, TCPSDealer, *tcpsCase]
}

func (m *TCPSMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, tcpsDealerCreators)
}
func (m *TCPSMesher) OnShutdown() {
	// We don't close(m.Shut) here.
	for _, gate := range m.gates {
		gate.shut()
	}
}

func (m *TCPSMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// configure m here
	m.configureSubs()
}
func (m *TCPSMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// prepare m here
	m.prepareSubs()
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
	m.IncSub(len(m.dealers) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // dealers, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if Debug() >= 2 {
		Printf("tcpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

// tcpsGate is an opening gate of TCPSMesher.
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	mesher *TCPSMesher
	// States
	gate *net.TCPListener // the real gate. set after open
}

func (g *tcpsGate) init(mesher *TCPSMesher, id int32) {
	g.Gate_.Init(mesher.stage, id, mesher.address, mesher.maxConnsPerGate)
	g.mesher = mesher
}

func (g *tcpsGate) open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option
		return system.SetReusePort(rawConn)
	}
	gate, err := listenConfig.Listen(context.Background(), "tcp", g.address)
	if err == nil {
		g.gate = gate.(*net.TCPListener)
	}
	return err
}
func (g *tcpsGate) shut() error {
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
				g.justClose(tcpConn)
				continue
			}
			conn := getTCPSConn(connID, g.stage, g.mesher, g, tcpConn, rawConn)
			if Debug() >= 1 {
				Printf("%+v\n", conn)
			}
			go g.execute(conn) // conn is put to pool in execute()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("tcpsGate=%d TCP done\n", g.id)
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
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			conn := getTCPSConn(connID, g.stage, g.mesher, g, tlsConn, nil)
			go g.execute(conn) // conn is put to pool in execute()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("tcpsGate=%d TLS done\n", g.id)
	}
	g.mesher.SubDone()
}

func (g *tcpsGate) execute(conn *TCPSConn) { // goroutine
	for _, kase := range g.mesher.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if processed := kase.execute(conn); processed {
			break
		}
	}
	conn.closeConn()
	putTCPSConn(conn)
}

func (g *tcpsGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.onConnClosed()
}
func (g *tcpsGate) onConnClosed() {
	g.DecConns()
	g.SubDone()
}

// TCPSDealer
type TCPSDealer interface {
	// Imports
	Component
	// Methods
	Deal(conn *TCPSConn) (next bool)
}

// TCPSDealer_
type TCPSDealer_ struct {
	// Mixins
	Component_
	// States
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSMesher, TCPSDealer]
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
	value := conn.unsafeVariable(c.varIndex)
	return c.matcher(c, conn, value)
}

var tcpsCaseMatchers = map[string]func(kase *tcpsCase, conn *TCPSConn, value []byte) bool{
	"==": (*tcpsCase).equalMatch,
	"^=": (*tcpsCase).prefixMatch,
	"$=": (*tcpsCase).suffixMatch,
	"*=": (*tcpsCase).containMatch,
	"~=": (*tcpsCase).regexpMatch,
	"!=": (*tcpsCase).notEqualMatch,
	"!^": (*tcpsCase).notPrefixMatch,
	"!$": (*tcpsCase).notSuffixMatch,
	"!*": (*tcpsCase).notContainMatch,
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
func (c *tcpsCase) containMatch(conn *TCPSConn, value []byte) bool { // value *= patterns
	return c.case_.containMatch(value)
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
func (c *tcpsCase) notContainMatch(conn *TCPSConn, value []byte) bool { // value !* patterns
	return c.case_.notContainMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *tcpsCase) execute(conn *TCPSConn) (processed bool) {
	for _, dealer := range c.dealers {
		if next := dealer.Deal(conn); !next {
			return true
		}
	}
	return false
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

// TCPSConn is the TCP/TLS connection coming from TCPSMesher.
type TCPSConn struct {
	// Conn states (stocks)
	stockInput [8192]byte // for c.input
	// Conn states (controlled)
	// Conn states (non-zeros)
	id        int64           // connection id
	stage     *Stage          // current stage
	mesher    *TCPSMesher     // from mesher
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
}

func (c *TCPSConn) onGet(id int64, stage *Stage, mesher *TCPSMesher, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.mesher = mesher
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.region.Init()
	c.input = c.stockInput[:]
	c.closeSema.Store(2)
}
func (c *TCPSConn) onPut() {
	c.stage = nil
	c.mesher = nil
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

func (c *TCPSConn) Recv() (p []byte, err error) { // p == nil means EOF
	// TODO: deadline
	n, err := c.netConn.Read(c.input)
	if n > 0 {
		p = c.input[:n]
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
	if c.mesher.TLSMode() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TCPSConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnClosed()
}

func (c *TCPSConn) unsafeVariable(index int16) (value []byte) {
	return tcpsConnVariables[index](c)
}

// tcpsConnVariables
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varIndexes in config.go
	nil, // srcHost
	nil, // srcPort
	nil, // transport
	nil, // serverName
	nil, // nextProto
}
