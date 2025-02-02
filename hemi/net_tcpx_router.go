// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) router implementation. See RFC 9293.

package hemi

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/library/system"
)

// TCPXRouter
type TCPXRouter struct {
	// Parent
	Server_[*tcpxGate]
	// Mixins
	_tcpxHolder_ // to carry configs used by gates
	_accessLogger_
	// Assocs
	dealets compDict[TCPXDealet] // defined dealets. indexed by component name
	cases   []*tcpxCase          // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	maxConcurrentConnsPerGate int32 // max concurrent connections allowed per gate
}

func (r *TCPXRouter) onCreate(compName string, stage *Stage) {
	r.Server_.OnCreate(compName, stage)
	r.dealets = make(compDict[TCPXDealet])
}

func (r *TCPXRouter) OnConfigure() {
	r.Server_.OnConfigure()
	r._tcpxHolder_.onConfigure(r)
	r._accessLogger_.onConfigure(r)

	// .maxConcurrentConnsPerGate
	r.ConfigureInt32("maxConcurrentConnsPerGate", &r.maxConcurrentConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConcurrentConnsPerGate has an invalid value")
	}, 10000)

	// sub components
	r.dealets.walk(TCPXDealet.OnConfigure)
	for _, kase := range r.cases {
		kase.OnConfigure()
	}
}
func (r *TCPXRouter) OnPrepare() {
	r.Server_.OnPrepare()
	r._tcpxHolder_.onPrepare(r)
	r._accessLogger_.onPrepare(r)

	// sub components
	r.dealets.walk(TCPXDealet.OnPrepare)
	for _, kase := range r.cases {
		kase.OnPrepare()
	}
}

func (r *TCPXRouter) MaxConcurrentConnsPerGate() int32 { return r.maxConcurrentConnsPerGate }

