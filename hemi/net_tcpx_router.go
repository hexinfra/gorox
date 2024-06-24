// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) router.

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

// TCPXRouter
type TCPXRouter struct {
	// Parent
	Server_[*tcpxGate]
	// Assocs
	dealets compDict[TCPXDealet] // defined dealets. indexed by name
	cases   compList[*tcpxCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *LogConfig // ...
	logger    *Logger    // router access logger
}

func (r *TCPXRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[TCPXDealet])
}

func (r *TCPXRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO

	// sub components
	r.dealets.walk(TCPXDealet.OnConfigure)
	r.cases.walk((*tcpxCase).OnConfigure)
}
func (r *TCPXRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog.logFile, r.accessLog.rotate)
	}

	// sub components
	r.dealets.walk(TCPXDealet.OnPrepare)
	r.cases.walk((*tcpxCase).OnPrepare)
}

func (r *TCPXRouter) createDealet(sign string, name string) TCPXDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := tcpxDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
}
func (r *TCPXRouter) createCase(name string) *tcpxCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(tcpxCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *TCPXRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *TCPXRouter) Log(s string) {
	// TODO
}
func (r *TCPXRouter) Logln(s string) {
	// TODO
}
func (r *TCPXRouter) Logf(f string, v ...any) {
	// TODO
}

func (r *TCPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpxGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		if r.IsTLS() {
			go gate.serveTLS()
		} else if r.IsUDS() {
			go gate.serveUDS()
		} else {
			go gate.serveTCP()
		}
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	r.cases.walk((*tcpxCase).OnShutdown)
	r.dealets.walk(TCPXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("tcpxRouter=%s done\n", r.Name())
	}
	r.stage.DecSub() // router
}

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
	Gate_
	// Assocs
	router *TCPXRouter
	// States
	listener net.Listener // the real gate. set after open
}

func (g *tcpxGate) init(id int32, router *TCPXRouter) {
	g.Gate_.Init(id, router.MaxConnsPerGate())
	g.router = router
}

func (g *tcpxGate) Server() Server  { return g.router }
func (g *tcpxGate) Address() string { return g.router.Address() }
func (g *tcpxGate) IsTLS() bool     { return g.router.IsTLS() }
func (g *tcpxGate) IsUDS() bool     { return g.router.IsUDS() }

func (g *tcpxGate) Open() error {
	var (
		listener net.Listener
		err      error
	)
	if g.IsUDS() {
		address := g.Address()
		// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
		// This affects graceful upgrading, maybe we can implement fd transfer in the future.
		os.Remove(address)
		listener, err = net.Listen("unix", address)
		if err == nil {
			g.listener = listener.(*net.UnixListener)
		}
	} else {
		listenConfig := new(net.ListenConfig)
		listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
			// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option?
			return system.SetReusePort(rawConn)
		}
		listener, err = listenConfig.Listen(context.Background(), "tcp", g.Address())
		if err == nil {
			g.listener = listener.(*net.TCPListener)
		}
	}
	return err
}
func (g *tcpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *tcpxGate) serveTLS() { // runner
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
		g.IncSub() // conn
		if g.ReachLimit() {
			g.router.Logf("tcpxGate=%d: too many TLS connections!\n", g.id)
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.router.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			conn := getTCPXConn(connID, g, tlsConn, nil)
			go g.router.serveConn(conn) // conn is put to pool in serveConn()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TLS done\n", g.id)
	}
	g.router.DecSub() // gate
}
func (g *tcpxGate) serveUDS() { // runner
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
		g.IncSub() // conn
		if g.ReachLimit() {
			g.router.Logf("tcpxGate=%d: too many UDS connections!\n", g.id)
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				continue
			}
			conn := getTCPXConn(connID, g, unixConn, rawConn)
			go g.router.serveConn(conn) // conn is put to pool in serveConn()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.router.DecSub() // gate
}
func (g *tcpxGate) serveTCP() { // runner
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
		g.IncSub() // conn
		if g.ReachLimit() {
			g.router.Logf("tcpxGate=%d: too many TCP connections!\n", g.id)
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				continue
			}
			conn := getTCPXConn(connID, g, tcpConn, rawConn)
			if DebugLevel() >= 2 {
				Printf("%+v\n", conn)
			}
			go g.router.serveConn(conn) // conn is put to pool in serveConn()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.router.DecSub() // gate
}

