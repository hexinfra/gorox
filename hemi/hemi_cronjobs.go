// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Cronjobs are background tasks that are scheduled to run periodically.

package hemi

import (
	"time"
)

// Cronjob component
type Cronjob interface {
	// Imports
	Component
	// Methods
	Schedule() // runner
}

// Cronjob_ is a parent.
type Cronjob_ struct { // for all cronjobs
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (j *Cronjob_) OnCreate(compName string, stage *Stage) {
	j.MakeComp(compName)
	j.stage = stage
}

func (j *Cronjob_) Stage() *Stage { return j.stage }

func init() {
	RegisterCronjob("statCronjob", func(compName string, stage *Stage) Cronjob {
		j := new(statCronjob)
		j.onCreate(compName, stage)
		return j
	})
}

// statCronjob reports statistics about current stage.
type statCronjob struct {
	// Parent
	Cronjob_
	// States
}

func (j *statCronjob) onCreate(compName string, stage *Stage) {
	j.Cronjob_.OnCreate(compName, stage)
}
func (j *statCronjob) OnShutdown() { close(j.ShutChan) } // notifies Schedule()

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
		Printf("statCronjob=%s done\n", j.CompName())
	}
	j.stage.DecCronjob()
}
