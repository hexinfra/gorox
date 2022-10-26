// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Basic internal facilities.

package internal

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

var ( // global variables shared between stages
	_debug atomic.Bool // enable debug?
	_devel atomic.Bool // developer mode?
)

func IsDebug() bool { return _debug.Load() }
func IsDevel() bool { return _devel.Load() }

func SetDebug(debug bool) {
	_debug.Store(debug)
}
func SetDevel(devel bool) {
	_devel.Store(devel)
}

var ( // global variables shared between stages
	_baseOnce sync.Once    // protects _baseDir
	_baseDir  atomic.Value // directory of the executable
	_dataOnce sync.Once    // protects _dataDir
	_dataDir  atomic.Value // directory of the run-time datum
	_logsOnce sync.Once    // protects _logsDir
	_logsDir  atomic.Value // directory of the log files
	_tempOnce sync.Once    // protects _tempDir
	_tempDir  atomic.Value // directory of the run-time files
)

func BaseDir() string { return _baseDir.Load().(string) }
func DataDir() string { return _dataDir.Load().(string) }
func LogsDir() string { return _logsDir.Load().(string) }
func TempDir() string { return _tempDir.Load().(string) }

func SetBaseDir(dir string) { _baseOnce.Do(func() { _baseDir.Store(dir) }) } // only once
func SetDataDir(dir string) { _dataOnce.Do(func() { _dataDir.Store(dir) }) } // only once
func SetLogsDir(dir string) { _logsOnce.Do(func() { _logsDir.Store(dir) }) } // only once
func SetTempDir(dir string) { _tempOnce.Do(func() { _tempDir.Store(dir) }) } // only once

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
