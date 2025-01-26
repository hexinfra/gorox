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

const Version = "0.2.2"

// debug level
var _debugLevel atomic.Int32

func DebugLevel() int32         { return _debugLevel.Load() }
func SetDebugLevel(level int32) { _debugLevel.Store(level) }

var ( // topDir
	_topDir  atomic.Value // directory of the executable
	_topOnce sync.Once    // protects _topDir
)

func TopDir() string { return _topDir.Load().(string) }
func SetTopDir(dir string) { // only once!
	_topOnce.Do(func() {
		_topDir.Store(dir)
	})
}

var ( // logDir
	_logDir  atomic.Value // directory of the log files
	_logOnce sync.Once    // protects _logDir
)

func LogDir() string { return _logDir.Load().(string) }
func SetLogDir(dir string) { // only once!
	_logOnce.Do(func() {
		_logDir.Store(dir)
		_mustMkdir(dir)
	})
}

var ( // tmpDir
	_tmpDir  atomic.Value // directory of the temp files
	_tmpOnce sync.Once    // protects _tmpDir
)

func TmpDir() string { return _tmpDir.Load().(string) }
func SetTmpDir(dir string) { // only once!
	_tmpOnce.Do(func() {
		_tmpDir.Store(dir)
		_mustMkdir(dir)
	})
}

var ( // varDir
	_varDir  atomic.Value // directory of the run-time data
	_varOnce sync.Once    // protects _varDir
)

func VarDir() string { return _varDir.Load().(string) }
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

func StageFromText(configText string) (*Stage, error) {
	_checkDirs()
	var c configurator
	return c.stageFromText(configText)
}
func StageFromFile(configBase string, configFile string) (*Stage, error) {
	_checkDirs()
	var c configurator
	return c.stageFromFile(configBase, configFile)
}

func _checkDirs() {
	if _topDir.Load() == nil || _logDir.Load() == nil || _tmpDir.Load() == nil || _varDir.Load() == nil {
		UseExitln("topDir, logDir, tmpDir, and varDir must all be set!")
	}
}
