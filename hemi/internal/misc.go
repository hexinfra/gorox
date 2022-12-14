// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Misc facilities and utilities.

package internal

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var ( // global variables shared between stages
	_debug    atomic.Int32 // debug level
	_baseOnce sync.Once    // protects _baseDir
	_baseDir  atomic.Value // directory of the executable
	_dataOnce sync.Once    // protects _dataDir
	_dataDir  atomic.Value // directory of the run-time data
	_logsOnce sync.Once    // protects _logsDir
	_logsDir  atomic.Value // directory of the log files
	_tempOnce sync.Once    // protects _tempDir
	_tempDir  atomic.Value // directory of the temp files
)

func IsDebug(level int32) bool { return _debug.Load() >= level }
func BaseDir() string          { return _baseDir.Load().(string) }
func DataDir() string          { return _dataDir.Load().(string) }
func LogsDir() string          { return _logsDir.Load().(string) }
func TempDir() string          { return _tempDir.Load().(string) }

func SetDebug(level int32)  { _debug.Store(level) }
func SetBaseDir(dir string) { _baseOnce.Do(func() { _baseDir.Store(dir) }) } // only once
func SetDataDir(dir string) { _dataOnce.Do(func() { _dataDir.Store(dir) }) } // only once
func SetLogsDir(dir string) { _logsOnce.Do(func() { _logsDir.Store(dir) }) } // only once
func SetTempDir(dir string) { _tempOnce.Do(func() { _tempDir.Store(dir) }) } // only once

func Debug(args ...any) {
	fmt.Print(args...)
}
func Debugln(args ...any) {
	fmt.Println(args...)
}
func Debugf(format string, args ...any) {
	fmt.Printf(format, args...)
}

const ( // exit codes. keep sync with ../hemi.go
	CodeBug = 20
	CodeUse = 21
	CodeEnv = 22
)

func BugExitln(args ...any) { exitln(CodeBug, "[BUG] ", args...) }
func UseExitln(args ...any) { exitln(CodeUse, "[USE] ", args...) }
func EnvExitln(args ...any) { exitln(CodeEnv, "[ENV] ", args...) }

func BugExitf(format string, args ...any) { exitf(CodeBug, "[BUG] ", format, args...) }
func UseExitf(format string, args ...any) { exitf(CodeUse, "[USE] ", format, args...) }
func EnvExitf(format string, args ...any) { exitf(CodeEnv, "[ENV] ", format, args...) }

func exitln(exitCode int, prefix string, args ...any) {
	fmt.Fprint(os.Stderr, prefix)
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCode)
}
func exitf(exitCode int, prefix, format string, args ...any) {
	fmt.Fprintf(os.Stderr, prefix+format, args...)
	os.Exit(exitCode)
}

// waiter_
type waiter_ struct {
	subs sync.WaitGroup
}

func (w *waiter_) IncSub(n int) { w.subs.Add(n) }
func (w *waiter_) WaitSubs()    { w.subs.Wait() }
func (w *waiter_) SubDone()     { w.subs.Done() }

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
	shell.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) bool { return value >= 0 }, defaultMaxStreams)
}
func (s *streamHolder_) onPrepare(shell Component) {
}

func (s *streamHolder_) MaxStreamsPerConn() int32 { return s.maxStreamsPerConn }

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
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) bool { return value != "" }, defaultDir)
}
func (s *contentSaver_) onPrepare(shell Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir }

// loadBalancer_
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
	shell.ConfigureString("balancer", &b.balancer, func(value string) bool {
		return value == "roundRobin" || value == "ipHash" || value == "random"
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
		BugExitln("this should not happen")
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
	// TODO
	return 0
}

// ider
type ider interface {
	ID() uint8
	setID(id uint8)
}

// ider_ is a mixin.
type ider_ struct {
	id uint8
}

func (i *ider_) ID() uint8      { return i.id }
func (i *ider_) setID(id uint8) { i.id = id }

func Loop(interval time.Duration, shut chan struct{}, fn func(now time.Time)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-shut:
			return
		case now := <-ticker.C:
			fn(now)
		}
	}
}

// poolBlock
var poolBlock sync.Pool

func GetBlock() *Block { // only exported to hemi, to be used by revisers.
	if x := poolBlock.Get(); x == nil {
		block := new(Block)
		block.pool = true // other blocks are not pooled.
		return block
	} else {
		return x.(*Block)
	}
}
func putBlock(block *Block) {
	poolBlock.Put(block)
}

