// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

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
	_baseOnce sync.Once    // protects _baseDir
	_baseDir  atomic.Value // directory of the executable
	_logsOnce sync.Once    // protects _logsDir
	_logsDir  atomic.Value // directory of the log files
	_tmpsOnce sync.Once    // protects _tmpsDir
	_tmpsDir  atomic.Value // directory of the temp files
	_varsOnce sync.Once    // protects _varsDir
	_varsDir  atomic.Value // directory of the run-time data

	_debug atomic.Int32 // debug level
)

func SetBaseDir(dir string) { // only once!
	_baseOnce.Do(func() {
		_baseDir.Store(dir)
	})
}
func SetLogsDir(dir string) { // only once!
	_logsOnce.Do(func() {
		_logsDir.Store(dir)
		_mustMkdir(dir)
	})
}
func SetTmpsDir(dir string) { // only once!
	_tmpsOnce.Do(func() {
		_tmpsDir.Store(dir)
		_mustMkdir(dir)
	})
}
func SetVarsDir(dir string) { // only once!
	_varsOnce.Do(func() {
		_varsDir.Store(dir)
		_mustMkdir(dir)
	})
}
func _mustMkdir(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(0)
	}
}

func SetDebug(level int32) { _debug.Store(level) }

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
	if _baseDir.Load() == nil || _logsDir.Load() == nil || _tmpsDir.Load() == nil || _varsDir.Load() == nil {
		UseExitln("baseDir, logsDir, tmpsDir, and varsDir must all be set")
	}
}

func Debug() int32    { return _debug.Load() }
func BaseDir() string { return _baseDir.Load().(string) }
func LogsDir() string { return _logsDir.Load().(string) }
func TmpsDir() string { return _tmpsDir.Load().(string) }
func VarsDir() string { return _varsDir.Load().(string) }

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
