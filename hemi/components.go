// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Component is the configurable component.

package hemi

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
	"unsafe"
)

const ( // list of component types
	compTypeStage      int16 = 1 + iota // stage
	compTypeFixture                     // clock, fcache, resolv, ...
	compTypeBackend                     // http1Backend, quixBackend, udpxBackend, ...
	compTypeNode                        // node
	compTypeQUIXRouter                  // quixRouter
	compTypeQUIXDealet                  // quixProxy, ...
	compTypeTCPXRouter                  // tcpxRouter
	compTypeTCPXDealet                  // tcpxProxy, redisProxy, ...
	compTypeUDPXRouter                  // udpxRouter
	compTypeUDPXDealet                  // udpxProxy, dnsProxy, ...
	compTypeCase                        // case
	compTypeService                     // service
	compTypeHstate                      // localHstate, redisHstate, ...
	compTypeHcache                      // localHcache, redisHcache, ...
	compTypeWebapp                      // webapp
	compTypeHandlet                     // static, httpProxy, ...
	compTypeReviser                     // gzipReviser, wrapReviser, ...
	compTypeSocklet                     // helloSocklet, ...
	compTypeRule                        // rule
	compTypeServer                      // httpxServer, hrpcServer, echoServer, ...
	compTypeCronjob                     // statCronjob, cleanCronjob, ...
)

var signedComps = map[string]int16{ // signed comps. more dynamic comps are signed using _signComp() below
	"stage":      compTypeStage,
	"node":       compTypeNode,
	"quixRouter": compTypeQUIXRouter,
	"tcpxRouter": compTypeTCPXRouter,
	"udpxRouter": compTypeUDPXRouter,
	"case":       compTypeCase,
	"service":    compTypeService,
	"webapp":     compTypeWebapp,
	"rule":       compTypeRule,
}

func _signComp(compSign string, compType int16) {
	if signedType, ok := signedComps[compSign]; ok {
		BugExitf("conflicting component sign: compType=%d compSign=%s\n", signedType, compSign)
	}
	signedComps[compSign] = compType
}

var fixtureSigns = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required

func registerFixture(compSign string) {
	if _, ok := fixtureSigns[compSign]; ok {
		BugExitln("fixture sign conflicted")
	}
	fixtureSigns[compSign] = true
	_signComp(compSign, compTypeFixture)
}

var ( // component creators
	creatorsLock       sync.RWMutex
	backendCreators    = make(map[string]func(compName string, stage *Stage) Backend) // indexed by compSign, same below.
	hstateCreators     = make(map[string]func(compName string, stage *Stage) Hstate)
	hcacheCreators     = make(map[string]func(compName string, stage *Stage) Hcache)
	serverCreators     = make(map[string]func(compName string, stage *Stage) Server)
	cronjobCreators    = make(map[string]func(compName string, stage *Stage) Cronjob)
	quixDealetCreators = make(map[string]func(compName string, stage *Stage, router *QUIXRouter) QUIXDealet)
	tcpxDealetCreators = make(map[string]func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet)
	udpxDealetCreators = make(map[string]func(compName string, stage *Stage, router *UDPXRouter) UDPXDealet)
	handletCreators    = make(map[string]func(compName string, stage *Stage, webapp *Webapp) Handlet)
	reviserCreators    = make(map[string]func(compName string, stage *Stage, webapp *Webapp) Reviser)
	sockletCreators    = make(map[string]func(compName string, stage *Stage, webapp *Webapp) Socklet)
)

func RegisterBackend(compSign string, create func(compName string, stage *Stage) Backend) {
	_registerComponent0(compSign, compTypeBackend, backendCreators, create)
}
func RegisterHstate(compSign string, create func(compName string, stage *Stage) Hstate) {
	_registerComponent0(compSign, compTypeHstate, hstateCreators, create)
}
func RegisterHcache(compSign string, create func(compName string, stage *Stage) Hcache) {
	_registerComponent0(compSign, compTypeHcache, hcacheCreators, create)
}
func RegisterServer(compSign string, create func(compName string, stage *Stage) Server) {
	_registerComponent0(compSign, compTypeServer, serverCreators, create)
}
func RegisterCronjob(compSign string, create func(compName string, stage *Stage) Cronjob) {
	_registerComponent0(compSign, compTypeCronjob, cronjobCreators, create)
}
func _registerComponent0[T Component](compSign string, compType int16, creators map[string]func(string, *Stage) T, create func(string, *Stage) T) { // backend, hstate, hcache, server, cronjob
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[compSign]; ok {
		BugExitln("component0 sign conflicted")
	}
	creators[compSign] = create
	_signComp(compSign, compType)
}

