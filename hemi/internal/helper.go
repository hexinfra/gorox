// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Helpers.

package internal

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// poolPairs
var poolPairs sync.Pool

const maxPairs = 250 // 24B*250=6000B

func getPairs() []pair {
	if x := poolPairs.Get(); x == nil {
		return make([]pair, 0, maxPairs)
	} else {
		return x.([]pair)
	}
}
func putPairs(pairs []pair) {
	if cap(pairs) != maxPairs {
		BugExitln("bad pairs")
	}
	pairs = pairs[0:0:maxPairs] // reset
	poolPairs.Put(pairs)
}

// pair is used to hold queries, headers, cookies, forms, trailers, and params.
type pair struct { // 24 bytes
	hash     uint16 // name hash, to support fast search. 0 means empty
	kind     int8   // see pair kinds
	nameSize uint8  // name ends at nameFrom+nameSize
	nameFrom int32  // name begins from
	value    text   // the value
	place    int8   // see pair places
	flags    byte   // fields only. see field flags
	params   zone   // fields only. refers to a zone of pairs
	dataEdge int32  // fields only. data ends at
}

func (p *pair) zero() { *p = pair{} }

const ( // pair kinds
	kindUnknown = iota
	kindQuery
	kindHeader
	kindCookie
	kindForm
	kindTrailer
	kindParam // parameter of fields
)

const ( // pair places
	placeInput = iota
	placeArray
	placeStatic2
	placeStatic3
)

const ( // field flags
	flagParsed     = 0b10000000 // data and params are parsed or not
	flagSingleton  = 0b01000000 // singleton or not. mainly used by proxies
	flagSubField   = 0b00100000 // sub field or not. mainly used by apps
	flagLiteral    = 0b00010000 // keep literal or not. used in HTTP/2 and HTTP/3
	flagPseudo     = 0b00001000 // pseudo header or not. used in HTTP/2 and HTTP/3
	flagUnderscore = 0b00000100 // name contains '_' or not. some agents (like fcgi) need this information
	flagCommaValue = 0b00000010 // value has comma or not
	flagQuoted     = 0b00000001 // data is quoted or not. for non comma-value field only. MUST be 0b00000001
)

// If "example-name" is not a field, and has a value "example-value", then it looks like this:
//
//                    [   value    )
//        [   name    )
//       +-------------------------+
//       |example-nameexample-value|
//       +-------------------------+
//        ^           ^            ^
//        |           |            |
// nameFrom           |            |
//    nameFrom+nameSize            |
//           value.from   value.edge
//
// flags, params, and dataEdge are NOT used in this case.
//
// If "accept-type" field is defined as: `allowQuote=true allowEmpty=false allowParam=true`, then a non-comma "accept-type" field may looks like this:
//
//                     [             value                  )
//        [   name   )  [  data   )[         params         )
//       +--------------------------------------------------+
//       |accept-type: "text/plain"; charset="utf-8";lang=en|
//       +--------------------------------------------------+
//        ^          ^ ^^         ^                         ^
//        |          | ||         |                         |
// nameFrom          | ||  dataEdge                value.edge
//   nameFrom+nameSize ||
//            value.from|
//                      value.from+(flags&flagQuoted)
//
// If data is quoted, then flagQuoted is set, so flags&flagQuoted is 1, which skips '"' exactly.
//
// A has-comma "accept-types" field may looks like this (needs further parsing into sub fields):
//
// +-----------------------------------------------------------------------------------------------------------------+
// |accept-types: "text/plain"; ;charset="utf-8";langs="en,zh" ,,; ;charset="" ,,application/octet-stream ;,image/png|
// +-----------------------------------------------------------------------------------------------------------------+

