// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HRPC server implementation.

package hemi

import (
	"bytes"
	"errors"
	"time"
)

func init() {
	RegisterServer("hrpcServer", func(name string, stage *Stage) Server {
		s := new(hrpcServer)
		s.onCreate(name, stage)
		return s
	})
}

// hrpcServer is the HRPC server.
type hrpcServer struct {
	// Parent
	Server_[*hrpcGate]
	// Assocs
	defaultService *Service // default service if not found
	// States
	forServices    []string                // for what services
	exactServices  []*hostnameTo[*Service] // like: ("example.com")
	suffixServices []*hostnameTo[*Service] // like: ("*.example.com")
	prefixServices []*hostnameTo[*Service] // like: ("www.example.*")
	recvTimeout    time.Duration           // timeout to recv the whole message content
	sendTimeout    time.Duration           // timeout to send the whole message
}

func (s *hrpcServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}
func (s *hrpcServer) OnShutdown() {
	s.Server_.OnShutdown()
}

func (s *hrpcServer) OnConfigure() {
	s.Server_.OnConfigure()

	// forServices
	s.ConfigureStringList("forServices", &s.forServices, nil, []string{})

	// recvTimeout
	s.ConfigureDuration("recvTimeout", &s.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, 60*time.Second)

	// sendTimeout
	s.ConfigureDuration("sendTimeout", &s.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, 60*time.Second)
}
func (s *hrpcServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *hrpcServer) BindServices() {
	for _, serviceName := range s.forServices {
		service := s.stage.Service(serviceName)
		if service == nil {
			continue
		}
		service.BindServer(s)
		// TODO: use hash table?
		for _, hostname := range service.exactHostnames {
			s.exactServices = append(s.exactServices, &hostnameTo[*Service]{hostname, service})
		}
		// TODO: use radix trie?
		for _, hostname := range service.suffixHostnames {
			s.suffixServices = append(s.suffixServices, &hostnameTo[*Service]{hostname, service})
		}
		// TODO: use radix trie?
		for _, hostname := range service.prefixHostnames {
			s.prefixServices = append(s.prefixServices, &hostnameTo[*Service]{hostname, service})
		}
	}
}
func (s *hrpcServer) findService(hostname []byte) *Service {
	// TODO: use hash table?
	for _, exactMap := range s.exactServices {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixServices {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixServices {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return nil
}

func (s *hrpcServer) Serve() { // runner
	// TODO
}

// hrpcGate is a gate of hrpcServer.
type hrpcGate struct {
	// Parent
	Gate_
	// Assocs
	// States
}

func (g *hrpcGate) init(id int32, server *hrpcServer) {
	g.Gate_.Init(id, server)
}

func (g *hrpcGate) Open() error {
	// TODO
	return nil
}
func (g *hrpcGate) _openUnix() error {
	// TODO
	return nil
}
func (g *hrpcGate) _openInet() error {
	// TODO
	return nil
}
func (g *hrpcGate) Shut() error {
	g.MarkShut()
	// TODO // breaks serve()
	return nil
}

func (g *hrpcGate) serve() { // runner
	// TODO
}

// hrpcConn
type hrpcConn struct {
	// Parent
	ServerConn_
}

func (c *hrpcConn) onGet(id int64, gate *hrpcGate) {
	c.ServerConn_.OnGet(id, gate)
}
func (c *hrpcConn) onPut() {
	c.ServerConn_.OnPut()
}

func (c *hrpcConn) rpcServer() *hrpcServer { return c.Server().(*hrpcServer) }

// Service is the HRPC service.
type Service struct {
	// Parent
	Component_
	// Assocs
	stage   *Stage        // current stage
	stater  Stater        // the stater which is used by this service
	servers []*hrpcServer // bound hrpc servers. may be empty
	// States
	hostnames       [][]byte           // ...
	accessLog       *logcfg            // ...
	logger          *logger            // service access logger
	maxContentSize  int64              // max content size allowed
	exactHostnames  [][]byte           // like: ("example.com")
	suffixHostnames [][]byte           // like: ("*.example.com")
	prefixHostnames [][]byte           // like: ("www.example.*")
	bundlets        map[string]Bundlet // ...
}

func (s *Service) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *Service) OnShutdown() {
	close(s.ShutChan) // notifies maintain() which shutdown sub components
}

func (s *Service) OnConfigure() {
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
func (s *Service) OnPrepare() {
	if s.accessLog != nil {
		//s.logger = newLogger(s.accessLog.logFile, s.accessLog.rotate)
	}

	initsLock.RLock()
	serviceInit := serviceInits[s.name]
	initsLock.RUnlock()
	if serviceInit != nil {
		if err := serviceInit(s); err != nil {
			UseExitln(err.Error())
		}
	}
}

func (s *Service) Log(str string) {
	if s.logger != nil {
		s.logger.Log(str)
	}
}
func (s *Service) Logln(str string) {
	if s.logger != nil {
		s.logger.Logln(str)
	}
}
func (s *Service) Logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Logf(format, args...)
	}
}

func (s *Service) BindServer(server *hrpcServer) { s.servers = append(s.servers, server) }
func (s *Service) Servers() []*hrpcServer        { return s.servers }

func (s *Service) maintain() { // runner
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("service=%s done\n", s.Name())
	}
	s.stage.DecSub()
}

/*
func (s *Service) dispatch(exchan) {
	// TODO
}
*/

// Bundlet is a collection of related procedures in a service. A service has many bundlets.
// Bundlets are not components.
type Bundlet interface {
}

// Bundlet_ is the parent for all bundlets.
type Bundlet_ struct {
}

/*
func (b *Bundlet_) dispatch(exchan) {
}
*/
