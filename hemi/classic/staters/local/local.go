// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Local stater implementation.

package local

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterStater("localStater", func(compName string, stage *Stage) Stater {
		s := new(localStater)
		s.onCreate(compName, stage)
		return s
	})
}

// localStater
type localStater struct {
	// Parent
	Stater_
	// Assocs
	stage *Stage // current stage
	// States
	stateDir string // /path/to/dir
}

func (s *localStater) onCreate(compName string, stage *Stage) {
	s.MakeComp(compName)
	s.stage = stage
}
func (s *localStater) OnShutdown() {
	close(s.ShutChan) // notifies Maintain()
}

func (s *localStater) OnConfigure() {
	// stateDir
	s.ConfigureString("stateDir", &s.stateDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".stateDir has an invalid value")
	}, VarDir()+"/staters/"+s.CompName())
}
func (s *localStater) OnPrepare() {
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (s *localStater) Maintain() { // runner
	s.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("localStater=%s done\n", s.CompName())
	}
	s.stage.DecSub() // stater
}

func (s *localStater) Set(sid []byte, session *Session) error {
	return nil
}
func (s *localStater) Get(sid []byte) (session *Session, err error) {
	return
}
func (s *localStater) Del(sid []byte) error {
	return nil
}