func (p *pair) nameAt(t []byte) []byte { return t[p.nameFrom : p.nameFrom+int32(p.nameSize)] }
func (p *pair) nameEqualString(t []byte, x string) bool {
	return int(p.nameSize) == len(x) && string(t[p.nameFrom:p.nameFrom+int32(p.nameSize)]) == x
}
func (p *pair) nameEqualBytes(t []byte, x []byte) bool {
	return int(p.nameSize) == len(x) && bytes.Equal(t[p.nameFrom:p.nameFrom+int32(p.nameSize)], x)
}
func (p *pair) valueAt(t []byte) []byte { return t[p.value.from:p.value.edge] }

func (p *pair) setParsed()         { p.flags |= flagParsed }
func (p *pair) setSingleton()      { p.flags |= flagSingleton }
func (p *pair) setSubField()       { p.flags |= flagSubField }
func (p *pair) setLiteral()        { p.flags |= flagLiteral }
func (p *pair) setPseudo()         { p.flags |= flagPseudo }
func (p *pair) setUnderscore()     { p.flags |= flagUnderscore }
func (p *pair) setCommaValue()     { p.flags |= flagCommaValue }
func (p *pair) setQuoted()         { p.flags |= flagQuoted }
func (p *pair) isParsed() bool     { return p.flags&flagParsed > 0 }
func (p *pair) isSingleton() bool  { return p.flags&flagSingleton > 0 }
func (p *pair) isSubField() bool   { return p.flags&flagSubField > 0 }
func (p *pair) isLiteral() bool    { return p.flags&flagLiteral > 0 }
func (p *pair) isPseudo() bool     { return p.flags&flagPseudo > 0 }
func (p *pair) isUnderscore() bool { return p.flags&flagUnderscore > 0 }
func (p *pair) isCommaValue() bool { return p.flags&flagCommaValue > 0 }
func (p *pair) isQuoted() bool     { return p.flags&flagQuoted > 0 }

func (p *pair) dataAt(t []byte) []byte { return t[p.value.from+int32(p.flags&flagQuoted) : p.dataEdge] }
func (p *pair) dataEmpty() bool        { return p.value.from+int32(p.flags&flagQuoted) == p.dataEdge }

func (p *pair) show(place []byte) {
	var kind string
	switch p.kind {
	case kindQuery:
		kind = "query"
	case kindHeader:
		kind = "header"
	case kindCookie:
		kind = "cookie"
	case kindForm:
		kind = "form"
	case kindTrailer:
		kind = "trailer"
	case kindParam:
		kind = "param"
	default:
		kind = "unknown"
	}
	var plase string
	switch p.place {
	case placeInput:
		plase = "input"
	case placeArray:
		plase = "array"
	case placeStatic2:
		plase = "static2"
	case placeStatic3:
		plase = "static3"
	default:
		plase = "unknown"
	}
	var flags []string
	if p.isParsed() {
		flags = append(flags, "parsed")
	}
	if p.isSingleton() {
		flags = append(flags, "singleton")
	}
	if p.isSubField() {
		flags = append(flags, "subField")
	}
	if p.isCommaValue() {
		flags = append(flags, "commaValue")
	}
	if p.isQuoted() {
		flags = append(flags, "quoted")
	}
	if len(flags) == 0 {
		flags = append(flags, "nothing")
	}
	Debugf("{hash=%4d kind=%7s place=[%7s] flags=[%s] dataEdge=%d params=%v value=%v %s=%s}\n", p.hash, kind, plase, strings.Join(flags, ","), p.dataEdge, p.params, p.value, p.nameAt(place), p.valueAt(place))
}

// defaultDesc
var defaultDesc = &desc{
	allowQuote: true,
	allowEmpty: false,
	allowParam: true,
	hasComment: false,
}

