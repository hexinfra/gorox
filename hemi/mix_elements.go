// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General elements for net, rpc, and web.

package hemi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"regexp"
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
		r.stocks = [4][]byte{}
		r.blocks = nil
	}
}

// tempFile is used to temporarily save incoming content in local file system.
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
	unsafeVariable(varCode int16, varName string) (varValue []byte)
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
