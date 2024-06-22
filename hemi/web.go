// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Misc elements for web.

package hemi

import (
	"bytes"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// httpServend collects shared methods between HTTPServer or HTTPBackend.
type httpServend interface {
	// Imports
	contentSaver
	// Methods
	Stage() *Stage
	RecvTimeout() time.Duration // timeout to recv the whole message content
	SendTimeout() time.Duration // timeout to send the whole message
	MaxContentSize() int64
	MaxMemoryContentSize() int32
	MaxStreamsPerConn() int32
}

// _httpServend_ is a mixin for httpServer_ and HTTP[1-3]Backend.
type _httpServend_ struct {
	// Mixins
	_contentSaver_ // so responses can save their large contents in local file system.
	// States
	recvTimeout          time.Duration // timeout to recv the whole message content
	sendTimeout          time.Duration // timeout to send the whole message
	maxContentSize       int64         // max content size allowed to receive
	maxMemoryContentSize int32         // max content size that can be loaded into memory directly
	maxStreamsPerConn    int32         // max streams of one conn. 0 means infinite
}

func (s *_httpServend_) onConfigure(servend Component, recvTimeout time.Duration, sendTimeout time.Duration, defaultMaxStreams int32, defaultDir string) {
	s._contentSaver_.onConfigure(servend, defaultDir)

	// recvTimeout
	servend.ConfigureDuration("recvTimeout", &s.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, recvTimeout)

	// sendTimeout
	servend.ConfigureDuration("sendTimeout", &s.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, sendTimeout)

	// maxContentSize
	servend.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)

	// maxMemoryContentSize
	servend.ConfigureInt32("maxMemoryContentSize", &s.maxMemoryContentSize, func(value int32) error {
		if value > 0 && value <= _1G { // DO NOT CHANGE THIS, otherwise integer overflow may occur
			return nil
		}
		return errors.New(".maxMemoryContentSize has an invalid value")
	}, _16M)

	// maxStreamsPerConn
	servend.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, defaultMaxStreams)
}
func (s *_httpServend_) onPrepare(servend Component) {
	s._contentSaver_.onPrepare(servend, 0755)
}

func (s *_httpServend_) RecvTimeout() time.Duration  { return s.recvTimeout }
func (s *_httpServend_) SendTimeout() time.Duration  { return s.sendTimeout }
func (s *_httpServend_) MaxContentSize() int64       { return s.maxContentSize }
func (s *_httpServend_) MaxMemoryContentSize() int32 { return s.maxMemoryContentSize }
func (s *_httpServend_) MaxStreamsPerConn() int32    { return s.maxStreamsPerConn }

// httpConn collects shared methods between *server[1-3]Conn and *backend[1-3]Conn.
type httpConn interface {
	ID() int64
	IsTLS() bool
	IsUDS() bool
	MakeTempName(p []byte, unixTime int64) int
	markBroken()
	isBroken() bool
}

// httpStream collects shared methods between *server[1-3]Stream and *backend[1-3]Stream.
type httpStream interface {
	httpServend() httpServend
	httpConn() httpConn

	remoteAddr() net.Addr

	buffer256() []byte
	unsafeMake(size int) []byte

	isBroken() bool // returns true if either side of the stream is broken
	markBroken()    // mark stream as broken

	setReadDeadline() error
	setWriteDeadline() error

	read(p []byte) (int, error)
	readFull(p []byte) (int, error)
	write(p []byte) (int, error)
	writev(vector *net.Buffers) (int64, error)
}

// poolPiece
var poolPiece sync.Pool

func GetPiece() *Piece {
	if x := poolPiece.Get(); x == nil {
		piece := new(Piece)
		piece.pool = true // other pieces are not pooled.
		return piece
	} else {
		return x.(*Piece)
	}
}
func putPiece(piece *Piece) { poolPiece.Put(piece) }

// Piece is a member of content chain.
type Piece struct { // 64 bytes
	next *Piece   // next piece
	pool bool     // true if this piece is got from poolPiece. don't change this after set!
	shut bool     // close file on free()?
	kind int8     // 0:text 1:*os.File
	_    [5]byte  // padding
	text []byte   // text
	file *os.File // file
	size int64    // size of text or file
	time int64    // file mod time
}

