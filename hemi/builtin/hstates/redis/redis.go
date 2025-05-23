// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Redis hstate implementation.

package redis

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/library/connectors/redis"
)

func init() {
	RegisterHstate("redisHstate", func(compName string, stage *Stage) Hstate {
		s := new(redisHstate)
		s.onCreate(compName, stage)
		return s
	})
}

// redisHstate
type redisHstate struct {
	// Parent
	Hstate_
	// States
	nodes []string
}

func (s *redisHstate) onCreate(compName string, stage *Stage) {
	s.Hstate_.OnCreate(compName, stage)
}
func (s *redisHstate) OnShutdown() { close(s.ShutChan) } // notifies Maintain()

func (s *redisHstate) OnConfigure() {
	// TODO
}
func (s *redisHstate) OnPrepare() {
	// TODO
}

func (s *redisHstate) Maintain() { // runner
	s.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("redisHstate=%s done\n", s.CompName())
	}
	s.Stage().DecHstate()
}

func (s *redisHstate) Set(sid []byte, sess *Session) error {
	return nil
}
func (s *redisHstate) Get(sid []byte) (sess *Session, err error) {
	return
}
func (s *redisHstate) Del(sid []byte) error {
	return nil
}
