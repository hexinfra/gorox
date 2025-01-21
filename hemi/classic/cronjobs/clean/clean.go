// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Clean cronjobs clean old logs.

package clean

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterCronjob("cleanCronjob", func(compName string, stage *Stage) Cronjob {
		j := new(cleanCronjob)
		j.onCreate(compName, stage)
		return j
	})
}

// cleanCronjob
type cleanCronjob struct {
	// Parent
	Cronjob_
	// States
}

func (j *cleanCronjob) onCreate(compName string, stage *Stage) {
	j.Cronjob_.OnCreate(compName, stage)
}
func (j *cleanCronjob) OnShutdown() { close(j.ShutChan) } // notifies Schedule()

func (j *cleanCronjob) OnConfigure() {
	// TODO
}
func (j *cleanCronjob) OnPrepare() {
	// TODO
}

func (j *cleanCronjob) Schedule() { // runner
	j.LoopRun(time.Minute, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("cleanCronjob=%s done\n", j.CompName())
	}
	j.Stage().DecSub() // cronjob
}