func (r *TCPXRouter) createDealet(compSign string, compName string) TCPXDealet {
	if _, ok := r.dealets[compName]; ok {
		UseExitln("conflicting dealet with a same component name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := tcpxDealetCreators[compSign]
	if !ok {
		UseExitln("unknown dealet sign: " + compSign)
	}
	dealet := create(compName, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[compName] = dealet
	return dealet
}
func (r *TCPXRouter) createCase(compName string) *tcpxCase {
	if r.hasCase(compName) {
		UseExitln("conflicting case with a same component name")
	}
	kase := new(tcpxCase)
	kase.onCreate(compName, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *TCPXRouter) hasCase(compName string) bool {
	for _, kase := range r.cases {
		if kase.CompName() == compName {
			return true
		}
	}
	return false
}

func (r *TCPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpxGate)
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
	r.dealets.goWalk(TCPXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	r.CloseLog()
	if DebugLevel() >= 2 {
		Printf("tcpxRouter=%s done\n", r.CompName())
	}
	r.stage.DecSub() // router
}

func (r *TCPXRouter) tcpxHolder() _tcpxHolder_ { return r._tcpxHolder_ }

func (r *TCPXRouter) serveConn(conn *TCPXConn) { // runner
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
	putTCPXConn(conn)
}

// tcpxGate is an opening gate of TCPXRouter.
type tcpxGate struct {
	// Parent
	Gate_[*TCPXRouter]
	// Mixins
	_tcpxHolder_
	// States
	maxConcurrentConns int32        // max concurrent conns allowed for this gate
	concurrentConns    atomic.Int32 // TODO: false sharing
	listener           net.Listener // the real gate. set after open
}

func (g *tcpxGate) onNew(router *TCPXRouter, id int32) {
	g.Gate_.OnNew(router, id)
	g._tcpxHolder_ = router.tcpxHolder()
	g.maxConcurrentConns = router.MaxConcurrentConnsPerGate()
	g.concurrentConns.Store(0)
}

func (g *tcpxGate) DecConcurrentConns() int32 { return g.concurrentConns.Add(-1) }
func (g *tcpxGate) IncConcurrentConns() int32 { return g.concurrentConns.Add(1) }
func (g *tcpxGate) ReachLimit(concurrentConns int32) bool {
	return concurrentConns > g.maxConcurrentConns
}

func (g *tcpxGate) Open() error {
	var (
		listener net.Listener
		err      error
	)
	if g.UDSMode() {
		address := g.Address()
		// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
		// This affects graceful upgrading, maybe we can implement fd transfer in the future.
		os.Remove(address)
		if listener, err = net.Listen("unix", address); err == nil {
			g.listener = listener.(*net.UnixListener)
		}
	} else {
		listenConfig := new(net.ListenConfig)
		listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
			// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option?
			return system.SetReusePort(rawConn)
		}
		if listener, err = listenConfig.Listen(context.Background(), "tcp", g.Address()); err == nil {
			g.listener = listener.(*net.TCPListener)
		}
	}
	return err
}
func (g *tcpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *tcpxGate) Serve() { // runner
	if g.UDSMode() {
		g.serveUDS()
	} else if g.TLSMode() {
		g.serveTLS()
	} else {
		g.serveTCP()
	}
}

func (g *tcpxGate) serveUDS() {
	listener := g.listener.(*net.UnixListener)
	connID := int64(1)
	for {
		udsConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSubConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			//g.server.Logf("tcpxGate=%d: too many UDS connections!\n", g.id)
			g.justClose(udsConn)
			continue
		}
		rawConn, err := udsConn.SyscallConn()
		if err != nil {
			g.justClose(udsConn)
			continue
		}
		conn := getTCPXConn(connID, g, udsConn, rawConn)
		go g.server.serveConn(conn) // conn is put to pool in serveConn()
		connID++
	}
	g.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSubGate()
}
func (g *tcpxGate) serveTLS() {
	listener := g.listener.(*net.TCPListener)
	connID := int64(1)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSubConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			//g.server.Logf("tcpxGate=%d: too many TLS connections!\n", g.id)
			g.justClose(tcpConn)
			continue
		}
		tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
		// TODO: configure timeout
		if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
			g.justClose(tlsConn)
			continue
		}
		conn := getTCPXConn(connID, g, tlsConn, nil)
		go g.server.serveConn(conn) // conn is put to pool in serveConn()
		connID++
	}
	g.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TLS done\n", g.id)
	}
	g.server.DecSubGate()
}
func (g *tcpxGate) serveTCP() {
	listener := g.listener.(*net.TCPListener)
	connID := int64(1)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSubConn()
		if concurrentConns := g.IncConcurrentConns(); g.ReachLimit(concurrentConns) {
			//g.server.Logf("tcpxGate=%d: too many TCP connections!\n", g.id)
			g.justClose(tcpConn)
			continue
		}
		rawConn, err := tcpConn.SyscallConn()
		if err != nil {
			g.justClose(tcpConn)
			continue
		}
		conn := getTCPXConn(connID, g, tcpConn, rawConn)
		if DebugLevel() >= 2 {
			Printf("%+v\n", conn)
		}
		go g.server.serveConn(conn) // conn is put to pool in serveConn()
		connID++
	}
	g.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSubGate()
}

func (g *tcpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecConcurrentConns()
	g.DecSubConn()
}

// TCPXConn is a TCPX connection coming from TCPXRouter.
type TCPXConn struct {
	// Parent
	tcpxConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *tcpxGate
	// Conn states (zeros)
}

var poolTCPXConn sync.Pool

func getTCPXConn(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) *TCPXConn {
	var conn *TCPXConn
	if x := poolTCPXConn.Get(); x == nil {
		conn = new(TCPXConn)
	} else {
		conn = x.(*TCPXConn)
	}
	conn.onGet(id, gate, netConn, rawConn)
	return conn
}
func putTCPXConn(conn *TCPXConn) {
	conn.onPut()
	poolTCPXConn.Put(conn)
}

func (c *TCPXConn) onGet(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.tcpxConn_.onGet(id, gate, netConn, rawConn)

	c.gate = gate
}
func (c *TCPXConn) onPut() {
	c.gate = nil

	c.tcpxConn_.onPut()
}

func (c *TCPXConn) CloseRead() {
	c._checkClose()
}
func (c *TCPXConn) CloseWrite() {
	if c.gate.UDSMode() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else if c.gate.TLSMode() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
	c._checkClose()
}
func (c *TCPXConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.Close()
	}
}

