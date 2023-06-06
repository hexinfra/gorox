// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Local storer implementation.

package local

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterStorer("localStorer", func(name string, stage *Stage) Storer {
		s := new(localStorer)
		s.onCreate(name, stage)
		return s
	})
}

// localStorer
type localStorer struct {
	// Mixins
	Storer_
	// Assocs
	stage *Stage
	// States
	storeDir string // /path/to/dir
}

func (s *localStorer) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *localStorer) OnShutdown() {
	close(s.Shut)
}

func (s *localStorer) OnConfigure() {
	// storeDir
	s.ConfigureString("storeDir", &s.storeDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".storeDir has an invalid value")
	}, VarsDir()+"/storers/"+s.Name())
}
func (s *localStorer) OnPrepare() {
	// mkdirs
	if err := os.MkdirAll(s.storeDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (s *localStorer) Maintain() { // goroutine
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Printf("localStorer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *localStorer) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (s *localStorer) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (s *localStorer) Del(key []byte) bool {
	// TODO
	return false
}