func RegisterQUIXDealet(compSign string, create func(compName string, stage *Stage, router *QUIXRouter) QUIXDealet) {
	_registerComponent1(compSign, compTypeQUIXDealet, quixDealetCreators, create)
}
func RegisterTCPXDealet(compSign string, create func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet) {
	_registerComponent1(compSign, compTypeTCPXDealet, tcpxDealetCreators, create)
}
func RegisterUDPXDealet(compSign string, create func(compName string, stage *Stage, router *UDPXRouter) UDPXDealet) {
	_registerComponent1(compSign, compTypeUDPXDealet, udpxDealetCreators, create)
}
func RegisterHandlet(compSign string, create func(compName string, stage *Stage, webapp *Webapp) Handlet) {
	_registerComponent1(compSign, compTypeHandlet, handletCreators, create)
}
func RegisterReviser(compSign string, create func(compName string, stage *Stage, webapp *Webapp) Reviser) {
	_registerComponent1(compSign, compTypeReviser, reviserCreators, create)
}
func RegisterSocklet(compSign string, create func(compName string, stage *Stage, webapp *Webapp) Socklet) {
	_registerComponent1(compSign, compTypeSocklet, sockletCreators, create)
}
func _registerComponent1[T Component, C Component](compSign string, compType int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // dealet, handlet, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[compSign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[compSign] = create
	_signComp(compSign, compType)
}

var ( // initializers of services & webapps
	initsLock    sync.RWMutex
	serviceInits = make(map[string]func(service *Service) error) // indexed by compName, same below.
	webappInits  = make(map[string]func(webapp *Webapp) error)
)

func RegisterServiceInit(compName string, init func(service *Service) error) {
	initsLock.Lock()
	serviceInits[compName] = init
	initsLock.Unlock()
}
func RegisterWebappInit(compName string, init func(webapp *Webapp) error) {
	initsLock.Lock()
	webappInits[compName] = init
	initsLock.Unlock()
}

// Component is the interface for all components.
type Component interface {
	MakeComp(compName string)
	CompName() string

	OnConfigure()
	Find(propName string) (propValue Value, ok bool)
	Prop(propName string) (propValue Value, ok bool)
	ConfigureBool(propName string, prop *bool, defaultValue bool)
	ConfigureInt64(propName string, prop *int64, check func(value int64) error, defaultValue int64)
	ConfigureInt32(propName string, prop *int32, check func(value int32) error, defaultValue int32)
	ConfigureInt16(propName string, prop *int16, check func(value int16) error, defaultValue int16)
	ConfigureInt8(propName string, prop *int8, check func(value int8) error, defaultValue int8)
	ConfigureInt(propName string, prop *int, check func(value int) error, defaultValue int)
	ConfigureString(propName string, prop *string, check func(value string) error, defaultValue string)
	ConfigureBytes(propName string, prop *[]byte, check func(value []byte) error, defaultValue []byte)
	ConfigureDuration(propName string, prop *time.Duration, check func(value time.Duration) error, defaultValue time.Duration)
	ConfigureStringList(propName string, prop *[]string, check func(value []string) error, defaultValue []string)
	ConfigureBytesList(propName string, prop *[][]byte, check func(value [][]byte) error, defaultValue [][]byte)
	ConfigureStringDict(propName string, prop *map[string]string, check func(value map[string]string) error, defaultValue map[string]string)

	OnPrepare()

	OnShutdown()
	DecSub() // called by sub components or objects of this component

	setName(compName string)
	setShell(shell Component)
	setParent(parent Component)
	getParent() Component
	setInfo(info any)
	setProp(propName string, propValue Value)
}

