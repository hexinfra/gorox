// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Local stater implementation.

package local

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterStater("localStater", func(name string, stage *Stage) Stater {
		s := new(localStater)
		s.onCreate(name, stage)
		return s
	})
}

// localStater
type localStater struct {
	// Mixins
	Stater_
	// Assocs
	stage *Stage
	// States
	stateDir string // /path/to/dir
}

func (s *localStater) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *localStater) OnShutdown() {
	close(s.Shut)
}

func (s *localStater) OnConfigure() {
	// stateDir
	s.ConfigureString("stateDir", &s.stateDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".stateDir is a invalid value")
	}, VarsDir()+"/staters/"+s.Name())
}
func (s *localStater) OnPrepare() {
	// mkdirs
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (s *localStater) Maintain() { // goroutine
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("localStater=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *localStater) Set(sid []byte, session *Session) {
}
func (s *localStater) Get(sid []byte) (session *Session) {
	return
}
func (s *localStater) Del(sid []byte) bool {
	return false
}
