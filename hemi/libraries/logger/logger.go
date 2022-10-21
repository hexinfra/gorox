// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Logger.

package logger

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"os"
	"sync"
	"time"
)

// Logger
type Logger struct {
	filePath string   // file prefix indeed
	divideBy string   // "day", "hour"
	cantOpen bool     // failed to open/create the file?
	absPath  string   // absolute file path (with suffix)
	osFile   *os.File // opened file cache for absPath

	mutex    sync.Mutex // protect following queues
	queueOne *logQueue
	queueTwo *logQueue
	qCurrent *logQueue
	final    *logQueue // the final queue on close

	nowRWMutex sync.RWMutex
	now        []byte
}

const loggerTimeFormat = "[2006-01-02 15:04:05.000] "

func New(filePath string, divideBy string) *Logger {
	l := new(Logger)
	l.filePath = filePath
	l.divideBy = divideBy
	l.queueOne = newLogQueue(8)
	l.queueTwo = newLogQueue(8)
	l.qCurrent = l.queueOne
	l.now = make([]byte, len(loggerTimeFormat))
	l.setTime()
	go l.timer()
	go l.saver()
	return l
}

func (l *Logger) setTime() {
	t := time.Now()
	l.nowRWMutex.Lock()
	t.AppendFormat(l.now[:0], loggerTimeFormat)
	l.nowRWMutex.Unlock()
}
func (l *Logger) timer() {
	for {
		time.Sleep(47 * time.Millisecond)
		l.setTime()
	}
}

func (l *Logger) log(s string) {
	l.mutex.Lock()
	if l.qCurrent != nil {
		l.nowRWMutex.RLock()
		l.qCurrent.log(risky.WeakString(l.now))
		l.nowRWMutex.RUnlock()
		l.qCurrent.log(s)
	}
	l.mutex.Unlock()
}
func (l *Logger) logln(s string) {
	l.mutex.Lock()
	if l.qCurrent != nil {
		l.nowRWMutex.RLock()
		l.qCurrent.log(risky.WeakString(l.now))
		l.nowRWMutex.RUnlock()
		l.qCurrent.log(s)
		l.qCurrent.log("\n")
	}
	l.mutex.Unlock()
}
func (l *Logger) logf(format string, args ...any) {
	l.log(fmt.Sprintf(format, args...))
}

func (l *Logger) saver() {
	var dirty *logQueue
	closed := false
	for {
		time.Sleep(97 * time.Millisecond)

		// Switch current queue between queue one and queue two
		l.mutex.Lock()
		if l.qCurrent == l.queueOne {
			l.qCurrent = l.queueTwo
			dirty = l.queueOne
		} else if l.qCurrent == l.queueTwo {
			l.qCurrent = l.queueOne
			dirty = l.queueTwo
		} else {
			// Cannot switch as logger is closed.
			closed = true
		}
		l.mutex.Unlock()

		if closed {
			l.save(l.final, true)
			return
		}
		if !dirty.isEmpty {
			l.save(dirty, false)
		}
	}
}

func (l *Logger) save(queue *logQueue, forceClose bool) {
	if l.cantOpen {
		return
	}

	absPath := l.filePath
	// TODO(diogin): Eliminate memory allocations?
	if l.divideBy == "day" {
		absPath += "." + time.Now().Format("2006-01-02")
	} else if l.divideBy == "hour" {
		absPath += "." + time.Now().Format("2006-01-02.15")
	}

	needOpen := false
	if l.absPath == "" {
		needOpen = true
	} else if l.absPath != absPath {
		l.osFile.Close()
		needOpen = true
	}

	if needOpen {
		file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			l.cantOpen = true
			return
		}
		l.absPath = absPath
		l.osFile = file
	}

	queue.saveTo(l.osFile)

	if forceClose {
		l.osFile.Close()
		l.absPath = ""
	}
}

func (l *Logger) close() {
	l.mutex.Lock()
	if l.qCurrent != nil {
		l.final = l.qCurrent
		l.qCurrent = nil
	}
	l.mutex.Unlock()
}

// logQueue
type logQueue struct {
	nBlocks int
	isEmpty bool
	head    *logBlock
	tail    *logBlock
	free    *logBlock
}

func newLogQueue(nBlocks int) *logQueue {
	if nBlocks < 1 {
		nBlocks = 1
	}
	q := new(logQueue)
	q.nBlocks = nBlocks
	q.isEmpty = true
	q.head = newLogBlock()
	block := q.head
	for i := 1; i < nBlocks; i++ {
		next := newLogBlock()
		block.next = next
		block = next
	}
	q.tail = block
	q.free = q.head
	return q
}

func (q *logQueue) log(s string) {
	q.isEmpty = false
	for logged := 0; logged != len(s); {
		left := s[logged:]
		if q.free == nil {
			q.expandBlocks(left)
			if q.free == nil {
				// No free space, drop left
				return
			}
		}
		logged += q.free.write(left)
		if q.free.isFull() {
			q.free = q.free.next
		}
	}
}

func (q *logQueue) expandBlocks(s string) {
	const maxBlockCountPerQueue = 4096 // max 4096 * 16KiB = 64MiB per queue
	if q.nBlocks == maxBlockCountPerQueue {
		// Queue is full, do nothing
		return
	}
	blockSize := q.head.size()
	if (maxBlockCountPerQueue-q.nBlocks)*blockSize < len(s) {
		// Even max free space cannot place s, do nothing
		return
	}
	// Determine how many logBlock to expand
	const threshold = maxBlockCountPerQueue / 8
	var nMore int
	if q.nBlocks > threshold {
		nMore = threshold
	} else {
		nMore = q.nBlocks
	}
	if q.nBlocks+nMore > maxBlockCountPerQueue {
		nMore = maxBlockCountPerQueue - q.nBlocks
	}
	// Now expand nMore logBlock
	head := newLogBlock()
	q.tail.next = head
	q.free = head
	block := head
	for i := 1; i < nMore; i++ {
		next := newLogBlock()
		block.next = next
		block = next
	}
	q.tail = block
	q.nBlocks += nMore
}

func (q *logQueue) saveTo(file *os.File) {
	saved := 0
	defer func() {
		if saved > 0 {
			file.Sync()
			q.isEmpty = true
		}
	}()
	for block := q.head; block != nil; block = block.next {
		if !block.isFree() {
			logs := block.take()
			file.Write(logs)
			saved++
			continue
		}
		q.free = q.head
		// Need shrink?
		if q.nBlocks > 8 && saved < q.nBlocks/4 {
			nBlocks := saved * 3
			if nBlocks < 8 {
				nBlocks = 8
			}
			from := block
			step := nBlocks - saved
			for i := 1; i < step; i++ {
				from = from.next
			}
			from.next = nil
			q.tail = from
			q.nBlocks = nBlocks
		}
		return
	}
}

// logBlock
type logBlock struct {
	next *logBlock
	used int
	logs [16368]byte
}

func newLogBlock() *logBlock {
	b := new(logBlock) // 16KiB
	b.next = nil
	b.used = 0
	return b
}

func (b *logBlock) write(s string) int {
	n := copy(b.logs[b.used:], s)
	b.used += n
	return n
}

func (b *logBlock) size() int {
	return len(b.logs)
}
func (b *logBlock) isFull() bool {
	return b.used == len(b.logs)
}
func (b *logBlock) isFree() bool {
	return b.used == 0
}

func (b *logBlock) take() []byte {
	logs := b.logs[:b.used]
	b.used = 0
	return logs
}
