// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The file system cache. Caches file descriptors and contents.

package internal

import (
	"errors"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"io"
	"os"
	"runtime"
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
	maxSmallFiles int32 // max number of small files. for small files, contents are cached
	maxLargeFiles int32 // max number of large files. for large files, *os.File are cached
	cacheDuration time.Duration
	rwMutex       sync.RWMutex // protects entries below
	entries       map[string]*filesysEntry
}

func (f *filesysFixture) init(stage *Stage) {
	f.fixture_.init(signFilesys, stage)
	f.entries = make(map[string]*filesysEntry)
}

func (f *filesysFixture) OnConfigure() {
	// smallFileSize
	f.ConfigureInt64("smallFileSize", &f.smallFileSize, func(value int64) bool { return value > 0 }, _64K1)
	// maxSmallFiles
	f.ConfigureInt32("maxSmallFiles", &f.maxSmallFiles, func(value int32) bool { return value > 0 }, 1000)
	// maxLargeFiles
	f.ConfigureInt32("maxLargeFiles", &f.maxLargeFiles, func(value int32) bool { return value > 0 }, 200)
	// cacheDuration
	var defaultDuration = 10 * time.Second
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" { // these operating systems are mainly used for developing
		defaultDuration = time.Second
	}
	f.ConfigureDuration("cacheDuration", &f.cacheDuration, func(value time.Duration) bool { return value > 0 }, defaultDuration)
}
func (f *filesysFixture) OnPrepare() {
}
func (f *filesysFixture) OnShutdown() {
	// TODO
}

func (f *filesysFixture) run() { // goroutine
	for {
		time.Sleep(time.Second)
		now := time.Now()
		f.rwMutex.Lock()
		for path, entry := range f.entries {
			if entry.last.After(now) {
				continue
			}
			if Debug(2) {
				fmt.Printf("filesys entry deleted: %s\n", path)
			}
			delete(f.entries, path) // files we be closed automatically thanks to finalizers
		}
		f.rwMutex.Unlock()
	}
}

func (f *filesysFixture) getEntry(path []byte) (*filesysEntry, error) {
	f.rwMutex.RLock()
	defer f.rwMutex.RUnlock()

	if entry, ok := f.entries[risky.WeakString(path)]; ok {
		return entry, nil
	} else {
		return nil, filesysNotExist
	}
}

var filesysNotExist = errors.New("entry not exist")

func (f *filesysFixture) newEntry(path string) (*filesysEntry, error) {
	f.rwMutex.Lock()
	defer f.rwMutex.Unlock()

	if entry, ok := f.entries[path]; ok {
		return entry, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	entry := new(filesysEntry)
	if info.IsDir() {
		entry.kind = 2
		file.Close()
	} else if fileSize := info.Size(); fileSize <= f.smallFileSize {
		data := make([]byte, fileSize)
		if _, err := io.ReadFull(file, data); err != nil {
			file.Close()
			return nil, err
		}
		entry.kind = 0
		entry.info = info
		entry.data = data
		file.Close()
	} else { // large file
		entry.kind = 1
		entry.file = file
		entry.info = info
	}
	entry.last = time.Now().Add(f.cacheDuration)
	f.entries[path] = entry

	return entry, nil
}

// filesysEntry
type filesysEntry struct {
	kind int8        // 0:small file, 1:large file 2:directory
	file *os.File    // only for large file
	info os.FileInfo // only for files, not directories
	data []byte      // content of small file
	last time.Time   // expire time
}

func (e *filesysEntry) isDir() bool   { return e.kind == 2 }
func (e *filesysEntry) isSmall() bool { return e.kind == 0 }