// Component_ is the parent for all components.
type Component_ struct {
	// Assocs
	shell  Component // the concrete Component
	parent Component // the parent component, used by configurator
	// States
	compName string           // main, proxy1, ...
	props    map[string]Value // name1=value1, ...
	ShutChan chan struct{}    // used to notify the component to shutdown
	subs     sync.WaitGroup   // sub components/objects to wait for
	info     any              // hold extra info about this component, used by configurator
}

func (c *Component_) MakeComp(compName string) {
	c.compName = compName
	c.props = make(map[string]Value)
	c.ShutChan = make(chan struct{})
}
func (c *Component_) CompName() string { return c.compName }

func (c *Component_) Find(propName string) (propValue Value, ok bool) {
	for component := c.shell; component != nil; component = component.getParent() {
		if propValue, ok = component.Prop(propName); ok {
			break
		}
	}
	return
}
func (c *Component_) Prop(propName string) (propValue Value, ok bool) {
	propValue, ok = c.props[propName]
	return
}

func (c *Component_) ConfigureBool(propName string, prop *bool, defaultValue bool) {
	_configureProp(c, propName, prop, (*Value).Bool, nil, defaultValue)
}
func (c *Component_) ConfigureInt64(propName string, prop *int64, check func(value int64) error, defaultValue int64) {
	_configureProp(c, propName, prop, (*Value).Int64, check, defaultValue)
}
func (c *Component_) ConfigureInt32(propName string, prop *int32, check func(value int32) error, defaultValue int32) {
	_configureProp(c, propName, prop, (*Value).Int32, check, defaultValue)
}
func (c *Component_) ConfigureInt16(propName string, prop *int16, check func(value int16) error, defaultValue int16) {
	_configureProp(c, propName, prop, (*Value).Int16, check, defaultValue)
}
func (c *Component_) ConfigureInt8(propName string, prop *int8, check func(value int8) error, defaultValue int8) {
	_configureProp(c, propName, prop, (*Value).Int8, check, defaultValue)
}
func (c *Component_) ConfigureInt(propName string, prop *int, check func(value int) error, defaultValue int) {
	_configureProp(c, propName, prop, (*Value).Int, check, defaultValue)
}
func (c *Component_) ConfigureString(propName string, prop *string, check func(value string) error, defaultValue string) {
	_configureProp(c, propName, prop, (*Value).String, check, defaultValue)
}
func (c *Component_) ConfigureBytes(propName string, prop *[]byte, check func(value []byte) error, defaultValue []byte) {
	_configureProp(c, propName, prop, (*Value).Bytes, check, defaultValue)
}
func (c *Component_) ConfigureDuration(propName string, prop *time.Duration, check func(value time.Duration) error, defaultValue time.Duration) {
	_configureProp(c, propName, prop, (*Value).Duration, check, defaultValue)
}
func (c *Component_) ConfigureStringList(propName string, prop *[]string, check func(value []string) error, defaultValue []string) {
	_configureProp(c, propName, prop, (*Value).StringList, check, defaultValue)
}
func (c *Component_) ConfigureBytesList(propName string, prop *[][]byte, check func(value [][]byte) error, defaultValue [][]byte) {
	_configureProp(c, propName, prop, (*Value).BytesList, check, defaultValue)
}
func (c *Component_) ConfigureStringDict(propName string, prop *map[string]string, check func(value map[string]string) error, defaultValue map[string]string) {
	_configureProp(c, propName, prop, (*Value).StringDict, check, defaultValue)
}
func _configureProp[T any](c *Component_, propName string, prop *T, conv func(*Value) (T, bool), check func(value T) error, defaultValue T) {
	if propValue, ok := c.Find(propName); ok {
		if value, ok := conv(&propValue); ok && check == nil {
			*prop = value
		} else if ok && check != nil {
			if err := check(value); err == nil {
				*prop = value
			} else {
				// TODO: line number
				UseExitln(fmt.Sprintf("%s is error in %s: %s", propName, c.compName, err.Error()))
			}
		} else {
			UseExitln(fmt.Sprintf("invalid %s in %s", propName, c.compName))
		}
	} else { // not found. use default value
		*prop = defaultValue
	}
}

