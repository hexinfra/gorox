// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader control agent.

package manager

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/msgx"
	"net"
)

// agentMain is main() for control agent.
func agentMain(action string) {
	if tell, ok := tellActions[action]; ok {
		tell()
	} else if call, ok := callActions[action]; ok {
		call()
	} else {
		fmt.Printf("unknown action: %s\n", action)
	}
}

const ( // for tells
	comdRun = iota // start server. this is in fact a fake command. must be 0
	comdQuit
	comdStop
	comdReadmin
	comdRework
	comdCPU
	comdHeap
	comdThread
	comdGoroutine
	comdBlock
)

var tellActions = map[string]func(){
	"quit":      tellQuit,
	"stop":      tellStop,
	"readmin":   tellReadmin,
	"rework":    tellRework,
	"cpu":       tellCPU,
	"heap":      tellHeap,
	"thread":    tellThread,
	"goroutine": tellGoroutine,
	"block":     tellBlock,
}

func tellQuit()      { _tellLeader(comdQuit, 0, nil) }
func tellStop()      { _tellLeader(comdStop, 0, nil) }
func tellReadmin()   { _tellLeader(comdReadmin, 0, map[string]string{"newAddr": adminAddr}) }
func tellRework()    { _tellLeader(comdRework, 0, nil) }
func tellCPU()       { _tellLeader(comdCPU, 0, nil) }
func tellHeap()      { _tellLeader(comdHeap, 0, nil) }
func tellThread()    { _tellLeader(comdThread, 0, nil) }
func tellGoroutine() { _tellLeader(comdGoroutine, 0, nil) }
func tellBlock()     { _tellLeader(comdBlock, 0, nil) }

func _tellLeader(comd uint8, flag uint16, args map[string]string) {
	admConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		fmt.Printf("tell leader failed: %s\n", err.Error())
		return
	}
	defer admConn.Close()
	if msgx.Tell(admConn, msgx.NewMessage(comd, flag, args)) {
		fmt.Printf("tell leader at %s: ok!\n", targetAddr)
	} else {
		fmt.Printf("tell leader at %s: failed!\n", targetAddr)
	}
}

const ( // for calls
	comdPing = iota // must be 0
	comdInfo
	comdReconf
)

var callActions = map[string]func(){
	"ping":   callPing,
	"info":   callInfo,
	"reconf": callReconf,
}

func callPing() {
	if resp, ok := _callLeader(comdPing, 0, nil); ok && resp.Comd == comdPing && resp.Flag == 0 {
		fmt.Printf("call leader at %s: pong\n", targetAddr)
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callInfo() {
	if resp, ok := _callLeader(comdInfo, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s=%s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callReconf() {
	if resp, ok := _callLeader(comdReconf, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s=%s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}

func _callLeader(comd uint8, flag uint16, args map[string]string) (*msgx.Message, bool) {
	admConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return nil, false
	}
	defer admConn.Close()
	return msgx.Call(admConn, msgx.NewMessage(comd, flag, args))
}
