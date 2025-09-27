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

const Version = "0.2.5"

var (
	_develMode  atomic.Bool
	_debugLevel atomic.Int32
	_topDir     atomic.Value // directory of the executable
	_topOnce    sync.Once    // protects _topDir
	_logDir     atomic.Value // directory of the log files
	_logOnce    sync.Once    // protects _logDir
	_tmpDir     atomic.Value // directory of the temp files
	_tmpOnce    sync.Once    // protects _tmpDir
	_varDir     atomic.Value // directory of the run-time data
	_varOnce    sync.Once    // protects _varDir
)

func DevelMode() bool   { return _develMode.Load() }
func DebugLevel() int32 { return _debugLevel.Load() }
func TopDir() string    { return _topDir.Load().(string) }
func LogDir() string    { return _logDir.Load().(string) }
func TmpDir() string    { return _tmpDir.Load().(string) }
func VarDir() string    { return _varDir.Load().(string) }

func SetDevelMode(devel bool)   { _develMode.Store(devel) }
func SetDebugLevel(level int32) { _debugLevel.Store(level) }
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
