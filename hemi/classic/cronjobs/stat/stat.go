// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Stat cronjobs report statistics about current stage.

package stat

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterCronjob("statCronjob", func(name string, stage *Stage) Cronjob {
		j := new(statCronjob)
		j.onCreate(name, stage)
		return j
	})
}

// statCronjob
type statCronjob struct {
	// Parent
	Cronjob_
	// Assocs
	stage *Stage // current stage
	// States
}

func (j *statCronjob) onCreate(name string, stage *Stage) {
	j.MakeComp(name)
	j.stage = stage
}
func (j *statCronjob) OnShutdown() {
	close(j.ShutChan) // notifies Schedule()
}

func (j *statCronjob) OnConfigure() {
	// TODO
}
func (j *statCronjob) OnPrepare() {
	// TODO
}

func (j *statCronjob) Schedule() { // runner
	j.LoopRun(time.Minute, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("statCronjob=%s done\n", j.Name())
	}
	j.stage.DecSub() // cronjob
}