func (p *Piece) zero() {
	p.closeFile()
	p.next = nil
	p.shut = false
	p.kind = 0
	p.text = nil
	p.file = nil
	p.size = 0
	p.time = 0
}
func (p *Piece) closeFile() {
	if p.IsText() {
		return
	}
	if p.shut {
		p.file.Close()
	}
	if DebugLevel() >= 2 {
		if p.shut {
			Println("file closed in Piece.closeFile()")
		} else {
			Println("file *NOT* closed in Piece.closeFile()")
		}
	}
}

func (p *Piece) copyTo(buffer []byte) error { // buffer is large enough, and p is a file.
	if p.IsText() {
		BugExitln("copyTo when piece is text")
	}
	sizeRead := int64(0)
	for {
		if sizeRead == p.size {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := p.size - sizeRead; sizeLeft < readSize {
			readSize = sizeLeft
		}
		n, err := p.file.ReadAt(buffer[:readSize], sizeRead)
		sizeRead += int64(n)
		if err != nil && sizeRead != p.size {
			return err
		}
	}
}

func (p *Piece) Next() *Piece { return p.next }

func (p *Piece) IsText() bool { return p.kind == 0 }
func (p *Piece) IsFile() bool { return p.kind == 1 }

func (p *Piece) SetText(text []byte) {
	p.closeFile()
	p.shut = false
	p.kind = 0
	p.text = text
	p.file = nil
	p.size = int64(len(text))
	p.time = 0
}
func (p *Piece) SetFile(file *os.File, info os.FileInfo, shut bool) {
	p.closeFile()
	p.shut = shut
	p.kind = 1
	p.text = nil
	p.file = file
	p.size = info.Size()
	p.time = info.ModTime().Unix()
}

func (p *Piece) Text() []byte {
	if !p.IsText() {
		BugExitln("piece is not text")
	}
	if p.size == 0 {
		return nil
	}
	return p.text
}
func (p *Piece) File() *os.File {
	if !p.IsFile() {
		BugExitln("piece is not file")
	}
	return p.file
}

// Chain is a linked-list of pieces.
type Chain struct { // 24 bytes
	head *Piece
	tail *Piece
	qnty int
}

func (c *Chain) free() {
	if DebugLevel() >= 2 {
		Printf("chain.free() called, qnty=%d\n", c.qnty)
	}
	if c.qnty == 0 {
		return
	}
	piece := c.head
	c.head, c.tail = nil, nil
	qnty := 0
	for piece != nil {
		next := piece.next
		piece.zero()
		if piece.pool { // only put those got from poolPiece because they are not fixed
			putPiece(piece)
		}
		qnty++
		piece = next
	}
	if qnty != c.qnty {
		BugExitf("bad chain: qnty=%d c.qnty=%d\n", qnty, c.qnty)
	}
	c.qnty = 0
}

func (c *Chain) Qnty() int { return c.qnty }
func (c *Chain) Size() (int64, bool) {
	size := int64(0)
	for piece := c.head; piece != nil; piece = piece.next {
		size += piece.size
		if size < 0 {
			return 0, false
		}
	}
	return size, true
}

func (c *Chain) PushHead(piece *Piece) {
	if piece == nil {
		return
	}
	if c.qnty == 0 {
		c.head, c.tail = piece, piece
	} else {
		piece.next = c.head
		c.head = piece
	}
	c.qnty++
}
func (c *Chain) PushTail(piece *Piece) {
	if piece == nil {
		return
	}
	if c.qnty == 0 {
		c.head, c.tail = piece, piece
	} else {
		c.tail.next = piece
		c.tail = piece
	}
	c.qnty++
}

// Range defines a range.
type Range struct { // 16 bytes
	From, Last int64 // [From:Last], inclusive
}

// defaultFdesc
var defaultFdesc = &fdesc{
	allowQuote: true,
	allowEmpty: false,
	allowParam: true,
	hasComment: false,
}

// fdesc describes an http field.
type fdesc struct {
	hash       uint16 // name hash
	allowQuote bool   // allow data quote or not
	allowEmpty bool   // allow empty data or not
	allowParam bool   // allow parameters or not
	hasComment bool   // has comment or not
	name       []byte // field name
}

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
	hash     uint16 // name hash, to support fast search. 0 means empty pair
	kind     int8   // see pair kinds
	nameSize uint8  // name ends at nameFrom+nameSize
	nameFrom int32  // name begins from
	value    span   // the value
	place    int8   // see pair places
	flags    byte   // fields only. see field flags
	params   zone   // fields only. refers to a zone of pairs
	dataEdge int32  // fields only. data ends at
}

