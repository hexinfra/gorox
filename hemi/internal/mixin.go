// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Misc mixins.

package internal

import (
	"os"
	"sync"
	"sync/atomic"
)

// streamHolder
type streamHolder interface {
	MaxStreamsPerConn() int32
}

// streamHolder_ is a mixin.
type streamHolder_ struct {
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (s *streamHolder_) onConfigure(shell Component, defaultMaxStreams int32) {
	// maxStreamsPerConn
	shell.ConfigureInt32("maxStreamsPerConn", &s.maxStreamsPerConn, func(value int32) bool { return value >= 0 }, defaultMaxStreams)
}
func (s *streamHolder_) onPrepare(shell Component) {
}

func (s *streamHolder_) MaxStreamsPerConn() int32 { return s.maxStreamsPerConn }

// contentSaver
type contentSaver interface {
	SaveContentFilesDir() string
}

// contentSaver_ is a mixin.
type contentSaver_ struct {
	// States
	saveContentFilesDir string
}

func (s *contentSaver_) onConfigure(shell Component, defaultDir string) {
	// saveContentFilesDir
	shell.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) bool { return value != "" && len(value) <= 232 }, defaultDir)
}
func (s *contentSaver_) onPrepare(shell Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'

// loadBalancer_
type loadBalancer_ struct {
	// States
	balancer  string       // roundRobin, ipHash, random, ...
	indexGet  func() int64 // ...
	nodeIndex atomic.Int64 // for roundRobin. won't overflow because it is so large!
	numNodes  int64        // num of nodes
}

func (b *loadBalancer_) init() {
	b.nodeIndex.Store(-1)
}

func (b *loadBalancer_) onConfigure(shell Component) {
	// balancer
	shell.ConfigureString("balancer", &b.balancer, func(value string) bool {
		return value == "roundRobin" || value == "ipHash" || value == "random"
	}, "roundRobin")
}
func (b *loadBalancer_) onPrepare(numNodes int) {
	switch b.balancer {
	case "roundRobin":
		b.indexGet = b.getNextByRoundRobin
	case "ipHash":
		b.indexGet = b.getNextByIPHash
	case "random":
		b.indexGet = b.getNextByRandom
	default:
		BugExitln("this should not happen")
	}
	b.numNodes = int64(numNodes)
}

func (b *loadBalancer_) getNext() int64 { return b.indexGet() }

func (b *loadBalancer_) getNextByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *loadBalancer_) getNextByIPHash() int64 {
	// TODO
	return 0
}
func (b *loadBalancer_) getNextByRandom() int64 {
	// TODO
	return 0
}

// subsWaiter_
type subsWaiter_ struct {
	subs sync.WaitGroup
}

func (w *subsWaiter_) IncSub(n int) { w.subs.Add(n) }
func (w *subsWaiter_) WaitSubs()    { w.subs.Wait() }
func (w *subsWaiter_) SubDone()     { w.subs.Done() }

// identifiable
type identifiable interface {
	ID() uint8
	setID(id uint8)
}

// identifiable_ is a mixin.
type identifiable_ struct {
	id uint8
}

func (i *identifiable_) ID() uint8      { return i.id }
func (i *identifiable_) setID(id uint8) { i.id = id }
