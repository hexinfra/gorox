// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCPS (TCP/TLS/UDS) reverse proxy.

package hemi

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

// TCPSRouter
type TCPSRouter struct {
	// Mixins
	router_[*TCPSRouter, *tcpsGate, TCPSDealet, *tcpsCase]
}

func (r *TCPSRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, tcpsDealetCreators)
}
func (r *TCPSRouter) OnShutdown() {
	r.router_.onShutdown()
}

func (r *TCPSRouter) OnConfigure() {
	r.router_.onConfigure()
	// configure r here
	r.configureSubs()
}
func (r *TCPSRouter) OnPrepare() {
	r.router_.onPrepare()
	// prepare r here
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

func (r *TCPSRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub(1)
		if r.IsUDS() {
			go gate.serveUDS()
		} else if r.IsTLS() {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealets) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if Debug() >= 2 {
		Printf("tcpsRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

func (r *TCPSRouter) dispatch(conn *TCPSConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// TCPSDealet
type TCPSDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *TCPSConn) (dealt bool)
}

// TCPSDealet_
type TCPSDealet_ struct {
	// Mixins
	Component_
	// States
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSRouter, TCPSDealet]
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
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *tcpsCase) execute(conn *TCPSConn) (dealt bool) {
	for _, dealet := range c.dealets {
		if dealt := dealet.Deal(conn); dealt {
			return true
		}
	}
	return false
}

func (c *tcpsCase) equalMatch(conn *TCPSConn, value []byte) bool { // value == patterns
	return c.case_._equalMatch(value)
}
func (c *tcpsCase) prefixMatch(conn *TCPSConn, value []byte) bool { // value ^= patterns
	return c.case_._prefixMatch(value)
}
func (c *tcpsCase) suffixMatch(conn *TCPSConn, value []byte) bool { // value $= patterns
	return c.case_._suffixMatch(value)
}
func (c *tcpsCase) containMatch(conn *TCPSConn, value []byte) bool { // value *= patterns
	return c.case_._containMatch(value)
}
func (c *tcpsCase) regexpMatch(conn *TCPSConn, value []byte) bool { // value ~= patterns
	return c.case_._regexpMatch(value)
}
func (c *tcpsCase) notEqualMatch(conn *TCPSConn, value []byte) bool { // value != patterns
	return c.case_._notEqualMatch(value)
}
func (c *tcpsCase) notPrefixMatch(conn *TCPSConn, value []byte) bool { // value !^ patterns
	return c.case_._notPrefixMatch(value)
}
func (c *tcpsCase) notSuffixMatch(conn *TCPSConn, value []byte) bool { // value !$ patterns
	return c.case_._notSuffixMatch(value)
}
func (c *tcpsCase) notContainMatch(conn *TCPSConn, value []byte) bool { // value !* patterns
	return c.case_._notContainMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool { // value !~ patterns
	return c.case_._notRegexpMatch(value)
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

// tcpsGate is an opening gate of TCPSRouter.
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	// States
	listener *net.TCPListener // the real gate. set after open
}

func (g *tcpsGate) init(id int32, router *TCPSRouter) {
	g.Gate_.Init(id, router)
}

func (g *tcpsGate) Open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option
		return system.SetReusePort(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.listener = listener.(*net.TCPListener)
	}
	return err
}
func (g *tcpsGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *tcpsGate) serveTCP() { // runner
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
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
			conn := getTCPSConn(connID, g, tcpConn, rawConn)
			if Debug() >= 1 {
				Printf("%+v\n", conn)
			}
			go conn.mesh() // conn is put to pool in mesh()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("tcpsGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}
func (g *tcpsGate) serveTLS() { // runner
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
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
			tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			conn := getTCPSConn(connID, g, tlsConn, nil)
			go conn.mesh() // conn is put to pool in mesh()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("tcpsGate=%d TLS done\n", g.id)
	}
	g.server.SubDone()
}
func (g *tcpsGate) serveUDS() { // runner
	// TODO
}

func (g *tcpsGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
}

// poolTCPSConn
var poolTCPSConn sync.Pool

func getTCPSConn(id int64, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) *TCPSConn {
	var tcpsConn *TCPSConn
	if x := poolTCPSConn.Get(); x == nil {
		tcpsConn = new(TCPSConn)
	} else {
		tcpsConn = x.(*TCPSConn)
	}
	tcpsConn.onGet(id, gate, netConn, rawConn)
	return tcpsConn
}
func putTCPSConn(tcpsConn *TCPSConn) {
	tcpsConn.onPut()
	poolTCPSConn.Put(tcpsConn)
}

// TCPSConn is a TCP/TLS/UDS connection coming from TCPSRouter.
type TCPSConn struct {
	// Mixins
	ServerConn_
	// Conn states (stocks)
	stockInput  [8192]byte // for c.input
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn   net.Conn        // the connection (TCP/TLS/UDS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	region    Region          // a region-based memory pool
	input     []byte          // input buffer
	closeSema atomic.Int32
	// Conn states (zeros)
	tcpsConn0
}
type tcpsConn0 struct { // for fast reset, entirely
}

func (c *TCPSConn) onGet(id int64, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id, gate)
	c.netConn = netConn
	c.rawConn = rawConn
	c.region.Init()
	c.input = c.stockInput[:]
	c.closeSema.Store(2)
}
func (c *TCPSConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.region.Free()
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
		c.input = nil
	}
	c.tcpsConn0 = tcpsConn0{}
	c.ServerConn_.OnPut()
}

func (c *TCPSConn) mesh() { // runner
	router := c.Server().(*TCPSRouter)
	router.dispatch(c)
	c.closeConn()
	putTCPSConn(c)
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
	if router := c.Server(); router.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else if router.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TCPSConn) closeConn() {
	c.netConn.Close()
	c.gate.OnConnClosed()
}

func (c *TCPSConn) unsafeVariable(code int16, name string) (value []byte) {
	return tcpsConnVariables[code](c)
}

// tcpsConnVariables
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varCodes in config.go
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // udsMode
	nil, // tlsMode
	nil, // serverName
	nil, // nextProto
}
