// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Basic elements exist between multiple stages.

package hemi

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const Version = "0.2.0-dev"

var ( // basic variables
	_topOnce sync.Once    // protects _topDir
	_topDir  atomic.Value // directory of the executable

	_logOnce sync.Once    // protects _logDir
	_logDir  atomic.Value // directory of the log files

	_tmpOnce sync.Once    // protects _tmpDir
	_tmpDir  atomic.Value // directory of the temp files

	_varOnce sync.Once    // protects _varDir
	_varDir  atomic.Value // directory of the run-time data

	_debugLevel atomic.Int32 // debug level
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

func SetDebugLevel(level int32) { _debugLevel.Store(level) }

func NewStageText(text string) (*Stage, error) {
	_checkDirs()
	var c config
	return c.newStageText(text)
}
func NewStageFile(base string, file string) (*Stage, error) {
	_checkDirs()
	var c config
	return c.newStageFile(base, file)
}
func _checkDirs() {
	if _topDir.Load() == nil || _logDir.Load() == nil || _tmpDir.Load() == nil || _varDir.Load() == nil {
		UseExitln("topDir, logDir, tmpDir, and varDir must all be set")
	}
}

func DebugLevel() int32 { return _debugLevel.Load() }
func TopDir() string    { return _topDir.Load().(string) }
func LogDir() string    { return _logDir.Load().(string) }
func TmpDir() string    { return _tmpDir.Load().(string) }
func VarDir() string    { return _varDir.Load().(string) }

func Print(args ...any) {
	_printTime(os.Stdout)
	fmt.Fprint(os.Stdout, args...)
}
func Println(args ...any) {
	_printTime(os.Stdout)
	fmt.Fprintln(os.Stdout, args...)
}
func Printf(format string, args ...any) {
	_printTime(os.Stdout)
	fmt.Fprintf(os.Stdout, format, args...)
}
func Error(args ...any) {
	_printTime(os.Stderr)
	fmt.Fprint(os.Stderr, args...)
}
func Errorln(args ...any) {
	_printTime(os.Stderr)
	fmt.Fprintln(os.Stderr, args...)
}
func Errorf(format string, args ...any) {
	_printTime(os.Stderr)
	fmt.Fprintf(os.Stdout, format, args...)
}
func _printTime(file *os.File) {
	fmt.Fprintf(file, "[%s] ", time.Now().Format("2006-01-02 15:04:05 MST"))
}

const ( // exit codes
	CodeBug = 20
	CodeUse = 21
	CodeEnv = 22
)

func BugExitln(args ...any) { _exitln(CodeBug, "[BUG] ", args...) }
func UseExitln(args ...any) { _exitln(CodeUse, "[USE] ", args...) }
func EnvExitln(args ...any) { _exitln(CodeEnv, "[ENV] ", args...) }

func BugExitf(format string, args ...any) { _exitf(CodeBug, "[BUG] ", format, args...) }
func UseExitf(format string, args ...any) { _exitf(CodeUse, "[USE] ", format, args...) }
func EnvExitf(format string, args ...any) { _exitf(CodeEnv, "[ENV] ", format, args...) }

func _exitln(exitCode int, prefix string, args ...any) {
	fmt.Fprint(os.Stderr, prefix)
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCode)
}
func _exitf(exitCode int, prefix, format string, args ...any) {
	fmt.Fprintf(os.Stderr, prefix+format, args...)
	os.Exit(exitCode)
}
