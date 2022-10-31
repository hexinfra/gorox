// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Single mode of manager. Only a single foreground process is started in this mode.

package manager

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi"
)

// singleMain is main() for single mode.
func singleMain() {
	stage, err := hemi.ApplyFile(getConfig())
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	stage.StartAlone()
	select {}
}
