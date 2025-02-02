// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Loggers log events.

package hemi

import (
	"sync"
)

var (
	loggersLock    sync.RWMutex
	loggerCreators = make(map[string]func(config *LogConfig) Logger) // indexed by loggerSign
)

func RegisterLogger(loggerSign string, create func(config *LogConfig) Logger) {
	loggersLock.Lock()
	defer loggersLock.Unlock()

	if _, ok := loggerCreators[loggerSign]; ok {
		BugExitln("logger conflicts")
	}
	loggerCreators[loggerSign] = create
}
func loggerRegistered(loggerSign string) bool {
	loggersLock.Lock()
	_, ok := loggerCreators[loggerSign]
	loggersLock.Unlock()
	return ok
}
func createLogger(loggerSign string, config *LogConfig) Logger {
	loggersLock.Lock()
	defer loggersLock.Unlock()

	if create := loggerCreators[loggerSign]; create != nil {
		return create(config)
	}
	return nil
}

// LogConfig
type LogConfig struct {
	Target  string   // "/path/to/file.log", "1.2.3.4:5678", ...
	Rotate  string   // "day", "hour", ...
	Fields  []string // ("uri", "status"), ...
	BufSize int32    // size of log buffer
}

// Logger is logger for routers, services, and webapps.
type Logger interface {
	Log(v ...any)
	Logln(v ...any)
	Logf(f string, v ...any)
	Close()
}

func init() {
	RegisterLogger("noop", func(config *LogConfig) Logger {
		return noopLogger{}
	})
}

// noopLogger
type noopLogger struct{}

func (noopLogger) Log(v ...any)            {}
func (noopLogger) Logln(v ...any)          {}
func (noopLogger) Logf(f string, v ...any) {}
func (noopLogger) Close()                  {}
