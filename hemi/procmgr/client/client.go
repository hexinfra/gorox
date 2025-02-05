// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Control client.

package client

import (
	"fmt"
	"net"
	"os"

	"github.com/hexinfra/gorox/hemi/library/msgx"
	"github.com/hexinfra/gorox/hemi/procmgr/common"
)

func Main(action string) {
	if call, ok := calls[action]; ok {
		call()
	} else if tell, ok := tells[action]; ok {
		tell()
	} else {
		fmt.Fprintf(os.Stderr, "unknown action: %s\n", action)
	}
}

var calls = map[string]func(){ // indexed by action
	"pids": func() {
		if resp, err := _call(common.ComdPids, 0, nil); err == nil {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		}
	},
	"leader": func() {
		if resp, err := _call(common.ComdLeader, 0, nil); err == nil {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		}
	},
	"worker": func() {
		if resp, err := _call(common.ComdWorker, 0, nil); err == nil {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		}
	},
	"reconf": func() {
		if resp, err := _call(common.ComdReconf, 0, nil); err == nil && resp.Flag == 0 {
			fmt.Println("reconf ok!")
		}
	},
}

func _call(comd uint8, flag uint16, args map[string]string) (*msgx.Message, error) {
	cmdConn, err := net.Dial("tcp", common.TargetAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to call leader at %s: %s\n", common.TargetAddr, err.Error())
		return nil, err
	}
	defer cmdConn.Close()

	return msgx.Call(cmdConn, msgx.NewMessage(comd, flag, args), 16<<20)
}

var tells = map[string]func(){ // indexed by action
	"stop":      func() { _tell(common.ComdStop, 0, nil) },
	"quit":      func() { _tell(common.ComdQuit, 0, nil) },
	"recmd":     func() { _tell(common.ComdRecmd, 0, map[string]string{"newAddr": common.CmdUIAddr}) },
	"reweb":     func() { _tell(common.ComdReweb, 0, map[string]string{"newAddr": common.WebUIAddr}) },
	"rework":    func() { _tell(common.ComdRework, 0, nil) },
	"cpu":       func() { _tell(common.ComdCPU, 0, nil) },
	"heap":      func() { _tell(common.ComdHeap, 0, nil) },
	"thread":    func() { _tell(common.ComdThread, 0, nil) },
	"goroutine": func() { _tell(common.ComdGoroutine, 0, nil) },
	"block":     func() { _tell(common.ComdBlock, 0, nil) },
	"gc":        func() { _tell(common.ComdGC, 0, nil) },
}

func _tell(comd uint8, flag uint16, args map[string]string) {
	cmdConn, err := net.Dial("tcp", common.TargetAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to tell leader at %s: %s\n", common.TargetAddr, err.Error())
		return
	}
	defer cmdConn.Close()

	if err := msgx.Tell(cmdConn, msgx.NewMessage(comd, flag, args)); err == nil {
		fmt.Printf("tell leader at %s: ok!\n", common.TargetAddr)
	} else {
		fmt.Fprintf(os.Stderr, "tell leader at %s: %s\n", common.TargetAddr, err.Error())
	}
}
