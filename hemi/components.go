// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Component is the configurable component.

package hemi

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

func init() {
	registerFixture(signClock)
	registerFixture(signFcache)
	registerFixture(signResolv)
}

const ( // list of components
	compStage      = 1 + iota // stage
	compFixture               // clock, fcache, resolv, ...
	compBackend               // http1Backend, quixBackend, udpxBackend, ...
	compNode                  // node
	compQUIXRouter            // quixRouter
	compQUIXDealet            // quixProxy, ...
	compTCPXRouter            // tcpxRouter
	compTCPXDealet            // tcpxProxy, redisProxy, ...
	compUDPXRouter            // udpxRouter
	compUDPXDealet            // udpxProxy, dnsProxy, ...
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
	"tcpxRouter": compTCPXRouter,
	"udpxRouter": compUDPXRouter,
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
	tcpxDealetCreators = make(map[string]func(name string, stage *Stage, router *TCPXRouter) TCPXDealet)
	udpxDealetCreators = make(map[string]func(name string, stage *Stage, router *UDPXRouter) UDPXDealet)
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
func RegisterTCPXDealet(sign string, create func(name string, stage *Stage, router *TCPXRouter) TCPXDealet) {
	_registerComponent1(sign, compTCPXDealet, tcpxDealetCreators, create)
}
func RegisterUDPXDealet(sign string, create func(name string, stage *Stage, router *UDPXRouter) UDPXDealet) {
	_registerComponent1(sign, compUDPXDealet, udpxDealetCreators, create)
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
	DecSub() // called by sub components or objects of this component

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
	// Assocs
	shell  Component // the concrete Component
	parent Component // the parent component, used by config
	// States
	name     string           // main, proxy1, ...
	props    map[string]Value // name1=value1, ...
	info     any              // extra info about this component, used by config
	subs     sync.WaitGroup   // sub components or objects to wait for
	ShutChan chan struct{}    // used to notify shutdown
}

func (c *Component_) MakeComp(name string) {
	c.name = name
	c.props = make(map[string]Value)
	c.ShutChan = make(chan struct{})
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

func (c *Component_) IncSub()       { c.subs.Add(1) }
func (c *Component_) IncSubs(n int) { c.subs.Add(n) }
func (c *Component_) WaitSubs()     { c.subs.Wait() }
func (c *Component_) DecSub()       { c.subs.Done() }
func (c *Component_) DecSubs(n int) { c.subs.Add(-n) }

func (c *Component_) Loop(interval time.Duration, callback func(now time.Time)) {
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
	resolv      *resolvFixture        // for fast accessing
	fixtures    compDict[fixture]     // indexed by sign
	backends    compDict[Backend]     // indexed by backendName
	quixRouters compDict[*QUIXRouter] // indexed by routerName
	tcpxRouters compDict[*TCPXRouter] // indexed by routerName
	udpxRouters compDict[*UDPXRouter] // indexed by routerName
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
	s.resolv = createResolv(s)
	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFcache] = s.fcache
	s.fixtures[signResolv] = s.resolv
	s.backends = make(compDict[Backend])
	s.quixRouters = make(compDict[*QUIXRouter])
	s.tcpxRouters = make(compDict[*TCPXRouter])
	s.udpxRouters = make(compDict[*UDPXRouter])
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
	s.IncSubs(len(s.cronjobs))
	s.cronjobs.goWalk(Cronjob.OnShutdown)
	s.WaitSubs()

	// servers
	s.IncSubs(len(s.servers))
	s.servers.goWalk(Server.OnShutdown)
	s.WaitSubs()

	// webapps & services
	s.IncSubs(len(s.webapps) + len(s.services))
	s.webapps.goWalk((*Webapp).OnShutdown)
	s.services.goWalk((*Service).OnShutdown)
	s.WaitSubs()

	// cachers & staters
	s.IncSubs(len(s.cachers) + len(s.staters))
	s.cachers.goWalk(Cacher.OnShutdown)
	s.staters.goWalk(Stater.OnShutdown)
	s.WaitSubs()

	// routers
	s.IncSubs(len(s.udpxRouters) + len(s.tcpxRouters) + len(s.quixRouters))
	s.udpxRouters.goWalk((*UDPXRouter).OnShutdown)
	s.tcpxRouters.goWalk((*TCPXRouter).OnShutdown)
	s.quixRouters.goWalk((*QUIXRouter).OnShutdown)
	s.WaitSubs()

	// backends
	s.IncSubs(len(s.backends))
	s.backends.goWalk(Backend.OnShutdown)
	s.WaitSubs()

	// fixtures, manually one by one

	s.IncSub() // fcache
	s.fcache.OnShutdown()
	s.WaitSubs()

	s.IncSub() // resolv
	s.resolv.OnShutdown()
	s.WaitSubs()

	s.IncSub() // clock
	s.clock.OnShutdown()
	s.WaitSubs()

	// stage
	if DebugLevel() >= 2 {
		Println("stage close log file")
	}
}

func (s *Stage) OnConfigure() {
	tmpDir := TmpDir()

	// cpuFile
	s.ConfigureString("cpuFile", &s.cpuFile, func(value string) error {
		if value == "" {
			return errors.New(".cpuFile has an invalid value")
		}
		return nil
	}, tmpDir+"/cpu.prof")

	// hepFile
	s.ConfigureString("hepFile", &s.hepFile, func(value string) error {
		if value == "" {
			return errors.New(".hepFile has an invalid value")
		}
		return nil
	}, tmpDir+"/hep.prof")

	// thrFile
	s.ConfigureString("thrFile", &s.thrFile, func(value string) error {
		if value == "" {
			return errors.New(".thrFile has an invalid value")
		}
		return nil
	}, tmpDir+"/thr.prof")

	// grtFile
	s.ConfigureString("grtFile", &s.grtFile, func(value string) error {
		if value == "" {
			return errors.New(".grtFile has an invalid value")
		}
		return nil
	}, tmpDir+"/grt.prof")

	// blkFile
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
	s.tcpxRouters.walk((*TCPXRouter).OnPrepare)
	s.udpxRouters.walk((*UDPXRouter).OnPrepare)
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
func (s *Stage) createTCPXRouter(name string) *TCPXRouter {
	if s.TCPXRouter(name) != nil {
		UseExitf("conflicting tcpxRouter with a same name '%s'\n", name)
	}
	router := new(TCPXRouter)
	router.onCreate(name, s)
	router.setShell(router)
	s.tcpxRouters[name] = router
	return router
}
func (s *Stage) createUDPXRouter(name string) *UDPXRouter {
	if s.UDPXRouter(name) != nil {
		UseExitf("conflicting udpxRouter with a same name '%s'\n", name)
	}
	router := new(UDPXRouter)
	router.onCreate(name, s)
	router.setShell(router)
	s.udpxRouters[name] = router
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
func (s *Stage) Resolv() *resolvFixture             { return s.resolv }
func (s *Stage) Fixture(sign string) fixture        { return s.fixtures[sign] }
func (s *Stage) Backend(name string) Backend        { return s.backends[name] }
func (s *Stage) QUIXRouter(name string) *QUIXRouter { return s.quixRouters[name] }
func (s *Stage) TCPXRouter(name string) *TCPXRouter { return s.tcpxRouters[name] }
func (s *Stage) UDPXRouter(name string) *UDPXRouter { return s.udpxRouters[name] }
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

	// Init running environment
	rand.Seed(time.Now().UnixNano())
	if err := os.Chdir(TopDir()); err != nil {
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
		if hrpcServer, ok := server.(*hrpcServer); ok {
			hrpcServer.BindServices()
		}
	}
}
func (s *Stage) bindServerWebapps() {
	if DebugLevel() >= 1 {
		Println("bind webapps to http servers")
	}
	for _, server := range s.servers {
		if httpServer, ok := server.(HTTPServer); ok {
			httpServer.bindApps()
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
	for _, tcpxRouter := range s.tcpxRouters {
		if DebugLevel() >= 1 {
			Printf("tcpxRouter=%s go serve()\n", tcpxRouter.Name())
		}
		go tcpxRouter.Serve()
	}
	for _, udpxRouter := range s.udpxRouters {
		if DebugLevel() >= 1 {
			Printf("udpxRouter=%s go serve()\n", udpxRouter.Name())
		}
		go udpxRouter.Serve()
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
// Some critical functions, like clock and resolv, are implemented as fixtures.
//
// Fixtures are singletons in stage.
type fixture interface {
	// Imports
	Component
	// Methods
	run() // runner
}

const signClock = "clock"

func createClock(stage *Stage) *clockFixture {
	clock := new(clockFixture)
	clock.onCreate(stage)
	clock.setShell(clock)
	return clock
}

// clockFixture
type clockFixture struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	resolution time.Duration
	date       atomic.Int64 // 4, 4+4 4 4+4+4+4 4+4:4+4:4+4 = 56bit
}

func (f *clockFixture) onCreate(stage *Stage) {
	f.MakeComp(signClock)
	f.stage = stage
	f.resolution = 100 * time.Millisecond
	f.date.Store(0x7394804991b60000) // Sun, 06 Nov 1994 08:49:37
}
func (f *clockFixture) OnShutdown() {
	close(f.ShutChan) // notifies run()
}

func (f *clockFixture) OnConfigure() {
}
func (f *clockFixture) OnPrepare() {
}

func (f *clockFixture) run() { // runner
	f.Loop(f.resolution, func(now time.Time) {
		now = now.UTC()
		weekday := now.Weekday()       // weekday: 0-6
		year, month, day := now.Date() // month: 1-12
		hour, minute, second := now.Clock()
		date := int64(0)
		date |= int64(second%10) << 60
		date |= int64(second/10) << 56
		date |= int64(minute%10) << 52
		date |= int64(minute/10) << 48
		date |= int64(hour%10) << 44
		date |= int64(hour/10) << 40
		date |= int64(year%10) << 36
		date |= int64(year/10%10) << 32
		date |= int64(year/100%10) << 28
		date |= int64(year/1000) << 24
		date |= int64(month) << 20
		date |= int64(day%10) << 16
		date |= int64(day/10) << 12
		date |= int64(weekday) << 8
		f.date.Store(date)
	})
	if DebugLevel() >= 2 {
		Println("clock done")
	}
	f.stage.DecSub() // clock
}

func (f *clockFixture) writeDate1(p []byte) int {
	i := copy(p, "date: ")
	i += f.writeDate(p[i:])
	p[i] = '\r'
	p[i+1] = '\n'
	return i + 2
}
func (f *clockFixture) writeDate(p []byte) int {
	date := f.date.Load()
	s := clockDayString[3*(date>>8&0xf):]
	p[0] = s[0] // 'S'
	p[1] = s[1] // 'u'
	p[2] = s[2] // 'n'
	p[3] = ','
	p[4] = ' '
	p[5] = byte(date>>12&0xf) + '0' // '0'
	p[6] = byte(date>>16&0xf) + '0' // '6'
	p[7] = ' '
	s = clockMonthString[3*(date>>20&0xf-1):]
	p[8] = s[0]  // 'N'
	p[9] = s[1]  // 'o'
	p[10] = s[2] // 'v'
	p[11] = ' '
	p[12] = byte(date>>24&0xf) + '0' // '1'
	p[13] = byte(date>>28&0xf) + '0' // '9'
	p[14] = byte(date>>32&0xf) + '0' // '9'
	p[15] = byte(date>>36&0xf) + '0' // '4'
	p[16] = ' '
	p[17] = byte(date>>40&0xf) + '0' // '0'
	p[18] = byte(date>>44&0xf) + '0' // '8'
	p[19] = ':'
	p[20] = byte(date>>48&0xf) + '0' // '4'
	p[21] = byte(date>>52&0xf) + '0' // '9'
	p[22] = ':'
	p[23] = byte(date>>56&0xf) + '0' // '3'
	p[24] = byte(date>>60&0xf) + '0' // '7'
	p[25] = ' '
	p[26] = 'G'
	p[27] = 'M'
	p[28] = 'T'
	return clockHTTPDateSize
}

func clockWriteHTTPDate1(p []byte, name []byte, unixTime int64) int {
	i := copy(p, name)
	p[i] = ':'
	p[i+1] = ' '
	i += 2
	date := time.Unix(unixTime, 0)
	date = date.UTC()
	i += clockWriteHTTPDate(p[i:], date)
	p[i] = '\r'
	p[i+1] = '\n'
	return i + 2
}
func clockWriteHTTPDate(p []byte, date time.Time) int {
	if len(p) < clockHTTPDateSize {
		BugExitln("invalid buffer for clockWriteHTTPDate")
	}
	s := clockDayString[3*date.Weekday():]
	p[0] = s[0] // 'S'
	p[1] = s[1] // 'u'
	p[2] = s[2] // 'n'
	p[3] = ','
	p[4] = ' '
	year, month, day := date.Date() // month: 1-12
	p[5] = byte(day/10) + '0'       // '0'
	p[6] = byte(day%10) + '0'       // '6'
	p[7] = ' '
	s = clockMonthString[3*(month-1):]
	p[8] = s[0]  // 'N'
	p[9] = s[1]  // 'o'
	p[10] = s[2] // 'v'
	p[11] = ' '
	p[12] = byte(year/1000) + '0'   // '1'
	p[13] = byte(year/100%10) + '0' // '9'
	p[14] = byte(year/10%10) + '0'  // '9'
	p[15] = byte(year%10) + '0'     // '4'
	p[16] = ' '
	hour, minute, second := date.Clock()
	p[17] = byte(hour/10) + '0' // '0'
	p[18] = byte(hour%10) + '0' // '8'
	p[19] = ':'
	p[20] = byte(minute/10) + '0' // '4'
	p[21] = byte(minute%10) + '0' // '9'
	p[22] = ':'
	p[23] = byte(second/10) + '0' // '3'
	p[24] = byte(second%10) + '0' // '7'
	p[25] = ' '
	p[26] = 'G'
	p[27] = 'M'
	p[28] = 'T'
	return clockHTTPDateSize
}

func clockParseHTTPDate(date []byte) (int64, bool) {
	// format 0: Sun, 06 Nov 1994 08:49:37 GMT
	// format 1: Sunday, 06-Nov-94 08:49:37 GMT
	// format 2: Sun Nov  6 08:49:37 1994
	var format int
	fore, edge := 0, len(date)
	if n := len(date); n == clockHTTPDateSize {
		format = 0
		fore = 5 // skip 'Sun, ', stops at '0'
	} else if n >= 30 && n <= 33 {
		format = 1
		for fore < edge && date[fore] != ' ' { // skip 'Sunday, ', stops at '0'
			fore++
		}
		if edge-fore != 23 {
			return 0, false
		}
		fore++
	} else if n == clockASCTimeSize {
		format = 2
		fore = 4 // skip 'Sun ', stops at 'N'
	} else {
		return 0, false
	}
	var year, month, day, hour, minute, second int
	var b, b0, b1, b2, b3 byte
	if format != 2 {
		if b0, b1 = date[fore], date[fore+1]; b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
			day = int(b0-'0')*10 + int(b1-'0')
		} else {
			return 0, false
		}
		fore += 3
		if b = date[fore-1]; (format == 0 && b != ' ') || (format == 1 && b != '-') {
			return 0, false
		}
	}
	hash := uint16(date[fore]) + uint16(date[fore+1]) + uint16(date[fore+2])
	m := clockMonthTable[clockMonthFind(hash)]
	if m.hash == hash && string(date[fore:fore+3]) == clockMonthString[m.from:m.edge] {
		month = int(m.month)
	} else {
		return 0, false
	}
	fore += 4
	if b = date[fore-1]; (format == 1 && b != '-') || (format != 1 && b != ' ') {
		return 0, false
	}
	if format == 0 {
		b0, b1, b2, b3 = date[fore], date[fore+1], date[fore+2], date[fore+3]
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' && b2 >= '0' && b2 <= '9' && b3 >= '0' && b3 <= '9' {
			year = int(b0-'0')*1000 + int(b1-'0')*100 + int(b2-'0')*10 + int(b3-'0')
			fore += 5
		} else {
			return 0, false
		}
	} else if format == 1 {
		b0, b1 = date[fore], date[fore+1]
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
			year = int(b0-'0')*10 + int(b1-'0')
			if year < 70 {
				year += 2000
			} else {
				year += 1900
			}
			fore += 3
		} else {
			return 0, false
		}
	} else {
		b0, b1 = date[fore], date[fore+1]
		if b0 == ' ' {
			b0 = '0'
		}
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
			day = int(b0-'0')*10 + int(b1-'0')
		} else {
			return 0, false
		}
		fore += 3
	}
	b0, b1 = date[fore], date[fore+1]
	if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
		hour = int(b0-'0')*10 + int(b1-'0')
		fore += 3
	} else {
		return 0, false
	}
	b0, b1 = date[fore], date[fore+1]
	if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
		minute = int(b0-'0')*10 + int(b1-'0')
		fore += 3
	} else {
		return 0, false
	}
	b0, b1 = date[fore], date[fore+1]
	if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
		second = int(b0-'0')*10 + int(b1-'0')
		fore += 3
	} else {
		return 0, false
	}
	if date[fore-1] != ' ' || date[fore-4] != ':' || date[fore-7] != ':' || date[fore-10] != ' ' || hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}
	if format == 2 {
		b0, b1, b2, b3 = date[fore], date[fore+1], date[fore+2], date[fore+3]
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' && b2 >= '0' && b2 <= '9' && b3 >= '0' && b3 <= '9' {
			year = int(b0-'0')*1000 + int(b1-'0')*100 + int(b2-'0')*10 + int(b3-'0')
		} else {
			return 0, false
		}
	} else if date[fore] != 'G' || date[fore+1] != 'M' || date[fore+2] != 'T' {
		return 0, false
	}
	leap := year%4 == 0 && (year%100 != 0 || year%400 == 0)
	if day == 29 && month == 2 {
		if !leap {
			return 0, false
		}
	} else if day > int(m.days) {
		return 0, false
	}
	days := int(m.past)
	if year > 0 {
		year--
		days += (year/4 - year/100 + year/400 + 1) // year 0000 is a leap year
		days += (year + 1) * 365
	}
	if leap && month > 2 {
		days++
	}
	days += (day - 1) // today has not past
	days -= 719528    // total days between [0000-01-01 00:00:00, 1970-01-01 00:00:00)
	return int64(days)*86400 + int64(hour*3600+minute*60+second), true
}

