// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package diogin

import (
	"fmt"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/system"
)

func Main() {
	hemi.SetBaseDir(system.ExeDir)
	hemi.SetLogsDir(system.ExeDir + "/logs")
	hemi.SetTempDir(system.ExeDir + "/temp")
	hemi.SetVarsDir(system.ExeDir + "/vars")

	stage, err := hemi.FromText("stage{}")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	stage.Start(1)
	defer stage.Quit()

	outgate := stage.HTTP1Outgate()

	conn, err := outgate.Dial("httpwg.org:80", false)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer conn.Close()

	stream := conn.UseStream()
	defer conn.EndStream(stream)

	req := stream.Request()
	req.SetMethodURI("GET", "/", false)
	req.AddHeader("host", "httpwg.org")

	if err := stream.ExecuteExchan(); err != nil {
		fmt.Println(err.Error())
		return
	}

	resp := stream.Response()
	fmt.Printf("status=%d\n", resp.Status())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
