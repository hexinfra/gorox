// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Caller.

package client

import (
	"fmt"
	"net"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var calls = map[string]func(){ // indexed by action
	"pids": func() {
		if resp, ok := _call(common.ComdPids, 0, nil); ok {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		} else {
			fmt.Printf("call leader at %s: failed!\n", common.TargetAddr)
		}
	},
	"leader": func() {
		if resp, ok := _call(common.ComdLeader, 0, nil); ok {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		} else {
			fmt.Printf("call leader at %s: failed!\n", common.TargetAddr)
		}
	},
	"worker": func() {
		if resp, ok := _call(common.ComdWorker, 0, nil); ok {
			for name, value := range resp.Args {
				fmt.Printf("%s: %s\n", name, value)
			}
		} else {
			fmt.Printf("call leader at %s: failed!\n", common.TargetAddr)
		}
	},
}

func _call(comd uint8, flag uint16, args map[string]string) (*msgx.Message, bool) {
	admConn, err := net.Dial("tcp", common.TargetAddr)
	if err != nil {
		return nil, false
	}
	defer admConn.Close()

	return msgx.Call(admConn, msgx.NewMessage(comd, flag, args), 16<<20)
}
