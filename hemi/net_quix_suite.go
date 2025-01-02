// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (QUIC over UDP/UDS) router, reverse proxy, and backend implementation. See RFC 8999, RFC 9000, RFC 9001, and RFC 9002.

package hemi

import (
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/quic"
)

//////////////////////////////////////// QUIX router implementation ////////////////////////////////////////

// QUIXRouter
type QUIXRouter struct {
	// Parent
	Server_[*quixGate]
	// Assocs
	dealets compDict[QUIXDealet] // defined dealets. indexed by name
	cases   compList[*quixCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *LogConfig // ...
	logger    *Logger    // router access logger
}

func (r *QUIXRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[QUIXDealet])
}

func (r *QUIXRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// sub components
	r.dealets.walk(QUIXDealet.OnConfigure)
	r.cases.walk((*quixCase).OnConfigure)
}
func (r *QUIXRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog)
	}

	// sub components
	r.dealets.walk(QUIXDealet.OnPrepare)
	r.cases.walk((*quixCase).OnPrepare)
}

func (r *QUIXRouter) createDealet(sign string, name string) QUIXDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := quixDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
}
func (r *QUIXRouter) createCase(name string) *quixCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quixCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *QUIXRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *QUIXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quixGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		go gate.serveTLS()
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	r.cases.walk((*quixCase).OnShutdown)
	r.dealets.walk(QUIXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("quixRouter=%s done\n", r.Name())
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
	Gate_
	// Assocs
	router *QUIXRouter
	// States
	maxActives int32          // max concurrent conns allowed for this gate
	curActives atomic.Int32   // TODO: false sharing
	listener   *quic.Listener // the real gate. set after open
}

func (g *quixGate) init(id int32, router *QUIXRouter) {
	g.Gate_.Init(id)
	g.maxActives = router.MaxConnsPerGate()
	g.curActives.Store(0)
	g.router = router
}

func (g *quixGate) DecActives() int32             { return g.curActives.Add(-1) }
func (g *quixGate) IncActives() int32             { return g.curActives.Add(1) }
func (g *quixGate) ReachLimit(actives int32) bool { return actives > g.maxActives }

func (g *quixGate) Server() Server  { return g.router }
func (g *quixGate) Address() string { return g.router.Address() }
func (g *quixGate) IsUDS() bool     { return g.router.IsUDS() }
func (g *quixGate) IsTLS() bool     { return g.router.IsTLS() }

func (g *quixGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quixGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *quixGate) serveUDS() { // runner
	// TODO
}
func (g *quixGate) serveTLS() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.router.DecSub() // gate
}

func (g *quixGate) justClose(quicConn *quic.Conn) {
	quicConn.Close()
	g.DecConn()
}

// QUIXConn is a QUIX connection coming from QUIXRouter.
type QUIXConn struct {
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	id       int64
	gate     *quixGate
	quicConn *quic.Conn
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastRead  time.Time    // deadline of last read operation
	lastWrite time.Time    // deadline of last write operation
}

var poolQUIXConn sync.Pool

func getQUIXConn(id int64, gate *quixGate, quicConn *quic.Conn) *QUIXConn {
	var quixConn *QUIXConn
	if x := poolQUIXConn.Get(); x == nil {
		quixConn = new(QUIXConn)
	} else {
		quixConn = x.(*QUIXConn)
	}
	quixConn.onGet(id, gate, quicConn)
	return quixConn
}
func putQUIXConn(quixConn *QUIXConn) {
	quixConn.onPut()
	poolQUIXConn.Put(quixConn)
}

func (c *QUIXConn) onGet(id int64, gate *quixGate, quicConn *quic.Conn) {
	c.id = id
	c.gate = gate
	c.quicConn = quicConn
}
func (c *QUIXConn) onPut() {
	c.quicConn = nil
	c.gate = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
}

func (c *QUIXConn) IsUDS() bool { return c.gate.IsUDS() }
func (c *QUIXConn) IsTLS() bool { return c.gate.IsTLS() }

func (c *QUIXConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.gate.router.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *QUIXConn) closeConn() error {
	// TODO
	return nil
}

func (c *QUIXConn) unsafeVariable(code int16, name string) (value []byte) {
	return quixConnVariables[code](c)
}

// quixConnVariables
var quixConnVariables = [...]func(*QUIXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // isUDS
	3: nil, // isTLS
}

// QUIXStream
type QUIXStream struct {
	// Parent
	// Stream states (non-zeros)
	quicStream *quic.Stream
	// Stream states (zeros)
}