const (
	clockHTTPDateSize = len("Sun, 06 Nov 1994 08:49:37 GMT")
	clockASCTimeSize  = len("Sun Nov  6 08:49:37 1994")
	clockDayString    = "SunMonTueWedThuFriSat"
	clockMonthString  = "JanFebMarAprMayJunJulAugSepOctNovDec"
)

var ( // perfect hash table for months
	clockMonthTable = [12]struct {
		hash  uint16
		from  int8
		edge  int8
		month int8
		days  int8
		past  int16
	}{
		0:  {285, 21, 24, 8, 31, 212},  // Aug
		1:  {296, 24, 27, 9, 30, 243},  // Sep
		2:  {268, 33, 36, 12, 31, 334}, // Dec
		3:  {288, 6, 9, 3, 31, 59},     // Mar
		4:  {301, 15, 18, 6, 30, 151},  // Jun
		5:  {295, 12, 15, 5, 31, 120},  // May
		6:  {307, 30, 33, 11, 30, 304}, // Nov
		7:  {299, 18, 21, 7, 31, 181},  // Jul
		8:  {294, 27, 30, 10, 31, 273}, // Oct
		9:  {291, 9, 12, 4, 30, 90},    // Apr
		10: {269, 3, 6, 2, 28, 31},     // Feb
		11: {281, 0, 3, 1, 31, 0},      // Jan
	}
	clockMonthFind = func(hash uint16) int { return (5509728 / int(hash)) % 12 }
)

