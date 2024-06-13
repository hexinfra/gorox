// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Misc components and elements for net, rpc, and web.

package hemi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"
)

const ( // array kinds
	arrayKindStock = iota // refers to stock buffer. must be 0
	arrayKindPool         // got from sync.Pool
	arrayKindMake         // made from make([]byte)
)

var ( // defined errors
	errNodeDown = errors.New("node is down")
	errNodeBusy = errors.New("node is busy")
)

func makeTempName(p []byte, stageID int64, connID int64, unixTime int64, counter int64) int {
	// TODO: improvement
	// stageID(8) | connID(16) | seconds(24) | counter(16)
	stageID &= 0x7f
	connID &= 0xffff
	unixTime &= 0xffffff
	counter &= 0xffff
	return i64ToDec(stageID<<56|connID<<40|unixTime<<16|counter, p)
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
	target   T
}

// varHolder holdes values of variables.
type varHolder interface {
	unsafeVariable(code int16, name string) (value []byte)
}

var varCodes = map[string]int16{ // TODO
	// general conn vars for quix, tcps, and udps
	"srcHost": 0,
	"srcPort": 1,
	"isTLS":   2,
	"isUDS":   3,

	// quix conn vars

	// tcps conn vars
	"serverName": 4,
	"nextProto":  5,

	// udps conn vars

	// web request vars
	"method":      0, // GET, POST, ...
	"scheme":      1, // http, https
	"authority":   2, // example.com, example.org:8080
	"hostname":    3, // example.com, example.org
	"colonPort":   4, // :80, :8080
	"path":        5, // /abc, /def/
	"uri":         6, // /abc?x=y, /%cc%dd?y=z&z=%ff
	"encodedPath": 7, // /abc, /%cc%dd
	"queryString": 8, // ?x=y, ?y=z&z=%ff
	"contentType": 9, // application/json
}

// tempFile is used to temporarily save request/response content in local file system.
type tempFile interface {
	Name() string // used by os.Remove()
	Write(p []byte) (n int, err error)
	Seek(offset int64, whence int) (ret int64, err error)
	Close() error
}

// _fakeFile implements tempFile.
type _fakeFile struct{}

func (f _fakeFile) Name() string                           { return "" }
func (f _fakeFile) Write(p []byte) (n int, err error)      { return }
func (f _fakeFile) Seek(int64, int) (ret int64, err error) { return }
func (f _fakeFile) Close() error                           { return nil }

// fakeFile
var fakeFile _fakeFile

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

// contentSaver
type contentSaver interface {
	SaveContentFilesDir() string
}

// _contentSaver_ is a mixin.
type _contentSaver_ struct {
	// States
	saveContentFilesDir string
}

func (s *_contentSaver_) onConfigure(component Component, defaultDir string) {
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

func (s *_contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// _subsWaiter_ is a mixin.
type _subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *_subsWaiter_) IncSub()        { w.subs.Add(1) }
func (w *_subsWaiter_) SubsAddn(n int) { w.subs.Add(n) }
func (w *_subsWaiter_) WaitSubs()      { w.subs.Wait() }
func (w *_subsWaiter_) DecSub()        { w.subs.Done() }

// _shutdownable_ is a mixin.
type _shutdownable_ struct {
	ShutChan chan struct{} // used to notify target to shutdown
}

func (s *_shutdownable_) init() {
	s.ShutChan = make(chan struct{})
}

func (s *_shutdownable_) Loop(interval time.Duration, callback func(now time.Time)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ShutChan:
			return
		case now := <-ticker.C:
			callback(now)
		}
	}
}

/*
// _connPool_
type _connPool_[T any] struct {
	freeList struct{
		sync.Mutex
		head T
		tail T
		qnty int
	}
}

func (p *_connPool_[T]) pullConn() T {
	list := &p.freeList

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.next
	conn.setNext(nil)
	list.qnty--

	return conn
}
*/

// logcfg
type logcfg struct {
	logFile string
	rotate  string
	format  string
	bufSize int
}

// logger is logger for routers, services, and webapps.
type logger struct {
	file   *os.File
	queue  chan string
	buffer []byte
	size   int
	used   int
}

func newLogger(logFile string) (*logger, error) {
	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	l := new(logger)
	l.file = file
	l.queue = make(chan string)
	l.buffer = make([]byte, 1048576)
	l.size = len(l.buffer)
	l.used = 0
	go l.saver()
	return l, nil
}

func (l *logger) Log(v ...any) {
	if s := fmt.Sprint(v...); s != "" {
		l.queue <- s
	}
}
func (l *logger) Logln(v ...any) {
	if s := fmt.Sprintln(v...); s != "" {
		l.queue <- s
	}
}
func (l *logger) Logf(f string, v ...any) {
	if s := fmt.Sprintf(f, v...); s != "" {
		l.queue <- s
	}
}

func (l *logger) Close() { l.queue <- "" }

func (l *logger) saver() { // runner
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
func (l *logger) write(s string) {
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
func (l *logger) clear() {
	if l.used > 0 {
		l.flush(l.buffer[:l.used])
		l.used = 0
	}
}
func (l *logger) flush(p []byte) { l.file.Write(p) }
