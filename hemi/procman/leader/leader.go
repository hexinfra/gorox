// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

package leader

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"

	_ "github.com/hexinfra/gorox/hemi/procman/leader/webui"
)

var (
	logger   *log.Logger
	webStage *hemi.Stage
)

func Main() {
	logFile := common.LogFile
	if logFile == "" {
		logFile = common.LogsDir + "/" + common.Program + "-leader.log"
	} else if !filepath.IsAbs(logFile) {
		logFile = common.BaseDir + "/" + logFile
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		common.Crash(err.Error())
	}
	osFile, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		common.Crash(err.Error())
	}
	logger = log.New(osFile, "", log.Ldate|log.Ltime)

	// Check worker's config
	base, file := common.GetConfig()
	logger.Printf("parse worker config: base=%s file=%s\n", base, file)
	if _, err := hemi.ApplyFile(base, file); err != nil {
		common.Crash("leader: " + err.Error())
	}

	// Start the worker
	keeperChan = make(chan chan *msgx.Message)
	go workerKeeper(base, file)
	<-keeperChan // wait for workerKeeper() to ensure worker is started.
	logger.Println("worker process started")

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
	logger.Printf("open cmdui interface: %s\n", common.CmdUIAddr)
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
			logger.Println(err.Error())
			continue
		}
		if err := cmdConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			logger.Println(err.Error())
			goto closeNext
		}
		req, ok = msgx.Recv(cmdConn, 16<<20)
		if !ok {
			goto closeNext
		}
		logger.Printf("received from client: %v\n", req)
		if req.IsTell() {
			switch req.Comd { // some messages are telling leader only, hijack them.
			case common.ComdStop:
				logger.Println("received stop")
				common.Stop() // worker will stop immediately after msgConn is closed
			case common.ComdRecmd:
				newAddr := req.Get("newAddr") // succeeding cmduiAddr
				if newAddr == "" {
					goto closeNext
				}
				if newGate, err := net.Listen("tcp", newAddr); err == nil {
					cmdGate.Close()
					cmdGate = newGate
					logger.Printf("cmdui re-opened to %s\n", newAddr)
					goto closeNext
				} else {
					logger.Printf("recmd failed: %s\n", err.Error())
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
			logger.Printf("send response: %v\n", resp)
			msgx.Send(cmdConn, resp)
		}
	closeNext:
		cmdConn.Close()
	}
}

func webuiServer() {
	// TODO
	webChan := make(chan *msgx.Message)
	_ = webChan
}

func myroxClient() {
	// TODO
	roxChan := make(chan *msgx.Message)
	_ = roxChan
	fmt.Println("TODO")
}
