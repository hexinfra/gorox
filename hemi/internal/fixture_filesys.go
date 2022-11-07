// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The file system cache. Caches file descriptors and contents.

package internal

import (
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"os"
	"sync"
	"time"
)

func init() {
	registerFixture(signFilesys)
}

const signFilesys = "filesys"

func createFilesys(stage *Stage) *filesysFixture {
	filesys := new(filesysFixture)
	filesys.init(stage)
	filesys.setShell(filesys)
	return filesys
}

// filesysFixture
type filesysFixture struct {
	// Mixins
	fixture_
	// States
	smallFileSize int64 // what size is considered as small file
	maxSmallFiles int32 // max number of small files
	maxLargeFiles int32 // max number of large files
	cacheDuration time.Duration
	rwMutex       sync.RWMutex
	entries       map[string]*filesysEntry
}

func (f *filesysFixture) init(stage *Stage) {
	f.fixture_.init(signFilesys, stage)
}

func (f *filesysFixture) OnConfigure() {
	// smallFileSize
	f.ConfigureInt64("smallFileSize", &f.smallFileSize, func(value int64) bool { return value > 0 }, _4K)
	// maxSmallFiles
	f.ConfigureInt32("maxSmallFiles", &f.maxSmallFiles, func(value int32) bool { return value > 0 }, 1000)
	// maxLargeFiles
	f.ConfigureInt32("maxLargeFiles", &f.maxLargeFiles, func(value int32) bool { return value > 0 }, 500)
	// cacheDuration
	f.ConfigureDuration("cacheDuration", &f.cacheDuration, func(value time.Duration) bool { return value > 0 }, 10*time.Second)
}
func (f *filesysFixture) OnPrepare() {
}
func (f *filesysFixture) OnShutdown() {
}

func (f *filesysFixture) run() { // goroutine
	for {
		time.Sleep(time.Second)
	}
}

func (f *filesysFixture) getFile(path string) (entry *filesysEntry) {
	var ok bool

	// Fast path
	f.rwMutex.RLock()
	if entry, ok = f.entries[path]; ok {
		f.rwMutex.RUnlock()
		return
	}
	f.rwMutex.RUnlock()

	// Slow path
	f.rwMutex.Lock()
	defer f.rwMutex.Unlock()

	if entry, ok = f.entries[path]; ok {
		return
	}

	entry = new(filesysEntry)
	var (
		file *os.File
		info os.FileInfo
		err  error
	)
	file, err = os.Open(path)
	if err == nil {
		info, err = file.Stat()
		if err != nil {
			file.Close()
		}
	}
	if err == nil {
		entry.code = 0
		if info.IsDir() {
			entry.kind = 2
		} else if info.Size() <= f.smallFileSize {
			entry.kind = 0
		} else {
			entry.kind = 1
		}
		entry.file = file
		entry.info = info
		// TODO: load small file
	} else {
		if system.IsNotExist(err) {
			entry.code = 2
		} else {
			entry.code = 1
		}
	}
	f.entries[path] = entry

	return entry
}
func (f *filesysFixture) putFile(entry *filesysEntry) {
}

// filesysEntry
type filesysEntry struct {
	code int8 // 0:valid 1:error 2:not-exist
	kind int8 // 0:small 1:large 2:dir
	file *os.File
	info os.FileInfo
	data []byte // content of small file
}

func (e *filesysEntry) isValid() bool    { return e.code == 0 }
func (e *filesysEntry) isNotExist() bool { return e.code == 2 }
