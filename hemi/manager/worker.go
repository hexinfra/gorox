// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker process(es) of manager.

// Currently we support two worker process modes: together mode and isolated mode.
// Together mode has one worker process, with all threads and CPUs running on it.
// Isolated mode has many worker processes, each of which has many threads and one CPU.
// Users can choose one of the two modes.
// The purpose of isolated mode is to support SO_ATTACH_REUSEPORT_CBPF in the future,
// because Go's runtime doesn't allow us to control threads individually. This also affects cpu pinning.

package manager

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/libraries/msgx"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	workTogether uint16 = 0 // only one worker process
	workIsolated uint16 = 1 // multiple worker processes
)

var (
	configBase   string
	configFile   string
	lastStage    *hemi.Stage // last stage
	currentStage *hemi.Stage // current stage
)

// workerMain is main() for worker process(es).
func workerMain(token string) {
	parts := strings.Split(token, "|") // ip:port|pipeKey|workerID
	if len(parts) != 3 {
		crash("bad token")
	}
	workerID, err := strconv.Atoi(parts[2])
	if err != nil {
		crash(err.Error())
	}

	// Contact leader process and register this worker
	msgPipe, err := net.Dial("tcp", parts[0]) // ip:port
	if err != nil {
		crash("dial leader failed: " + err.Error())
	}
	loginReq := msgx.NewMessage(0, 0, map[string]string{
		"pipeKey":  parts[1],
		"workerID": parts[2],
	})
	loginResp, ok := msgx.Call(msgPipe, loginReq)
	if !ok {
		crash("call leader failed")
	}

	// Register succeeded. Now start the initial stage
	configBase = loginResp.Get("base")
	configFile = loginResp.Get("file")
	currentStage, err = hemi.ApplyFile(configBase, configFile)
	if err != nil {
		crash(err.Error())
	}

	for { // each message from leader process
		req, ok := msgx.RecvMessage(msgPipe)
		if !ok {
			// Leader process must be gone.
			break
		}
		if req.Comd == comdServe {
			if req.Flag == workTogether {
				currentStage.StartTogether()
			} else {
				currentStage.StartIsolated(int32(workerID))
			}
		} else if req.IsCall() {
			resp := msgx.NewMessage(req.Comd, 0, nil)
			if onCall, ok := onCalls[req.Comd]; ok {
				onCall(currentStage, req, resp)
			} else {
				resp.Flag = 404
			}
			if !msgx.SendMessage(msgPipe, resp) {
				break
			}
		} else if onTell, ok := onTells[req.Comd]; ok {
			onTell(currentStage, req)
		} else {
			// Unknown tell command, ignore.
		}
	}

	stop() // the loop is broken. simply stop worker
}

var onCalls = map[uint8]func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message){ // call commands
	comdInfo: func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message) {
		resp.Set("pid", fmt.Sprintf("%d", os.Getpid()))
	},
	comdReconf: func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message) {
		/*
			newStage, err := hemi.ApplyFile(configBase, configFile)
			if err != nil {
				resp.Set("result", "false")
				return
			}
			currentStage = newStage
			stage.Shutdown()
		*/
		resp.Set("result", "true")
	},
}

var onTells = map[uint8]func(stage *hemi.Stage, req *msgx.Message){ // tell commands
	comdQuit: func(stage *hemi.Stage, req *msgx.Message) {
		stage.Shutdown()
	},
	comdCPU: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfCPU()
	},
	comdHeap: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfHeap()
	},
	comdThread: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfThread()
	},
	comdGoroutine: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfGoroutine()
	},
	comdBlock: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfBlock()
	},
}