// desc describes an HTTP field.
type desc struct {
	hash       uint16 // name hash
	allowQuote bool   // allow data quote or not
	allowEmpty bool   // allow empty data or not
	allowParam bool   // allow parameters or not
	hasComment bool   // has comment or not
	name       []byte // field name
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
type Block struct { // 64 bytes
	next *Block   // next block
	pool bool     // true if this block is got from poolBlock. don't change this after set
	shut bool     // close file on free()?
	kind int8     // 0:data 1:*os.File
	file *os.File // for general use
	data []byte   // data
	size int64    // size of data or file
	time int64    // file mod time
}

func (b *Block) free() {
	b.closeFile()
	b.shut = false
	b.kind = 0
	b.data = nil
	b.size = 0
	b.time = 0
}
func (b *Block) closeFile() {
	if b.IsData() {
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
	if b.IsData() {
		BugExitln("copyTo when block is data")
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

func (b *Block) IsData() bool { return b.kind == 0 }
func (b *Block) IsFile() bool { return b.kind == 1 }

func (b *Block) SetData(data []byte) {
	b.closeFile()
	b.shut = false
	b.kind = 0
	b.data = data
	b.size = int64(len(data))
	b.time = 0
}
func (b *Block) SetFile(file *os.File, info os.FileInfo, shut bool) {
	b.data = nil
	b.shut = shut
	b.kind = 1
	b.file = file
	b.size = info.Size()
	b.time = info.ModTime().Unix()
}

func (b *Block) Data() []byte {
	if !b.IsData() {
		BugExitln("block is not data")
	}
	if b.size == 0 {
		return nil
	}
	return b.data
}
func (b *Block) File() *os.File {
	if !b.IsFile() {
		BugExitln("block is not file")
	}
	return b.file
}

func (b *Block) ToData() error { // used by revisers
	if b.IsData() {
		return nil
	}
	data := make([]byte, b.size)
	num, err := io.ReadFull(b.file, data) // TODO: convT()?
	b.SetData(data[:num])
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

// booker
type booker struct {
	file   *os.File
	queue  chan string
	buffer []byte
	size   int
	used   int
}

func newBooker(file string) (*booker, error) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	b := new(booker)
	b.file = f
	b.queue = make(chan string)
	b.buffer = make([]byte, 1048576)
	b.size = len(b.buffer)
	b.used = 0
	go b.saver()
	return b, nil
}

func (b *booker) Log(v ...any) {
	if s := fmt.Sprint(v...); s != "" {
		b.queue <- s
	}
}
func (b *booker) Logln(v ...any) {
	if s := fmt.Sprintln(v...); s != "" {
		b.queue <- s
	}
}
func (b *booker) Logf(f string, v ...any) {
	if s := fmt.Sprintf(f, v...); s != "" {
		b.queue <- s
	}
}

func (b *booker) Close() { b.queue <- "" }

func (b *booker) saver() { // goroutine
	for {
		s := <-b.queue
		if s != "" {
			b.write(s)
		} else {
			b.clear()
			b.file.Close()
			return
		}
	more:
		for {
			select {
			case s = <-b.queue:
				if s != "" {
					b.write(s)
				} else {
					b.clear()
					b.file.Close()
					return
				}
			default:
				b.clear()
				break more
			}
		}
	}
}
func (b *booker) write(s string) {
	n := len(s)
	if n >= b.size {
		b.clear()
		b.flush(risky.ConstBytes(s))
		return
	}
	w := copy(b.buffer[b.used:], s)
	b.used += w
	if b.used == b.size {
		b.clear()
		if n -= w; n > 0 {
			copy(b.buffer, s[w:])
			b.used = n
		}
	}
}
func (b *booker) clear() {
	if b.used > 0 {
		b.flush(b.buffer[:b.used])
		b.used = 0
	}
}
func (b *booker) flush(p []byte) { b.file.Write(p) }

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
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) bool { return value != "" && len(value) <= 232 }, defaultDir)
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

// subsWaiter_
type subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *subsWaiter_) IncSub(n int) { w.subs.Add(n) }
func (w *subsWaiter_) WaitSubs()    { w.subs.Wait() }
func (w *subsWaiter_) SubDone()     { w.subs.Done() }

// identifiable
type identifiable interface {
	ID() uint8
	setID(id uint8)
}

// identifiable_ is a mixin.
type identifiable_ struct {
	id uint8
}

func (i *identifiable_) ID() uint8      { return i.id }
func (i *identifiable_) setID(id uint8) { i.id = id }