const signFcache = "fcache"

func createFcache(stage *Stage) *fcacheFixture {
	fcache := new(fcacheFixture)
	fcache.onCreate(stage)
	fcache.setShell(fcache)
	return fcache
}

// fcacheFixture caches file descriptors and contents.
type fcacheFixture struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	smallFileSize int64 // what size is considered as small file
	maxSmallFiles int32 // max number of small files. for small files, contents are cached
	maxLargeFiles int32 // max number of large files. for large files, *os.File are cached
	cacheTimeout  time.Duration
	rwMutex       sync.RWMutex // protects entries below
	entries       map[string]*fcacheEntry
}

func (f *fcacheFixture) onCreate(stage *Stage) {
	f.MakeComp(signFcache)
	f.stage = stage
	f.entries = make(map[string]*fcacheEntry)
}
func (f *fcacheFixture) OnShutdown() {
	close(f.ShutChan) // notifies run()
}

func (f *fcacheFixture) OnConfigure() {
	// smallFileSize
	f.ConfigureInt64("smallFileSize", &f.smallFileSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".smallFileSize has an invalid value")
	}, _64K1)

	// maxSmallFiles
	f.ConfigureInt32("maxSmallFiles", &f.maxSmallFiles, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxSmallFiles has an invalid value")
	}, 1000)

	// maxLargeFiles
	f.ConfigureInt32("maxLargeFiles", &f.maxLargeFiles, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxLargeFiles has an invalid value")
	}, 500)

	// cacheTimeout
	f.ConfigureDuration("cacheTimeout", &f.cacheTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".cacheTimeout has an invalid value")
	}, 1*time.Second)
}
func (f *fcacheFixture) OnPrepare() {
}

