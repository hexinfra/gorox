// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader control client.

package procman

import (
	"fmt"
	"net"

	"github.com/hexinfra/gorox/hemi/common/msgx"
)

// clientMain is main() for control client.
func clientMain(action string) {
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
	comdReadmin
	comdReload
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
	"readmin":   tellReadmin,
	"reload":    tellReload,
	"cpu":       tellCPU,
	"heap":      tellHeap,
	"thread":    tellThread,
	"goroutine": tellGoroutine,
	"block":     tellBlock,
	"gc":        tellGC,
}

func tellStop()      { _tell(comdStop, 0, nil) }
func tellQuit()      { _tell(comdQuit, 0, nil) }
func tellRework()    { _tell(comdRework, 0, nil) }
func tellReadmin()   { _tell(comdReadmin, 0, map[string]string{"newAddr": adminAddr}) }
func tellReload()    { _tell(comdReload, 0, nil) }
func tellCPU()       { _tell(comdCPU, 0, nil) }
func tellHeap()      { _tell(comdHeap, 0, nil) }
func tellThread()    { _tell(comdThread, 0, nil) }
func tellGoroutine() { _tell(comdGoroutine, 0, nil) }
func tellBlock()     { _tell(comdBlock, 0, nil) }
func tellGC()        { _tell(comdGC, 0, nil) }

func _tell(comd uint8, flag uint16, args map[string]string) {
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
	comdPing = iota
	comdPid
	comdLeader
	comdWorker
)

var callActions = map[string]func(){
	"ping":   callPing,
	"pid":    callPid,
	"leader": callLeader,
	"worker": callWorker,
}

func callPing() {
	if resp, ok := _call(comdPing, 0, nil); ok && resp.Comd == comdPing && resp.Flag == 0 {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callPid() {
	if resp, ok := _call(comdPid, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callLeader() {
	if resp, ok := _call(comdLeader, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}
func callWorker() {
	if resp, ok := _call(comdWorker, 0, nil); ok {
		for name, value := range resp.Args {
			fmt.Printf("%s: %s\n", name, value)
		}
	} else {
		fmt.Printf("call leader at %s: failed!\n", targetAddr)
	}
}

func _call(comd uint8, flag uint16, args map[string]string) (*msgx.Message, bool) {
	admConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return nil, false
	}
	defer admConn.Close()
	return msgx.Call(admConn, msgx.NewMessage(comd, flag, args), 16<<20)
}
