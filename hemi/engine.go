// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Basic elements that exist between multiple stages.

package hemi

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

const Version = "0.3.0-dev"

var ( // basic variables
	_topOnce sync.Once // protects _topDir
	_logOnce sync.Once // protects _logDir
	_tmpOnce sync.Once // protects _tmpDir
	_varOnce sync.Once // protects _varDir

	_topDir atomic.Value // directory of the executable
	_logDir atomic.Value // directory of the log files
	_tmpDir atomic.Value // directory of the temp files
	_varDir atomic.Value // directory of the run-time data
)

func SetTopDir(dir string) { // only once!
	_topOnce.Do(func() {
		_topDir.Store(dir)
	})
}
func SetLogDir(dir string) { // only once!
	_logOnce.Do(func() {
		_logDir.Store(dir)
		_mustMkdir(dir)
	})
}
func SetTmpDir(dir string) { // only once!
	_tmpOnce.Do(func() {
		_tmpDir.Store(dir)
		_mustMkdir(dir)
	})
}
func SetVarDir(dir string) { // only once!
	_varOnce.Do(func() {
		_varDir.Store(dir)
		_mustMkdir(dir)
	})
}
func _mustMkdir(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(0)
	}
}

func TopDir() string { return _topDir.Load().(string) }
func LogDir() string { return _logDir.Load().(string) }
func TmpDir() string { return _tmpDir.Load().(string) }
func VarDir() string { return _varDir.Load().(string) }

var _debugLevel atomic.Int32 // debug level

func SetDebugLevel(level int32) { _debugLevel.Store(level) }
func DebugLevel() int32         { return _debugLevel.Load() }

func StageFromText(text string) (*Stage, error) {
	_checkDirs()
	var c configurator
	return c.stageFromText(text)
}
func StageFromFile(base string, file string) (*Stage, error) {
	_checkDirs()
	var c configurator
	return c.stageFromFile(base, file)
}
func _checkDirs() {
	if _topDir.Load() == nil || _logDir.Load() == nil || _tmpDir.Load() == nil || _varDir.Load() == nil {
		UseExitln("topDir, logDir, tmpDir, and varDir must all be set")
	}
}
