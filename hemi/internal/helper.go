// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Common helpers.

package internal

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const ( // array kinds
	arrayKindStock = iota // refers to stock buffer. must be 0
	arrayKindPool         // got from sync.Pool
	arrayKindMake         // made from make([]byte)
)

// fakeFile
var fakeFile _fakeFile

func makeTempName(p []byte, stageID int64, connID int64, unixTime int64, counter int64) (from int, edge int) {
	// TODO: improvement
	stageID &= 0x7f
	connID &= 0xffff
	unixTime &= 0xffffffff
	counter &= 0xff
	// stageID(8) | connID(16) | seconds(32) | counter(8)
	i64 := stageID<<56 | connID<<40 | unixTime<<8 | counter
	return i64ToDec(i64, p)
}

// hostnameTo
type hostnameTo[T Component] struct {
	hostname []byte // "example.com" for exact map, ".example.com" for suffix map, "www.example." for prefix map
	target   T
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

// contentSaver_ is a mixin.
type contentSaver_ struct {
	// States
	saveContentFilesDir string
}

func (s *contentSaver_) onConfigure(shell Component, defaultDir string) {
	// saveContentFilesDir
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) error {
		if value != "" && len(value) <= 232 {
			return nil
		}
		return errors.New(".saveContentFilesDir has an invalid value")
	}, defaultDir)
}
func (s *contentSaver_) onPrepare(shell Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// streamHolder
type streamHolder interface {
	MaxStreamsPerConn() int32
}

// streamHolder_ is a mixin.
type streamHolder_ struct {
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (s *streamHolder_) onConfigure(shell Component, defaultMaxStreams int32) {
	// maxStreamsPerConn
	shell.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, defaultMaxStreams)
}
func (s *streamHolder_) onPrepare(shell Component) {
}

func (s *streamHolder_) MaxStreamsPerConn() int32 { return s.maxStreamsPerConn }

// loadBalancer_ is a mixin.
type loadBalancer_ struct {
	// States
	balancer  string       // roundRobin, ipHash, random, ...
	indexGet  func() int64 // ...
	nodeIndex atomic.Int64 // for roundRobin. won't overflow because it is so large!
	numNodes  int64        // num of nodes
}

func (b *loadBalancer_) init() {
	b.nodeIndex.Store(-1)
}

func (b *loadBalancer_) onConfigure(shell Component) {
	// balancer
	shell.ConfigureString("balancer", &b.balancer, func(value string) error {
		if value == "roundRobin" || value == "ipHash" || value == "random" {
			return nil
		}
		return errors.New(".balancer has an invalid value")
	}, "roundRobin")
}
func (b *loadBalancer_) onPrepare(numNodes int) {
	switch b.balancer {
	case "roundRobin":
		b.indexGet = b.getNextByRoundRobin
	case "ipHash":
		b.indexGet = b.getNextByIPHash
	case "random":
		b.indexGet = b.getNextByRandom
	default:
		BugExitln("unknown balancer")
	}
	b.numNodes = int64(numNodes)
}

func (b *loadBalancer_) getNext() int64 { return b.indexGet() }

func (b *loadBalancer_) getNextByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *loadBalancer_) getNextByIPHash() int64 {
	// TODO
	return 0
}
func (b *loadBalancer_) getNextByRandom() int64 {
	return rand.Int63n(b.numNodes)
}

// subsWaiter_ is a mixin.
type subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *subsWaiter_) IncSub(n int) { w.subs.Add(n) }
func (w *subsWaiter_) WaitSubs()    { w.subs.Wait() }
func (w *subsWaiter_) SubDone()     { w.subs.Done() }

// shutdownable_ is a mixin.
type shutdownable_ struct {
	ShutChan chan struct{} // used to notify target to shutdown
}

func (s *shutdownable_) init() {
	s.ShutChan = make(chan struct{})
}