func (c *Component_) IncSub()       { c.subs.Add(1) }
func (c *Component_) IncSubs(n int) { c.subs.Add(n) }
func (c *Component_) DecSub()       { c.subs.Done() }
func (c *Component_) DecSubs(n int) { c.subs.Add(-n) }
func (c *Component_) WaitSubs()     { c.subs.Wait() }

func (c *Component_) LoopRun(interval time.Duration, callback func(now time.Time)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ShutChan:
			return
		case now := <-ticker.C:
			callback(now)
		}
	}
}

func (c *Component_) setShell(shell Component)                 { c.shell = shell }
func (c *Component_) setParent(parent Component)               { c.parent = parent }
func (c *Component_) getParent() Component                     { return c.parent }
func (c *Component_) setName(compName string)                  { c.compName = compName }
func (c *Component_) setProp(propName string, propValue Value) { c.props[propName] = propValue }
func (c *Component_) setInfo(info any)                         { c.info = info }

// compDict
type compDict[T Component] map[string]T

func (d compDict[T]) walk(method func(T)) {
	for _, component := range d {
		method(component)
	}
}
func (d compDict[T]) goWalk(method func(T)) {
	for _, component := range d {
		go method(component)
	}
}

// Stage represents a running stage in the worker process.
//
// A worker process may have many stages in its lifetime, especially
// when new configuration is applied, a new stage is created, or the
// old one is told to quit.
type Stage struct {
	// Parent
	Component_
	// Assocs
	clock       *clockFixture         // for fast accessing
	fcache      *fcacheFixture        // for fast accessing
	resolv      *resolvFixture        // for fast accessing
	fixtures    compDict[fixture]     // indexed by compSign
	backends    compDict[Backend]     // indexed by compName
	quixRouters compDict[*QUIXRouter] // indexed by compName
	tcpxRouters compDict[*TCPXRouter] // indexed by compName
	udpxRouters compDict[*UDPXRouter] // indexed by compName
	services    compDict[*Service]    // indexed by compName
	hstates     compDict[Hstate]      // indexed by compName
	hcaches     compDict[Hcache]      // indexed by compName
	webapps     compDict[*Webapp]     // indexed by compName
	servers     compDict[Server]      // indexed by compName
	cronjobs    compDict[Cronjob]     // indexed by compName
	// States
	id      int32
	numCPU  int32
	cpuFile string
	hepFile string
	thrFile string
	grtFile string
	blkFile string
}

// createStage creates a new stage which runs alongside existing stage.
func createStage() *Stage {
	stage := new(Stage)
	stage.onCreate()
	stage.setShell(stage)
	return stage
}

func (s *Stage) onCreate() {
	s.MakeComp("stage")

	s.clock = createClock(s)
	s.fcache = createFcache(s)
	s.resolv = createResolv(s)
	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFcache] = s.fcache
	s.fixtures[signResolv] = s.resolv

	s.backends = make(compDict[Backend])
	s.quixRouters = make(compDict[*QUIXRouter])
	s.tcpxRouters = make(compDict[*TCPXRouter])
	s.udpxRouters = make(compDict[*UDPXRouter])
	s.services = make(compDict[*Service])
	s.hstates = make(compDict[Hstate])
	s.hcaches = make(compDict[Hcache])
	s.webapps = make(compDict[*Webapp])
	s.servers = make(compDict[Server])
	s.cronjobs = make(compDict[Cronjob])
}
func (s *Stage) OnShutdown() {
	if DebugLevel() >= 2 {
		Printf("stage id=%d shutdown start!!\n", s.id)
	}

	// Cronjobs
	s.IncSubs(len(s.cronjobs))
	s.cronjobs.goWalk(Cronjob.OnShutdown)
	s.WaitSubs()

	// Servers
	s.IncSubs(len(s.servers))
	s.servers.goWalk(Server.OnShutdown)
	s.WaitSubs()

	// Webapps
	s.IncSubs(len(s.webapps))
	s.webapps.goWalk((*Webapp).OnShutdown)
	s.WaitSubs()

	// Hcaches & Hstates
	s.IncSubs(len(s.hcaches) + len(s.hstates))
	s.hcaches.goWalk(Hcache.OnShutdown)
	s.hstates.goWalk(Hstate.OnShutdown)
	s.WaitSubs()

	// Services
	s.IncSubs(len(s.services))
	s.services.goWalk((*Service).OnShutdown)
	s.WaitSubs()

	// Routers
	s.IncSubs(len(s.udpxRouters) + len(s.tcpxRouters) + len(s.quixRouters))
	s.udpxRouters.goWalk((*UDPXRouter).OnShutdown)
	s.tcpxRouters.goWalk((*TCPXRouter).OnShutdown)
	s.quixRouters.goWalk((*QUIXRouter).OnShutdown)
	s.WaitSubs()

	// Backends
	s.IncSubs(len(s.backends))
	s.backends.goWalk(Backend.OnShutdown)
	s.WaitSubs()

	// Fixtures, manually one by one. Mind the order!

	s.IncSub() // fcache
	s.fcache.OnShutdown()
	s.WaitSubs()

	s.IncSub() // resolv
	s.resolv.OnShutdown()
	s.WaitSubs()

	s.IncSub() // clock
	s.clock.OnShutdown()
	s.WaitSubs()

	// Stage
	if DebugLevel() >= 2 {
		// TODO
		Println("stage close log file")
	}
}

