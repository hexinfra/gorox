// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Mixins that are not components.

package internal

import (
	"os"
	"sync"
	"sync/atomic"
)

// waiter_
type waiter_ struct {
	subs sync.WaitGroup
}

func (w *waiter_) IncSub(n int) { w.subs.Add(n) }
func (w *waiter_) WaitSubs()    { w.subs.Wait() }
func (w *waiter_) SubDone()     { w.subs.Done() }

// streamHolder
type streamHolder interface {
	MaxStreamsPerConn() int32
}

// streamHolder_ is a mixin.
type streamHolder_ struct {
	// States
	maxStreamsPerConn int32 // max streams of one conn
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

func (s *contentSaver_) makeContentFilesDir(perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}
func (s *contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir }

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

func (b *loadBalancer_) onConfigure(c Component) {
	// balancer
	c.ConfigureString("balancer", &b.balancer, func(value string) bool {
		return value == "roundRobin" || value == "ipHash" || value == "random"
	}, "roundRobin")
}
func (b *loadBalancer_) onPrepare(numNodes int) {
	switch b.balancer {
	case "roundRobin":
		b.indexGet = b.getIndexByRoundRobin
	case "ipHash":
		b.indexGet = b.getIndexByIPHash
	case "random":
		b.indexGet = b.getIndexByRandom
	default:
		BugExitln("this should not happen")
	}
	b.numNodes = int64(numNodes)
}

func (b *loadBalancer_) getIndex() int64 {
	return b.indexGet()
}

func (b *loadBalancer_) getIndexByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *loadBalancer_) getIndexByIPHash() int64 {
	// TODO
	return 0
}
func (b *loadBalancer_) getIndexByRandom() int64 {
	// TODO
	return 0
}

// ider
type ider interface {
	ID() uint8
	setID(id uint8)
}

// ider_ is a mixin.
type ider_ struct {
	id uint8
}

func (i *ider_) ID() uint8      { return i.id }
func (i *ider_) setID(id uint8) { i.id = id }
