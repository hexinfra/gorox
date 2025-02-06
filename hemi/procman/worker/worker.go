// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Worker process.

package worker

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/library/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var (
	configBase string      // base string of config file
	configFile string      // config file path
	curStage   *hemi.Stage // current stage
)

func Main(token string) {
	parts := strings.Split(token, "|") // ip:port|connKey
	if len(parts) != 2 {
		common.Crash("bad token")
	}

	// Contact leader process
	admConn, err := net.Dial("tcp", parts[0]) // ip:port
	if err != nil {
		common.Crash("dial leader failed: " + err.Error())
	}

	// Register worker to leader
	if loginResp, err := msgx.Call(admConn, msgx.NewMessage(0, 0, map[string]string{
		"connKey": parts[1],
	}), 16<<20); err == nil {
		configBase = loginResp.Get("configBase")
		configFile = loginResp.Get("configFile")
	} else {
		common.Crash("call leader failed: " + err.Error())
	}

	// Register succeeded. Now start the initial stage
	curStage, err = hemi.StageFromFile(configBase, configFile)
	if err != nil {
		common.Crash(err.Error())
	}
	curStage.Start(0)

	// Initial stage started. Now waiting for leader's commands
	for { // each message from leader process
		req, err := msgx.Recv(admConn, 16<<20)
		if err != nil { // leader must be gone
			break
		}
		hemi.Printf("[worker] recv from leader: %+v\n", req)
		if req.IsCall() {
			resp := msgx.NewMessage(req.Comd, 0, nil)
			if onCall, ok := onCalls[req.Comd]; ok {
				onCall(req, resp)
			} else {
				resp.Flag = 404
			}
			if msgx.Send(admConn, resp) != nil { // leader must be gone
				break
			}
		} else if onTell, ok := onTells[req.Comd]; ok {
			onTell(req)
		} else {
			// Unknown tell command, ignore.
		}
	}

	common.Stop() // the loop is broken. simply stop worker process
}

var onCalls = map[uint8]func(req *msgx.Message, resp *msgx.Message){
	common.ComdWorker: func(req *msgx.Message, resp *msgx.Message) {
		resp.Set("goroutines", strconv.Itoa(runtime.NumGoroutine())) // TODO: other infos
	},
	common.ComdReconf: func(req *msgx.Message, resp *msgx.Message) {
		if newStage, err := hemi.StageFromFile(configBase, configFile); err == nil {
			oldStage := curStage
			newStage.Start(oldStage.ID() + 1)
			curStage = newStage
			oldStage.Quit()
		} else {
			hemi.Errorln(err.Error())
			resp.Flag = 500
		}
	},
}

var onTells = map[uint8]func(req *msgx.Message){
	common.ComdQuit: func(req *msgx.Message) {
		curStage.Quit() // blocking
		os.Exit(0)
	},
	common.ComdCPU: func(req *msgx.Message) {
		curStage.ProfCPU()
	},
	common.ComdHeap: func(req *msgx.Message) {
		curStage.ProfHeap()
	},
	common.ComdThread: func(req *msgx.Message) {
		curStage.ProfThread()
	},
	common.ComdGoroutine: func(req *msgx.Message) {
		curStage.ProfGoroutine()
	},
	common.ComdBlock: func(req *msgx.Message) {
		curStage.ProfBlock()
	},
	common.ComdGC: func(req *msgx.Message) {
		runtime.GC()
	},
}
