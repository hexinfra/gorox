// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

package leader

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"

	_ "github.com/hexinfra/gorox/hemi/procman/leader/webui"
)

func Main() {
	// Check worker's config
	configBase, configFile := common.GetConfig()
	fmt.Printf("[leader] check worker configBase=%s configFile=%s\n", configBase, configFile)
	if _, err := hemi.ApplyFile(configBase, configFile); err != nil {
		common.Crash("leader: " + err.Error())
	}

	// Start the worker
	keeperChan = make(chan chan *msgx.Message)
	go workerKeeper(configBase, configFile)
	<-keeperChan // wait for workerKeeper() to ensure worker is started.

	if common.MyroxAddr != "" {
		go myroxClient()
	} else {
		// TODO: sync
		go webuiServer()
		go cmduiServer()
	}

	select {} // waiting forever
}

func myroxClient() {
	// TODO
	roxChan := make(chan *msgx.Message)
	_ = roxChan
	fmt.Println("[leader] TODO")
}

func cmduiServer() {
	cmdChan := make(chan *msgx.Message)
	fmt.Printf("[leader] open cmdui interface: %s\n", common.CmdUIAddr)
	cmdGate, err := net.Listen("tcp", common.CmdUIAddr) // cmdGate is for receiving cmdConns from control client
	if err != nil {
		common.Crash(err.Error())
	}
	var (
		req *msgx.Message
		ok  bool
	)
	for { // each cmdConn from control client
		cmdConn, err := cmdGate.Accept()
		if err != nil {
			fmt.Println("[leader] " + err.Error())
			continue
		}
		if err := cmdConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			fmt.Println("[leader] " + err.Error())
			goto closeNext
		}
		req, ok = msgx.Recv(cmdConn, 16<<20)
		if !ok {
			goto closeNext
		}
		fmt.Printf("[leader] received from client: %v\n", req)
		if req.IsTell() {
			switch req.Comd { // some messages are telling leader only, hijack them.
			case common.ComdStop:
				fmt.Println("[leader] received stop")
				common.Stop() // worker will stop immediately after msgConn is closed
			case common.ComdRecmd:
				newAddr := req.Get("newAddr") // succeeding cmduiAddr
				if newAddr == "" {
					goto closeNext
				}
				if newGate, err := net.Listen("tcp", newAddr); err == nil {
					cmdGate.Close()
					cmdGate = newGate
					fmt.Printf("[leader] cmdui re-opened to %s\n", newAddr)
					goto closeNext
				} else {
					fmt.Printf("[leader] recmd failed: %s\n", err.Error())
				}
			case common.ComdReweb:
				// TODO
			default: // other messages are sent to workerKeeper().
				keeperChan <- cmdChan
				cmdChan <- req
			}
		} else { // call
			var resp *msgx.Message
			switch req.Comd { // some messages also call leader, hijack them.
			case common.ComdLeader:
				resp = msgx.NewMessage(common.ComdLeader, req.Flag, nil)
				resp.Set("goroutines", strconv.Itoa(runtime.NumGoroutine()))
			default: // other messages are sent to workerKeeper().
				keeperChan <- cmdChan
				cmdChan <- req
				resp = <-cmdChan
			}
			fmt.Printf("[leader] send response: %v\n", resp)
			msgx.Send(cmdConn, resp)
		}
	closeNext:
		cmdConn.Close()
	}
}

var (
	webStage *hemi.Stage
	webChan  chan *msgx.Message
)

func webuiServer() {
	webChan = make(chan *msgx.Message)
	fmt.Printf("[leader] open webui interface: %s\n", common.WebUIAddr)
	// TODO
}