func (g *tcpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecConns()
	g.DecSub()
}

// poolTCPXConn
var poolTCPXConn sync.Pool

func getTCPXConn(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) *TCPXConn {
	var tcpxConn *TCPXConn
	if x := poolTCPXConn.Get(); x == nil {
		tcpxConn = new(TCPXConn)
		tcpxConn.waitChan = make(chan struct{}, 1)
	} else {
		tcpxConn = x.(*TCPXConn)
	}
	tcpxConn.onGet(id, gate, netConn, rawConn)
	return tcpxConn
}
func putTCPXConn(tcpxConn *TCPXConn) {
	tcpxConn.onPut()
	poolTCPXConn.Put(tcpxConn)
}

// TCPXConn is a TCPX connection coming from TCPXRouter.
type TCPXConn struct {
	// Conn states (stocks)
	stockInput  [8192]byte // for c.input
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	waitChan chan struct{} // ...
	// Conn states (non-zeros)
	id        int64 // the conn id
	gate      *tcpxGate
	netConn   net.Conn        // the connection (TCP/TLS/UDS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	input     []byte          // input buffer
	region    Region          // a region-based memory pool
	closeSema atomic.Int32    // controls read/write close
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastRead  time.Time    // deadline of last read operation
	lastWrite time.Time    // deadline of last write operation
}

func (c *TCPXConn) onGet(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.input = c.stockInput[:]
	c.region.Init()
	c.closeSema.Store(2)
}
func (c *TCPXConn) onPut() {
	c.region.Free()
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
	}
	c.input = nil
	c.netConn = nil
	c.rawConn = nil
	c.gate = nil

	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
}

func (c *TCPXConn) IsTLS() bool { return c.gate.IsTLS() }
func (c *TCPXConn) IsUDS() bool { return c.gate.IsUDS() }

func (c *TCPXConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.gate.router.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *TCPXConn) SetReadDeadline() error {
	deadline := time.Now().Add(c.gate.router.ReadTimeout())
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *TCPXConn) SetWriteDeadline() error {
	deadline := time.Now().Add(c.gate.router.WriteTimeout())
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *TCPXConn) Recv() (p []byte, err error) {
	n, err := c.netConn.Read(c.input)
	p = c.input[:n]
	return
}
func (c *TCPXConn) Send(p []byte) (err error) {
	_, err = c.netConn.Write(p)
	return
}

func (c *TCPXConn) CloseRead() {
	c._checkClose()
}
func (c *TCPXConn) CloseWrite() {
	if router := c.gate.router; router.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else if router.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
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

func (c *TCPXConn) done() { c.waitChan <- struct{}{} }
func (c *TCPXConn) wait() { <-c.waitChan }

func (c *TCPXConn) unsafeVariable(code int16, name string) (value []byte) {
	return tcpxConnVariables[code](c)
}

// tcpxConnVariables
var tcpxConnVariables = [...]func(*TCPXConn) []byte{ // keep sync with varCodes
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isTLS
	nil, // isUDS
	nil, // serverName
	nil, // nextProto
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

func (c *tcpxCase) onCreate(name string, router *TCPXRouter) {
	c.MakeComp(name)
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
		if dealt := dealet.Deal(conn); dealt {
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
	Deal(conn *TCPXConn) (dealt bool)
}

// TCPXDealet_
type TCPXDealet_ struct {
	// Parent
	Component_
	// States
}
