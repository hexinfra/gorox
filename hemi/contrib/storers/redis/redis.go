// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis storer implementation.

package redis

import (
	"time"

	. "github.com/hexinfra/gorox/hemi/internal"

	_ "github.com/hexinfra/gorox/hemi/contrib/backends/redis"
)

func init() {
	RegisterStorer("redisStorer", func(name string, stage *Stage) Storer {
		s := new(redisStorer)
		s.onCreate(name, stage)
		return s
	})
}

// redisStorer
type redisStorer struct {
	// Mixins
	Storer_
	// Assocs
	stage *Stage
	// States
	nodes []string
}

func (s *redisStorer) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *redisStorer) OnShutdown() {
	close(s.Shut)
}

func (s *redisStorer) OnConfigure() {
	// TODO
}
func (s *redisStorer) OnPrepare() {
	// TODO
}

func (s *redisStorer) Maintain() { // goroutine
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Printf("redisStorer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *redisStorer) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (s *redisStorer) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (s *redisStorer) Del(key []byte) bool {
	// TODO
	return false
}