func (s *Stage) OnConfigure() {
	tmpDir := TmpDir()

	// .cpuFile
	s.ConfigureString("cpuFile", &s.cpuFile, func(value string) error {
		if value == "" {
			return errors.New(".cpuFile has an invalid value")
		}
		return nil
	}, tmpDir+"/cpu.prof")

	// .hepFile
	s.ConfigureString("hepFile", &s.hepFile, func(value string) error {
		if value == "" {
			return errors.New(".hepFile has an invalid value")
		}
		return nil
	}, tmpDir+"/hep.prof")

	// .thrFile
	s.ConfigureString("thrFile", &s.thrFile, func(value string) error {
		if value == "" {
			return errors.New(".thrFile has an invalid value")
		}
		return nil
	}, tmpDir+"/thr.prof")

	// .grtFile
	s.ConfigureString("grtFile", &s.grtFile, func(value string) error {
		if value == "" {
			return errors.New(".grtFile has an invalid value")
		}
		return nil
	}, tmpDir+"/grt.prof")

	// .blkFile
	s.ConfigureString("blkFile", &s.blkFile, func(value string) error {
		if value == "" {
			return errors.New(".blkFile has an invalid value")
		}
		return nil
	}, tmpDir+"/blk.prof")

	// sub components
	s.fixtures.walk(fixture.OnConfigure)
	s.backends.walk(Backend.OnConfigure)
	s.quixRouters.walk((*QUIXRouter).OnConfigure)
	s.tcpxRouters.walk((*TCPXRouter).OnConfigure)
	s.udpxRouters.walk((*UDPXRouter).OnConfigure)
	s.services.walk((*Service).OnConfigure)
	s.hstates.walk(Hstate.OnConfigure)
	s.hcaches.walk(Hcache.OnConfigure)
	s.webapps.walk((*Webapp).OnConfigure)
	s.servers.walk(Server.OnConfigure)
	s.cronjobs.walk(Cronjob.OnConfigure)
}
func (s *Stage) OnPrepare() {
	for _, file := range []string{s.cpuFile, s.hepFile, s.thrFile, s.grtFile, s.blkFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			EnvExitln(err.Error())
		}
	}

	// sub components
	s.fixtures.walk(fixture.OnPrepare)
	s.backends.walk(Backend.OnPrepare)
	s.quixRouters.walk((*QUIXRouter).OnPrepare)
	s.tcpxRouters.walk((*TCPXRouter).OnPrepare)
	s.udpxRouters.walk((*UDPXRouter).OnPrepare)
	s.services.walk((*Service).OnPrepare)
	s.hstates.walk(Hstate.OnPrepare)
	s.hcaches.walk(Hcache.OnPrepare)
	s.webapps.walk((*Webapp).OnPrepare)
	s.servers.walk(Server.OnPrepare)
	s.cronjobs.walk(Cronjob.OnPrepare)
}