func (c *TCPXConn) Close() {
	c.gate.justClose(c.netConn)
}

func (c *TCPXConn) unsafeVariable(varCode int16, varName string) (varValue []byte) {
	return tcpxConnVariables[varCode](c)
}

// tcpxConnVariables
var tcpxConnVariables = [...]func(*TCPXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // udsMode
	3: nil, // tlsMode
	4: nil, // serverName
	5: nil, // nextProto
}

// tcpxCase
type tcpxCase struct {
	// Parent
	Component_
	// Assocs
	router  *TCPXRouter
	dealets []TCPXDealet
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
	matcher  func(kase *tcpxCase, conn *TCPXConn, value []byte) bool
}

func (c *tcpxCase) onCreate(compName string, router *TCPXRouter) {
	c.MakeComp(compName)
	c.router = router
}
func (c *tcpxCase) OnShutdown() {
	c.router.DecSub() // case
}

func (c *tcpxCase) OnConfigure() {
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
	if matcher, ok := tcpxCaseMatchers[cond.compare]; ok {
		c.matcher = matcher
	} else {
		UseExitln("unknown compare in case condition")
	}
}
func (c *tcpxCase) OnPrepare() {
}

func (c *tcpxCase) addDealet(dealet TCPXDealet) { c.dealets = append(c.dealets, dealet) }

func (c *tcpxCase) isMatch(conn *TCPXConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *tcpxCase) execute(conn *TCPXConn) (dealt bool) {
	for _, dealet := range c.dealets {
		if dealt := dealet.DealWith(conn); dealt {
			return true
		}
	}
	return false
}

var tcpxCaseMatchers = map[string]func(kase *tcpxCase, conn *TCPXConn, value []byte) bool{
	"==": (*tcpxCase).equalMatch,
	"^=": (*tcpxCase).prefixMatch,
	"$=": (*tcpxCase).suffixMatch,
	"*=": (*tcpxCase).containMatch,
	"~=": (*tcpxCase).regexpMatch,
	"!=": (*tcpxCase).notEqualMatch,
	"!^": (*tcpxCase).notPrefixMatch,
	"!$": (*tcpxCase).notSuffixMatch,
	"!*": (*tcpxCase).notContainMatch,
	"!~": (*tcpxCase).notRegexpMatch,
}

func (c *tcpxCase) equalMatch(conn *TCPXConn, value []byte) bool { // value == patterns
	return equalMatch(value, c.patterns)
}
func (c *tcpxCase) prefixMatch(conn *TCPXConn, value []byte) bool { // value ^= patterns
	return prefixMatch(value, c.patterns)
}
func (c *tcpxCase) suffixMatch(conn *TCPXConn, value []byte) bool { // value $= patterns
	return suffixMatch(value, c.patterns)
}
func (c *tcpxCase) containMatch(conn *TCPXConn, value []byte) bool { // value *= patterns
	return containMatch(value, c.patterns)
}
func (c *tcpxCase) regexpMatch(conn *TCPXConn, value []byte) bool { // value ~= patterns
	return regexpMatch(value, c.regexps)
}
func (c *tcpxCase) notEqualMatch(conn *TCPXConn, value []byte) bool { // value != patterns
	return notEqualMatch(value, c.patterns)
}
func (c *tcpxCase) notPrefixMatch(conn *TCPXConn, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, c.patterns)
}
func (c *tcpxCase) notSuffixMatch(conn *TCPXConn, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, c.patterns)
}
func (c *tcpxCase) notContainMatch(conn *TCPXConn, value []byte) bool { // value !* patterns
	return notContainMatch(value, c.patterns)
}
func (c *tcpxCase) notRegexpMatch(conn *TCPXConn, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, c.regexps)
}

// TCPXDealet
type TCPXDealet interface {
	// Imports
	Component
	// Methods
	DealWith(conn *TCPXConn) (dealt bool)
}

// TCPXDealet_
type TCPXDealet_ struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (d *TCPXDealet_) OnCreate(compName string, stage *Stage) {
	d.MakeComp(compName)
	d.stage = stage
}

func (d *TCPXDealet_) Stage() *Stage { return d.stage }