func (f *fcacheFixture) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		f.rwMutex.Lock()
		for path, entry := range f.entries {
			if entry.last.After(now) {
				continue
			}
			if entry.isLarge() {
				entry.decRef()
			}
			delete(f.entries, path)
			if DebugLevel() >= 2 {
				Printf("fcache entry deleted: %s\n", path)
			}
		}
		f.rwMutex.Unlock()
	})
	f.rwMutex.Lock()
	f.entries = nil
	f.rwMutex.Unlock()

	if DebugLevel() >= 2 {
		Println("fcache done")
	}
	f.stage.DecSub() // fcache
}

func (f *fcacheFixture) getEntry(path []byte) (*fcacheEntry, error) {
	f.rwMutex.RLock()
	defer f.rwMutex.RUnlock()

	if entry, ok := f.entries[WeakString(path)]; ok {
		if entry.isLarge() {
			entry.addRef()
		}
		return entry, nil
	} else {
		return nil, fcacheNotExist
	}
}

var fcacheNotExist = errors.New("entry not exist")

func (f *fcacheFixture) newEntry(path string) (*fcacheEntry, error) {
	f.rwMutex.RLock()
	if entry, ok := f.entries[path]; ok {
		if entry.isLarge() {
			entry.addRef()
		}

		f.rwMutex.RUnlock()
		return entry, nil
	}
	f.rwMutex.RUnlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	entry := new(fcacheEntry)
	if info.IsDir() {
		entry.kind = fcacheKindDir
		file.Close()
	} else if fileSize := info.Size(); fileSize <= f.smallFileSize {
		text := make([]byte, fileSize)
		if _, err := io.ReadFull(file, text); err != nil {
			file.Close()
			return nil, err
		}
		entry.kind = fcacheKindSmall
		entry.info = info
		entry.text = text
		file.Close()
	} else { // large file
		entry.kind = fcacheKindLarge
		entry.file = file
		entry.info = info
		entry.nRef.Store(1) // current caller
	}
	entry.last = time.Now().Add(f.cacheTimeout)

	f.rwMutex.Lock()
	f.entries[path] = entry
	f.rwMutex.Unlock()

	return entry, nil
}

