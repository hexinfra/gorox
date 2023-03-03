// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP Helpers.

package internal

import (
	"bytes"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"io"
	"os"
	"sync"
)

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

// pair is used to hold queries, headers, cookies, forms, and trailers.
type pair struct { // 20 bytes
	hash     uint16 // name hash, to support fast search. 0 means empty
	kind     int8   // see pair kinds
	place    int8   // see pair places
	from     int32  // name begins from
	flags    uint8  // see field flags
	nameSize uint8  // name ends at from+nameSize. <= 255
	valueOff uint16 // value begins from from+valueOff
	dataBack uint16 // value data without quote and paras
	paras    zone   // refers to a zone of paras
	edge     int32  // value ends at
}

// If "example-type" is defined as: quote=true, para=true, then a non-comma "example-type" field looks like this:
//
//  [   name   ]   [     data    ][--------dataBack--------]
// +--------------------------------------------------------+
// |example-type: "text/javascript"; charset="utf-8";lang=en|
// +--------------------------------------------------------+
//  ^           ^ ^^              ^[         paras         ]^
//  |           | ||              |                         |
//  from        | ||              edge-dataBack          edge
//  from+nameSize ||
//    from+valueOff|
//                 from+valueOff+(flags&flagQuoted)
//
// If data is quoted, flagQuoted is set, so flags&flagQuoted is 1.

func (p *pair) zero() { *p = pair{} }

func (p *pair) nameAt(t []byte) []byte { return t[p.from : p.from+int32(p.nameSize)] }
func (p *pair) nameEqualString(t []byte, x string) bool {
	return int(p.nameSize) == len(x) && string(t[p.from:p.from+int32(p.nameSize)]) == x
}
func (p *pair) nameEqualBytes(t []byte, x []byte) bool {
	return int(p.nameSize) == len(x) && bytes.Equal(t[p.from:p.from+int32(p.nameSize)], x)
}
func (p *pair) valueText() text         { return text{p.from + int32(p.valueOff), p.edge} }
func (p *pair) isEmpty() bool           { return p.from+int32(p.valueOff) == p.edge }
func (p *pair) valueAt(t []byte) []byte { return t[p.from+int32(p.valueOff) : p.edge] }
func (p *pair) dataAt(t []byte) []byte {
	return t[p.from+int32(p.valueOff)+int32(p.flags&flagQuoted) : p.edge-int32(p.dataBack)]
}
func (p *pair) paraAt(t []byte, name []byte) []byte {
	if !p.paras.isEmpty() {
		for i := p.paras.from; i < p.paras.edge; i++ {
			// TODO
		}
	}
	return nil
}

const ( // pair kinds
	kindQuery   = iota // prime->array, extra->array
	kindHeader         // prime->input, extra->array
	kindCookie         // prime->input, extra->array
	kindForm           // prime->array, extra->array
	kindTrailer        // prime->array, extra->array
)

const ( // pair places
	placeInput = iota // prime headers, prime cookies
	placeArray        // prime queries, prime forms, prime trailers, all extras
	placeStatic2
	placeStatic3
)

const ( // field flags
	flagSingleton  = 0b10000000 // singleton or not
	flagSubField   = 0b01000000 // sub field or not
	flagLiteral    = 0b00100000 // keep literal or not. used in HTTP/2 and HTTP/3
	flagPseudo     = 0b00010000 // pseudo header or not. used in HTTP/2 and HTTP/3
	flagUnderscore = 0b00001000 // name contains '_' or not. some agents (like fcgi) need this
	flagCommaValue = 0b00000100 // value has comma or not
	flagWeakETag   = 0b00000010 // weak etag or not
	flagQuoted     = 0b00000001 // value is quoted or not. for non comma-value field only. MUST be 0b00000001
)

func (p *pair) setSingleton()  { p.flags |= flagSingleton }
func (p *pair) setSubField()   { p.flags |= flagSubField }
func (p *pair) setLiteral()    { p.flags |= flagLiteral }
func (p *pair) setPseudo()     { p.flags |= flagPseudo }
func (p *pair) setUnderscore() { p.flags |= flagUnderscore }
func (p *pair) setCommaValue() { p.flags |= flagCommaValue }
func (p *pair) setWeakETag()   { p.flags |= flagWeakETag }
func (p *pair) setQuoted()     { p.flags |= flagQuoted }

func (p *pair) isSingleton() bool  { return p.flags&flagSingleton > 0 }
func (p *pair) isSubField() bool   { return p.flags&flagSubField > 0 }
func (p *pair) isLiteral() bool    { return p.flags&flagLiteral > 0 }
func (p *pair) isPseudo() bool     { return p.flags&flagPseudo > 0 }
func (p *pair) isUnderscore() bool { return p.flags&flagUnderscore > 0 }
func (p *pair) isCommaValue() bool { return p.flags&flagCommaValue > 0 }
func (p *pair) isWeakETag() bool   { return p.flags&flagWeakETag > 0 }
func (p *pair) isQuoted() bool     { return p.flags&flagQuoted > 0 }

// poolParas
var poolParas sync.Pool

func getParas() []para {
	if x := poolParas.Get(); x == nil {
		return make([]para, 0, 128)
	} else {
		return x.([]para)
	}
}
func putParas(paras []para) {
	if cap(paras) != 128 {
		BugExitln("bad paras")
	}
	paras = paras[0:0:128] // reset
	poolParas.Put(paras)
}

// para
type para struct { // 8 bytes
	name, value span
}

func (p *para) zero() { *p = para{} }

func (p *para) nameAt(t []byte) []byte  { return t[p.name.from:p.name.edge] }
func (p *para) valueAt(t []byte) []byte { return t[p.value.from:p.value.edge] }
func (p *para) nameEqualString(t []byte, x string) bool {
	return p.name.size() == len(x) && string(t[p.name.from:p.name.edge]) == x
}
func (p *para) nameEqualBytes(t []byte, x []byte) bool {
	return p.name.size() == len(x) && bytes.Equal(t[p.name.from:p.name.edge], x)
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

// hostnameTo
type hostnameTo[T Component] struct {
	hostname []byte // "example.com" for exact map, ".example.com" for suffix map, "www.example." for prefix map
	target   T
}