func (s *Stage) createBackend(compSign string, compName string) Backend {
	if s.Backend(compName) != nil {
		UseExitf("conflicting backend with a same component name '%s'\n", compName)
	}
	create, ok := backendCreators[compSign]
	if !ok {
		UseExitln("unknown backend type: " + compSign)
	}
	backend := create(compName, s)
	backend.setShell(backend)
	s.backends[compName] = backend
	return backend
}
func (s *Stage) createQUIXRouter(compName string) *QUIXRouter {
	if s.QUIXRouter(compName) != nil {
		UseExitf("conflicting quixRouter with a same component name '%s'\n", compName)
	}
	router := new(QUIXRouter)
	router.onCreate(compName, s)
	router.setShell(router)
	s.quixRouters[compName] = router
	return router
}
func (s *Stage) createTCPXRouter(compName string) *TCPXRouter {
	if s.TCPXRouter(compName) != nil {
		UseExitf("conflicting tcpxRouter with a same component name '%s'\n", compName)
	}
	router := new(TCPXRouter)
	router.onCreate(compName, s)
	router.setShell(router)
	s.tcpxRouters[compName] = router
	return router
}
func (s *Stage) createUDPXRouter(compName string) *UDPXRouter {
	if s.UDPXRouter(compName) != nil {
		UseExitf("conflicting udpxRouter with a same component name '%s'\n", compName)
	}
	router := new(UDPXRouter)
	router.onCreate(compName, s)
	router.setShell(router)
	s.udpxRouters[compName] = router
	return router
}
func (s *Stage) createService(compName string) *Service {
	if s.Service(compName) != nil {
		UseExitf("conflicting service with a same component name '%s'\n", compName)
	}
	service := new(Service)
	service.onCreate(compName, s)
	service.setShell(service)
	s.services[compName] = service
	return service
}
func (s *Stage) createHstate(compSign string, compName string) Hstate {
	if s.Hstate(compName) != nil {
		UseExitf("conflicting hstate with a same component name '%s'\n", compName)
	}
	create, ok := hstateCreators[compSign]
	if !ok {
		UseExitln("unknown hstate type: " + compSign)
	}
	hstate := create(compName, s)
	hstate.setShell(hstate)
	s.hstates[compName] = hstate
	return hstate
}
func (s *Stage) createHcache(compSign string, compName string) Hcache {
	if s.Hcache(compName) != nil {
		UseExitf("conflicting hcache with a same component name '%s'\n", compName)
	}
	create, ok := hcacheCreators[compSign]
	if !ok {
		UseExitln("unknown hcache type: " + compSign)
	}
	hcache := create(compName, s)
	hcache.setShell(hcache)
	s.hcaches[compName] = hcache
	return hcache
}
func (s *Stage) createWebapp(compName string) *Webapp {
	if s.Webapp(compName) != nil {
		UseExitf("conflicting webapp with a same component name '%s'\n", compName)
	}
	webapp := new(Webapp)
	webapp.onCreate(compName, s)
	webapp.setShell(webapp)
	s.webapps[compName] = webapp
	return webapp
}
func (s *Stage) createServer(compSign string, compName string) Server {
	if s.Server(compName) != nil {
		UseExitf("conflicting server with a same component name '%s'\n", compName)
	}
	create, ok := serverCreators[compSign]
	if !ok {
		UseExitln("unknown server type: " + compSign)
	}
	server := create(compName, s)
	server.setShell(server)
	s.servers[compName] = server
	return server
}
func (s *Stage) createCronjob(compSign string, compName string) Cronjob {
	if s.Cronjob(compName) != nil {
		UseExitf("conflicting cronjob with a same component name '%s'\n", compName)
	}
	create, ok := cronjobCreators[compSign]
	if !ok {
		UseExitln("unknown cronjob type: " + compSign)
	}
	cronjob := create(compName, s)
	cronjob.setShell(cronjob)
	s.cronjobs[compName] = cronjob
	return cronjob
}

func (s *Stage) ID() int32     { return s.id }
func (s *Stage) NumCPU() int32 { return s.numCPU }

func (s *Stage) Clock() *clockFixture            { return s.clock }
func (s *Stage) Fcache() *fcacheFixture          { return s.fcache }
func (s *Stage) Resolv() *resolvFixture          { return s.resolv }
func (s *Stage) Fixture(compSign string) fixture { return s.fixtures[compSign] }

