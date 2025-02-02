// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Filesystem hstate implementation.

package filesys

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHstate("filesysHstate", func(compName string, stage *Stage) Hstate {
		s := new(filesysHstate)
		s.onCreate(compName, stage)
		return s
	})
}

// filesysHstate
type filesysHstate struct {
	// Parent
	Hstate_
	// States
	stateDir string // /path/to/dir
}

func (s *filesysHstate) onCreate(compName string, stage *Stage) {
	s.Hstate_.OnCreate(compName, stage)
}
func (s *filesysHstate) OnShutdown() { close(s.ShutChan) } // notifies Maintain()

func (s *filesysHstate) OnConfigure() {
	// .stateDir
	s.ConfigureString("stateDir", &s.stateDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".stateDir has an invalid value")
	}, VarDir()+"/hstates/"+s.CompName())
}
func (s *filesysHstate) OnPrepare() {
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (s *filesysHstate) Maintain() { // runner
	s.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("filesysHstate=%s done\n", s.CompName())
	}
	s.Stage().DecSub() // hstate
}

func (s *filesysHstate) Set(sid []byte, sess *Session) error {
	return nil
}
func (s *filesysHstate) Get(sid []byte) (sess *Session, err error) {
	return
}
func (s *filesysHstate) Del(sid []byte) error {
	return nil
}
