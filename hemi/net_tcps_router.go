// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPS (TCP/TLS/UDS) router.

package hemi

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/library/system"
)

// TCPSRouter
type TCPSRouter struct {
	// Parent
	Server_[*tcpsGate]
	// Assocs
	dealets compDict[TCPSDealet] // defined dealets. indexed by name
	cases   compList[*tcpsCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *logcfg // ...
	logger    *logger // router access logger
}

func (r *TCPSRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[TCPSDealet])
}
func (r *TCPSRouter) OnShutdown() {
	r.Server_.OnShutdown()
}

func (r *TCPSRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO

	// sub components
	r.dealets.walk(TCPSDealet.OnConfigure)
	r.cases.walk((*tcpsCase).OnConfigure)
}
func (r *TCPSRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = newLogger(r.accessLog.logFile, r.accessLog.rotate)
	}

	// sub components
	r.dealets.walk(TCPSDealet.OnPrepare)
	r.cases.walk((*tcpsCase).OnPrepare)
}

func (r *TCPSRouter) createDealet(sign string, name string) TCPSDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := tcpsDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
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
func (r *TCPSRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *TCPSRouter) Log(str string) {
}
func (r *TCPSRouter) Logln(str string) {
}
func (r *TCPSRouter) Logf(str string) {
}

func (r *TCPSRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub()
		if r.IsTLS() {
			go gate.serveTLS()
		} else if r.IsUDS() {
			go gate.serveUDS()
		} else {
			go gate.serveTCP()
		}
	}
	r.WaitSubs() // gates

	r.SubsAddn(len(r.dealets) + len(r.cases))
	r.cases.walk((*tcpsCase).OnShutdown)
	r.dealets.walk(TCPSDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("tcpsRouter=%s done\n", r.Name())
	}
	r.stage.DecSub()
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

// tcpsGate is an opening gate of TCPSRouter.
type tcpsGate struct {
	// Parent
	Gate_
	// Assocs
	router *TCPSRouter
	// States
	listener net.Listener // the real gate. set after open
}

func (g *tcpsGate) init(id int32, router *TCPSRouter) {
	g.Gate_.Init(id, router.MaxConnsPerGate())
	g.router = router
}

func (g *tcpsGate) Server() Server  { return g.router }
func (g *tcpsGate) Address() string { return g.router.Address() }
func (g *tcpsGate) IsTLS() bool     { return g.router.IsTLS() }
func (g *tcpsGate) IsUDS() bool     { return g.router.IsUDS() }

func (g *tcpsGate) Open() error {
	if g.IsUDS() {
		return g._openUnix()
	} else {
		return g._openInet()
	}
}
func (g *tcpsGate) _openUnix() error {
	address := g.Address()
	// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
	// This affects graceful upgrading, maybe we can implement fd transfer in the future.
	os.Remove(address)
	listener, err := net.Listen("unix", address)
	if err == nil {
		g.listener = listener.(*net.UnixListener)
	}
	return err
}
func (g *tcpsGate) _openInet() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option?
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
	return g.listener.Close() // breaks serve()
}

func (g *tcpsGate) serveTLS() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.router.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			conn := getTCPSConn(connID, g, tlsConn, nil)
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpsGate=%d TLS done\n", g.id)
	}
	g.router.DecSub()
}
func (g *tcpsGate) serveUDS() { // runner
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
	for {
		unixConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				continue
			}
			conn := getTCPSConn(connID, g, unixConn, rawConn)
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpsGate=%d TCP done\n", g.id)
	}
	g.router.DecSub()
}
func (g *tcpsGate) serveTCP() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				continue
			}
			conn := getTCPSConn(connID, g, tcpConn, rawConn)
			if DebugLevel() >= 1 {
				Printf("%+v\n", conn)
			}
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpsGate=%d TCP done\n", g.id)
	}
	g.router.DecSub()
}

func (g *tcpsGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
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
	// Parent
	Component_
	// States
}

// tcpsCase
type tcpsCase struct {
	// Parent
	Component_
	// Assocs
	router  *TCPSRouter
	dealets []TCPSDealet
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
	matcher  func(kase *tcpsCase, conn *TCPSConn, value []byte) bool
}

func (c *tcpsCase) onCreate(name string, router *TCPSRouter) {
	c.MakeComp(name)
	c.router = router
}
func (c *tcpsCase) OnShutdown() {
	c.router.DecSub()
}

func (c *tcpsCase) OnConfigure() {
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
	if matcher, ok := tcpsCaseMatchers[cond.compare]; ok {
		c.matcher = matcher
	} else {
		UseExitln("unknown compare in case condition")
	}
}
func (c *tcpsCase) OnPrepare() {
}

func (c *tcpsCase) addDealet(dealet TCPSDealet) { c.dealets = append(c.dealets, dealet) }

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
	return equalMatch(value, c.patterns)
}
func (c *tcpsCase) prefixMatch(conn *TCPSConn, value []byte) bool { // value ^= patterns
	return prefixMatch(value, c.patterns)
}
func (c *tcpsCase) suffixMatch(conn *TCPSConn, value []byte) bool { // value $= patterns
	return suffixMatch(value, c.patterns)
}
func (c *tcpsCase) containMatch(conn *TCPSConn, value []byte) bool { // value *= patterns
	return containMatch(value, c.patterns)
}
func (c *tcpsCase) regexpMatch(conn *TCPSConn, value []byte) bool { // value ~= patterns
	return regexpMatch(value, c.regexps)
}
func (c *tcpsCase) notEqualMatch(conn *TCPSConn, value []byte) bool { // value != patterns
	return notEqualMatch(value, c.patterns)
}
func (c *tcpsCase) notPrefixMatch(conn *TCPSConn, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, c.patterns)
}
func (c *tcpsCase) notSuffixMatch(conn *TCPSConn, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, c.patterns)
}
func (c *tcpsCase) notContainMatch(conn *TCPSConn, value []byte) bool { // value !* patterns
	return notContainMatch(value, c.patterns)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, c.regexps)
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

// TCPSConn is a TCPS connection coming from TCPSRouter.
type TCPSConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	stockInput  [8192]byte // for c.input
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	router    *TCPSRouter
	gate      *tcpsGate
	netConn   net.Conn        // the connection (TCP/TLS/UDS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	region    Region          // a region-based memory pool
	input     []byte          // input buffer
	closeSema atomic.Int32
	// Conn states (zeros)
}

func (c *TCPSConn) onGet(id int64, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id)
	c.router = gate.router
	c.gate = gate
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
	c.gate = nil
	c.router = nil
	c.ServerConn_.OnPut()
}

func (c *TCPSConn) IsTLS() bool { return c.router.IsTLS() }
func (c *TCPSConn) IsUDS() bool { return c.router.IsUDS() }

func (c *TCPSConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.router.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *TCPSConn) serve() { // runner
	c.router.dispatch(c)
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
	if c.router.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else if c.router.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
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
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varCodes
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isTLS
	nil, // isUDS
	nil, // serverName
	nil, // nextProto
}
