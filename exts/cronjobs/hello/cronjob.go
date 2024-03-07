// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello cronjob showing how to use Gorox to host a cronjob.

package hello

import (
	"fmt"
	"time"

	. "github.com/hexinfra/gorox/hemi"
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
	// Parent
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
	close(j.ShutChan) // notifies Schedule()
}

func (j *helloCronjob) OnConfigure() {
}
func (j *helloCronjob) OnPrepare() {
}

func (j *helloCronjob) Schedule() { // runner
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-j.ShutChan:
			break loop
		case now := <-ticker.C:
			fmt.Printf("hello, gorox! time=%s\n", now.String())
		}
	}
	if Debug() >= 2 {
		Printf("helloCronjob=%s done\n", j.Name())
	}
	j.stage.DecSub()
}
