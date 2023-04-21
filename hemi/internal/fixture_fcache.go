// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The file system cache. Caches file descriptors and contents.

package internal

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
)

func init() {
	registerFixture(signFcache)
}

const signFcache = "fcache"

func createFcache(stage *Stage) *fcacheFixture {
	fcache := new(fcacheFixture)
	fcache.onCreate(stage)
	fcache.setShell(fcache)
	return fcache
}

// fcacheFixture
type fcacheFixture struct {
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
	entries       map[string]*fcacheEntry
}

func (f *fcacheFixture) onCreate(stage *Stage) {
	f.MakeComp(signFcache)
	f.stage = stage
	f.entries = make(map[string]*fcacheEntry)
}
func (f *fcacheFixture) OnShutdown() {
	close(f.Shut)
}

func (f *fcacheFixture) OnConfigure() {
	// smallFileSize
	f.ConfigureInt64("smallFileSize", &f.smallFileSize, func(value int64) bool { return value > 0 }, _64K1)
	// maxSmallFiles
	f.ConfigureInt32("maxSmallFiles", &f.maxSmallFiles, func(value int32) bool { return value > 0 }, 1000)
	// maxLargeFiles
	f.ConfigureInt32("maxLargeFiles", &f.maxLargeFiles, func(value int32) bool { return value > 0 }, 500)
	// cacheTimeout
	f.ConfigureDuration("cacheTimeout", &f.cacheTimeout, func(value time.Duration) bool { return value > 0 }, 1*time.Second)
}
func (f *fcacheFixture) OnPrepare() {
}

func (f *fcacheFixture) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
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
				Debugf("fcache entry deleted: %s\n", path)
			}
		}
		f.rwMutex.Unlock()
	})
	f.rwMutex.Lock()
	f.entries = nil
	f.rwMutex.Unlock()

	if IsDebug(2) {
		Debugln("fcache done")
	}
	f.stage.SubDone()
}

func (f *fcacheFixture) getEntry(path []byte) (*fcacheEntry, error) {
	f.rwMutex.RLock()
	defer f.rwMutex.RUnlock()

	if entry, ok := f.entries[risky.WeakString(path)]; ok {
		if entry.isLarge() {
			entry.addRef()
		}
		return entry, nil
	} else {
		return nil, fcacheNotExist
	}
}

var fcacheNotExist = errors.New("entry not exist")

func (f *fcacheFixture) newEntry(path string) (*fcacheEntry, error) {
	f.rwMutex.RLock()
	if entry, ok := f.entries[path]; ok {
		if entry.isLarge() {
			entry.addRef()
		}

		f.rwMutex.RUnlock()
		return entry, nil
	}
	f.rwMutex.RUnlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	entry := new(fcacheEntry)
	if info.IsDir() {
		entry.kind = fcacheKindDir
		file.Close()
	} else if fileSize := info.Size(); fileSize <= f.smallFileSize {
		text := make([]byte, fileSize)
		if _, err := io.ReadFull(file, text); err != nil {
			file.Close()
			return nil, err
		}
		entry.kind = fcacheKindSmall
		entry.info = info
		entry.text = text
		file.Close()
	} else { // large file
		entry.kind = fcacheKindLarge
		entry.file = file
		entry.info = info
		entry.nRef.Store(1) // current caller
	}
	entry.last = time.Now().Add(f.cacheTimeout)

	f.rwMutex.Lock()
	f.entries[path] = entry
	f.rwMutex.Unlock()

	return entry, nil
}

// fcacheEntry
type fcacheEntry struct {
	kind int8         // see fcacheKindXXX
	file *os.File     // only for large file
	info os.FileInfo  // only for files, not directories
	text []byte       // content of small file
	last time.Time    // expire time
	nRef atomic.Int64 // only for large file
}

const (
	fcacheKindDir = iota
	fcacheKindSmall
	fcacheKindLarge
)

func (e *fcacheEntry) isDir() bool   { return e.kind == fcacheKindDir }
func (e *fcacheEntry) isLarge() bool { return e.kind == fcacheKindLarge }
func (e *fcacheEntry) isSmall() bool { return e.kind == fcacheKindSmall }

func (e *fcacheEntry) addRef() {
	e.nRef.Add(1)
}
func (e *fcacheEntry) decRef() {
	if e.nRef.Add(-1) < 0 {
		if IsDebug(2) {
			Debugf("fcache large entry closed: %s\n", e.file.Name())
		}
		e.file.Close()
	}
}
