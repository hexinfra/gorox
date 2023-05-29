// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Teller.

package client

import (
	"fmt"
	"net"
	"os"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

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
		fmt.Fprintf(os.Stderr, "tell leader at %s failed: %s\n", common.TargetAddr, err.Error())
		return
	}
	defer cmdConn.Close()

	if msgx.Tell(cmdConn, msgx.NewMessage(comd, flag, args)) {
		fmt.Printf("tell leader at %s: ok!\n", common.TargetAddr)
	} else {
		fmt.Fprintf(os.Stderr, "tell leader at %s: failed!\n", common.TargetAddr)
	}
}