// If "accept-type" field is defined as: `allowQuote=true allowEmpty=false allowParam=true`, then a non-comma "accept-type" field may looks like this:
//
//                                 [         params         )
//                     [               value                )
//        [   name   )  [  data   )  [    param1    )[param2)
//       +--------------------------------------------------+
//       |accept-type: "text/plain"; charset="utf-8";lang=en|
//       +--------------------------------------------------+
//        ^          ^ ^^         ^                         ^
//        |          | ||         |                         |
// nameFrom          | ||         dataEdge                  |
//   nameFrom+nameSize ||                                   |
//            value.from|                          value.edge
//                      |
//                      dataFrom=value.from+(flags&flagQuoted)
//
// For dataFrom, if data is quoted, then flagQuoted is set, so flags&flagQuoted is 1, which skips '"' exactly.
//
// A has-comma "accept-types" field may looks like this (needs further parsing into sub fields):
//
// +-----------------------------------------------------------------------------------------------------------------+
// |accept-types: "text/plain"; ;charset="utf-8";langs="en,zh" ,,; ;charset="" ,,application/octet-stream ;,image/png|
// +-----------------------------------------------------------------------------------------------------------------+

const ( // pair kinds
	pairUnknown = iota
	pairQuery   // normal
	pairHeader  // field
	pairCookie  // normal
	pairForm    // normal
	pairTrailer // field
	pairParam   // parameter of fields, normal
)

const ( // pair places
	placeInput   = iota
	placeArray   // parsed
	placeStatic2 // http/2 static table
	placeStatic3 // http/3 static table
)

const ( // field flags
	flagParsed     = 0b10000000 // data and params are parsed or not
	flagSingleton  = 0b01000000 // singleton or not. mainly used by proxies
	flagSubField   = 0b00100000 // sub field or not. mainly used by webapps
	flagLiteral    = 0b00010000 // keep literal or not. used in HTTP/2 and HTTP/3
	flagPseudo     = 0b00001000 // pseudo header or not. used in HTTP/2 and HTTP/3
	flagUnderscore = 0b00000100 // name contains '_' or not. some proxies need this information
	flagCommaValue = 0b00000010 // value has comma or not
	flagQuoted     = 0b00000001 // data is quoted or not. for non comma-value field only. MUST be 0b00000001
)

func (p *pair) zero() { *p = pair{} }

func (p *pair) nameAt(t []byte) []byte { return t[p.nameFrom : p.nameFrom+int32(p.nameSize)] }
func (p *pair) nameEqualString(t []byte, x string) bool {
	return int(p.nameSize) == len(x) && string(t[p.nameFrom:p.nameFrom+int32(p.nameSize)]) == x
}
func (p *pair) nameEqualBytes(t []byte, x []byte) bool {
	return int(p.nameSize) == len(x) && bytes.Equal(t[p.nameFrom:p.nameFrom+int32(p.nameSize)], x)
}
func (p *pair) valueAt(t []byte) []byte { return t[p.value.from:p.value.edge] }

func (p *pair) setParsed()     { p.flags |= flagParsed }
func (p *pair) setSingleton()  { p.flags |= flagSingleton }
func (p *pair) setSubField()   { p.flags |= flagSubField }
func (p *pair) setLiteral()    { p.flags |= flagLiteral }
func (p *pair) setPseudo()     { p.flags |= flagPseudo }
func (p *pair) setUnderscore() { p.flags |= flagUnderscore }
func (p *pair) setCommaValue() { p.flags |= flagCommaValue }
func (p *pair) setQuoted()     { p.flags |= flagQuoted }

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

func (p *pair) show(place []byte) { // TODO: optimize, or simply remove
	var kind string
	switch p.kind {
	case pairQuery:
		kind = "query"
	case pairHeader:
		kind = "header"
	case pairCookie:
		kind = "cookie"
	case pairForm:
		kind = "form"
	case pairTrailer:
		kind = "trailer"
	case pairParam:
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
	Printf("{hash=%4d kind=%7s place=[%7s] flags=[%s] dataEdge=%d params=%v value=%v %s=%s}\n", p.hash, kind, plase, strings.Join(flags, ","), p.dataEdge, p.params, p.value, p.nameAt(place), p.valueAt(place))
}

// para is a name-value parameter in multipart/form-data.
type para struct { // 16 bytes
	name, value span
}

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
