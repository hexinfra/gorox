// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Cronjobs are background tasks that are scheduled to run periodically.

package hemi

// Cronjob component
type Cronjob interface {
	// Imports
	Component
	// Methods
	Schedule() // runner
}

// Cronjob_ is the parent for all cronjobs.
type Cronjob_ struct {
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
