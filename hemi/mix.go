// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General elements for net, rpc, and web.

package hemi

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// holder collects shared methods between Gate and Node.
type holder interface {
	Stage() *Stage
	Address() string
	UDSMode() bool
	TLSMode() bool
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// _holder_ is a mixin for Server_, Gate_, and Node_.
type _holder_ struct {
	// Assocs
	stage *Stage // current stage
	// States
	address      string        // :port, hostname:port, /path/to/unix.sock
	udsMode      bool          // is address a unix domain socket?
	tlsMode      bool          // use tls to secure the transport?
	tlsConfig    *tls.Config   // set if tls mode is true
	readTimeout  time.Duration // read() timeout
	writeTimeout time.Duration // write() timeout
}

func (h *_holder_) onConfigure(component Component, defaultRead time.Duration, defaultWrite time.Duration) {
	// tlsMode
	component.ConfigureBool("tlsMode", &h.tlsMode, false)
	if h.tlsMode {
		h.tlsConfig = new(tls.Config)
	}

	// readTimeout
	component.ConfigureDuration("readTimeout", &h.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, defaultRead)

	// writeTimeout
	component.ConfigureDuration("writeTimeout", &h.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, defaultWrite)
}
func (h *_holder_) onPrepare(component Component) {
}

func (h *_holder_) Stage() *Stage { return h.stage }

func (h *_holder_) Address() string             { return h.address }
func (h *_holder_) UDSMode() bool               { return h.udsMode }
func (h *_holder_) TLSMode() bool               { return h.tlsMode }
func (h *_holder_) TLSConfig() *tls.Config      { return h.tlsConfig }
func (h *_holder_) ReadTimeout() time.Duration  { return h.readTimeout }
func (h *_holder_) WriteTimeout() time.Duration { return h.writeTimeout }

// Server component. A Server has a group of Gates.
type Server interface {
	// Imports
	Component
	// Methods
	Serve()           // runner
	holder() _holder_ // used by gates
}

// Server_ is the parent for all servers.
type Server_[G Gate] struct {
	// Parent
	Component_
	// Mixins
	_holder_ // to carry configs used by gates
	// Assocs
	gates []G // a server has many gates
	// States
	colonport         string // like: ":9876"
	colonportBytes    []byte // []byte(colonport)
	udsColonport      string // uds doesn't have a port. we can use this as its colonport if server is listening at uds
	udsColonportBytes []byte // []byte(udsColonport)
	numGates          int32  // number of gates
}

func (s *Server_[G]) OnCreate(compName string, stage *Stage) {
	s.MakeComp(compName)
	s.stage = stage
}
func (s *Server_[G]) OnShutdown() {
	// We don't use close(s.ShutChan) to notify gates.
	for _, gate := range s.gates {
		gate.Shut() // this causes gate to close and return immediately
	}
}

func (s *Server_[G]) OnConfigure() {
	s._holder_.onConfigure(s, 60*time.Second, 60*time.Second)

	// address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if p := strings.IndexByte(address, ':'); p == -1 {
				s.udsMode = true
			} else {
				s.colonport = address[p:]
				s.colonportBytes = []byte(s.colonport)
			}
			s.address = address
		} else {
			UseExitln(".address should be of string type")
		}
	} else {
		UseExitln(".address is required for servers")
	}

	// udsColonport
	s.ConfigureString("udsColonport", &s.udsColonport, nil, ":80")
	s.udsColonportBytes = []byte(s.udsColonport)

	// numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".numGates has an invalid value")
	}, s.stage.NumCPU())
}
func (s *Server_[G]) OnPrepare() {
	s._holder_.onPrepare(s)

	if s.udsMode { // unix domain socket does not support reuseaddr/reuseport.
		s.numGates = 1
	}
}

func (s *Server_[G]) NumGates() int32 { return s.numGates }
func (s *Server_[G]) AddGate(gate G)  { s.gates = append(s.gates, gate) }

func (s *Server_[G]) Colonport() string {
	if s.udsMode {
		return s.udsColonport
	} else {
		return s.colonport
	}
}
func (s *Server_[G]) ColonportBytes() []byte {
	if s.udsMode {
		return s.udsColonportBytes
	} else {
		return s.colonportBytes
	}
}

