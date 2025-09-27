// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// A simple logger.

package simple

import (
	"fmt"
	"os"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterLogger("simple", func(logConfig *LogConfig) Logger {
		logFile, err := os.OpenFile(logConfig.Target, os.O_WRONLY|os.O_CREATE, 0700)
		if err != nil {
			return nil
		}
		l := new(simpleLogger)
		l.config = logConfig
		l.file = logFile
		l.queue = make(chan string)
		l.buffer = make([]byte, logConfig.BufSize)
		l.size = len(l.buffer)
		l.used = 0
		go l.saver()
		return l
	})
}

// simpleLogger implements Logger.
type simpleLogger struct {
	config *LogConfig
	file   *os.File
	queue  chan string
	buffer []byte
	size   int
	used   int
}

func (l *simpleLogger) Logf(f string, v ...any) {
	if s := fmt.Sprintf(f, v...); s != "" {
		l.queue <- s
	}
}
func (l *simpleLogger) Close() { l.queue <- "" }

func (l *simpleLogger) saver() { // runner
	for {
		s := <-l.queue
		if s == "" {
			goto over
		}
		l.write(s)
	more:
		for {
			select {
			case s = <-l.queue:
				if s == "" {
					goto over
				}
				l.write(s)
			default:
				l.clear()
				break more
			}
		}
	}
over:
	l.clear()
	l.file.Close()
}
func (l *simpleLogger) write(s string) {
	n := len(s)
	if n >= l.size {
		l.clear()
		l.flush(ConstBytes(s))
		return
	}
	w := copy(l.buffer[l.used:], s)
	l.used += w
	if l.used == l.size {
		l.clear()
		if n -= w; n > 0 {
			copy(l.buffer, s[w:])
			l.used = n
		}
	}
}
func (l *simpleLogger) clear() {
	if l.used > 0 {
		l.flush(l.buffer[:l.used])
		l.used = 0
	}
}
func (l *simpleLogger) flush(logs []byte) {
	l.file.Write(logs)
}
