// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Simple file abstraction for systems other than Linux.

//go:build !linux

package system

import (
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"os"
)

// File
type File struct {
	*os.File
}

func Open0(path []byte) (file File, err error) { // path must be NUL terminated
	path = path[0 : len(path)-1] // excluding NUL
	file.File, err = os.Open(risky.WeakString(path))
	return
}
func IsNotExist(err error) bool { return os.IsNotExist(err) }

func (f File) Stat() (info FileInfo, err error) {
	stat, err := f.File.Stat()
	if err == nil {
		info.FileInfo = stat
	}
	return
}

// FileInfo
type FileInfo struct {
	os.FileInfo
}

func Stat0(path []byte) (info FileInfo, err error) { // path must be NUL terminated
	path = path[0 : len(path)-1] // excluding NUL
	stat, err := os.Stat(risky.WeakString(path))
	if err == nil {
		info.FileInfo = stat
	}
	return
}

func (i *FileInfo) AsOS() os.FileInfo { return i.FileInfo }
func (i *FileInfo) Reset()            { *i = FileInfo{} }
func (i *FileInfo) Valid() bool       { return i.FileInfo != nil }
func (i *FileInfo) ModTime() int64    { return i.FileInfo.ModTime().Unix() }
