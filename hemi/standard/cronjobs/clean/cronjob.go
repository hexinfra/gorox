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
		j.onCreate(name, stage)
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

func (j *cleanCronjob) onCreate(name string, stage *Stage) {
	j.CompInit(name)
	j.stage = stage
}
func (j *cleanCronjob) OnShutdown() {
	j.Shutdown()
}

func (j *cleanCronjob) OnConfigure() {
}
func (j *cleanCronjob) OnPrepare() {
}

func (j *cleanCronjob) Schedule() { // goroutine
	Loop(time.Minute, j.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Printf("cleanCronjob=%s done\n", j.Name())
	}
	j.stage.SubDone()
}