// fcacheEntry
type fcacheEntry struct {
	kind int8         // see fcacheKindXXX
	file *os.File     // only for large file
	info os.FileInfo  // only for files, not directories
	text []byte       // content of small file
	last time.Time    // expire time
	nRef atomic.Int64 // only for large file
}

const (
	fcacheKindDir = iota
	fcacheKindSmall
	fcacheKindLarge
)

func (e *fcacheEntry) isDir() bool   { return e.kind == fcacheKindDir }
func (e *fcacheEntry) isLarge() bool { return e.kind == fcacheKindLarge }
func (e *fcacheEntry) isSmall() bool { return e.kind == fcacheKindSmall }

func (e *fcacheEntry) addRef() {
	e.nRef.Add(1)
}
func (e *fcacheEntry) decRef() {
	if e.nRef.Add(-1) < 0 {
		if DebugLevel() >= 2 {
			Printf("fcache large entry closed: %s\n", e.file.Name())
		}
		e.file.Close()
	}
}

const signResolv = "resolv"

func createResolv(stage *Stage) *resolvFixture {
	resolv := new(resolvFixture)
	resolv.onCreate(stage)
	resolv.setShell(resolv)
	return resolv
}

// resolvFixture resolves names.
type resolvFixture struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (f *resolvFixture) onCreate(stage *Stage) {
	f.MakeComp(signResolv)
	f.stage = stage
}
func (f *resolvFixture) OnShutdown() {
	close(f.ShutChan) // notifies run()
}

func (f *resolvFixture) OnConfigure() {
}
func (f *resolvFixture) OnPrepare() {
}

func (f *resolvFixture) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Println("resolv done")
	}
	f.stage.DecSub() // resolv
}

func (f *resolvFixture) Register(name string, addresses []string) bool {
	// TODO
	return false
}

func (f *resolvFixture) Resolve(name string) (address string) {
	// TODO
	return ""
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