func (s *QUIXStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUIXStream) Read(p []byte) (n int, err error) {
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

func (c *quixCase) onCreate(name string, router *QUIXRouter) {
	c.MakeComp(name)
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
	Deal(conn *QUIXConn, stream *QUIXStream) (dealt bool)
}

// QUIXDealet_
type QUIXDealet_ struct {
	// Parent
	Component_
	// States
}

//////////////////////////////////////// QUIX reverse proxy implementation ////////////////////////////////////////

func init() {
	RegisterQUIXDealet("quixProxy", func(name string, stage *Stage, router *QUIXRouter) QUIXDealet {
		d := new(quixProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// quixProxy dealet passes QUIX connections to QUIX backends.
type quixProxy struct {
	// Parent
	QUIXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *QUIXRouter  // the router to which the dealet belongs
	backend *QUIXBackend // the backend to pass to
	// States
	QUIXProxyConfig // embeded
}

func (d *quixProxy) onCreate(name string, stage *Stage, router *QUIXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *quixProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *quixProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quixBackend, ok := backend.(*QUIXBackend); ok {
				d.backend = quixBackend
			} else {
				UseExitf("incorrect backend '%s' for quixProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quixProxy")
	}
}
func (d *quixProxy) OnPrepare() {
}

func (d *quixProxy) Deal(conn *QUIXConn, stream *QUIXStream) (dealt bool) {
	QUIXReverseProxy(conn, stream, d.backend, &d.QUIXProxyConfig)
	return true
}

// QUIXProxyConfig
type QUIXProxyConfig struct {
	// TODO
}

// QUIXReverseProxy
func QUIXReverseProxy(conn *QUIXConn, stream *QUIXStream, backend *QUIXBackend, proxyConfig *QUIXProxyConfig) {
	// TODO
}

//////////////////////////////////////// QUIX backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("quixBackend", func(name string, stage *Stage) Backend {
		b := new(QUIXBackend)
		b.onCreate(name, stage)
		return b
	})
}

// QUIXBackend component.
type QUIXBackend struct {
	// Parent
	Backend_[*quixNode]
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (b *QUIXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *QUIXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// maxStreamsPerConn
	b.ConfigureInt32("maxStreamsPerConn", &b.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, 1000)

	// sub components
	b.ConfigureNodes()
}
func (b *QUIXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *QUIXBackend) MaxStreamsPerConn() int32 { return b.maxStreamsPerConn }

func (b *QUIXBackend) CreateNode(name string) Node {
	node := new(quixNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *QUIXBackend) Dial() (*QConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

func (b *QUIXBackend) FetchStream() (*QStream, error) {
	// TODO
	return nil, nil
}
func (b *QUIXBackend) StoreStream(qStream *QStream) {
	// TODO
}

// quixNode is a node in QUIXBackend.
type quixNode struct {
	// Parent
	Node_
	// Assocs
	backend *QUIXBackend
	// States
}

func (n *quixNode) onCreate(name string, backend *QUIXBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *quixNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *quixNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *quixNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("quixNode=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *quixNode) dial() (*QConn, error) {
	// TODO. note: use n.IncSub()?
	return nil, nil
}

func (n *quixNode) fetchStream() (*QStream, error) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *quixNode) storeStream(qStream *QStream) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
}

// QConn is a backend-side quix connection to quixNode.
type QConn struct {
	// Conn states (non-zeros)
	id         int64 // the conn id
	node       *quixNode
	quicConn   *quic.Conn
	maxStreams int32 // how many streams are allowed on this connection?
	// Conn states (zeros)
	counter     atomic.Int64 // can be used to generate a random number
	lastWrite   time.Time    // deadline of last write operation
	lastRead    time.Time    // deadline of last read operation
	usedStreams atomic.Int32 // how many streams have been used?
	broken      atomic.Bool  // is connection broken?
}

var poolQConn sync.Pool

func getQConn(id int64, node *quixNode, quicConn *quic.Conn) *QConn {
	var qConn *QConn
	if x := poolQConn.Get(); x == nil {
		qConn = new(QConn)
	} else {
		qConn = x.(*QConn)
	}
	qConn.onGet(id, node, quicConn)
	return qConn
}
func putQConn(qConn *QConn) {
	qConn.onPut()
	poolQConn.Put(qConn)
}

func (c *QConn) onGet(id int64, node *quixNode, quicConn *quic.Conn) {
	c.id = id
	c.node = node
	c.quicConn = quicConn
	c.maxStreams = node.backend.MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.quicConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
	c.node = nil
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *QConn) IsUDS() bool { return c.node.IsUDS() }
func (c *QConn) IsTLS() bool { return c.node.IsTLS() }

func (c *QConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.node.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *QConn) runOut() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *QConn) markBroken()    { c.broken.Store(true) }
func (c *QConn) isBroken() bool { return c.broken.Load() }

func (c *QConn) FetchStream() (*QStream, error) {
	// TODO
	return nil, nil
}
func (c *QConn) StoreStream(stream *QStream) {
	// TODO
}

func (c *QConn) Close() error {
	quicConn := c.quicConn
	putQConn(c)
	return quicConn.Close()
}

// QStream is a bidirectional stream of QConn.
type QStream struct {
	// TODO
	conn       *QConn
	quicStream *quic.Stream
}

var poolQStream sync.Pool

func getQStream(conn *QConn, quicStream *quic.Stream) *QStream {
	var stream *QStream
	if x := poolQStream.Get(); x == nil {
		stream = new(QStream)
	} else {
		stream = x.(*QStream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putQStream(stream *QStream) {
	stream.onEnd()
	poolQStream.Put(stream)
}

func (s *QStream) onUse(conn *QConn, quicStream *quic.Stream) {
	s.conn = conn
	s.quicStream = quicStream
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quicStream = nil
}

func (s *QStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

func (s *QStream) Close() error {
	// TODO
	return nil
}
