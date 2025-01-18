// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (QUIC over UDP/UDS) router and backend implementation. See RFC 8999, RFC 9000, RFC 9001, and RFC 9002.

package hemi

import (
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
)

//////////////////////////////////////// QUIX general implementation ////////////////////////////////////////

// quixHolder is the interface for _quixHolder_.
type quixHolder interface {
	MaxCumulativeStreamsPerConn() int32
	MaxConcurrentStreamsPerConn() int32
}

// _quixHolder_ is a mixin for QUIXRouter, QUIXGate, and quixNode.
type _quixHolder_ struct {
	// States
	maxCumulativeStreamsPerConn int32 // max cumulative streams of one conn. 0 means infinite
	maxConcurrentStreamsPerConn int32 // max concurrent streams of one conn
}

func (h *_quixHolder_) onConfigure(comp Component) {
	// .maxCumulativeStreamsPerConn
	comp.ConfigureInt32("maxCumulativeStreamsPerConn", &h.maxCumulativeStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxCumulativeStreamsPerConn has an invalid value")
	}, 1000)

	// .maxCumulativeStreamsPerConn
	comp.ConfigureInt32("maxConcurrentStreamsPerConn", &h.maxConcurrentStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxConcurrentStreamsPerConn has an invalid value")
	}, 1000)
}
func (h *_quixHolder_) onPrepare(comp Component) {
}

func (h *_quixHolder_) MaxCumulativeStreamsPerConn() int32 { return h.maxCumulativeStreamsPerConn }
func (h *_quixHolder_) MaxConcurrentStreamsPerConn() int32 { return h.maxConcurrentStreamsPerConn }

// quixConn collects shared methods between *QUIXConn and *QConn.
type quixConn interface {
}

// quixConn_ is the parent for QUIXConn and QConn.
type quixConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id                   int64  // the conn id
	stage                *Stage // current stage, for convenience
	quicConn             *tcp2.Conn
	udsMode              bool  // for convenience
	tlsMode              bool  // for convenience
	maxCumulativeStreams int32 // how many streams are allowed on this connection?
	maxConcurrentStreams int32 // how many concurrent streams are allowed on this connection?
	// Conn states (zeros)
	counter           atomic.Int64 // can be used to generate a random number
	lastRead          time.Time    // deadline of last read operation
	lastWrite         time.Time    // deadline of last write operation
	broken            atomic.Bool  // is connection broken?
	cumulativeStreams atomic.Int32 // how many streams have been used?
	concurrentStreams atomic.Int32 // how many concurrent streams?
}

func (c *quixConn_) onGet(id int64, stage *Stage, quicConn *tcp2.Conn, udsMode bool, tlsMode bool, maxCumulativeStreams int32, maxConcurrentStreams int32) {
	c.id = id
	c.stage = stage
	c.quicConn = quicConn
	c.udsMode = udsMode
	c.tlsMode = tlsMode
	c.maxCumulativeStreams = maxCumulativeStreams
	c.maxConcurrentStreams = maxConcurrentStreams
}
func (c *quixConn_) onPut() {
	c.stage = nil
	c.quicConn = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.broken.Store(false)
	c.cumulativeStreams.Store(0)
	c.concurrentStreams.Store(0)
}

func (c *quixConn_) UDSMode() bool { return c.udsMode }
func (c *quixConn_) TLSMode() bool { return c.tlsMode }

func (c *quixConn_) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.stage.ID(), c.id, unixTime, c.counter.Add(1))
}

func (c *quixConn_) markBroken()    { c.broken.Store(true) }
func (c *quixConn_) isBroken() bool { return c.broken.Load() }

// quixStream collects shared methods between *QUIXStream and *QStream.
type quixStream interface {
}

// quixStream_ is the parent for QUIXStream and QStream.
type quixStream_ struct {
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Stream states (controlled)
	// Stream states (non-zeros)
	quicStream *tcp2.Stream
	// Stream states (zeros)
}

