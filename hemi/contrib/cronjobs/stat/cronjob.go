// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Stat cronjobs report statistics about current stage.

package stat

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
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
	// Mixins
	Cronjob_
	// Assocs
	stage *Stage
	// States
}

func (j *statCronjob) onCreate(name string, stage *Stage) {
	j.MakeComp(name)
	j.stage = stage
}
func (j *statCronjob) OnShutdown() {
	close(j.Shut)
}

func (j *statCronjob) OnConfigure() {
}
func (j *statCronjob) OnPrepare() {
}

func (j *statCronjob) Schedule() { // goroutine
	Loop(time.Minute, j.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("statCronjob=%s done\n", j.Name())
	}
	j.stage.SubDone()
}
