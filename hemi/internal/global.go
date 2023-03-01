// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Global elements exist between multiple stages.

package internal

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

var ( // global variables shared between stages
	_debug    atomic.Int32 // debug level
	_baseOnce sync.Once    // protects _baseDir
	_baseDir  atomic.Value // directory of the executable
	_dataOnce sync.Once    // protects _dataDir
	_dataDir  atomic.Value // directory of the run-time data
	_logsOnce sync.Once    // protects _logsDir
	_logsDir  atomic.Value // directory of the log files
	_tempOnce sync.Once    // protects _tempDir
	_tempDir  atomic.Value // directory of the temp files
)

func IsDebug(level int32) bool { return _debug.Load() >= level }
func BaseDir() string          { return _baseDir.Load().(string) }
func DataDir() string          { return _dataDir.Load().(string) }
func LogsDir() string          { return _logsDir.Load().(string) }
func TempDir() string          { return _tempDir.Load().(string) }

func SetDebug(level int32)  { _debug.Store(level) }
func SetBaseDir(dir string) { _baseOnce.Do(func() { _baseDir.Store(dir) }) }
func SetDataDir(dir string) {
	_mkdir(dir)
	_dataOnce.Do(func() { _dataDir.Store(dir) })
}
func SetLogsDir(dir string) {
	_mkdir(dir)
	_logsOnce.Do(func() { _logsDir.Store(dir) })
}
func SetTempDir(dir string) {
	_mkdir(dir)
	_tempOnce.Do(func() { _tempDir.Store(dir) })
}
func _mkdir(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf(err.Error())
		os.Exit(0)
	}
}

func Debug(args ...any)                 { fmt.Print(args...) }
func Debugln(args ...any)               { fmt.Println(args...) }
func Debugf(format string, args ...any) { fmt.Printf(format, args...) }

const ( // exit codes. keep sync with ../hemi.go
	CodeBug = 20
	CodeUse = 21
	CodeEnv = 22
)

func BugExitln(args ...any) { exitln(CodeBug, "[BUG] ", args...) }
func UseExitln(args ...any) { exitln(CodeUse, "[USE] ", args...) }
func EnvExitln(args ...any) { exitln(CodeEnv, "[ENV] ", args...) }

func BugExitf(format string, args ...any) { exitf(CodeBug, "[BUG] ", format, args...) }
func UseExitf(format string, args ...any) { exitf(CodeUse, "[USE] ", format, args...) }
func EnvExitf(format string, args ...any) { exitf(CodeEnv, "[ENV] ", format, args...) }

func exitln(exitCode int, prefix string, args ...any) {
	fmt.Fprint(os.Stderr, prefix)
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCode)
}
func exitf(exitCode int, prefix, format string, args ...any) {
	fmt.Fprintf(os.Stderr, prefix+format, args...)
	os.Exit(exitCode)
}