func (s *quixStream_) onUse(quicStream *tcp2.Stream) {
	s.quicStream = quicStream
}
func (s *quixStream_) onEnd() {
	s.quicStream = nil
}

//////////////////////////////////////// QUIX router implementation ////////////////////////////////////////

// QUIXRouter
type QUIXRouter struct {
	// Parent
	Server_[*quixGate]
	// Mixins
	_quixHolder_ // to carry configs used by gates
	// Assocs
	dealets compDict[QUIXDealet] // defined dealets. indexed by component name
	cases   []*quixCase          // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	maxConcurrentConnsPerGate int32      // max concurrent connections allowed per gate
	accessLog                 *LogConfig // ...
	logger                    *Logger    // router access logger
}

func (r *QUIXRouter) onCreate(compName string, stage *Stage) {
	r.Server_.OnCreate(compName, stage)
	r.dealets = make(compDict[QUIXDealet])
}

func (r *QUIXRouter) OnConfigure() {
	r.Server_.OnConfigure()
	r._quixHolder_.onConfigure(r)

	// .maxConcurrentConnsPerGate
	r.ConfigureInt32("maxConcurrentConnsPerGate", &r.maxConcurrentConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConcurrentConnsPerGate has an invalid value")
	}, 10000)

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

func (r *QUIXRouter) MaxConcurrentConnsPerGate() int32 { return r.maxConcurrentConnsPerGate }

