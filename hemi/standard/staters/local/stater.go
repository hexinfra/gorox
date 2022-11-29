// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Local stater implementation.

package local

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi/internal"
	"os"
	"time"
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
	s.CompInit(name)
	s.stage = stage
}

func (s *localStater) OnConfigure() {
	// stateDir
	s.ConfigureString("stateDir", &s.stateDir, func(value string) bool { return value != "" }, VarsDir()+"/staters/"+s.Name())
}
func (s *localStater) OnPrepare() {
	// mkdirs
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (s *localStater) OnShutdown() {
	s.Shutdown()
}

func (s *localStater) Maintain() { // goroutine
	Loop(time.Second, s.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Printf("localStater=%s done\n", s.Name())
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
