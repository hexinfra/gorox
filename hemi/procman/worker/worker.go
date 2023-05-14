// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker process.

package worker

import (
	"net"
	"strings"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var (
	configBase   string      // base string of config file
	configFile   string      // config file path
	currentStage *hemi.Stage // current stage
)

func Main(token string) {
	parts := strings.Split(token, "|") // ip:port|connKey
	if len(parts) != 2 {
		common.Crash("bad token")
	}

	// Contact leader process
	cmdConn, err := net.Dial("tcp", parts[0]) // ip:port
	if err != nil {
		common.Crash("dial leader failed: " + err.Error())
	}

	// Register worker to leader
	if loginResp, ok := msgx.Call(cmdConn, msgx.NewMessage(0, 0, map[string]string{
		"connKey": parts[1],
	}), 16<<20); ok {
		configBase = loginResp.Get("base")
		configFile = loginResp.Get("file")
	} else {
		common.Crash("call leader failed")
	}

	// Register succeeded. Now start the initial stage
	currentStage, err = hemi.ApplyFile(configBase, configFile)
	if err != nil {
		common.Crash(err.Error())
	}
	currentStage.Start(0)

	// Stage started, now waiting for leader's commands.
	for { // each message from leader process
		req, ok := msgx.Recv(cmdConn, 16<<20)
		if !ok { // leader must be gone
			break
		}
		if hemi.IsDebug(2) {
			hemi.Debugf("worker received req=%v\n", req)
		}
		if req.IsCall() {
			resp := msgx.NewMessage(req.Comd, 0, nil)
			if onCall, ok := onCalls[req.Comd]; ok {
				onCall(currentStage, req, resp)
			} else {
				resp.Flag = 404
			}
			if !msgx.Send(cmdConn, resp) { // leader must be gone
				break
			}
		} else if onTell, ok := onTells[req.Comd]; ok {
			onTell(currentStage, req)
		} else {
			// Unknown tell command, ignore.
		}
	}

	common.Stop() // the loop is broken. simply stop worker
}