func (s *shutdownable_) Loop(interval time.Duration, callback func(now time.Time)) {
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

// identifiable
type identifiable interface {
	ID() uint8
	setID(id uint8)
}

// identifiable_ is a mixin.
type identifiable_ struct {
	id uint8
}

func (i *identifiable_) ID() uint8 { return i.id }

func (i *identifiable_) setID(id uint8) { i.id = id }

// zone
type zone struct { // 2 bytes
	from, edge uint8 // edge is ensured to be <= 255
}

func (z *zone) zero() { *z = zone{} }

func (z *zone) size() int      { return int(z.edge - z.from) }
func (z *zone) isEmpty() bool  { return z.from == z.edge }
func (z *zone) notEmpty() bool { return z.from != z.edge }

// span
type span struct { // 8 bytes
	from, edge int32 // p[from:edge] is the bytes. edge is ensured to be <= 2147483647
}

func (s *span) zero() { *s = span{} }

func (s *span) size() int      { return int(s.edge - s.from) }
func (s *span) isEmpty() bool  { return s.from == s.edge }
func (s *span) notEmpty() bool { return s.from != s.edge }

func (s *span) set(from int32, edge int32) {
	s.from, s.edge = from, edge
}
func (s *span) sub(delta int32) {
	if s.from >= delta {
		s.from -= delta
		s.edge -= delta
	}
}

const ( // units
	K = 1 << 10
	M = 1 << 20
	G = 1 << 30
	T = 1 << 40
)
const ( // sizes
	_1K   = 1 * K    // mostly used by stock buffers
	_2K   = 2 * K    // mostly used by stock buffers
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
	_1T   = 1 * T
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
	if x := pool.Get(); x == nil {
		return make([]byte, size)
	} else {
		return x.([]byte)
	}
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

func decToI64(dec []byte) (int64, bool) {
	if n := len(dec); n == 0 || n > 19 { // the max number of int64 is 19 bytes
		return 0, false
	}
	var i64 int64
	for _, b := range dec {
		if b < '0' || b > '9' {
			return 0, false
		}
		i64 = i64*10 + int64(b-'0')
		if i64 < 0 {
			return 0, false
		}
	}
	return i64, true
}
func i64ToDec(i64 int64, dec []byte) (from int, edge int) {
	n := len(dec)
	if n < 19 { // 19 bytes are enough to hold a positive int64
		BugExitln("dec is too small")
	}
	j := n - 1
	for i64 >= 10 {
		dec[j] = byte(i64%10 + '0')
		j--
		i64 /= 10
	}
	dec[j] = byte(i64 + '0')
	return j, n
}
func hexToI64(hex []byte) (int64, bool) {
	if n := len(hex); n == 0 || n > 16 {
		return 0, false
	}
	var i64 int64
	for _, b := range hex {
		if b >= '0' && b <= '9' {
			b = b - '0'
		} else if b >= 'a' && b <= 'f' {
			b = b - 'a' + 10
		} else if b >= 'A' && b <= 'F' {
			b = b - 'A' + 10
		} else {
			return 0, false
		}
		i64 <<= 4
		i64 += int64(b)
		if i64 < 0 {
			return 0, false
		}
	}
	return i64, true
}
func i64ToHex(i64 int64, hex []byte) int {
	const digits = "0123456789abcdef"
	if len(hex) < 16 { // 16 bytes are enough to hold an int64 hex
		BugExitln("hex is too small")
	}
	if i64 == 0 {
		hex[0] = '0'
		return 1
	}
	var tmp [16]byte
	j := len(tmp) - 1
	for i64 >= 16 {
		s := i64 / 16
		tmp[j] = digits[i64-s*16]
		j--
		i64 = s
	}
	tmp[j] = digits[i64]
	n := 0
	for j < len(tmp) {
		hex[n] = tmp[j]
		j++
		n++
	}
	return n
}

func byteIsBlank(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }
func byteIsAlpha(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }
func byteIsDigit(b byte) bool { return b >= '0' && b <= '9' }
func byteIsAlnum(b byte) bool { return byteIsAlpha(b) || byteIsDigit(b) }
func byteIsIdent(b byte) bool { return byteIsAlnum(b) || b == '_' }

func byteFromHex(b byte) (n byte, ok bool) {
	if b >= '0' && b <= '9' {
		return b - '0', true
	}
	if b >= 'A' && b <= 'F' {
		return b - 'A' + 10, true
	}
	if b >= 'a' && b <= 'f' {
		return b - 'a' + 10, true
	}
	return 0, false
}

func bytesToLower(p []byte) {
	for i := 0; i < len(p); i++ {
		if b := p[i]; b >= 'A' && b <= 'Z' {
			p[i] = b + 0x20 // to lower
		}
	}
}
func bytesToUpper(p []byte) {
	for i := 0; i < len(p); i++ {
		if b := p[i]; b >= 'a' && b <= 'z' {
			p[i] = b - 0x20 // to upper
		}
	}
}
func bytesHash(p []byte) uint16 {
	hash := uint16(0)
	for _, b := range p {
		hash += uint16(b)
	}
	return hash
}

func stringHash(s string) uint16 {
	hash := uint16(0)
	for i := 0; i < len(s); i++ {
		hash += uint16(s[i])
	}
	return hash
}

func bytesesSort(byteses [][]byte) {
	for i := 1; i < len(byteses); i++ {
		elem := byteses[i]
		j := i
		for j > 0 && bytes.Compare(byteses[j-1], elem) > 0 {
			byteses[j] = byteses[j-1]
			j--
		}
		byteses[j] = elem
	}
}
func bytesesFind(byteses [][]byte, elem []byte) bool {
	from, last := 0, len(byteses)-1
	for from <= last {
		mid := from + (last-from)/2
		if result := bytes.Compare(byteses[mid], elem); result == 0 {
			return true
		} else if result < 0 {
			from = mid + 1
		} else {
			last = mid - 1
		}
	}
	return false
}

func loadURL(scheme string, host string, path string) (content string, err error) {
	addr := host
	if strings.IndexByte(host, ':') == -1 {
		if scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}

	var conn net.Conn
	netDialer := net.Dialer{
		Timeout: 2 * time.Second,
	}
	if scheme == "https" {
		tlsDialer := tls.Dialer{
			NetDialer: &netDialer,
			Config:    nil,
		}
		conn, err = tlsDialer.Dial("tcp", addr)
	} else {
		conn, err = netDialer.Dial("tcp", addr)
	}
	if err != nil {
		return
	}
	defer conn.Close()

	if err = conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return
	}

	request := []byte(fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, host))
	if _, err = conn.Write(request); err != nil {
		return
	}

	response, err := io.ReadAll(conn)
	if err != nil {
		return
	}
	if p := bytes.Index(response, []byte("\r\n\r\n")); p == -1 {
		return "", errors.New("bad http response")
	} else if len(response) < 12 || response[9] != '2' { // HTTP/1.1 200
		return "", errors.New("invalid http response")
	} else {
		return string(response[p+4:]), nil
	}
}
