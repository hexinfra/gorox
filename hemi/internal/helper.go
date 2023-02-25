// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Helper types.

package internal

import (
	"bytes"
	"encoding/binary"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"io"
	"os"
	"sync"
)

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
		r.stocks = [cap(r.stocks)][]byte{}
		r.blocks = nil
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

// booker
type booker struct {
}

// zone
type zone struct { // 2 bytes
	from, edge uint8 // edge is ensured to be <= 255
}

func (z *zone) zero() { *z = zone{} }

func (z *zone) size() int      { return int(z.edge - z.from) }
func (z *zone) isEmpty() bool  { return z.from == z.edge }
func (z *zone) notEmpty() bool { return z.from != z.edge }

// text
type text struct { // 8 bytes
	from, edge int32 // p[from:edge] is the bytes. edge is ensured to be <= 2147483647
}

func (t *text) zero() { *t = text{} }

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

// poolPairs
var poolPairs sync.Pool

func getPairs() []pair {
	if x := poolPairs.Get(); x == nil {
		return make([]pair, 0, 204) // 20B*204=4080B
	} else {
		return x.([]pair)
	}
}
func putPairs(pairs []pair) {
	if cap(pairs) != 204 {
		BugExitln("bad pairs")
	}
	pairs = pairs[0:0:204] // reset
	poolPairs.Put(pairs)
}

// pair is used to hold queries, headers (and sub headers), cookies, forms, and trailers.
type pair struct { // 20 bytes
	hash     uint16 // name hash, to support fast search. hash == 0 means empty
	mode     int8   // modeXXX
	extra    int8   // extraXXX
	place    int8   // placeXXX
	copied   bool   // if pair is copied to the other side, set true
	flags    uint8  // see pair flags
	nameSize uint8  // name size, <= 255
	nameFrom int32  // like: "content-type"
	value    text   // like: "text/html; charset=utf-8"
}

func (p *pair) zero() { *p = pair{} }

func (p *pair) nameAt(t []byte) []byte  { return t[p.nameFrom : p.nameFrom+int32(p.nameSize)] }
func (p *pair) valueAt(t []byte) []byte { return t[p.value.from:p.value.edge] }
func (p *pair) nameEqualString(t []byte, x string) bool {
	return int(p.nameSize) == len(x) && string(t[p.nameFrom:p.nameFrom+int32(p.nameSize)]) == x
}
func (p *pair) nameEqualBytes(t []byte, x []byte) bool {
	return int(p.nameSize) == len(x) && bytes.Equal(t[p.nameFrom:p.nameFrom+int32(p.nameSize)], x)
}

const ( // pair modes
	modeAlone = iota // singleton. like content-length, ...
	mode0Plus        // #value
	mode1Plus        // 1#value
)

const ( // extra kinds
	extraNone = iota
	extraQuery
	extraHeader
	extraCookie
	extraTrailer
)

const ( // pair places
	placeInput = iota
	placeArray
	placeStatic2
	placeStatic3
)

const ( // pair flags
	flagMultivalued = 0b10000000 // multivalued or not
	flagWeakETag    = 0b01000000 // weak etag or not
	flagLiteral     = 0b00100000 // keep literal or not. used in HTTP/2 and HTTP/3
	flagPseudo      = 0b00010000 // pseudo header or not. used in HTTP/2 and HTTP/3
	flagUnderscore  = 0b00001000 // pair name contains '_' or not
	flagSubField    = 0b00000100 // sub field or not. currently only used by headers
)

func (p *pair) setMultivalued(multivalued bool) { p._setFlag(flagMultivalued, multivalued) }
func (p *pair) isMultivalued() bool             { return p.flags&flagMultivalued > 0 }

func (p *pair) setWeakETag(weak bool) { p._setFlag(flagWeakETag, weak) }
func (p *pair) isWeakETag() bool      { return p.flags&flagWeakETag > 0 }

func (p *pair) setLiteral(literal bool) { p._setFlag(flagLiteral, literal) }
func (p *pair) isLiteral() bool         { return p.flags&flagLiteral > 0 }

func (p *pair) setPseudo(pseudo bool) { p._setFlag(flagPseudo, pseudo) }
func (p *pair) isPseudo() bool        { return p.flags&flagPseudo > 0 }

func (p *pair) setUnderscore(underscore bool) { p._setFlag(flagUnderscore, underscore) }
func (p *pair) isUnderscore() bool            { return p.flags&flagUnderscore > 0 }

func (p *pair) setSubField(subField bool) { p._setFlag(flagSubField, subField) }
func (p *pair) isSubField() bool          { return p.flags&flagSubField > 0 }

func (p *pair) _setFlag(flag uint8, on bool) {
	if on {
		p.flags |= flag
	} else { // off
		p.flags &^= flag
	}
}

// nava is a name-value pair.
type nava struct { // 16 bytes
	name, value text
}

// span defines a range.
type span struct { // 16 bytes
	from, last int64 // including last
}
