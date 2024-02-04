// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
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
	hemi.SetTmpsDir(system.ExeDir + "/tmps")
	hemi.SetVarsDir(system.ExeDir + "/vars")

	stage, err := hemi.BootText("stage{}")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	stage.Start(1)
	stage.Quit()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