// Block is an item of http message content linked list.
type Block struct { // 56 bytes
	next *Block      // next block
	pool bool        // true if this block is got from poolBlock. don't change this after set
	shut bool        // close file on free()?
	kind int8        // 0:blob 1:*os.File
	file *os.File    // for general use
	data risky.Refer // blob, or buffer if buff is true
	size int64       // size of blob or file
	time int64       // file mod time
}

func (b *Block) free() {
	b.closeFile()
	b.shut = false
	b.kind = 0
	b.data.Reset()
	b.size = 0
	b.time = 0
}
func (b *Block) closeFile() {
	if b.IsBlob() {
		return
	}
	if b.shut {
		b.file.Close()
	}
	if IsDebug(2) {
		if b.shut {
			Debugln("file closed on Block.closeFile()")
		} else {
			Debugln("file NOT closed on Block.closeFile()")
		}
	}
	b.file = nil
}

func (b *Block) copyTo(buffer []byte) error { // buffer is large enough, and b is a file.
	if b.IsBlob() {
		BugExitln("copyTo when block is blob")
	}
	nRead := int64(0)
	for {
		if nRead == b.size {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := b.size - nRead; sizeLeft < readSize {
			readSize = sizeLeft
		}
		num, err := b.file.ReadAt(buffer[:readSize], nRead)
		nRead += int64(num)
		if err != nil && nRead != b.size {
			return err
		}
	}
}

func (b *Block) Next() *Block { return b.next }

func (b *Block) IsBlob() bool { return b.kind == 0 }
func (b *Block) IsFile() bool { return b.kind == 1 }

func (b *Block) SetBlob(blob []byte) {
	b.closeFile()
	b.shut = false
	b.kind = 0
	b.data = risky.ReferTo(blob)
	b.size = int64(len(blob))
	b.time = 0
}
func (b *Block) SetFile(file *os.File, info os.FileInfo, shut bool) {
	if b.IsBlob() {
		b.data.Reset()
	}
	b.shut = shut
	b.kind = 1
	b.file = file
	b.size = info.Size()
	b.time = info.ModTime().Unix()
}

func (b *Block) Blob() []byte {
	if !b.IsBlob() {
		BugExitln("block is not a blob")
	}
	if b.size == 0 {
		return nil
	}
	return b.data.Bytes()
}
func (b *Block) File() *os.File {
	if !b.IsFile() {
		BugExitln("block is not a file")
	}
	return b.file
}

func (b *Block) ToBlob() error { // used by revisers
	if b.IsBlob() {
		return nil
	}
	blob := make([]byte, b.size)
	num, err := io.ReadFull(b.file, blob) // TODO: convT()?
	b.SetBlob(blob[:num])
	return err
}

// Chain is a linked-list of blocks.
type Chain struct {
	head *Block
	tail *Block
	size int
}

func (c *Chain) free() {
	if c.size == 0 {
		return
	}
	block := c.head
	c.head, c.tail = nil, nil
	size := 0
	for block != nil {
		next := block.next
		block.free()
		if block.pool { // only put those got from poolBlock because they are not fixed
			putBlock(block)
		}
		size++
		block = next
	}
	if size != c.size {
		BugExitln("bad chain")
	}
	c.size = 0
}

func (c *Chain) Size() int { return c.size }

func (c *Chain) PushHead(block *Block) {
	if block == nil {
		return
	}
	if c.size == 0 {
		c.head, c.tail = block, block
	} else {
		block.next = c.head
		c.head = block
	}
	c.size++
}
func (c *Chain) PushTail(block *Block) {
	if block == nil {
		return
	}
	if c.size == 0 {
		c.head, c.tail = block, block
	} else {
		c.tail.next = block
		c.tail = block
	}
	c.size++
}

// hostnameTo
type hostnameTo[T Component] struct {
	hostname []byte // "example.com" for exact map, ".example.com" for suffix map, "www.example." for prefix map
	target   T
}

const ( // array kinds
	arrayKindStock = iota // refers to stock buffer. must be 0
	arrayKindPool         // got from sync.Pool
	arrayKindMake         // made from make([]byte)
)

// pool255Pairs
var pool255Pairs sync.Pool

func get255Pairs() []pair {
	if x := pool255Pairs.Get(); x == nil {
		return make([]pair, 0, 255)
	} else {
		return x.([]pair)
	}
}
func put255Pairs(pairs []pair) {
	if cap(pairs) != 255 {
		BugExitln("bad pairs")
	}
	pairs = pairs[0:0:255] // reset
	pool255Pairs.Put(pairs)
}

// pair is used to hold queries, headers, cookies, forms (but not uploads), and trailers.
type pair struct { // 16 bytes
	hash     uint16 // name hash, to support fast search. hash == 0 means empty
	flags    uint8  // see pair flags
	nameSize uint8  // name size, <= 255
	nameFrom int32  // like: "content-type"
	value    text   // like: "text/html; charset=utf-8"
}

func (p *pair) zero() { *p = pair{} }
func (p *pair) nameAt(t []byte) []byte {
	return t[p.nameFrom : p.nameFrom+int32(p.nameSize)]
}
func (p *pair) valueAt(t []byte) []byte {
	return t[p.value.from:p.value.edge]
}
func (p *pair) nameEqualString(t []byte, x string) bool {
	return int(p.nameSize) == len(x) && string(t[p.nameFrom:p.nameFrom+int32(p.nameSize)]) == x
}
func (p *pair) nameEqualBytes(t []byte, x []byte) bool {
	return int(p.nameSize) == len(x) && bytes.Equal(t[p.nameFrom:p.nameFrom+int32(p.nameSize)], x)
}

const ( // pair flags
	pairMaskExtraKind = 0b11000000 // kind for extra pairs, values below
	extraKindQuery    = 0b00000000 // extra queries
	extraKindHeader   = 0b01000000 // extra headers
	extraKindCookie   = 0b10000000 // extra cookies
	extraKindTrailer  = 0b11000000 // extra trailers
	extraKindNoExtra  = 0b11111111 // no extra, for comparison only

	pairMaskPlace    = 0b00110000 // place where pair data is stored. values below
	pairPlaceInput   = 0b00000000 // in r.input
	pairPlaceArray   = 0b00010000 // in r.array
	pairPlaceStatic2 = 0b00100000 // in HTTP/2 static table
	pairPlaceStatic3 = 0b00110000 // in HTTP/3 static table

	pairFlagWeakETag = 0b00001000 // weak etag or not
	pairFlagLiteral  = 0b00000100 // keep literal or not. used in HTTP/2 and HTTP/3
	pairFlagPseudo   = 0b00000010 // pseudo header or not. used in HTTP/2 and HTTP/3
	pairFlagSubField = 0b00000001 // sub field or not. currently only used by headers
)

func (p *pair) setKind(kind uint8)     { p.flags = p.flags&^pairMaskExtraKind | kind }
func (p *pair) isKind(kind uint8) bool { return p.flags&pairMaskExtraKind == kind }

func (p *pair) setPlace(place uint8)     { p.flags = p.flags&^pairMaskPlace | place }
func (p *pair) inPlace(place uint8) bool { return p.flags&pairMaskPlace == place }

func (p *pair) setWeakETag(weak bool) { p._setFlag(pairFlagWeakETag, weak) }
func (p *pair) isWeakETag() bool      { return p.flags&pairFlagWeakETag > 0 }

func (p *pair) setLiteral(literal bool) { p._setFlag(pairFlagLiteral, literal) }
func (p *pair) isLiteral() bool         { return p.flags&pairFlagLiteral > 0 }

func (p *pair) setPseudo(pseudo bool) { p._setFlag(pairFlagPseudo, pseudo) }
func (p *pair) isPseudo() bool        { return p.flags&pairFlagPseudo > 0 }

func (p *pair) setSubField(subField bool) { p._setFlag(pairFlagSubField, subField) }
func (p *pair) isSubField() bool          { return p.flags&pairFlagSubField > 0 }

func (p *pair) _setFlag(pairFlag uint8, on bool) {
	if on {
		p.flags |= pairFlag
	} else { // off
		p.flags &^= pairFlag
	}
}

// nava is a name-value pair.
type nava struct { // 16 bytes
	name, value text
}

// zone
type zone struct { // 2 bytes
	from, edge uint8 // edge is ensured to be <= 255
}

func (z *zone) zero()          { *z = zone{} }
func (z *zone) size() int      { return int(z.edge - z.from) }
func (z *zone) isEmpty() bool  { return z.from == z.edge }
func (z *zone) notEmpty() bool { return z.from != z.edge }

// text
type text struct { // 8 bytes
	from, edge int32 // p[from:edge] is the bytes. edge is ensured to be <= 2147483647
}

func (t *text) zero()          { *t = text{} }
func (t *text) size() int      { return int(t.edge - t.from) }
func (t *text) isEmpty() bool  { return t.from == t.edge }
func (t *text) notEmpty() bool { return t.from != t.edge }

func (t *text) set(from int32, edge int32) {
	t.from, t.edge = from, edge
}
func (t *text) sub(delta int32) {
	if t.from >= delta {
		t.from -= delta
		t.edge -= delta
	}
}

// span defines a range.
type span struct { // 16 bytes
	from, last int64 // including last
}

func makeTempName(p []byte, stageID int64, connID int64, stamp int64, counter int64) (from int, edge int) {
	// TODO: improvement
	stageID &= 0xff
	connID &= 0xffff
	stamp &= 0xffffffff
	counter &= 0xff
	// stageID(8) | connID(16) | seconds(32) | counter(8)
	i64 := stageID<<56 | connID<<40 | stamp<<8 | counter
	i64 &= 0x7fffffffffffffff // clear left-most bit
	return i64ToDec(i64, p)
}

// TempFile is used to temporarily save request/response content in local file system.
type TempFile interface {
	Name() string // used by os.Remove()
	Write(p []byte) (n int, err error)
	Seek(offset int64, whence int) (ret int64, err error)
	Close() error
}

// FakeFile
var FakeFile _fakeFile

// _fakeFile implements TempFile.
type _fakeFile struct{}

func (f _fakeFile) Name() string                           { return "" }
func (f _fakeFile) Write(p []byte) (n int, err error)      { return }
func (f _fakeFile) Seek(int64, int) (ret int64, err error) { return }
func (f _fakeFile) Close() error                           { return nil }

// region
type region struct { // 512B
	blocks [][]byte  // the blocks. [<stocks>/make]
	stocks [4][]byte // for blocks. 96B
	block0 [392]byte // for blocks[0]
}

func (r *region) init() {
	r.blocks = r.stocks[0:1:cap(r.stocks)]                    // block0 always at 0
	r.stocks[0] = r.block0[:]                                 // first block is always block0
	binary.BigEndian.PutUint16(r.block0[cap(r.block0)-2:], 0) // reset used size of block0
}
func (r *region) alloc(size int) []byte { // good for a lot of small buffers
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
func (r *region) free() {
	for i := 1; i < len(r.blocks); i++ {
		PutNK(r.blocks[i])
		r.blocks[i] = nil
	}
	if cap(r.blocks) != cap(r.stocks) {
		r.stocks = [cap(r.stocks)][]byte{}
		r.blocks = nil
	}
}

// booker
type booker struct {
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
func getNK(pool *sync.Pool, size int) []byte {
	if x := pool.Get(); x == nil {
		return make([]byte, size)
	} else {
		return x.([]byte)
	}
}

func GetNK(n int64) []byte {
	if n <= _4K {
		return getNK(&pool4K, _4K)
	} else if n <= _16K {
		return getNK(&pool16K, _16K)
	} else {
		return getNK(&pool64K1, _64K1)
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

func byteIsAlpha(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }
func byteIsDigit(b byte) bool { return b >= '0' && b <= '9' }
func byteIsAlnum(b byte) bool { return byteIsAlpha(b) || byteIsDigit(b) }
func byteIsBlank(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

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
			p[i] = b - 0x20 // to lower
		}
	}
}
func bytesTrimLeft(p []byte, b byte) []byte {
	i := 0
	for i < len(p) && p[i] == b {
		i++
	}
	return p[i:]
}
func bytesTrimRight(p []byte, b byte) []byte {
	i := len(p) - 1
	for i >= 0 && p[i] == b {
		i--
	}
	return p[:i+1]
}
func bytesHash(p []byte) uint16 {
	hash := uint16(0)
	for _, b := range p {
		hash += uint16(b)
	}
	return hash
}

func stringIsWord(s string) bool {
	for i := 0; i < len(s); i++ {
		// '0-9a-zA-Z-_'
		if b := s[i]; !byteIsAlnum(b) && b != '-' && b != '_' {
			return false
		}
	}
	return true
}
func stringTrimLeft(s string, b byte) string {
	i := 0
	for i < len(s) && s[i] == b {
		i++
	}
	return s[i:]
}
func stringTrimRight(s string, b byte) string {
	i := len(s) - 1
	for i >= 0 && s[i] == b {
		i--
	}
	return s[:i+1]
}
func stringTrim(s string, b byte) string {
	i := 0
	for i < len(s) && s[i] == b {
		i++
	}
	s = s[i:]
	i = len(s) - 1
	for i >= 0 && s[i] == b {
		i--
	}
	return s[:i+1]
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
