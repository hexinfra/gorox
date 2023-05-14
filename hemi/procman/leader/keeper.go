// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker keeper.

package leader

import (
	"math/rand"
	"os"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

func keepWorker(base string, file string, msgChan chan *msgx.Message) { // goroutine
	deadWay := make(chan int) // dead worker go through this channel

	rand.Seed(time.Now().UnixNano())
	const chars = "0123456789"
	keyBuffer := make([]byte, 32)
	for i := 0; i < len(keyBuffer); i++ {
		keyBuffer[i] = chars[rand.Intn(10)]
	}
	connKey := string(keyBuffer)

	worker := newWorker(connKey)
	worker.start(base, file, deadWay)
	msgChan <- nil // reply to adminServer() that we have created the worker.

	for { // each event from adminServer() and worker
		select {
		case req := <-msgChan: // a message arrives from adminServer()
			if req.IsTell() {
				switch req.Comd {
				case common.ComdQuit:
					worker.tell(req)
					exitCode := <-deadWay
					os.Exit(exitCode)
				case common.ComdRework: // restart worker
					// Create new worker
					deadWay2 := make(chan int)
					worker2 := newWorker(connKey)
					worker2.start(base, file, deadWay2)
					// Quit old worker
					req.Comd = common.ComdQuit
					worker.tell(req)
					worker.reset()
					<-deadWay
					// Use new worker
					deadWay, worker = deadWay2, worker2
				default: // tell worker
					worker.tell(req)
				}
			} else { // call
				resp := worker.call(req)
				msgChan <- resp
			}
		case exitCode := <-deadWay: // worker process dies unexpectedly
			// TODO: more details
			if exitCode == common.CodeCrash || exitCode == common.CodeStop || exitCode == hemi.CodeBug || exitCode == hemi.CodeUse || exitCode == hemi.CodeEnv {
				booker.Println("worker critical error")
				common.Stop()
			} else if now := time.Now(); now.Sub(worker.lastDie) > time.Second {
				worker.reset()
				worker.lastDie = now
				worker.start(base, file, deadWay) // start again
			} else { // worker has suffered too frequent crashes, unable to serve!
				booker.Println("worker is broken!")
				common.Stop()
			}
		}
	}
}
