// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Memory storer implementation.

package mem

import (
	"time"

	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterStorer("memStorer", func(name string, stage *Stage) Storer {
		s := new(memStorer)
		s.onCreate(name, stage)
		return s
	})
}

// memStorer
type memStorer struct {
	// Mixins
	Storer_
	// Assocs
	stage *Stage
	// States
}

func (s *memStorer) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *memStorer) OnShutdown() {
	close(s.Shut)
}

func (s *memStorer) OnConfigure() {
	// TODO
}
func (s *memStorer) OnPrepare() {
	// TODO
}

func (s *memStorer) Maintain() { // goroutine
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Printf("memStorer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *memStorer) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (s *memStorer) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (s *memStorer) Del(key []byte) bool {
	// TODO
	return false
}
