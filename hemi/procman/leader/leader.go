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
	base, file := common.GetConfig()
	fmt.Printf("[leader] parse worker config: base=%s file=%s\n", base, file)
	if _, err := hemi.ApplyFile(base, file); err != nil {
		common.Crash("leader: " + err.Error())
	}

	// Start the worker
	keeperChan = make(chan chan *msgx.Message)
	go workerKeeper(base, file)
	<-keeperChan // wait for workerKeeper() to ensure worker is started.
	fmt.Println("[leader] worker process started")

	if common.MyroxAddr == "" {
		go cmduiServer()
		go webuiServer()
	} else {
		go myroxClient()
	}

	select {} // waiting forever
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

var webStage *hemi.Stage

func webuiServer() {
	// TODO
	webChan := make(chan *msgx.Message)
	_ = webChan
}

func myroxClient() {
	// TODO
	roxChan := make(chan *msgx.Message)
	_ = roxChan
	fmt.Println("[leader] TODO")
}
