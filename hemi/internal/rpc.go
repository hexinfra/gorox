// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// RPC service and related components.

package internal

import (
	"errors"
	"time"
)

// Svc is the RPC service.
type Svc struct {
	// Mixins
	Component_
	// Assocs
	stage   *Stage      // current stage
	stater  Stater      // the stater which is used by this svc
	servers []rpcServer // bound rpc servers. may be empty
	// States
	hostnames       [][]byte           // ...
	accessLog       *logcfg            // ...
	logger          *logger            // svc access logger
	maxContentSize  int64              // max content size allowed
	exactHostnames  [][]byte           // like: ("example.com")
	suffixHostnames [][]byte           // like: ("*.example.com")
	prefixHostnames [][]byte           // like: ("www.example.*")
	bundlets        map[string]Bundlet // ...
}

func (s *Svc) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *Svc) OnShutdown() {
	close(s.Shut)
}

func (s *Svc) OnConfigure() {
	// withStater
	if v, ok := s.Find("withStater"); ok {
		if name, ok := v.String(); ok && name != "" {
			if stater := s.stage.Stater(name); stater == nil {
				UseExitf("unknown stater: '%s'\n", name)
			} else {
				s.stater = stater
			}
		} else {
			UseExitln("invalid withStater")
		}
	}

	// maxContentSize
	s.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) error {
		if value > 0 && value <= _1G {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _16M)
}
func (s *Svc) OnPrepare() {
	if s.accessLog != nil {
		//s.logger = newLogger(s.accessLog.logFile, s.accessLog.rotate)
	}

	initsLock.RLock()
	svcInit := svcInits[s.name]
	initsLock.RUnlock()
	if svcInit != nil {
		if err := svcInit(s); err != nil {
			UseExitln(err.Error())
		}
	}
}

func (s *Svc) Log(str string) {
	if s.logger != nil {
		s.logger.Log(str)
	}
}
func (s *Svc) Logln(str string) {
	if s.logger != nil {
		s.logger.Logln(str)
	}
}
func (s *Svc) Logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Logf(format, args...)
	}
}

func (s *Svc) BindServer(server rpcServer) { s.servers = append(s.servers, server) }
func (s *Svc) Servers() []rpcServer        { return s.servers }

func (s *Svc) maintain() { // goroutine
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Printf("svc=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *Svc) dispatch(req Req, resp Resp) {
	// TODO
}

// Bundlet is a bundle of procedures in Svc. Bundlets are not components.
type Bundlet interface {
}
