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
	// Mixins
	_quixHolder_
	// Assocs
	dealets compDict[QUIXDealet] // defined dealets. indexed by name
	cases   []*quixCase          // defined cases. the order must be kept, so we use list. TODO: use ordered map?
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
	r._quixHolder_.onConfigure(r)

	// sub components
	r.dealets.walk(QUIXDealet.OnConfigure)
	for _, kase := range r.cases {
		kase.OnConfigure()
	}
}
func (r *QUIXRouter) OnPrepare() {
	r.Server_.OnPrepare()
	r._quixHolder_.onPrepare(r)

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = NewLogger(r.accessLog)
	}

	// sub components
	r.dealets.walk(QUIXDealet.OnPrepare)
	for _, kase := range r.cases {
		kase.OnPrepare()
	}
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
		gate.onNew(r, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		go gate.serveTLS()
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	for _, kase := range r.cases {
		go kase.OnShutdown()
	}
	r.dealets.goWalk(QUIXDealet.OnShutdown)
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
	Gate_[*QUIXRouter]
	// States
	maxActives int32          // max concurrent conns allowed for this gate
	curActives atomic.Int32   // TODO: false sharing
	listener   *quic.Listener // the real gate. set after open
}

func (g *quixGate) onNew(router *QUIXRouter, id int32) {
	g.Gate_.OnNew(router, id)
	g.maxActives = router.MaxConnsPerGate()
	g.curActives.Store(0)
}

func (g *quixGate) DecActives() int32             { return g.curActives.Add(-1) }
func (g *quixGate) IncActives() int32             { return g.curActives.Add(1) }
func (g *quixGate) ReachLimit(actives int32) bool { return actives > g.maxActives }

func (g *quixGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quixGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *quixGate) serveUDS() { // runner
	// TODO
}
func (g *quixGate) serveTLS() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.server.DecSub() // gate
}

func (g *quixGate) justClose(quicConn *quic.Conn) {
	quicConn.Close()
	g.DecConn()
}

// QUIXConn is a QUIX connection coming from QUIXRouter.
type QUIXConn struct {
	// Parent
	quixConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *quixGate
	// Conn states (zeros)
}

var poolQUIXConn sync.Pool

func getQUIXConn(id int64, gate *quixGate, quicConn *quic.Conn) *QUIXConn {
	var conn *QUIXConn
	if x := poolQUIXConn.Get(); x == nil {
		conn = new(QUIXConn)
	} else {
		conn = x.(*QUIXConn)
	}
	conn.onGet(id, gate, quicConn)
	return conn
}
func putQUIXConn(conn *QUIXConn) {
	conn.onPut()
	poolQUIXConn.Put(conn)
}

func (c *QUIXConn) onGet(id int64, gate *quixGate, quicConn *quic.Conn) {
	router := gate.server
	c.quixConn_.onGet(id, router.Stage().ID(), quicConn, gate.IsUDS(), gate.IsTLS(), 123) // TODO

	c.gate = gate
}
func (c *QUIXConn) onPut() {
	c.gate = nil

	c.quixConn_.onPut()
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
	// Assocs
	conn *QUIXConn
	// Stream states (non-zeros)
	quicStream *quic.Stream
	// Stream states (zeros)
}

func (s *QUIXStream) onUse(conn *QUIXConn, quicStream *quic.Stream) {
	s.conn = conn
	s.quicStream = quicStream
}
func (s *QUIXStream) onEnd() {
	s.conn = nil
	s.quicStream = nil
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
}

func (b *QUIXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *QUIXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *QUIXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *QUIXBackend) CreateNode(name string) Node {
	node := new(quixNode)
	node.onCreate(name, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *QUIXBackend) Dial() (*QConn, error) {
	node := b.nodes[b.nodeIndexGet()]
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
	Node_[*QUIXBackend]
	// Mixins
	_quixHolder_
	// States
}

func (n *quixNode) onCreate(name string, stage *Stage, backend *QUIXBackend) {
	n.Node_.OnCreate(name, stage, backend)
}

func (n *quixNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._quixHolder_.onConfigure(n)
}
func (n *quixNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._quixHolder_.onPrepare(n)
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
	// Parent
	quixConn_
	// Conn states (non-zeros)
	node *quixNode
	// Conn states (zeros)
}

var poolQConn sync.Pool

func getQConn(id int64, node *quixNode, quicConn *quic.Conn) *QConn {
	var conn *QConn
	if x := poolQConn.Get(); x == nil {
		conn = new(QConn)
	} else {
		conn = x.(*QConn)
	}
	conn.onGet(id, node, quicConn)
	return conn
}
func putQConn(conn *QConn) {
	conn.onPut()
	poolQConn.Put(conn)
}

func (c *QConn) onGet(id int64, node *quixNode, quicConn *quic.Conn) {
	c.quixConn_.onGet(id, node.Stage().ID(), quicConn, node.IsUDS(), node.IsTLS(), node.MaxStreamsPerConn())

	c.node = node
}
func (c *QConn) onPut() {
	c.node = nil

	c.quixConn_.onPut()
}

func (c *QConn) runOut() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}
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

//////////////////////////////////////// QUIX in/out implementation ////////////////////////////////////////

// quixHolder
type quixHolder interface {
}

// _quixHolder_
type _quixHolder_ struct {
	// States
	maxStreamsPerConn int32 // max cumulative streams of one conn. 0 means infinite
}

func (h *_quixHolder_) onConfigure(component Component) {
	// maxStreamsPerConn
	component.ConfigureInt32("maxStreamsPerConn", &h.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, 1000)
}
func (h *_quixHolder_) onPrepare(component Component) {
}

func (h *_quixHolder_) MaxStreamsPerConn() int32 { return h.maxStreamsPerConn }

// quixConn
type quixConn interface {
}

// quixConn_
type quixConn_ struct {
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	id         int64
	stageID    int32
	quicConn   *quic.Conn
	udsMode    bool
	tlsMode    bool
	maxStreams int32 // how many streams are allowed on this connection?
	// Conn states (zeros)
	counter     atomic.Int64 // can be used to generate a random number
	lastRead    time.Time    // deadline of last read operation
	lastWrite   time.Time    // deadline of last write operation
	broken      atomic.Bool  // is connection broken?
	usedStreams atomic.Int32 // how many streams have been used?
}

func (c *quixConn_) onGet(id int64, stageID int32, quicConn *quic.Conn, udsMode bool, tlsMode bool, maxStreams int32) {
	c.id = id
	c.stageID = stageID
	c.quicConn = quicConn
	c.udsMode = udsMode
	c.tlsMode = tlsMode
	c.maxStreams = maxStreams
}
func (c *quixConn_) onPut() {
	c.quicConn = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.broken.Store(false)
	c.usedStreams.Store(0)
}

func (c *quixConn_) IsUDS() bool { return c.udsMode }
func (c *quixConn_) IsTLS() bool { return c.tlsMode }

func (c *quixConn_) MakeTempName(to []byte, unixTime int64) int {
	return makeTempName(to, c.stageID, c.id, unixTime, c.counter.Add(1))
}

func (c *quixConn_) markBroken()    { c.broken.Store(true) }
func (c *quixConn_) isBroken() bool { return c.broken.Load() }

// quixStream
type quixStream interface {
}

// quixStream_
type quixStream_ struct {
}
