// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package diogin

import (
	"fmt"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/system"
)

func Main() {
	hemi.SetTopDir(system.ExeDir)
	hemi.SetLogDir(system.ExeDir + "/data/log")
	hemi.SetTmpDir(system.ExeDir + "/data/tmp")
	hemi.SetVarDir(system.ExeDir + "/data/var")

	stage, err := hemi.NewStageText("stage{}")
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
