// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis stater implementation.

package redis

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/contrib/backends/redis"
)

func init() {
	RegisterStater("redisStater", func(name string, stage *Stage) Stater {
		s := new(redisStater)
		s.onCreate(name, stage)
		return s
	})
}

// redisStater
type redisStater struct {
	// Parent
	Stater_
	// Assocs
	stage *Stage // current stage
	// States
	nodes []string
}

func (s *redisStater) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *redisStater) OnShutdown() {
	close(s.ShutChan) // notifies Maintain()
}

func (s *redisStater) OnConfigure() {
	// TODO
}
func (s *redisStater) OnPrepare() {
	// TODO
}

func (s *redisStater) Maintain() { // runner
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("redisStater=%s done\n", s.Name())
	}
	s.stage.DecSub()
}

func (s *redisStater) Set(sid []byte, session *Session) {
}
func (s *redisStater) Get(sid []byte) (session *Session) {
	return
}
func (s *redisStater) Del(sid []byte) bool {
	return false
}
