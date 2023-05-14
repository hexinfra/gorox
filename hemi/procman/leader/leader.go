// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

// admConn: control client ----> adminServer()
// admGate: Used by adminServer(), for receiving admConns from control client
// webConn: control browser ----> webuiServer()
// webGate: Used by webuiServer(), for receiving webConns from control browser
// roxConn: myroxClient() <---> myrox
// msgChan: adminServer()/webuiServer()/myroxClient() <---> keepWorker()
// deadWay: keepWorker() <---- worker.wait()
// cmdConn: leader process <---> worker process

package leader

import (
	"log"
	"os"
	"path/filepath"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var booker *log.Logger

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
	booker = log.New(osFile, "", log.Ldate|log.Ltime)

	if common.MyroxAddr == "" {
		// Load worker's config
		base, file := common.GetConfig()
		booker.Printf("parse worker config: base=%s file=%s\n", base, file)
		if _, err := hemi.ApplyFile(base, file); err != nil {
			common.Crash("leader: " + err.Error())
		}

		// Start the worker
		msgChan := make(chan *msgx.Message) // msgChan is the channel between adminServer()/webuiServer() and keepWorker()
		go keepWorker(base, file, msgChan)
		<-msgChan // wait for keepWorker() to ensure worker is started.
		booker.Println("worker process started")

		// TODO: msgChan MUST be protected against concurrent adminServer() and webuiServer()
		go adminServer(msgChan)
		go webuiServer(msgChan)
		select {} // waiting forever
	} else {
		myroxClient()
	}
}