func (s *Stage) Backend(compName string) Backend        { return s.backends[compName] }
func (s *Stage) QUIXRouter(compName string) *QUIXRouter { return s.quixRouters[compName] }
func (s *Stage) TCPXRouter(compName string) *TCPXRouter { return s.tcpxRouters[compName] }
func (s *Stage) UDPXRouter(compName string) *UDPXRouter { return s.udpxRouters[compName] }
func (s *Stage) Service(compName string) *Service       { return s.services[compName] }
func (s *Stage) Hstate(compName string) Hstate          { return s.hstates[compName] }
func (s *Stage) Hcache(compName string) Hcache          { return s.hcaches[compName] }
func (s *Stage) Webapp(compName string) *Webapp         { return s.webapps[compName] }
func (s *Stage) Server(compName string) Server          { return s.servers[compName] }
func (s *Stage) Cronjob(compName string) Cronjob        { return s.cronjobs[compName] }

func (s *Stage) Start(id int32) {
	s.id = id
	s.numCPU = int32(runtime.NumCPU())

	if DebugLevel() >= 2 {
		Printf("size of server1Conn = %d\n", unsafe.Sizeof(server1Conn{}))
		Printf("size of server2Conn = %d\n", unsafe.Sizeof(server2Conn{}))
		Printf("size of server2Stream = %d\n", unsafe.Sizeof(server2Stream{}))
		Printf("size of server3Conn = %d\n", unsafe.Sizeof(server3Conn{}))
		Printf("size of server3Stream = %d\n", unsafe.Sizeof(server3Stream{}))
		Printf("size of backend1Conn = %d\n", unsafe.Sizeof(backend1Conn{}))
		Printf("size of backend2Conn = %d\n", unsafe.Sizeof(backend2Conn{}))
		Printf("size of backend2Stream = %d\n", unsafe.Sizeof(backend2Stream{}))
		Printf("size of backend3Conn = %d\n", unsafe.Sizeof(backend3Conn{}))
		Printf("size of backend3Stream = %d\n", unsafe.Sizeof(backend3Stream{}))
	}
	if DebugLevel() >= 1 {
		Printf("stageID=%d\n", s.id)
		Printf("numCPU=%d\n", s.numCPU)
		Printf("topDir=%s\n", TopDir())
		Printf("logDir=%s\n", LogDir())
		Printf("tmpDir=%s\n", TmpDir())
		Printf("varDir=%s\n", VarDir())
	}

	// Init the running environment
	rand.Seed(time.Now().UnixNano())
	if err := os.Chdir(TopDir()); err != nil {
		EnvExitln(err.Error())
	}

	// Configure all components in current stage
	if err := s.configure(); err != nil {
		UseExitln(err.Error())
	}

	// Bind services and webapps to servers
	if DebugLevel() >= 1 {
		Println("bind services and webapps to servers")
	}
	for _, server := range s.servers {
		if rpcServer, ok := server.(RPCServer); ok {
			rpcServer.BindServices()
		} else if webServer, ok := server.(HTTPServer); ok {
			webServer.bindWebapps()
		}
	}

	// Prepare all components in current stage
	if err := s.prepare(); err != nil {
		EnvExitln(err.Error())
	}

	// Start all components in current stage
	s.startFixtures() // go fixture.run()
	s.startBackends() // go backend.maintain()
	s.startRouters()  // go router.serve()
	s.startServices() // go service.maintain()
	s.startHstates()  // go hstate.Maintain()
	s.startHcaches()  // go hcache.Maintain()
	s.startWebapps()  // go webapp.maintain()
	s.startServers()  // go server.Serve()
	s.startCronjobs() // go cronjob.Schedule()
}

func (s *Stage) configure() (err error) {
	if DebugLevel() >= 1 {
		Println("now configure stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if DebugLevel() >= 1 {
			Println("stage configured")
		}
	}()
	s.OnConfigure()
	return nil
}
func (s *Stage) prepare() (err error) {
	if DebugLevel() >= 1 {
		Println("now prepare stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if DebugLevel() >= 1 {
			Println("stage prepared")
		}
	}()
	s.OnPrepare()
	return nil
}