func (s *Server_[G]) holder() _holder_ { return s._holder_ }

// Gate is the interface for all gates. Gates are not components.
type Gate interface {
	// Imports
	holder
	// Methods
	Shut() error
	IsShut() bool
}

// Gate_ is the parent for all gates.
type Gate_[S Server] struct {
	// Mixins
	_holder_
	// Assocs
	server S
	// States
	id       int32          // gate id
	shut     atomic.Bool    // is gate shut?
	subConns sync.WaitGroup // sub conns to wait for
}

func (g *Gate_[S]) OnNew(server S, id int32) {
	g._holder_ = server.holder()
	g.server = server
	g.id = id
	g.shut.Store(false)
}

func (g *Gate_[S]) Server() S { return g.server }

func (g *Gate_[S]) ID() int32    { return g.id }
func (g *Gate_[S]) MarkShut()    { g.shut.Store(true) }
func (g *Gate_[S]) IsShut() bool { return g.shut.Load() }

func (g *Gate_[S]) IncSub()   { g.subConns.Add(1) }
func (g *Gate_[S]) WaitSubs() { g.subConns.Wait() }
func (g *Gate_[S]) DecSub()   { g.subConns.Done() }

// Backend component. A Backend is a group of nodes.
type Backend interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	CreateNode(compName string) Node
}

// Backend_ is the parent for backends.
type Backend_[N Node] struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	nodes []N    // nodes of this backend
	// States
	balancer     string       // roundRobin, ipHash, random, ...
	numNodes     int64        // num of nodes
	nodeIndexGet func() int64 // ...
	nodeIndex    atomic.Int64 // for roundRobin. won't overflow because it is so large!
	healthCheck  any          // TODO
}

func (b *Backend_[N]) OnCreate(compName string, stage *Stage) {
	b.MakeComp(compName)
	b.stage = stage
	b.nodeIndex.Store(-1)
	b.healthCheck = nil // TODO
}
func (b *Backend_[N]) OnShutdown() {
	close(b.ShutChan) // notifies Maintain() which will shutdown sub components
}

func (b *Backend_[N]) OnConfigure() {
	// balancer
	b.ConfigureString("balancer", &b.balancer, func(value string) error {
		if value == "roundRobin" || value == "ipHash" || value == "random" || value == "leastUsed" {
			return nil
		}
		return errors.New(".balancer has an invalid value")
	}, "roundRobin")
}
func (b *Backend_[N]) OnPrepare() {
	switch b.balancer {
	case "roundRobin":
		b.nodeIndexGet = b._nextIndexByRoundRobin
	case "random":
		b.nodeIndexGet = b._nextIndexByRandom
	case "ipHash":
		b.nodeIndexGet = b._nextIndexByIPHash
	case "leastUsed":
		b.nodeIndexGet = b._nextIndexByLeastUsed
	default:
		BugExitln("unknown balancer")
	}
	b.numNodes = int64(len(b.nodes))
}

func (b *Backend_[N]) ConfigureNodes() {
	for _, node := range b.nodes {
		node.OnConfigure()
	}
}
func (b *Backend_[N]) PrepareNodes() {
	for _, node := range b.nodes {
		node.OnPrepare()
	}
}

func (b *Backend_[N]) Stage() *Stage { return b.stage }

func (b *Backend_[N]) Maintain() { // runner
	for _, node := range b.nodes {
		b.IncSub() // node
		go node.Maintain()
	}

	<-b.ShutChan // waiting for shutdown signal

	// Current backend was told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		go node.OnShutdown()
	}
	b.WaitSubs() // nodes

	if DebugLevel() >= 2 {
		Printf("backend=%s done\n", b.CompName())
	}

	b.stage.DecSub() // backend
}

func (b *Backend_[N]) AddNode(node N) {
	node.setShell(node)
	b.nodes = append(b.nodes, node)
}

func (b *Backend_[N]) _nextIndexByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *Backend_[N]) _nextIndexByRandom() int64 {
	return rand.Int63n(b.numNodes)
}
func (b *Backend_[N]) _nextIndexByIPHash() int64 {
	// TODO
	return 0
}
func (b *Backend_[N]) _nextIndexByLeastUsed() int64 {
	// TODO
	return 0
}

