// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello cronjob showing how to use Gorox to host a cronjob.

package hello

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func init() {
	RegisterCronjob("helloCronjob", func(name string, stage *Stage) Cronjob {
		j := new(helloCronjob)
		j.onCreate(name, stage)
		return j
	})
}

// helloCronjob
type helloCronjob struct {
	// Mixins
	Cronjob_
	// Assocs
	stage *Stage
	// States
}

func (j *helloCronjob) onCreate(name string, stage *Stage) {
	j.MakeComp(name)
	j.stage = stage
}
func (j *helloCronjob) OnShutdown() {
	close(j.Shut)
}

func (j *helloCronjob) OnConfigure() {}
func (j *helloCronjob) OnPrepare()   {}

func (j *helloCronjob) Schedule() { // goroutine
	Loop(time.Second, j.Shut, func(now time.Time) {
		fmt.Println("hello, gorox!")
	})
	if IsDebug(2) {
		Debugf("helloCronjob=%s done\n", j.Name())
	}
	j.stage.SubDone()
}
