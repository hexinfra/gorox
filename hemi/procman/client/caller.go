// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Caller.

package client

import (
	"fmt"
	"net"
	"os"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var calls = map[string]func(){ // indexed by action
	"pids": func() {
		if resp, err := _call(common.ComdPids, 0, nil); err == nil {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		} else {
			fmt.Fprintf(os.Stderr, "call leader at %s: %s\n", common.TargetAddr, err.Error())
		}
	},
	"leader": func() {
		if resp, err := _call(common.ComdLeader, 0, nil); err == nil {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		} else {
			fmt.Fprintf(os.Stderr, "call leader at %s: %s\n", common.TargetAddr, err.Error())
		}
	},
	"worker": func() {
		if resp, err := _call(common.ComdWorker, 0, nil); err == nil {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		} else {
			fmt.Fprintf(os.Stderr, "call leader at %s: %s\n", common.TargetAddr, err.Error())
		}
	},
	"reload": func() {
		if resp, err := _call(common.ComdReload, 0, nil); err == nil && resp.Flag == 0 {
			fmt.Println("reload ok!")
		} else {
			fmt.Fprintf(os.Stderr, "call leader at %s: %s\n", common.TargetAddr, err.Error())
		}
	},
}

func _call(comd uint8, flag uint16, args map[string]string) (*msgx.Message, error) {
	cmdConn, err := net.Dial("tcp", common.TargetAddr)
	if err != nil {
		return nil, err
	}
	defer cmdConn.Close()

	return msgx.Call(cmdConn, msgx.NewMessage(comd, flag, args), 16<<20)
}
