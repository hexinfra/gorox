// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General backend for net, rpc, and web.

package hemi

import (
	"errors"
	"math/rand"
	"os"
	"sync/atomic"
	"time"
)

// Backend component. A Backend is a group of nodes.
type Backend interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	CreateNode(compName string) Node
}

// Backend_ is the parent for backends.
type Backend_[N Node] struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	nodes []N    // nodes of this backend
	// States
	balancer     string       // roundRobin, ipHash, random, ...
	numNodes     int64        // num of nodes
	nodeIndexGet func() int64 // ...
	nodeIndex    atomic.Int64 // for roundRobin. won't overflow because it is so large!
	healthCheck  any          // TODO
}

func (b *Backend_[N]) OnCreate(compName string, stage *Stage) {
	b.MakeComp(compName)
	b.stage = stage
	b.nodeIndex.Store(-1)
	b.healthCheck = nil // TODO
}
func (b *Backend_[N]) OnShutdown() { close(b.ShutChan) } // notifies Maintain() which also shutdown nodes

func (b *Backend_[N]) OnConfigure() {
	// .balancer
	b.ConfigureString("balancer", &b.balancer, func(value string) error {
		if value == "roundRobin" || value == "ipHash" || value == "random" || value == "leastUsed" {
			return nil
		}
		return errors.New(".balancer has an invalid value")
	}, "roundRobin")
}
func (b *Backend_[N]) OnPrepare() {
	switch b.balancer {
	case "roundRobin":
		b.nodeIndexGet = b._nextIndexByRoundRobin
	case "random":
		b.nodeIndexGet = b._nextIndexByRandom
	case "ipHash":
		b.nodeIndexGet = b._nextIndexByIPHash
	case "leastUsed":
		b.nodeIndexGet = b._nextIndexByLeastUsed
	default:
		BugExitln("unknown balancer")
	}
	b.numNodes = int64(len(b.nodes))
}

func (b *Backend_[N]) ConfigureNodes() {
	for _, node := range b.nodes {
		node.OnConfigure()
	}
}
func (b *Backend_[N]) PrepareNodes() {
	for _, node := range b.nodes {
		node.OnPrepare()
	}
}

func (b *Backend_[N]) Stage() *Stage { return b.stage }

func (b *Backend_[N]) Maintain() { // runner
	for _, node := range b.nodes {
		b.IncSub() // node
		go node.Maintain()
	}

	// Waiting for the shutdown signal
	<-b.ShutChan
	// Current backend was told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		go node.OnShutdown()
	}
	b.WaitSubs() // nodes

	if DebugLevel() >= 2 {
		Printf("backend=%s done\n", b.CompName())
	}

	b.stage.DecSub() // backend
}

func (b *Backend_[N]) AddNode(node N) { // CreateNode() uses this to append nodes
	node.setShell(node)
	b.nodes = append(b.nodes, node)
}

func (b *Backend_[N]) _nextIndexByRoundRobin() int64 {
	index := b.nodeIndex.Add(1)
	return index % b.numNodes
}
func (b *Backend_[N]) _nextIndexByRandom() int64 {
	return rand.Int63n(b.numNodes)
}
func (b *Backend_[N]) _nextIndexByIPHash() int64 {
	// TODO
	return 0
}
func (b *Backend_[N]) _nextIndexByLeastUsed() int64 {
	// TODO
	return 0
}

// Node is a member of backend.
type Node interface {
	// Imports
	Component
	holder
	// Methods
	Maintain() // runner
}

// Node_ is the parent for all backend nodes.
type Node_[B Backend] struct {
	// Parent
	Component_
	// Mixins
	_holder_
	// Assocs
	backend B
	// States
	dialTimeout time.Duration // dial remote timeout
	weight      int32         // 1, 22, 333, ...
	connID      atomic.Int64  // next conn id
	down        atomic.Bool   // TODO: false-sharing
	health      any           // TODO
}

func (n *Node_[B]) OnCreate(compName string, stage *Stage, backend B) {
	n.MakeComp(compName)
	n.stage = stage
	n.backend = backend
	n.health = nil // TODO
}
func (n *Node_[B]) OnShutdown() { close(n.ShutChan) } // notifies Node.Maintain() which also closes conns

func (n *Node_[B]) OnConfigure() {
	n._holder_.onConfigure(n, 30*time.Second, 30*time.Second)

	// .address
	if v, ok := n.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if address[0] == '@' { // abstract uds
				n.udsMode = true
			} else if _, err := os.Stat(address); err == nil { // pathname uds
				n.udsMode = true
			}
			n.address = address
		} else {
			UseExitln("bad address in node")
		}
	} else {
		UseExitln("address is required in node")
	}

	// .dialTimeout
	n.ConfigureDuration("dialTimeout", &n.dialTimeout, func(value time.Duration) error {
		if value >= time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

	// .weight
	n.ConfigureInt32("weight", &n.weight, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad node weight")
	}, 1)
}
func (n *Node_[B]) OnPrepare() {
	n._holder_.onPrepare(n)
}

func (n *Node_[B]) Backend() B { return n.backend }

func (n *Node_[B]) DialTimeout() time.Duration { return n.dialTimeout }

func (n *Node_[B]) nextConnID() int64 { return n.connID.Add(1) }

func (n *Node_[B]) markDown()    { n.down.Store(true) }
func (n *Node_[B]) markUp()      { n.down.Store(false) }
func (n *Node_[B]) isDown() bool { return n.down.Load() }

func (n *Node_[B]) IncSubConn()          { n.IncSub() }
func (n *Node_[B]) DecSubConn()          { n.DecSub() }
func (n *Node_[B]) DecSubConns(size int) { n.DecSubs(size) }
func (n *Node_[B]) WaitSubConns()        { n.WaitSubs() }
