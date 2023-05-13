// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

// admConn: control client ----> adminServer()
// admGate: Used by adminServer(), for receiving admConns from control client
// webConn: control browser ----> webuiServer()
// webGate: Used by webuiServer(), for receiving webConns from control browser
// msgChan: adminServer()+webuiServer() / myroxClient() <---> keepWorker()
// deadWay: keepWorker() <---- worker.wait()
// cmdConn: leader process <---> worker process
// roxConn: leader myroxClient <---> myrox

package leader

import (
	"log"
	"os"
	"path/filepath"

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
		go webuiServer()
		go adminServer()
		select {} // waiting forever
	} else {
		myroxClient()
	}
}
