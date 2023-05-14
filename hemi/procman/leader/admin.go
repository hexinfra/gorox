// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Admin server.

package leader

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

func adminServer(msgChan chan *msgx.Message) {
	booker.Printf("open admin interface: %s\n", common.AdminAddr)
	admGate, err := net.Listen("tcp", common.AdminAddr) // admGate is for receiving admConns from control client
	if err != nil {
		common.Crash(err.Error())
	}
	var (
		req *msgx.Message
		ok  bool
	)
	for { // each admConn from control client
		admConn, err := admGate.Accept()
		if err != nil {
			booker.Println(err.Error())
			continue
		}
		if err := admConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			booker.Println(err.Error())
			goto closeNext
		}
		req, ok = msgx.Recv(admConn, 16<<20)
		if !ok {
			goto closeNext
		}
		booker.Printf("received from client: %v\n", req)
		if req.IsTell() {
			switch req.Comd { // some messages are telling leader only, hijack them.
			case common.ComdStop:
				booker.Println("received stop")
				common.Stop() // worker will stop immediately after cmdConn is closed
			case common.ComdReadmin:
				newAddr := req.Get("newAddr") // succeeding adminAddr
				if newAddr == "" {
					goto closeNext
				}
				if newGate, err := net.Listen("tcp", newAddr); err == nil {
					admGate.Close()
					admGate = newGate
					booker.Printf("admin re-opened to %s\n", newAddr)
					goto closeNext
				} else {
					booker.Printf("readmin failed: %s\n", err.Error())
				}
			default: // other messages are sent to keepWorker().
				msgChan <- req
			}
		} else { // call
			var resp *msgx.Message
			switch req.Comd { // some messages also call leader, hijack them.
			case common.ComdPing:
				resp = msgx.NewMessage(common.ComdPing, req.Flag, nil)
				resp.Set(fmt.Sprintf("leader=%d", os.Getpid()), "pong")
			case common.ComdPids:
				msgChan <- req
				resp = <-msgChan
				resp.Set("leader", strconv.Itoa(os.Getpid()))
			case common.ComdLeader:
				resp = msgx.NewMessage(common.ComdLeader, req.Flag, nil)
				resp.Set("goroutines", "123") // TODO
			default: // other messages are sent to keepWorker().
				msgChan <- req
				resp = <-msgChan
			}
			booker.Printf("send response: %v\n", resp)
			msgx.Send(admConn, resp)
		}
	closeNext:
		admConn.Close()
	}
}
