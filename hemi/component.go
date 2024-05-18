// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Component is the configurable component.

package hemi

import (
	"crypto/tls"
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

const ( // list of components
	compStage      = 1 + iota // stage
	compFixture               // clock, fcache, namer, ...
	compBackend               // http1Backend, quixBackend, udpsBackend, ...
	compNode                  // node
	compQUIXRouter            // quixRouter
	compQUIXDealet            // quixProxy, ...
	compTCPSRouter            // tcpsRouter
	compTCPSDealet            // tcpsProxy, redisProxy, ...
	compUDPSRouter            // udpsRouter
	compUDPSDealet            // udpsProxy, dnsDealet, ...
	compCase                  // case
	compStater                // localStater, redisStater, ...
	compCacher                // localCacher, redisCacher, ...
	compService               // service
	compWebapp                // webapp
	compHandlet               // static, httpProxy, ...
	compReviser               // gzipReviser, wrapReviser, ...
	compSocklet               // helloSocklet, ...
	compRule                  // rule
	compServer                // httpxServer, hrpcServer, echoServer, ...
	compCronjob               // cleanCronjob, statCronjob, ...
)

var signedComps = map[string]int16{ // static comps. more dynamic comps are signed using signComp() below
	"stage":      compStage,
	"node":       compNode,
	"quixRouter": compQUIXRouter,
	"tcpsRouter": compTCPSRouter,
	"udpsRouter": compUDPSRouter,
	"case":       compCase,
	"service":    compService,
	"webapp":     compWebapp,
	"rule":       compRule,
}

func signComp(sign string, comp int16) {
	if have, signed := signedComps[sign]; signed {
		BugExitf("conflicting sign: comp=%d sign=%s\n", have, sign)
	}
	signedComps[sign] = comp
}

var fixtureSigns = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required

func registerFixture(sign string) {
	if _, ok := fixtureSigns[sign]; ok {
		BugExitln("fixture sign conflicted")
	}
	fixtureSigns[sign] = true
	signComp(sign, compFixture)
}

var ( // component creators
	creatorsLock       sync.RWMutex
	backendCreators    = make(map[string]func(name string, stage *Stage) Backend) // indexed by sign, same below.
	staterCreators     = make(map[string]func(name string, stage *Stage) Stater)
	cacherCreators     = make(map[string]func(name string, stage *Stage) Cacher)
	serverCreators     = make(map[string]func(name string, stage *Stage) Server)
	cronjobCreators    = make(map[string]func(name string, stage *Stage) Cronjob)
	quixDealetCreators = make(map[string]func(name string, stage *Stage, router *QUIXRouter) QUIXDealet)
	tcpsDealetCreators = make(map[string]func(name string, stage *Stage, router *TCPSRouter) TCPSDealet)
	udpsDealetCreators = make(map[string]func(name string, stage *Stage, router *UDPSRouter) UDPSDealet)
	handletCreators    = make(map[string]func(name string, stage *Stage, webapp *Webapp) Handlet)
	reviserCreators    = make(map[string]func(name string, stage *Stage, webapp *Webapp) Reviser)
	sockletCreators    = make(map[string]func(name string, stage *Stage, webapp *Webapp) Socklet)
)

func RegisterBackend(sign string, create func(name string, stage *Stage) Backend) {
	_registerComponent0(sign, compBackend, backendCreators, create)
}
func RegisterStater(sign string, create func(name string, stage *Stage) Stater) {
	_registerComponent0(sign, compStater, staterCreators, create)
}
func RegisterCacher(sign string, create func(name string, stage *Stage) Cacher) {
	_registerComponent0(sign, compCacher, cacherCreators, create)
}
func RegisterServer(sign string, create func(name string, stage *Stage) Server) {
	_registerComponent0(sign, compServer, serverCreators, create)
}
func RegisterCronjob(sign string, create func(name string, stage *Stage) Cronjob) {
	_registerComponent0(sign, compCronjob, cronjobCreators, create)
}
func RegisterQUIXDealet(sign string, create func(name string, stage *Stage, router *QUIXRouter) QUIXDealet) {
	_registerComponent1(sign, compQUIXDealet, quixDealetCreators, create)
}
func RegisterTCPSDealet(sign string, create func(name string, stage *Stage, router *TCPSRouter) TCPSDealet) {
	_registerComponent1(sign, compTCPSDealet, tcpsDealetCreators, create)
}
func RegisterUDPSDealet(sign string, create func(name string, stage *Stage, router *UDPSRouter) UDPSDealet) {
	_registerComponent1(sign, compUDPSDealet, udpsDealetCreators, create)
}
func RegisterHandlet(sign string, create func(name string, stage *Stage, webapp *Webapp) Handlet) {
	_registerComponent1(sign, compHandlet, handletCreators, create)
}
func RegisterReviser(sign string, create func(name string, stage *Stage, webapp *Webapp) Reviser) {
	_registerComponent1(sign, compReviser, reviserCreators, create)
}
func RegisterSocklet(sign string, create func(name string, stage *Stage, webapp *Webapp) Socklet) {
	_registerComponent1(sign, compSocklet, sockletCreators, create)
}

func _registerComponent0[T Component](sign string, comp int16, creators map[string]func(string, *Stage) T, create func(string, *Stage) T) { // backend, stater, cacher, server, cronjob
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[sign]; ok {
		BugExitln("component0 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}
func _registerComponent1[T Component, C Component](sign string, comp int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // dealet, handlet, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[sign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}

var ( // initializer of services and webapps
	initsLock    sync.RWMutex
	serviceInits = make(map[string]func(service *Service) error) // indexed by service name.
	webappInits  = make(map[string]func(webapp *Webapp) error)   // indexed by webapp name.
)

func RegisterServiceInit(name string, init func(service *Service) error) {
	initsLock.Lock()
	serviceInits[name] = init
	initsLock.Unlock()
}
func RegisterWebappInit(name string, init func(webapp *Webapp) error) {
	initsLock.Lock()
	webappInits[name] = init
	initsLock.Unlock()
}

// Component is the interface for all components.
type Component interface {
	MakeComp(name string)
	Name() string

	OnShutdown()
	DecSub() // called by sub components of this component

	OnConfigure()
	Find(name string) (value Value, ok bool)
	Prop(name string) (value Value, ok bool)
	ConfigureBool(name string, prop *bool, defaultValue bool)
	ConfigureInt64(name string, prop *int64, check func(value int64) error, defaultValue int64)
	ConfigureInt32(name string, prop *int32, check func(value int32) error, defaultValue int32)
	ConfigureInt16(name string, prop *int16, check func(value int16) error, defaultValue int16)
	ConfigureInt8(name string, prop *int8, check func(value int8) error, defaultValue int8)
	ConfigureInt(name string, prop *int, check func(value int) error, defaultValue int)
	ConfigureString(name string, prop *string, check func(value string) error, defaultValue string)
	ConfigureBytes(name string, prop *[]byte, check func(value []byte) error, defaultValue []byte)
	ConfigureDuration(name string, prop *time.Duration, check func(value time.Duration) error, defaultValue time.Duration)
	ConfigureStringList(name string, prop *[]string, check func(value []string) error, defaultValue []string)
	ConfigureBytesList(name string, prop *[][]byte, check func(value [][]byte) error, defaultValue [][]byte)
	ConfigureStringDict(name string, prop *map[string]string, check func(value map[string]string) error, defaultValue map[string]string)

	OnPrepare()

	setName(name string)
	setShell(shell Component)
	setParent(parent Component)
	getParent() Component
	setInfo(info any)
	setProp(name string, value Value)
}

// Component_ is the parent for all components.
type Component_ struct {
	// Mixins
	_subsWaiter_   // components can have sub components
	_shutdownable_ // to support shutdown
	// Assocs
	shell  Component // the concrete Component
	parent Component // the parent component, used by config
	// States
	name  string           // main, proxy1, ...
	props map[string]Value // name1=value1, ...
	info  any              // extra info about this component, used by config
}

func (c *Component_) MakeComp(name string) {
	c._shutdownable_.init()
	c.name = name
	c.props = make(map[string]Value)
}
func (c *Component_) Name() string { return c.name }

func (c *Component_) Find(name string) (value Value, ok bool) {
	for component := c.shell; component != nil; component = component.getParent() {
		if value, ok = component.Prop(name); ok {
			break
		}
	}
	return
}
func (c *Component_) Prop(name string) (value Value, ok bool) {
	value, ok = c.props[name]
	return
}
func (c *Component_) ConfigureBool(name string, prop *bool, defaultValue bool) {
	_configureProp(c, name, prop, (*Value).Bool, nil, defaultValue)
}
func (c *Component_) ConfigureInt64(name string, prop *int64, check func(value int64) error, defaultValue int64) {
	_configureProp(c, name, prop, (*Value).Int64, check, defaultValue)
}
func (c *Component_) ConfigureInt32(name string, prop *int32, check func(value int32) error, defaultValue int32) {
	_configureProp(c, name, prop, (*Value).Int32, check, defaultValue)
}
func (c *Component_) ConfigureInt16(name string, prop *int16, check func(value int16) error, defaultValue int16) {
	_configureProp(c, name, prop, (*Value).Int16, check, defaultValue)
}
func (c *Component_) ConfigureInt8(name string, prop *int8, check func(value int8) error, defaultValue int8) {
	_configureProp(c, name, prop, (*Value).Int8, check, defaultValue)
}
func (c *Component_) ConfigureInt(name string, prop *int, check func(value int) error, defaultValue int) {
	_configureProp(c, name, prop, (*Value).Int, check, defaultValue)
}
func (c *Component_) ConfigureString(name string, prop *string, check func(value string) error, defaultValue string) {
	_configureProp(c, name, prop, (*Value).String, check, defaultValue)
}
func (c *Component_) ConfigureBytes(name string, prop *[]byte, check func(value []byte) error, defaultValue []byte) {
	_configureProp(c, name, prop, (*Value).Bytes, check, defaultValue)
}
func (c *Component_) ConfigureDuration(name string, prop *time.Duration, check func(value time.Duration) error, defaultValue time.Duration) {
	_configureProp(c, name, prop, (*Value).Duration, check, defaultValue)
}
func (c *Component_) ConfigureStringList(name string, prop *[]string, check func(value []string) error, defaultValue []string) {
	_configureProp(c, name, prop, (*Value).StringList, check, defaultValue)
}
func (c *Component_) ConfigureBytesList(name string, prop *[][]byte, check func(value [][]byte) error, defaultValue [][]byte) {
	_configureProp(c, name, prop, (*Value).BytesList, check, defaultValue)
}
func (c *Component_) ConfigureStringDict(name string, prop *map[string]string, check func(value map[string]string) error, defaultValue map[string]string) {
	_configureProp(c, name, prop, (*Value).StringDict, check, defaultValue)
}

func _configureProp[T any](c *Component_, name string, prop *T, conv func(*Value) (T, bool), check func(value T) error, defaultValue T) {
	if v, ok := c.Find(name); ok {
		if value, ok := conv(&v); ok && check == nil {
			*prop = value
		} else if ok && check != nil {
			if err := check(value); err == nil {
				*prop = value
			} else {
				// TODO: line number
				UseExitln(fmt.Sprintf("%s is error in %s: %s", name, c.name, err.Error()))
			}
		} else {
			UseExitln(fmt.Sprintf("invalid %s in %s", name, c.name))
		}
	} else {
		*prop = defaultValue
	}
}

func (c *Component_) setName(name string)              { c.name = name }
func (c *Component_) setShell(shell Component)         { c.shell = shell }
func (c *Component_) setParent(parent Component)       { c.parent = parent }
func (c *Component_) getParent() Component             { return c.parent }
func (c *Component_) setInfo(info any)                 { c.info = info }
func (c *Component_) setProp(name string, value Value) { c.props[name] = value }

// compList
type compList[T Component] []T

func (l compList[T]) walk(method func(T)) {
	for _, component := range l {
		method(component)
	}
}
func (l compList[T]) goWalk(method func(T)) {
	for _, component := range l {
		go method(component)
	}
}

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

// newStage creates a new stage which runs alongside existing stage.
func newStage() *Stage {
	stage := new(Stage)
	stage.onCreate()
	stage.setShell(stage)
	return stage
}

// Stage represents a running stage in worker process.
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
	namer       *namerFixture         // for fast accessing
	fixtures    compDict[fixture]     // indexed by sign
	backends    compDict[Backend]     // indexed by backendName
	quixRouters compDict[*QUIXRouter] // indexed by routerName
	tcpsRouters compDict[*TCPSRouter] // indexed by routerName
	udpsRouters compDict[*UDPSRouter] // indexed by routerName
	staters     compDict[Stater]      // indexed by staterName
	cachers     compDict[Cacher]      // indexed by cacherName
	services    compDict[*Service]    // indexed by serviceName
	webapps     compDict[*Webapp]     // indexed by webappName
	servers     compDict[Server]      // indexed by serverName
	cronjobs    compDict[Cronjob]     // indexed by cronjobName
	// States
	id      int32
	numCPU  int32
	cpuFile string
	hepFile string
	thrFile string
	grtFile string
	blkFile string
}

func (s *Stage) onCreate() {
	s.MakeComp("stage")

	s.clock = createClock(s)
	s.fcache = createFcache(s)
	s.namer = createNamer(s)
	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFcache] = s.fcache
	s.fixtures[signNamer] = s.namer
	s.backends = make(compDict[Backend])
	s.quixRouters = make(compDict[*QUIXRouter])
	s.tcpsRouters = make(compDict[*TCPSRouter])
	s.udpsRouters = make(compDict[*UDPSRouter])
	s.staters = make(compDict[Stater])
	s.cachers = make(compDict[Cacher])
	s.services = make(compDict[*Service])
	s.webapps = make(compDict[*Webapp])
	s.servers = make(compDict[Server])
	s.cronjobs = make(compDict[Cronjob])
}
func (s *Stage) OnShutdown() {
	if DebugLevel() >= 2 {
		Printf("stage id=%d shutdown start!!\n", s.id)
	}

	// cronjobs
	s.SubsAddn(len(s.cronjobs))
	s.cronjobs.goWalk(Cronjob.OnShutdown)
	s.WaitSubs()

	// servers
	s.SubsAddn(len(s.servers))
	s.servers.goWalk(Server.OnShutdown)
	s.WaitSubs()

	// webapps & services
	s.SubsAddn(len(s.webapps) + len(s.services))
	s.webapps.goWalk((*Webapp).OnShutdown)
	s.services.goWalk((*Service).OnShutdown)
	s.WaitSubs()

	// cachers & staters
	s.SubsAddn(len(s.cachers) + len(s.staters))
	s.cachers.goWalk(Cacher.OnShutdown)
	s.staters.goWalk(Stater.OnShutdown)
	s.WaitSubs()

	// routers
	s.SubsAddn(len(s.udpsRouters) + len(s.tcpsRouters) + len(s.quixRouters))
	s.udpsRouters.goWalk((*UDPSRouter).OnShutdown)
	s.tcpsRouters.goWalk((*TCPSRouter).OnShutdown)
	s.quixRouters.goWalk((*QUIXRouter).OnShutdown)
	s.WaitSubs()

	// backends
	s.SubsAddn(len(s.backends))
	s.backends.goWalk(Backend.OnShutdown)
	s.WaitSubs()

	// fixtures, manually one by one

	s.IncSub()
	s.fcache.OnShutdown()
	s.WaitSubs()

	s.IncSub()
	s.namer.OnShutdown()
	s.WaitSubs()

	s.IncSub()
	s.clock.OnShutdown()
	s.WaitSubs()

	// stage
	if DebugLevel() >= 2 {
		Println("stage close log file")
	}
}

func (s *Stage) OnConfigure() {
	tmpsDir := TmpsDir()

	// cpuFile
	s.ConfigureString("cpuFile", &s.cpuFile, func(value string) error {
		if value == "" {
			return errors.New(".cpuFile has an invalid value")
		}
		return nil
	}, tmpsDir+"/cpu.prof")

	// hepFile
	s.ConfigureString("hepFile", &s.hepFile, func(value string) error {
		if value == "" {
			return errors.New(".hepFile has an invalid value")
		}
		return nil
	}, tmpsDir+"/hep.prof")

	// thrFile
	s.ConfigureString("thrFile", &s.thrFile, func(value string) error {
		if value == "" {
			return errors.New(".thrFile has an invalid value")
		}
		return nil
	}, tmpsDir+"/thr.prof")

	// grtFile
	s.ConfigureString("grtFile", &s.grtFile, func(value string) error {
		if value == "" {
			return errors.New(".grtFile has an invalid value")
		}
		return nil
	}, tmpsDir+"/grt.prof")

	// blkFile
	s.ConfigureString("blkFile", &s.blkFile, func(value string) error {
		if value == "" {
			return errors.New(".blkFile has an invalid value")
		}
		return nil
	}, tmpsDir+"/blk.prof")

	// sub components
	s.fixtures.walk(fixture.OnConfigure)
	s.backends.walk(Backend.OnConfigure)
	s.quixRouters.walk((*QUIXRouter).OnConfigure)
	s.tcpsRouters.walk((*TCPSRouter).OnConfigure)
	s.udpsRouters.walk((*UDPSRouter).OnConfigure)
	s.staters.walk(Stater.OnConfigure)
	s.cachers.walk(Cacher.OnConfigure)
	s.services.walk((*Service).OnConfigure)
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
	s.tcpsRouters.walk((*TCPSRouter).OnPrepare)
	s.udpsRouters.walk((*UDPSRouter).OnPrepare)
	s.staters.walk(Stater.OnPrepare)
	s.cachers.walk(Cacher.OnPrepare)
	s.services.walk((*Service).OnPrepare)
	s.webapps.walk((*Webapp).OnPrepare)
	s.servers.walk(Server.OnPrepare)
	s.cronjobs.walk(Cronjob.OnPrepare)
}

func (s *Stage) createBackend(sign string, name string) Backend {
	if s.Backend(name) != nil {
		UseExitf("conflicting backend with a same name '%s'\n", name)
	}
	create, ok := backendCreators[sign]
	if !ok {
		UseExitln("unknown backend type: " + sign)
	}
	backend := create(name, s)
	backend.setShell(backend)
	s.backends[name] = backend
	return backend
}
func (s *Stage) createQUIXRouter(name string) *QUIXRouter {
	if s.QUIXRouter(name) != nil {
		UseExitf("conflicting quixRouter with a same name '%s'\n", name)
	}
	router := new(QUIXRouter)
	router.onCreate(name, s)
	router.setShell(router)
	s.quixRouters[name] = router
	return router
}
func (s *Stage) createTCPSRouter(name string) *TCPSRouter {
	if s.TCPSRouter(name) != nil {
		UseExitf("conflicting tcpsRouter with a same name '%s'\n", name)
	}
	router := new(TCPSRouter)
	router.onCreate(name, s)
	router.setShell(router)
	s.tcpsRouters[name] = router
	return router
}
func (s *Stage) createUDPSRouter(name string) *UDPSRouter {
	if s.UDPSRouter(name) != nil {
		UseExitf("conflicting udpsRouter with a same name '%s'\n", name)
	}
	router := new(UDPSRouter)
	router.onCreate(name, s)
	router.setShell(router)
	s.udpsRouters[name] = router
	return router
}
func (s *Stage) createStater(sign string, name string) Stater {
	if s.Stater(name) != nil {
		UseExitf("conflicting stater with a same name '%s'\n", name)
	}
	create, ok := staterCreators[sign]
	if !ok {
		UseExitln("unknown stater type: " + sign)
	}
	stater := create(name, s)
	stater.setShell(stater)
	s.staters[name] = stater
	return stater
}
func (s *Stage) createCacher(sign string, name string) Cacher {
	if s.Cacher(name) != nil {
		UseExitf("conflicting cacher with a same name '%s'\n", name)
	}
	create, ok := cacherCreators[sign]
	if !ok {
		UseExitln("unknown cacher type: " + sign)
	}
	cacher := create(name, s)
	cacher.setShell(cacher)
	s.cachers[name] = cacher
	return cacher
}
func (s *Stage) createService(name string) *Service {
	if s.Service(name) != nil {
		UseExitf("conflicting service with a same name '%s'\n", name)
	}
	service := new(Service)
	service.onCreate(name, s)
	service.setShell(service)
	s.services[name] = service
	return service
}
func (s *Stage) createWebapp(name string) *Webapp {
	if s.Webapp(name) != nil {
		UseExitf("conflicting webapp with a same name '%s'\n", name)
	}
	webapp := new(Webapp)
	webapp.onCreate(name, s)
	webapp.setShell(webapp)
	s.webapps[name] = webapp
	return webapp
}
func (s *Stage) createServer(sign string, name string) Server {
	if s.Server(name) != nil {
		UseExitf("conflicting server with a same name '%s'\n", name)
	}
	create, ok := serverCreators[sign]
	if !ok {
		UseExitln("unknown server type: " + sign)
	}
	server := create(name, s)
	server.setShell(server)
	s.servers[name] = server
	return server
}
func (s *Stage) createCronjob(sign string, name string) Cronjob {
	if s.Cronjob(name) != nil {
		UseExitf("conflicting cronjob with a same name '%s'\n", name)
	}
	create, ok := cronjobCreators[sign]
	if !ok {
		UseExitln("unknown cronjob type: " + sign)
	}
	cronjob := create(name, s)
	cronjob.setShell(cronjob)
	s.cronjobs[name] = cronjob
	return cronjob
}

func (s *Stage) Clock() *clockFixture               { return s.clock }
func (s *Stage) Fcache() *fcacheFixture             { return s.fcache }
func (s *Stage) Namer() *namerFixture               { return s.namer }
func (s *Stage) Fixture(sign string) fixture        { return s.fixtures[sign] }
func (s *Stage) Backend(name string) Backend        { return s.backends[name] }
func (s *Stage) QUIXRouter(name string) *QUIXRouter { return s.quixRouters[name] }
func (s *Stage) TCPSRouter(name string) *TCPSRouter { return s.tcpsRouters[name] }
func (s *Stage) UDPSRouter(name string) *UDPSRouter { return s.udpsRouters[name] }
func (s *Stage) Stater(name string) Stater          { return s.staters[name] }
func (s *Stage) Cacher(name string) Cacher          { return s.cachers[name] }
func (s *Stage) Service(name string) *Service       { return s.services[name] }
func (s *Stage) Webapp(name string) *Webapp         { return s.webapps[name] }
func (s *Stage) Server(name string) Server          { return s.servers[name] }
func (s *Stage) Cronjob(name string) Cronjob        { return s.cronjobs[name] }

func (s *Stage) Start(id int32) {
	s.id = id
	s.numCPU = int32(runtime.NumCPU())

	if DebugLevel() >= 2 {
		Printf("size of http1Conn = %d\n", unsafe.Sizeof(http1Conn{}))
		Printf("size of http2Conn = %d\n", unsafe.Sizeof(http2Conn{}))
		Printf("size of http3Conn = %d\n", unsafe.Sizeof(http3Conn{}))
		Printf("size of http2Stream = %d\n", unsafe.Sizeof(http2Stream{}))
		Printf("size of http3Stream = %d\n", unsafe.Sizeof(http3Stream{}))
		Printf("size of H1Conn = %d\n", unsafe.Sizeof(H1Conn{}))
		Printf("size of H2Conn = %d\n", unsafe.Sizeof(H2Conn{}))
		Printf("size of H3Conn = %d\n", unsafe.Sizeof(H3Conn{}))
		Printf("size of H2Stream = %d\n", unsafe.Sizeof(H2Stream{}))
		Printf("size of H3Stream = %d\n", unsafe.Sizeof(H3Stream{}))
	}
	if DebugLevel() >= 1 {
		Printf("stageID=%d\n", s.id)
		Printf("numCPU=%d\n", s.numCPU)
		Printf("baseDir=%s\n", BaseDir())
		Printf("logsDir=%s\n", LogsDir())
		Printf("tmpsDir=%s\n", TmpsDir())
		Printf("varsDir=%s\n", VarsDir())
	}

	// Init running environment
	rand.Seed(time.Now().UnixNano())
	if err := os.Chdir(BaseDir()); err != nil {
		EnvExitln(err.Error())
	}

	// Configure all components in current stage
	if err := s.configure(); err != nil {
		UseExitln(err.Error())
	}

	// Bind services and webapps to servers
	s.bindServerServices()
	s.bindServerWebapps()

	// Prepare all components
	if err := s.prepare(); err != nil {
		EnvExitln(err.Error())
	}

	// Start all components
	s.startFixtures() // go fixture.run()
	s.startBackends() // go backend.maintain()
	s.startRouters()  // go router.serve()
	s.startStaters()  // go stater.Maintain()
	s.startCachers()  // go cacher.Maintain()
	s.startServices() // go service.maintain()
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
func (s *Stage) bindServerServices() {
	if DebugLevel() >= 1 {
		Println("bind services to rpc servers")
	}
	for _, server := range s.servers {
		if rpcServer, ok := server.(*hrpcServer); ok {
			rpcServer.BindServices()
		}
	}
}
func (s *Stage) bindServerWebapps() {
	if DebugLevel() >= 1 {
		Println("bind webapps to web servers")
	}
	for _, server := range s.servers {
		if webServer, ok := server.(WebServer); ok {
			webServer.bindApps()
		}
	}
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
			Printf("fixture=%s go run()\n", fixture.Name())
		}
		go fixture.run()
	}
}
func (s *Stage) startBackends() {
	for _, backend := range s.backends {
		if DebugLevel() >= 1 {
			Printf("backend=%s go maintain()\n", backend.Name())
		}
		go backend.Maintain()
	}
}
func (s *Stage) startRouters() {
	for _, quixRouter := range s.quixRouters {
		if DebugLevel() >= 1 {
			Printf("quixRouter=%s go serve()\n", quixRouter.Name())
		}
		go quixRouter.Serve()
	}
	for _, tcpsRouter := range s.tcpsRouters {
		if DebugLevel() >= 1 {
			Printf("tcpsRouter=%s go serve()\n", tcpsRouter.Name())
		}
		go tcpsRouter.Serve()
	}
	for _, udpsRouter := range s.udpsRouters {
		if DebugLevel() >= 1 {
			Printf("udpsRouter=%s go serve()\n", udpsRouter.Name())
		}
		go udpsRouter.Serve()
	}
}
func (s *Stage) startStaters() {
	for _, stater := range s.staters {
		if DebugLevel() >= 1 {
			Printf("stater=%s go Maintain()\n", stater.Name())
		}
		go stater.Maintain()
	}
}
func (s *Stage) startCachers() {
	for _, cacher := range s.cachers {
		if DebugLevel() >= 1 {
			Printf("cacher=%s go Maintain()\n", cacher.Name())
		}
		go cacher.Maintain()
	}
}
func (s *Stage) startServices() {
	for _, service := range s.services {
		if DebugLevel() >= 1 {
			Printf("service=%s go maintain()\n", service.Name())
		}
		go service.maintain()
	}
}
func (s *Stage) startWebapps() {
	for _, webapp := range s.webapps {
		if DebugLevel() >= 1 {
			Printf("webapp=%s go maintain()\n", webapp.Name())
		}
		go webapp.maintain()
	}
}
func (s *Stage) startServers() {
	for _, server := range s.servers {
		if DebugLevel() >= 1 {
			Printf("server=%s go Serve()\n", server.Name())
		}
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	for _, cronjob := range s.cronjobs {
		if DebugLevel() >= 1 {
			Printf("cronjob=%s go Schedule()\n", cronjob.Name())
		}
		go cronjob.Schedule()
	}
}

func (s *Stage) ID() int32     { return s.id }
func (s *Stage) NumCPU() int32 { return s.numCPU }

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

// fixture component.
//
// Fixtures only exist in internal, and are created by stage.
// Some critical functions, like clock and namer, are implemented as fixtures.
//
// Fixtures are singletons in stage.
type fixture interface {
	// Imports
	Component
	// Methods
	run() // runner
}

// Stater component is the interface to storages of Web/RPC states.
type Stater interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Set(sid []byte, session *Session)
	Get(sid []byte) (session *Session)
	Del(sid []byte) bool
}

// Stater_ is the parent for all staters.
type Stater_ struct {
	// Parent
	Component_
}

// Session is a Web/RPC session in stater
type Session struct {
	// TODO
	ID      [40]byte // session id
	Secret  [40]byte // secret key
	Created int64    // unix time
	Expires int64    // unix time
	Role    int8     // 0: default, >0: user defined values
	Device  int8     // terminal device type
	state1  int8     // user defined state1
	state2  int8     // user defined state2
	state3  int32    // user defined state3
	states  map[string]string
}

func (s *Session) init() {
	s.states = make(map[string]string)
}

func (s *Session) Get(name string) string        { return s.states[name] }
func (s *Session) Set(name string, value string) { s.states[name] = value }
func (s *Session) Del(name string)               { delete(s.states, name) }

// Backend component. A Backend is a group of nodes.
type Backend interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Stage() *Stage
	CreateNode(name string) Node
	DialTimeout() time.Duration
	ReadTimeout() time.Duration  // timeout for a single read operation
	WriteTimeout() time.Duration // timeout for a single write operation
	AliveTimeout() time.Duration
	nextConnID() int64
}

// Node is a member of backend.
type Node interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Backend() Backend
	IsUDS() bool
	IsTLS() bool
}

// Server component. A Server is a group of gates.
type Server interface {
	// Imports
	Component
	// Methods
	Serve() // runner
	Stage() *Stage
	ReadTimeout() time.Duration  // timeout for a single read operation
	WriteTimeout() time.Duration // timeout for a single write operation
	Address() string
	ColonPort() string
	ColonPortBytes() []byte
	IsUDS() bool
	IsTLS() bool
	TLSConfig() *tls.Config
	MaxConnsPerGate() int32
}

// Cronjob component
type Cronjob interface {
	// Imports
	Component
	// Methods
	Schedule() // runner
}

// Cronjob_ is the parent for all cronjobs.
type Cronjob_ struct {
	// Parent
	Component_
	// States
}
