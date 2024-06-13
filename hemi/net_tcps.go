// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPS (TCP/TLS/UDS) router, reverse proxy, and backend.

package hemi

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/common/system"
)

func init() {
	RegisterTCPSDealet("tcpsProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(tcpsProxy)
		d.onCreate(name, stage, router)
		return d
	})
	RegisterBackend("tcpsBackend", func(name string, stage *Stage) Backend {
		b := new(TCPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

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
	// States
	listener net.Listener // the real gate. set after open
}

func (g *tcpsGate) init(id int32, router *TCPSRouter) {
	g.Gate_.Init(id, router)
}

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
			tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
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
	g.server.DecSub()
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
	g.server.DecSub()
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
	g.server.DecSub()
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
	netConn   net.Conn        // the connection (TCP/TLS/UDS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	region    Region          // a region-based memory pool
	input     []byte          // input buffer
	closeSema atomic.Int32
	// Conn states (zeros)
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
	c.ServerConn_.OnPut()
}

func (c *TCPSConn) serve() { // runner
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
	if router := c.Server(); router.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else if router.IsUDS() {
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

// tcpsProxy passes TCPS connections to TCPS backends.
type tcpsProxy struct {
	// Parent
	TCPSDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPSRouter  // the router to which the dealet belongs
	backend *TCPSBackend // the backend to pass to
	// States
}

func (d *tcpsProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpsProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *tcpsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				d.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpsProxy proxy")
	}
}
func (d *tcpsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpsProxy) Deal(conn *TCPSConn) (dealt bool) {
	// TODO
	return true
}

// TCPSBackend component.
type TCPSBackend struct {
	// Parent
	Backend_[*tcpsNode]
	// Mixins
	// States
}

func (b *TCPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *TCPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *TCPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *TCPSBackend) CreateNode(name string) Node {
	node := new(tcpsNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *TCPSBackend) Dial() (*TConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Parent
	Node_
	// Assocs
	// States
	connPool struct { // free list of conns in this node
		sync.Mutex
		head *TConn // head element
		tail *TConn // tail element
		qnty int    // size of the list
	}
}

func (n *tcpsNode) onCreate(name string, backend *TCPSBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *tcpsNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *tcpsNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *tcpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	/*
		if size := n.closeFree(); size > 0 {
			n.SubsAddn(-size)
		}
		n.WaitSubs() // conns. TODO: max timeout?
	*/
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s\n", n.name, n.address)
	}
	var (
		tConn *TConn
		err   error
	)
	if n.IsTLS() {
		tConn, err = n._dialTLS()
	} else if n.IsUDS() {
		tConn, err = n._dialUDS()
	} else {
		tConn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSub()
	return tConn, err
}
func (n *tcpsNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	tlsConn := tls.Client(netConn, n.tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	return getTConn(connID, n, tlsConn, nil), nil
}
func (n *tcpsNode) _dialUDS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}
func (n *tcpsNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

// poolTConn
var poolTConn sync.Pool

func getTConn(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var tConn *TConn
	if x := poolTConn.Get(); x == nil {
		tConn = new(TConn)
	} else {
		tConn = x.(*TConn)
	}
	tConn.onGet(id, node, netConn, rawConn)
	return tConn
}
func putTConn(tConn *TConn) {
	tConn.onPut()
	poolTConn.Put(tConn)
}

// TConn is a backend-side connection to tcpsNode.
type TConn struct {
	// Parent
	BackendConn_
	// Assocs
	next *TConn // the linked-list
	// Conn states (stocks)
	stockInput [8192]byte // for c.input
	// Conn states (controlled)
	// Conn states (non-zeros)
	input   []byte
	netConn net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn syscall.RawConn // for syscall. only usable when netConn is TCP/UDS
	// Conn states (zeros)
	writeBroken atomic.Bool // write-side broken?
	readBroken  atomic.Bool // read-side broken?
}

func (c *TConn) onGet(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c.input = c.stockInput[:]
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *TConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.input = nil
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *TConn) TLSConn() *tls.Conn     { return c.netConn.(*tls.Conn) }
func (c *TConn) UDSConn() *net.UnixConn { return c.netConn.(*net.UnixConn) }
func (c *TConn) TCPConn() *net.TCPConn  { return c.netConn.(*net.TCPConn) }

func (c *TConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *TConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *TConn) Write(p []byte) (n int, err error)         { return c.netConn.Write(p) }
func (c *TConn) Writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(c.netConn) }
func (c *TConn) Read(p []byte) (n int, err error)          { return c.netConn.Read(p) }
func (c *TConn) ReadFull(p []byte) (n int, err error)      { return io.ReadFull(c.netConn, p) }
func (c *TConn) ReadAtLeast(p []byte, min int) (n int, err error) {
	return io.ReadAtLeast(c.netConn, p, min)
}

func (c *TConn) ProxyPass(conn *TCPSConn) error {
	var (
		p   []byte
		err error
	)
	for {
		p, err = conn.Recv()
		if err != nil {
			return err
		}
		if p == nil {
			if err = c.CloseWrite(); err != nil {
				c.markWriteBroken()
				return err
			}
			return nil
		}
		if _, err = c.Write(p); err != nil {
			c.markWriteBroken()
			return err
		}
	}
}

func (c *TConn) IsBroken() bool { return c.writeBroken.Load() || c.readBroken.Load() }
func (c *TConn) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}

func (c *TConn) markWriteBroken() { c.writeBroken.Store(true) }
func (c *TConn) markReadBroken()  { c.readBroken.Store(true) }

func (c *TConn) CloseWrite() error {
	if c.IsTLS() {
		return c.netConn.(*tls.Conn).CloseWrite()
	} else if c.IsUDS() {
		return c.netConn.(*net.UnixConn).CloseWrite()
	} else {
		return c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TConn) Close() error {
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}