func (r *QUIXRouter) createDealet(compSign string, compName string) QUIXDealet {
	if _, ok := r.dealets[compName]; ok {
		UseExitln("conflicting dealet with a same component name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := quixDealetCreators[compSign]
	if !ok {
		UseExitln("unknown dealet sign: " + compSign)
	}
	dealet := create(compName, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[compName] = dealet
	return dealet
}
func (r *QUIXRouter) createCase(compName string) *quixCase {
	if r.hasCase(compName) {
		UseExitln("conflicting case with a same component name")
	}
	kase := new(quixCase)
	kase.onCreate(compName, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *QUIXRouter) hasCase(compName string) bool {
	for _, kase := range r.cases {
		if kase.CompName() == compName {
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
		Printf("quixRouter=%s done\n", r.CompName())
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

func (r *QUIXRouter) quixHolder() _quixHolder_ { return r._quixHolder_ }

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
	// Mixins
	_quixHolder_
	// States
	maxConcurrentConns int32          // max concurrent conns allowed for this gate
	concurrentConns    atomic.Int32   // TODO: false sharing
	listener           *tcp2.Listener // the real gate. set after open
}

func (g *quixGate) onNew(router *QUIXRouter, id int32) {
	g.Gate_.OnNew(router, id)
	g._quixHolder_ = router.quixHolder()
	g.maxConcurrentConns = router.MaxConcurrentConnsPerGate()
	g.concurrentConns.Store(0)
}

func (g *quixGate) DecConcurrentConns() int32 { return g.concurrentConns.Add(-1) }
func (g *quixGate) IncConcurrentConns() int32 { return g.concurrentConns.Add(1) }
func (g *quixGate) ReachLimit(concurrentConns int32) bool {
	return concurrentConns > g.maxConcurrentConns
}

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

func (g *quixGate) justClose(quicConn *tcp2.Conn) {
	quicConn.Close()
	g.DecSub() // conn
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

func getQUIXConn(id int64, gate *quixGate, quicConn *tcp2.Conn) *QUIXConn {
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

func (c *QUIXConn) onGet(id int64, gate *quixGate, quicConn *tcp2.Conn) {
	c.quixConn_.onGet(id, gate.Stage(), quicConn, gate.UDSMode(), gate.TLSMode(), gate.MaxCumulativeStreamsPerConn(), gate.MaxConcurrentStreamsPerConn())

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

func (c *QUIXConn) unsafeVariable(varCode int16, varName string) (varValue []byte) {
	return quixConnVariables[varCode](c)
}

// quixConnVariables
var quixConnVariables = [...]func(*QUIXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // udsMode
	3: nil, // tlsMode
}

// QUIXStream
type QUIXStream struct {
	// Parent
	quixStream_
	// Assocs
	conn *QUIXConn
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolQUIXStream sync.Pool

func getQUIXStream(conn *QUIXConn, quicStream *tcp2.Stream) *QUIXStream {
	var stream *QUIXStream
	if x := poolQUIXStream.Get(); x == nil {
		stream = new(QUIXStream)
	} else {
		stream = x.(*QUIXStream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putQUIXStream(stream *QUIXStream) {
	stream.onEnd()
	poolQUIXStream.Put(stream)
}

func (s *QUIXStream) onUse(conn *QUIXConn, quicStream *tcp2.Stream) {
	s.quixStream_.onUse(quicStream)
	s.conn = conn
}
func (s *QUIXStream) onEnd() {
	s.conn = nil
	s.quixStream_.onEnd()
}

func (s *QUIXStream) Write(src []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUIXStream) Read(dst []byte) (n int, err error) {
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

func (c *quixCase) onCreate(compName string, router *QUIXRouter) {
	c.MakeComp(compName)
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
	DealWith(conn *QUIXConn, stream *QUIXStream) (dealt bool)
}

// QUIXDealet_
type QUIXDealet_ struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (d *QUIXDealet_) OnCreate(compName string, stage *Stage) {
	d.MakeComp(compName)
	d.stage = stage
}

func (d *QUIXDealet_) Stage() *Stage { return d.stage }

//////////////////////////////////////// QUIX backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("quixBackend", func(compName string, stage *Stage) Backend {
		b := new(QUIXBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// QUIXBackend component.
type QUIXBackend struct {
	// Parent
	Backend_[*quixNode]
	// States
}

func (b *QUIXBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
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

func (b *QUIXBackend) CreateNode(compName string) Node {
	node := new(quixNode)
	node.onCreate(compName, b.stage, b)
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

func (n *quixNode) onCreate(compName string, stage *Stage, backend *QUIXBackend) {
	n.Node_.OnCreate(compName, stage, backend)
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
		Printf("quixNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *quixNode) dial() (*QConn, error) {
	// TODO. note: use n.IncSub()?
	return nil, nil
}

func (n *quixNode) fetchStream() (*QStream, error) {
	// Note: A QConn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	return nil, nil
}
func (n *quixNode) storeStream(qStream *QStream) {
	// Note: A QConn can be used concurrently, limited by maxConcurrentStreams.
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

func getQConn(id int64, node *quixNode, quicConn *tcp2.Conn) *QConn {
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

func (c *QConn) onGet(id int64, node *quixNode, quicConn *tcp2.Conn) {
	c.quixConn_.onGet(id, node.Stage(), quicConn, node.UDSMode(), node.TLSMode(), node.MaxCumulativeStreamsPerConn(), node.MaxConcurrentStreamsPerConn())

	c.node = node
}
func (c *QConn) onPut() {
	c.node = nil

	c.quixConn_.onPut()
}

func (c *QConn) ranOut() bool { return c.cumulativeStreams.Add(1) > c.maxCumulativeStreams }
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
	// Parent
	quixStream_
	// Assocs
	conn *QConn
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolQStream sync.Pool

func getQStream(conn *QConn, quicStream *tcp2.Stream) *QStream {
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

func (s *QStream) onUse(conn *QConn, quicStream *tcp2.Stream) {
	s.quixStream_.onUse(quicStream)
	s.conn = conn
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quixStream_.onEnd()
}

func (s *QStream) Write(src []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(dst []byte) (n int, err error) {
	// TODO
	return
}

func (s *QStream) Close() error {
	// TODO
	return nil
}
