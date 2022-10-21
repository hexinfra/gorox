// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Simple file abstraction for Linux.

package system

import (
	"errors"
	"io"
	"os"
	"syscall"
	"unsafe"
)

const (
	_AT_FDCWD = -0x64 // for both amd64 & arm64
)

var (
	errBadPath = errors.New("bad path")
	errStat    = errors.New("stat")
)

// File
type File struct {
	file int // the file descriptor. -1 if invalid
}

func Open0(path []byte) (file File, err error) { // path must be NUL terminated
	file.file = -1
	if err = checkPath(path); err != nil {
		return
	}
	// We don't call syscall.Openat(), because it allocates extra memory in syscall.ByteSliceFromString().
	var dirfd int = _AT_FDCWD
	var flags int = syscall.O_RDONLY | syscall.O_LARGEFILE
	r0, _, e1 := syscall.Syscall6(syscall.SYS_OPENAT, uintptr(dirfd), uintptr(unsafe.Pointer(&path[0])), uintptr(flags), 0, 0, 0)
	file.file = int(r0)
	err = errnoErr(e1)
	return
}
func IsNotExist(err error) bool { return err == errENOENT }

func (f File) Stat() (info FileInfo, err error) {
	info.valid = 0
	var stat syscall.Stat_t
	_, _, e1 := syscall.Syscall(syscall.SYS_FSTAT, uintptr(f.file), uintptr(unsafe.Pointer(&stat)), 0)
	if e1 == 0 {
		info.valid = 1
		info.mode = stat.Mode
		info.size = stat.Size
		info.modTime = stat.Mtim.Sec + stat.Mtim.Nsec/1e9
	} else {
		err = errStat
	}
	return
}
func (f File) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		panic("zero p")
	}
	for {
		n, err = ignoringEINTRIO(syscall.Read, f.file, p)
		if err != nil {
			n = 0
		} else if n == 0 {
			err = io.EOF
		}
		return
	}
}
func (f File) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}
	var n0 int
	for {
		n0, err = ignoringEINTRIO(syscall.Write, f.file, p[n:])
		if n0 > 0 {
			n += n0
		}
		if n == len(p) || err != nil {
			return
		}
		if n0 == 0 {
			err = io.ErrUnexpectedEOF
		}
		return
	}
}
func (f File) Close() (err error) { return syscall.Close(f.file) }

func checkPath(path []byte) error {
	if n := len(path); n == 0 || path[n-1] != 0 {
		return errBadPath
	}
	return nil
}

// FileInfo
type FileInfo struct {
	valid   uint32
	mode    uint32
	size    int64
	modTime int64
}

func Stat0(path []byte) (info FileInfo, err error) { // path must be NUL terminated
	info.valid = 0
	if err = checkPath(path); err != nil {
		return
	}
	var dirfd int = _AT_FDCWD
	var stat syscall.Stat_t
	_, _, e1 := syscall.Syscall6(syscall.SYS_NEWFSTATAT, uintptr(dirfd), uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&stat)), 0, 0, 0)
	if e1 == 0 {
		info.valid = 1
		info.mode = stat.Mode
		info.size = stat.Size
		info.modTime = stat.Mtim.Sec + stat.Mtim.Nsec/1e9
	} else {
		err = errStat
	}
	return
}

func (i *FileInfo) AsOS() os.FileInfo { panic("not used") }
func (i *FileInfo) Reset()            { *i = FileInfo{} }
func (i *FileInfo) Valid() bool       { return i.valid != 0 }
func (i *FileInfo) IsDir() bool       { return i.mode&syscall.S_IFMT == syscall.S_IFDIR }
func (i *FileInfo) Size() int64       { return i.size }
func (i *FileInfo) ModTime() int64    { return i.modTime }
