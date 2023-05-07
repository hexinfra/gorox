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
	servers []RPCServer // linked rpc servers. may be empty
	// States
	hostnames       [][]byte // ...
	accessLog       []string // (file, rotate)
	logFormat       string   // log format
	logger          *logger  // svc access logger
	maxContentSize  int64    // max content size allowed
	exactHostnames  [][]byte // like: ("example.com")
	suffixHostnames [][]byte // like: ("*.example.com")
	prefixHostnames [][]byte // like: ("www.example.*")
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
		return errors.New(".maxContentSize is an invalid value")
	}, _16M)
}
func (s *Svc) OnPrepare() {
	if s.accessLog != nil {
		//s.logger = newLogger(s.accessLog[0], s.accessLog[1])
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
	// TODO
	if s.logger != nil {
		//s.logger.log(str)
	}
}
func (s *Svc) Logln(str string) {
	// TODO
	if s.logger != nil {
		//s.logger.logln(str)
	}
}
func (s *Svc) Logf(format string, args ...any) {
	// TODO
	if s.logger != nil {
		//s.logger.logf(format, args...)
	}
}

func (s *Svc) LinkServer(server RPCServer) { s.servers = append(s.servers, server) }
func (s *Svc) Servers() []RPCServer        { return s.servers }

func (s *Svc) maintain() { // goroutine
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("svc=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *Svc) dispatch(req RPCRequest, resp RPCResponse) {
	// TODO
}