func (s *Stage) startFixtures() {
	for _, fixture := range s.fixtures {
		if DebugLevel() >= 1 {
			Printf("fixture=%s go run()\n", fixture.CompName())
		}
		go fixture.run()
	}
}
func (s *Stage) startBackends() {
	for _, backend := range s.backends {
		if DebugLevel() >= 1 {
			Printf("backend=%s go maintain()\n", backend.CompName())
		}
		go backend.Maintain()
	}
}
func (s *Stage) startRouters() {
	for _, quixRouter := range s.quixRouters {
		if DebugLevel() >= 1 {
			Printf("quixRouter=%s go serve()\n", quixRouter.CompName())
		}
		go quixRouter.Serve()
	}
	for _, tcpxRouter := range s.tcpxRouters {
		if DebugLevel() >= 1 {
			Printf("tcpxRouter=%s go serve()\n", tcpxRouter.CompName())
		}
		go tcpxRouter.Serve()
	}
	for _, udpxRouter := range s.udpxRouters {
		if DebugLevel() >= 1 {
			Printf("udpxRouter=%s go serve()\n", udpxRouter.CompName())
		}
		go udpxRouter.Serve()
	}
}
func (s *Stage) startServices() {
	for _, service := range s.services {
		if DebugLevel() >= 1 {
			Printf("service=%s go maintain()\n", service.CompName())
		}
		go service.maintain()
	}
}
func (s *Stage) startHstates() {
	for _, hstate := range s.hstates {
		if DebugLevel() >= 1 {
			Printf("hstate=%s go Maintain()\n", hstate.CompName())
		}
		go hstate.Maintain()
	}
}
func (s *Stage) startHcaches() {
	for _, hcache := range s.hcaches {
		if DebugLevel() >= 1 {
			Printf("hcache=%s go Maintain()\n", hcache.CompName())
		}
		go hcache.Maintain()
	}
}
func (s *Stage) startWebapps() {
	for _, webapp := range s.webapps {
		if DebugLevel() >= 1 {
			Printf("webapp=%s go maintain()\n", webapp.CompName())
		}
		go webapp.maintain()
	}
}
func (s *Stage) startServers() {
	for _, server := range s.servers {
		if DebugLevel() >= 1 {
			Printf("server=%s go Serve()\n", server.CompName())
		}
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	for _, cronjob := range s.cronjobs {
		if DebugLevel() >= 1 {
			Printf("cronjob=%s go Schedule()\n", cronjob.CompName())
		}
		go cronjob.Schedule()
	}
}

func (s *Stage) ProfCPU() {
	file, err := os.Create(s.cpuFile)
	if err != nil {
		return
	}
	defer file.Close()
	pprof.StartCPUProfile(file)
	time.Sleep(5 * time.Second)
	pprof.StopCPUProfile()
}
func (s *Stage) ProfHeap() {
	file, err := os.Create(s.hepFile)
	if err != nil {
		return
	}
	defer file.Close()
	runtime.GC()
	time.Sleep(5 * time.Second)
	runtime.GC()
	pprof.Lookup("heap").WriteTo(file, 1)
}
func (s *Stage) ProfThread() {
	file, err := os.Create(s.thrFile)
	if err != nil {
		return
	}
	defer file.Close()
	time.Sleep(5 * time.Second)
	pprof.Lookup("threadcreate").WriteTo(file, 1)
}
func (s *Stage) ProfGoroutine() {
	file, err := os.Create(s.grtFile)
	if err != nil {
		return
	}
	defer file.Close()
	pprof.Lookup("goroutine").WriteTo(file, 2)
}
func (s *Stage) ProfBlock() {
	file, err := os.Create(s.blkFile)
	if err != nil {
		return
	}
	defer file.Close()
	runtime.SetBlockProfileRate(1)
	time.Sleep(5 * time.Second)
	pprof.Lookup("block").WriteTo(file, 1)
	runtime.SetBlockProfileRate(0)
}

func (s *Stage) Quit() {
	s.OnShutdown()
	if DebugLevel() >= 2 {
		Printf("stage id=%d: quit.\n", s.id)
	}
}
