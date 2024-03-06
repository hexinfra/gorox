// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Common elements.

package hemi

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
	"unsafe"
)

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

func ConstBytes(s string) (p []byte) { // WARNING: *DO NOT* mutate s through p!
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
func WeakString(p []byte) (s string) { // WARNING: *DO NOT* mutate p while s is in use!
	return unsafe.String(unsafe.SliceData(p), len(p))
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
		i64 = i64<<4 + int64(b)
		if i64 < 0 {
			return 0, false
		}
	}
	return i64, true
}
func decToI64(dec []byte) (int64, bool) {
	if n := len(dec); n == 0 || n > 19 { // the max number of int64 is 19 bytes
		return 0, false
	}
	var i64 int64
	for _, b := range dec {
		if b >= '0' && b <= '9' {
			b = b - '0'
		} else {
			return 0, false
		}
		i64 = i64*10 + int64(b)
		if i64 < 0 {
			return 0, false
		}
	}
	return i64, true
}

const hexDigits = "0123456789abcdef"

func i64ToHex(i64 int64, hex []byte) int { return intToHex(i64, hex, 16) }
func i32ToHex(i32 int32, hex []byte) int { return intToHex(i32, hex, 8) }
func intToHex[T int32 | int64](ixx T, hex []byte, bufSize int) int {
	if len(hex) < bufSize {
		BugExitln("hex is too small")
	}
	if ixx < 0 {
		BugExitln("negative numbers are not supported")
	}
	n := 1
	for i := ixx; i >= 0x10; i >>= 4 {
		n++
	}
	j := n - 1
	for ixx >= 0x10 {
		t := ixx >> 4
		hex[j] = hexDigits[ixx-t<<4]
		j--
		ixx = t
	}
	hex[j] = hexDigits[ixx]
	return n
}

func i64ToDec(i64 int64, dec []byte) int { return intToDec(i64, dec, 19) } // 19 bytes are enough to hold a positive int64
func i32ToDec(i32 int32, dec []byte) int { return intToDec(i32, dec, 10) } // 10 bytes are enough to hold a positive int32
func intToDec[T int32 | int64](ixx T, dec []byte, bufSize int) int {
	if len(dec) < bufSize {
		BugExitln("dec is too small")
	}
	if ixx < 0 {
		BugExitln("negative numbers are not supported")
	}
	n := 1
	for i := ixx; i >= 10; i /= 10 {
		n++
	}
	j := n - 1
	for ixx >= 10 {
		t := ixx / 10
		dec[j] = byte(ixx - t*10 + '0')
		j--
		ixx = t
	}
	dec[j] = byte(ixx + '0')
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
