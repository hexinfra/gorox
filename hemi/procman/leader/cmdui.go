// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// CmdUI server.

package leader

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

func cmduiServer(msgChan chan *msgx.Message) {
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
				// TODO: ignore?
			default: // other messages are sent to workerKeeper().
				msgChan <- req
			}
		} else { // call
			var resp *msgx.Message
			switch req.Comd { // some messages also call leader, hijack them.
			case common.ComdPids:
				msgChan <- req
				resp = <-msgChan
				resp.Set("leader", strconv.Itoa(os.Getpid()))
			case common.ComdLeader:
				resp = msgx.NewMessage(common.ComdLeader, req.Flag, nil)
				resp.Set("goroutines", strconv.Itoa(runtime.NumGoroutine()))
			default: // other messages are sent to workerKeeper().
				msgChan <- req
				resp = <-msgChan
			}
			logger.Printf("send response: %v\n", resp)
			msgx.Send(cmdConn, resp)
		}
	closeNext:
		cmdConn.Close()
	}
}
