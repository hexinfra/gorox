// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis stater implementation.

package redis

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/classic/backends/redis"
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
	s.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("redisStater=%s done\n", s.Name())
	}
	s.stage.DecSub() // stater
}

func (s *redisStater) Set(sid []byte, session *Session) error {
	return nil
}
func (s *redisStater) Get(sid []byte) (session *Session, err error) {
	return
}
func (s *redisStater) Del(sid []byte) error {
	return nil
}
