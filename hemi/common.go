// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Common elements in the engine.

package hemi

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
	"unsafe"
)

func Println(v ...any) {
	_printTime(os.Stdout)
	fmt.Fprintln(os.Stdout, v...)
}
func Printf(f string, v ...any) {
	_printTime(os.Stdout)
	fmt.Fprintf(os.Stdout, f, v...)
}

func Errorln(v ...any) {
	_printTime(os.Stderr)
	fmt.Fprintln(os.Stderr, v...)
}
func Errorf(f string, v ...any) {
	_printTime(os.Stderr)
	fmt.Fprintf(os.Stderr, f, v...)
}

func _printTime(file *os.File) {
	fmt.Fprintf(file, "[%s] ", time.Now().Format("2006-01-02 15:04:05 MST"))
}

const ( // exit codes
	CodeBug = 20
	CodeUse = 21
	CodeEnv = 22
)

func BugExitln(v ...any)          { _exitln(CodeBug, "[BUG] ", v...) }
func BugExitf(f string, v ...any) { _exitf(CodeBug, "[BUG] ", f, v...) }

func UseExitln(v ...any)          { _exitln(CodeUse, "[USE] ", v...) }
func UseExitf(f string, v ...any) { _exitf(CodeUse, "[USE] ", f, v...) }

func EnvExitln(v ...any)          { _exitln(CodeEnv, "[ENV] ", v...) }
func EnvExitf(f string, v ...any) { _exitf(CodeEnv, "[ENV] ", f, v...) }

func _exitln(exitCode int, prefix string, v ...any) {
	fmt.Fprint(os.Stderr, prefix)
	fmt.Fprintln(os.Stderr, v...)
	os.Exit(exitCode)
}
func _exitf(exitCode int, prefix, f string, v ...any) {
	fmt.Fprintf(os.Stderr, prefix+f, v...)
	os.Exit(exitCode)
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
	for i := range len(p) {
		if b := p[i]; b >= 'A' && b <= 'Z' {
			p[i] = b + 0x20 // to lower
		}
	}
}
func bytesToUpper(p []byte) {
	for i := range len(p) {
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
	for i := range len(s) {
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
		return "", errors.New("bad response")
	} else if len(response) < 12 || response[9] != '2' { // HTTP/1.1 200
		return "", errors.New("invalid response")
	} else {
		return string(response[p+4:]), nil
	}
}
