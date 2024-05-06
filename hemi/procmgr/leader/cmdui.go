// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// CmdUI server.

package leader

import (
	"net"
	"runtime"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procmgr/common"
)

func cmduiServer() { // runner
	if hemi.DebugLevel() >= 1 {
		hemi.Printf("[leader] open cmdui interface: %s\n", common.CmdUIAddr)
	}
	cmdGate, err := net.Listen("tcp", common.CmdUIAddr) // cmdGate is for receiving cmdConns from control client
	if err != nil {
		common.Crash(err.Error())
	}
	cmdChan := make(chan *msgx.Message)
	var req *msgx.Message
	for { // each cmdConn from control client
		cmdConn, err := cmdGate.Accept()
		if err != nil {
			if hemi.DebugLevel() >= 1 {
				hemi.Println("[leader] accept error: " + err.Error())
			}
			continue
		}
		if err = cmdConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			if hemi.DebugLevel() >= 1 {
				hemi.Println("[leader]: SetReadDeadline error: " + err.Error())
			}
			goto closeNext
		}
		if req, err = msgx.Recv(cmdConn, 16<<20); err != nil {
			goto closeNext
		}
		if hemi.DebugLevel() >= 1 {
			hemi.Printf("[leader] recv: %+v\n", req)
		}
		if req.IsTell() {
			switch req.Comd { // some messages are telling leader only, hijack them.
			case common.ComdStop:
				if hemi.DebugLevel() >= 1 {
					hemi.Println("[leader] received stop")
				}
				common.Stop() // worker will stop immediately after admConn is closed
			case common.ComdRecmd:
				newAddr := req.Get("newAddr") // succeeding cmduiAddr
				if newAddr == "" {
					goto closeNext
				}
				if newGate, err := net.Listen("tcp", newAddr); err == nil {
					cmdGate.Close()
					cmdGate = newGate
					if hemi.DebugLevel() >= 1 {
						hemi.Printf("[leader] cmdui re-opened to %s\n", newAddr)
					}
					goto closeNext
				} else {
					hemi.Errorf("[leader] recmd failed: %s\n", err.Error())
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
			if hemi.DebugLevel() >= 1 {
				hemi.Printf("[leader] send: %+v\n", resp)
			}
			msgx.Send(cmdConn, resp)
		}
	closeNext:
		cmdConn.Close()
	}
}
