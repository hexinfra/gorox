// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Clean cronjobs clean old logs.

package clean

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterCronjob("cleanCronjob", func(name string, stage *Stage) Cronjob {
		j := new(cleanCronjob)
		j.onCreate(name, stage)
		return j
	})
}

// cleanCronjob
type cleanCronjob struct {
	// Parent
	Cronjob_
	// Assocs
	stage *Stage // current stage
	// States
}

func (j *cleanCronjob) onCreate(name string, stage *Stage) {
	j.MakeComp(name)
	j.stage = stage
}
func (j *cleanCronjob) OnShutdown() {
	close(j.ShutChan) // notifies Schedule()
}

func (j *cleanCronjob) OnConfigure() {
	// TODO
}
func (j *cleanCronjob) OnPrepare() {
	// TODO
}

func (j *cleanCronjob) Schedule() { // runner
	j.Loop(time.Minute, func(now time.Time) {
		// TODO
	})
	if DbgLevel() >= 2 {
		Printf("cleanCronjob=%s done\n", j.Name())
	}
	j.stage.DecSub()
}
