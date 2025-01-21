// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Local hstate implementation.

package local

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHstate("localHstate", func(compName string, stage *Stage) Hstate {
		s := new(localHstate)
		s.onCreate(compName, stage)
		return s
	})
}

// localHstate
type localHstate struct {
	// Parent
	Hstate_
	// States
	stateDir string // /path/to/dir
}

func (s *localHstate) onCreate(compName string, stage *Stage) {
	s.Hstate_.OnCreate(compName, stage)
}
func (s *localHstate) OnShutdown() { close(s.ShutChan) } // notifies Maintain()

func (s *localHstate) OnConfigure() {
	// .stateDir
	s.ConfigureString("stateDir", &s.stateDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".stateDir has an invalid value")
	}, VarDir()+"/hstates/"+s.CompName())
}
func (s *localHstate) OnPrepare() {
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (s *localHstate) Maintain() { // runner
	s.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("localHstate=%s done\n", s.CompName())
	}
	s.Stage().DecSub() // hstate
}

func (s *localHstate) Set(sid []byte, session *Session) error {
	return nil
}
func (s *localHstate) Get(sid []byte) (session *Session, err error) {
	return
}
func (s *localHstate) Del(sid []byte) error {
	return nil
}
