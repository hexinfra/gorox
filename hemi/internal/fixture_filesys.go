// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The file system cache. Caches file descriptors and contents.

package internal

import (
	"errors"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	registerFixture(signFilesys)
}

const signFilesys = "filesys"

func createFilesys(stage *Stage) *filesysFixture {
	filesys := new(filesysFixture)
	filesys.onCreate(stage)
	filesys.setShell(filesys)
	return filesys
}

// filesysFixture
type filesysFixture struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	smallFileSize int64 // what size is considered as small file
	maxSmallFiles int32 // max number of small files. for small files, contents are cached
	maxLargeFiles int32 // max number of large files. for large files, *os.File are cached
	cacheTimeout  time.Duration
	rwMutex       sync.RWMutex // protects entries below
	entries       map[string]*filesysEntry
}

func (f *filesysFixture) onCreate(stage *Stage) {
	f.CompInit(signFilesys)
	f.stage = stage
	f.entries = make(map[string]*filesysEntry)
}
func (f *filesysFixture) OnShutdown() {
	f.Shutdown()
}

func (f *filesysFixture) OnConfigure() {
	// smallFileSize
	f.ConfigureInt64("smallFileSize", &f.smallFileSize, func(value int64) bool { return value > 0 }, _64K1)
	// maxSmallFiles
	f.ConfigureInt32("maxSmallFiles", &f.maxSmallFiles, func(value int32) bool { return value > 0 }, 1000)
	// maxLargeFiles
	f.ConfigureInt32("maxLargeFiles", &f.maxLargeFiles, func(value int32) bool { return value > 0 }, 500)
	// cacheTimeout
	f.ConfigureDuration("cacheTimeout", &f.cacheTimeout, func(value time.Duration) bool { return value > 0 }, 1*time.Second)
}
func (f *filesysFixture) OnPrepare() {
}

func (f *filesysFixture) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		f.rwMutex.Lock()
		for path, entry := range f.entries {
			if entry.last.After(now) {
				continue
			}
			if entry.isLarge() {
				entry.decRef()
			}
			delete(f.entries, path)
			if IsDebug(2) {
				Debugf("filesys entry deleted: %s\n", path)
			}
		}
		f.rwMutex.Unlock()
	})
	f.rwMutex.Lock()
	f.entries = nil
	f.rwMutex.Unlock()

	if IsDebug(2) {
		Debugln("filesys done")
	}
	f.stage.SubDone()
}

func (f *filesysFixture) getEntry(path []byte) (*filesysEntry, error) {
	f.rwMutex.RLock()
	defer f.rwMutex.RUnlock()

	if entry, ok := f.entries[risky.WeakString(path)]; ok {
		if entry.isLarge() {
			entry.addRef()
		}
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
		if entry.isLarge() {
			entry.addRef()
		}
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
		entry.kind = filesysKindDir
		file.Close()
	} else if fileSize := info.Size(); fileSize <= f.smallFileSize {
		data := make([]byte, fileSize)
		if _, err := io.ReadFull(file, data); err != nil {
			file.Close()
			return nil, err
		}
		entry.kind = filesysKindSmall
		entry.info = info
		entry.data = data
		file.Close()
	} else { // large file
		entry.kind = filesysKindLarge
		entry.file = file
		entry.info = info
		entry.nRef.Store(1) // current caller
	}
	entry.last = time.Now().Add(f.cacheTimeout)
	f.entries[path] = entry

	return entry, nil
}

// filesysEntry
type filesysEntry struct {
	kind int8         // see filesysKindXXX
	file *os.File     // only for large file
	info os.FileInfo  // only for files, not directories
	data []byte       // content of small file
	last time.Time    // expire time
	nRef atomic.Int64 // only for large file
}

const (
	filesysKindDir = iota
	filesysKindSmall
	filesysKindLarge
)

func (e *filesysEntry) isDir() bool   { return e.kind == filesysKindDir }
func (e *filesysEntry) isLarge() bool { return e.kind == filesysKindLarge }
func (e *filesysEntry) isSmall() bool { return e.kind == filesysKindSmall }

func (e *filesysEntry) addRef() {
	e.nRef.Add(1)
}
func (e *filesysEntry) decRef() {
	if e.nRef.Add(-1) < 0 {
		if IsDebug(2) {
			Debugf("filesys large entry closed: %s\n", e.file.Name())
		}
		e.file.Close()
	}
}
