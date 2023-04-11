// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader control agent.

package process

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
	comdServe = iota // start server. no tell action bound to this comd. must be 0
	comdStop
	comdQuit
	comdRework
	comdReopen
	comdReconf
	comdCPU
	comdHeap
	comdThread
	comdGoroutine
	comdBlock
	comdGC
)

var tellActions = map[string]func(){
	"stop":      tellStop,
	"quit":      tellQuit,
	"rework":    tellRework,
	"reopen":    tellReopen,
	"reconf":    tellReconf,
	"cpu":       tellCPU,
	"heap":      tellHeap,
	"thread":    tellThread,
	"goroutine": tellGoroutine,
	"block":     tellBlock,
	"gc":        tellGC,
}

func tellStop()      { _tellLeader(comdStop, 0, nil) }
func tellQuit()      { _tellLeader(comdQuit, 0, nil) }
func tellRework()    { _tellLeader(comdRework, 0, nil) }
func tellReopen()    { _tellLeader(comdReopen, 0, map[string]string{"newAddr": adminAddr}) }
func tellReconf()    { _tellLeader(comdReconf, 0, nil) }
func tellCPU()       { _tellLeader(comdCPU, 0, nil) }
func tellHeap()      { _tellLeader(comdHeap, 0, nil) }
func tellThread()    { _tellLeader(comdThread, 0, nil) }
func tellGoroutine() { _tellLeader(comdGoroutine, 0, nil) }
func tellBlock()     { _tellLeader(comdBlock, 0, nil) }
func tellGC()        { _tellLeader(comdGC, 0, nil) }

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
	comdPid = iota
	comdInfo
	comdPing
)

var callActions = map[string]func(){
	"pid":  callPid,
	"info": callInfo,
	"ping": callPing,
}

func callPid() {
	if resp, ok := _callLeader(comdPid, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callInfo() {
	if resp, ok := _callLeader(comdInfo, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callPing() {
	if resp, ok := _callLeader(comdPing, 0, nil); ok && resp.Comd == comdPing && resp.Flag == 0 {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
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
	return msgx.Call(admConn, msgx.NewMessage(comd, flag, args), 16<<20)
}