// Node is a member of backend.
type Node interface {
	// Imports
	Component
	holder
	// Methods
	Maintain() // runner
}

// Node_ is the parent for all backend nodes.
type Node_[B Backend] struct {
	// Parent
	Component_
	// Mixins
	_holder_
	// Assocs
	backend B
	// States
	dialTimeout time.Duration // dial remote timeout
	weight      int32         // 1, 22, 333, ...
	connID      atomic.Int64  // next conn id
	down        atomic.Bool   // TODO: false-sharing
	health      any           // TODO
}

func (n *Node_[B]) OnCreate(compName string, stage *Stage, backend B) {
	n.MakeComp(compName)
	n.stage = stage
	n.backend = backend
	n.health = nil // TODO
}
func (n *Node_[B]) OnShutdown() {
	close(n.ShutChan) // notifies Maintain() which will close conns
}

func (n *Node_[B]) OnConfigure() {
	n._holder_.onConfigure(n, 30*time.Second, 30*time.Second)

	// address
	if v, ok := n.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if address[0] == '@' { // abstract uds
				n.udsMode = true
			} else if _, err := os.Stat(address); err == nil { // pathname uds
				n.udsMode = true
			}
			n.address = address
		} else {
			UseExitln("bad address in node")
		}
	} else {
		UseExitln("address is required in node")
	}

	// dialTimeout
	n.ConfigureDuration("dialTimeout", &n.dialTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

	// weight
	n.ConfigureInt32("weight", &n.weight, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad node weight")
	}, 1)
}
func (n *Node_[B]) OnPrepare() {
	n._holder_.onPrepare(n)
}

func (n *Node_[B]) Backend() B { return n.backend }

func (n *Node_[B]) DialTimeout() time.Duration { return n.dialTimeout }

func (n *Node_[B]) nextConnID() int64 { return n.connID.Add(1) }

func (n *Node_[B]) markDown()    { n.down.Store(true) }
func (n *Node_[B]) markUp()      { n.down.Store(false) }
func (n *Node_[B]) isDown() bool { return n.down.Load() }

// contentSaver
type contentSaver interface {
	RecvTimeout() time.Duration  // timeout to recv the whole message content. zero means no timeout
	SendTimeout() time.Duration  // timeout to send the whole message. zero means no timeout
	MaxContentSize() int64       // max content size allowed
	SaveContentFilesDir() string // the dir to save content temporarily
}

// _contentSaver_ is a mixin.
type _contentSaver_ struct {
	// States
	recvTimeout         time.Duration // timeout to recv the whole message content. zero means no timeout
	sendTimeout         time.Duration // timeout to send the whole message. zero means no timeout
	maxContentSize      int64         // max content size allowed to receive
	saveContentFilesDir string        // temp content files are placed here
}

