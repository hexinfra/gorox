// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Stat cronjobs report statistics about current stage.

package stat

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
)

func init() {
	RegisterCronjob("statCronjob", func(name string, stage *Stage) Cronjob {
		j := new(statCronjob)
		j.init(name, stage)
		return j
	})
}

// statCronjob
type statCronjob struct {
	// Mixins
	Cronjob_
	// Assocs
	stage *Stage
	// States
}

func (j *statCronjob) init(name string, stage *Stage) {
	j.InitComp(name)
	j.stage = stage
}

func (j *statCronjob) OnConfigure() {
}
func (j *statCronjob) OnPrepare() {
}
func (j *statCronjob) OnShutdown() {
	j.SetShut()
}

func (j *statCronjob) Run() { // goroutine
	for !j.IsShut() {
		// TODO
		time.Sleep(time.Minute)
	}
	if Debug(1) {
		fmt.Printf("statCronjob=%s shut\n", j.Name())
	}
	j.stage.SubDone()
}
