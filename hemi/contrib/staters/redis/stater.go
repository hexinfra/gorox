// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis stater implementation.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"time"
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
	// Mixins
	Stater_
	// Assocs
	stage *Stage
	// States
	nodes []string
}

func (s *redisStater) onCreate(name string, stage *Stage) {
	s.CompInit(name)
	s.stage = stage
}
func (s *redisStater) OnShutdown() {
	close(s.Shut)
}

func (s *redisStater) OnConfigure() {
}
func (s *redisStater) OnPrepare() {
}

func (s *redisStater) Maintain() { // goroutine
	Loop(time.Second, s.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("redisStater=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *redisStater) Set(sid []byte, session *Session) {
}
func (s *redisStater) Get(sid []byte) (session *Session) {
	return
}
func (s *redisStater) Del(sid []byte) bool {
	return false
}