func (s *_contentSaver_) onConfigure(component Component, defaultRecv time.Duration, defaultSend time.Duration, defaultDir string) {
	// recvTimeout
	component.ConfigureDuration("recvTimeout", &s.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, defaultRecv)

	// sendTimeout
	component.ConfigureDuration("sendTimeout", &s.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, defaultSend)

	// maxContentSize
	component.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)

	// saveContentFilesDir
	component.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) error {
		if value != "" && len(value) <= 232 {
			return nil
		}
		return errors.New(".saveContentFilesDir has an invalid value")
	}, defaultDir)
}
func (s *_contentSaver_) onPrepare(component Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *_contentSaver_) RecvTimeout() time.Duration  { return s.recvTimeout }
func (s *_contentSaver_) SendTimeout() time.Duration  { return s.sendTimeout }
func (s *_contentSaver_) MaxContentSize() int64       { return s.maxContentSize }
func (s *_contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// LogConfig
type LogConfig struct {
	target  string
	rotate  string
	format  string
	bufSize int
}

// Logger is logger for routers, services, and webapps.
type Logger struct {
	config *LogConfig
	file   *os.File
	queue  chan string
	buffer []byte
	size   int
	used   int
}

func NewLogger(config *LogConfig) (*Logger, error) {
	file, err := os.OpenFile(config.target, os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	l := new(Logger)
	l.config = config
	l.file = file
	l.queue = make(chan string)
	l.buffer = make([]byte, 1048576)
	l.size = len(l.buffer)
	l.used = 0
	go l.saver()
	return l, nil
}

func (l *Logger) Log(v ...any) {
	if s := fmt.Sprint(v...); s != "" {
		l.queue <- s
	}
}
func (l *Logger) Logln(v ...any) {
	if s := fmt.Sprintln(v...); s != "" {
		l.queue <- s
	}
}
func (l *Logger) Logf(f string, v ...any) {
	if s := fmt.Sprintf(f, v...); s != "" {
		l.queue <- s
	}
}

func (l *Logger) Close() { l.queue <- "" }

func (l *Logger) saver() { // runner
	for {
		s := <-l.queue
		if s == "" {
			goto over
		}
		l.write(s)
	more:
		for {
			select {
			case s = <-l.queue:
				if s == "" {
					goto over
				}
				l.write(s)
			default:
				l.clear()
				break more
			}
		}
	}
over:
	l.clear()
	l.file.Close()
}
func (l *Logger) write(s string) {
	n := len(s)
	if n >= l.size {
		l.clear()
		l.flush(ConstBytes(s))
		return
	}
	w := copy(l.buffer[l.used:], s)
	l.used += w
	if l.used == l.size {
		l.clear()
		if n -= w; n > 0 {
			copy(l.buffer, s[w:])
			l.used = n
		}
	}
}
func (l *Logger) clear() {
	if l.used > 0 {
		l.flush(l.buffer[:l.used])
		l.used = 0
	}
}
func (l *Logger) flush(logs []byte) { l.file.Write(logs) }

// Region
type Region struct { // 512B
	blocks [][]byte  // the blocks. [<stocks>/make]
	stocks [4][]byte // for blocks. 96B
	block0 [392]byte // for blocks[0]
}

func (r *Region) Init() {
	r.blocks = r.stocks[0:1:cap(r.stocks)]                    // block0 always at 0
	r.stocks[0] = r.block0[:]                                 // first block is always block0
	binary.BigEndian.PutUint16(r.block0[cap(r.block0)-2:], 0) // reset used size of block0
}
func (r *Region) Make(size int) []byte { // good for a lot of small buffers
	if size <= 0 {
		BugExitln("bad size")
	}
	block := r.blocks[len(r.blocks)-1]
	edge := cap(block)
	ceil := edge - 2
	used := int(binary.BigEndian.Uint16(block[ceil:edge]))
	want := used + size
	if want <= 0 {
		BugExitln("size too large")
	}
	if want <= ceil {
		binary.BigEndian.PutUint16(block[ceil:edge], uint16(want))
		return block[used:want]
	}
	ceil = _4K - 2
	if size > ceil {
		return make([]byte, size)
	}
	block = Get4K()
	binary.BigEndian.PutUint16(block[ceil:_4K], uint16(size))
	r.blocks = append(r.blocks, block)
	return block[0:size]
}
func (r *Region) Free() {
	for i := 1; i < len(r.blocks); i++ {
		PutNK(r.blocks[i])
		r.blocks[i] = nil
	}
	if cap(r.blocks) != cap(r.stocks) {
		r.stocks = [4][]byte{}
		r.blocks = nil
	}
}

// tempFile is used to temporarily save request/response content in local file system.
type tempFile interface {
	Name() string // used by os.Remove()
	Write(src []byte) (n int, err error)
	Seek(offset int64, whence int) (ret int64, err error)
	Close() error
}

// _fakeFile implements tempFile.
type _fakeFile struct{}

func (f _fakeFile) Name() string                           { return "" }
func (f _fakeFile) Write(src []byte) (n int, err error)    { return }
func (f _fakeFile) Seek(int64, int) (ret int64, err error) { return }
func (f _fakeFile) Close() error                           { return nil }

// fakeFile
var fakeFile _fakeFile

const ( // units
	K = 1 << 10
	M = 1 << 20
	G = 1 << 30
	T = 1 << 40
)

const ( // sizes
	_1K   = 1 * K    // mostly used by stock buffers
	_4K   = 4 * K    // mostly used by pooled buffers
	_16K  = 16 * K   // mostly used by pooled buffers
	_64K1 = 64*K - 1 // mostly used by pooled buffers

	_128K = 128 * K
	_256K = 256 * K
	_512K = 512 * K
	_1M   = 1 * M
	_2M   = 2 * M
	_4M   = 4 * M
	_8M   = 8 * M
	_16M  = 16 * M
	_32M  = 32 * M
	_64M  = 64 * M
	_128M = 128 * M
	_256M = 256 * M
	_512M = 512 * M
	_1G   = 1 * G
	_2G1  = 2*G - 1 // suitable for max int32 [-2147483648, 2147483647]

	_1T = 1 * T
)

var ( // pools
	pool4K   sync.Pool
	pool16K  sync.Pool
	pool64K1 sync.Pool
)

func Get4K() []byte   { return getNK(&pool4K, _4K) }
func Get16K() []byte  { return getNK(&pool16K, _16K) }
func Get64K1() []byte { return getNK(&pool64K1, _64K1) }
func GetNK(n int64) []byte {
	if n <= _4K {
		return getNK(&pool4K, _4K)
	} else if n <= _16K {
		return getNK(&pool16K, _16K)
	} else { // n > _16K
		return getNK(&pool64K1, _64K1)
	}
}
func getNK(pool *sync.Pool, size int) []byte {
	if x := pool.Get(); x != nil {
		return x.([]byte)
	}
	return make([]byte, size)
}
func PutNK(p []byte) {
	switch cap(p) {
	case _4K:
		pool4K.Put(p)
	case _16K:
		pool16K.Put(p)
	case _64K1:
		pool64K1.Put(p)
	default:
		BugExitln("bad buffer")
	}
}

const ( // array kinds
	arrayKindStock = iota // refers to stock buffer. must be 0
	arrayKindPool         // got from sync.Pool
	arrayKindMake         // made from make([]byte)
)

var ( // defined errors
	errNodeDown = errors.New("node is down")
	errNodeBusy = errors.New("node is busy")
)

func makeTempName(dst []byte, stageID int32, connID int64, unixTime int64, counter int64) int {
	// TODO: improvement
	// stageID(8) | connID(16) | seconds(24) | counter(16)
	stageID &= 0x7f
	connID &= 0xffff
	unixTime &= 0xffffff
	counter &= 0xffff
	return i64ToDec(int64(stageID)<<56|connID<<40|unixTime<<16|counter, dst)
}

func equalMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func prefixMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func suffixMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func containMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.Contains(value, pattern) {
			return true
		}
	}
	return false
}
func regexpMatch(value []byte, regexps []*regexp.Regexp) bool {
	for _, regexp := range regexps {
		if regexp.Match(value) {
			return true
		}
	}
	return false
}
func notEqualMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func notPrefixMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func notSuffixMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func notContainMatch(value []byte, patterns [][]byte) bool {
	for _, pattern := range patterns {
		if bytes.Contains(value, pattern) {
			return false
		}
	}
	return true
}
func notRegexpMatch(value []byte, regexps []*regexp.Regexp) bool {
	for _, regexp := range regexps {
		if regexp.Match(value) {
			return false
		}
	}
	return true
}

// hostnameTo
type hostnameTo[T Component] struct {
	hostname []byte // "example.com" for exact map, ".example.com" for suffix map, "www.example." for prefix map
	target   T      // service or webapp
}

// varKeeper holdes values of variables.
type varKeeper interface {
	unsafeVariable(code int16, name string) (value []byte)
}

var varCodes = map[string]int16{ // TODO
	// general conn vars for quix, tcpx, and udpx
	"srcHost": 0,
	"srcPort": 1,
	"udsMode": 2,
	"tlsMode": 3,

	// quix conn vars

	// tcpx conn vars
	"serverName": 4,
	"nextProto":  5,

	// udpx conn vars

	// http request vars
	"method":      0, // GET, POST, ...
	"scheme":      1, // http, https
	"authority":   2, // example.com, example.org:8080
	"hostname":    3, // example.com, example.org
	"colonport":   4, // :80, :8080
	"path":        5, // /abc, /def/
	"uri":         6, // /abc?x=y, /%cc%dd?y=z&z=%ff
	"encodedPath": 7, // /abc, /%cc%dd
	"queryString": 8, // ?x=y, ?y=z&z=%ff
	"contentType": 9, // application/json
}
