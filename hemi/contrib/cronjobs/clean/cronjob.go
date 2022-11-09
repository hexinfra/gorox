// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Clean cronjobs clean old logs.

package clean

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
)

func init() {
	RegisterCronjob("cleanCronjob", func(name string, stage *Stage) Cronjob {
		j := new(cleanCronjob)
		j.init(name, stage)
		return j
	})
}

// cleanCronjob
type cleanCronjob struct {
	// Mixins
	Cronjob_
	// Assocs
	stage *Stage
	// States
}

func (j *cleanCronjob) init(name string, stage *Stage) {
	j.SetName(name)
	j.stage = stage
}

func (j *cleanCronjob) OnConfigure() {
}
func (j *cleanCronjob) OnPrepare() {
}
func (j *cleanCronjob) OnShutdown() {
	j.SetShut()
}

func (j *cleanCronjob) Run() { // goroutine
	for !j.Shut() {
		// TODO
		time.Sleep(time.Minute)
	}
	if Debug(1) {
		fmt.Printf("cleanCronjob=%s shut\n", j.Name())
	}
}
