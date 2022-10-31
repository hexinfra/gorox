// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis stater implementation.

package redis

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterStater("redisStater", func(name string, stage *Stage) Stater {
		s := new(redisStater)
		s.init(name, stage)
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

func (s *redisStater) init(name string, stage *Stage) {
	s.SetName(name)
	s.stage = stage
}

func (s *redisStater) OnConfigure() {
}
func (s *redisStater) OnPrepare() {
}
func (s *redisStater) OnShutdown() {
}

func (s *redisStater) Maintain() { // blocking
}

func (s *redisStater) Set(sid []byte, session *Session) {
}
func (s *redisStater) Get(sid []byte) (session *Session) {
	return
}
func (s *redisStater) Del(sid []byte) bool {
	return false
}
