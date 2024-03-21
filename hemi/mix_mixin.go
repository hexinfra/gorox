// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Mixins for net, rpc, and web.

package hemi

import (
	"errors"
	"os"
	"sync"
	"time"
)

// streamHolder
type streamHolder interface {
	MaxStreamsPerConn() int32
}

// _streamHolder_ is a mixin.
type _streamHolder_ struct {
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (s *_streamHolder_) onConfigure(shell Component, defaultMaxStreams int32) {
	// maxStreamsPerConn
	shell.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, defaultMaxStreams)
}
func (s *_streamHolder_) onPrepare(shell Component) {
}

func (s *_streamHolder_) MaxStreamsPerConn() int32 { return s.maxStreamsPerConn }

// contentSaver
type contentSaver interface {
	SaveContentFilesDir() string
}

// _contentSaver_ is a mixin.
type _contentSaver_ struct {
	// States
	saveContentFilesDir string
}

func (s *_contentSaver_) onConfigure(shell Component, defaultDir string) {
	// saveContentFilesDir
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) error {
		if value != "" && len(value) <= 232 {
			return nil
		}
		return errors.New(".saveContentFilesDir has an invalid value")
	}, defaultDir)
}
func (s *_contentSaver_) onPrepare(shell Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *_contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// _subsWaiter_ is a mixin.
type _subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *_subsWaiter_) IncSub()        { w.subs.Add(1) }
func (w *_subsWaiter_) SubsAddn(n int) { w.subs.Add(n) }
func (w *_subsWaiter_) WaitSubs()      { w.subs.Wait() }
func (w *_subsWaiter_) DecSub()        { w.subs.Done() }

// _shutdownable_ is a mixin.
type _shutdownable_ struct {
	ShutChan chan struct{} // used to notify target to shutdown
}

func (s *_shutdownable_) init() {
	s.ShutChan = make(chan struct{})
}

func (s *_shutdownable_) Loop(interval time.Duration, callback func(now time.Time)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ShutChan:
			return
		case now := <-ticker.C:
			callback(now)
		}
	}
}

// identifiable
type identifiable interface {
	ID() uint8
	setID(id uint8)
}

// _identifiable_ is a mixin.
type _identifiable_ struct {
	id uint8
}

func (i *_identifiable_) ID() uint8 { return i.id }

func (i *_identifiable_) setID(id uint8) { i.id = id }

/*
// _connPool_
type _connPool_[T any] struct {
	freeList struct{
		sync.Mutex
		head T
		tail T
		qnty int
	}
}

func (p *_connPool_[T]) pullConn() T {
	list := &p.freeList

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.next
	conn.setNext(nil)
	list.qnty--

	return conn
}
*/
