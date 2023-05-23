// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Component is the configurable component.

package internal

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
	"unsafe"
)

const ( // component list
	compStage      = 1 + iota // stage
	compFixture               // clock, fcache, resolver, http1Outgate, tcpsOutgate, ...
	compRunner                // ...
	compBackend               // HTTP1Backend, QUICBackend, UDPSBackend, ...
	compQUICRouter            // quicRouter
	compQUICDealer            // quicProxy, ...
	compQUICEditor            // ...
	compTCPSRouter            // tcpsRouter
	compTCPSDealer            // tcpsProxy, ...
	compTCPSEditor            // ...
	compUDPSRouter            // udpsRouter
	compUDPSDealer            // udpsProxy, ...
	compUDPSEditor            // ...
	compCase                  // case
	compStater                // localStater, redisStater, ...
	compCacher                // localCacher, redisCacher, ...
	compApp                   // app
	compHandlet               // static, ...
	compReviser               // gzipReviser, wrapReviser, ...
	compSocklet               // helloSocklet, ...
	compRule                  // rule
	compSvc                   // svc
	compServer                // httpxServer, echoServer, ...
	compCronjob               // cleanCronjob, statCronjob, ...
)

var signedComps = map[string]int16{ // static comps. more dynamic comps are signed using signComp() below
	"stage":      compStage,
	"quicRouter": compQUICRouter,
	"tcpsRouter": compTCPSRouter,
	"udpsRouter": compUDPSRouter,
	"case":       compCase,
	"app":        compApp,
	"rule":       compRule,
	"svc":        compSvc,
}

func signComp(sign string, comp int16) {
	if have, signed := signedComps[sign]; signed {
		BugExitf("conflicting sign: comp=%d sign=%s\n", have, sign)
	}
	signedComps[sign] = comp
}

var ( // global maps, shared between stages
	fixtureSigns       = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required
	creatorsLock       sync.RWMutex
	runnerCreators     = make(map[string]func(name string, stage *Stage) Runner) // indexed by sign, same below.
	backendCreators    = make(map[string]func(name string, stage *Stage) Backend)
	quicDealerCreators = make(map[string]func(name string, stage *Stage, router *QUICRouter) QUICDealer)
	quicEditorCreators = make(map[string]func(name string, stage *Stage, router *QUICRouter) QUICEditor)
	tcpsDealerCreators = make(map[string]func(name string, stage *Stage, router *TCPSRouter) TCPSDealer)
	tcpsEditorCreators = make(map[string]func(name string, stage *Stage, router *TCPSRouter) TCPSEditor)
	udpsDealerCreators = make(map[string]func(name string, stage *Stage, router *UDPSRouter) UDPSDealer)
	udpsEditorCreators = make(map[string]func(name string, stage *Stage, router *UDPSRouter) UDPSEditor)
	staterCreators     = make(map[string]func(name string, stage *Stage) Stater)
	cacherCreators     = make(map[string]func(name string, stage *Stage) Cacher)
	handletCreators    = make(map[string]func(name string, stage *Stage, app *App) Handlet)
	reviserCreators    = make(map[string]func(name string, stage *Stage, app *App) Reviser)
	sockletCreators    = make(map[string]func(name string, stage *Stage, app *App) Socklet)
	serverCreators     = make(map[string]func(name string, stage *Stage) Server)
	cronjobCreators    = make(map[string]func(name string, stage *Stage) Cronjob)
	initsLock          sync.RWMutex
	appInits           = make(map[string]func(app *App) error) // indexed by app name.
	svcInits           = make(map[string]func(svc *Svc) error) // indexed by svc name.
)

func registerFixture(sign string) {
	if _, ok := fixtureSigns[sign]; ok {
		BugExitln("fixture sign conflicted")
	}
	fixtureSigns[sign] = true
	signComp(sign, compFixture)
}

func RegisterRunner(sign string, create func(name string, stage *Stage) Runner) {
	_registerComponent0(sign, compRunner, runnerCreators, create)
}
func RegisterBackend(sign string, create func(name string, stage *Stage) Backend) {
	_registerComponent0(sign, compBackend, backendCreators, create)
}
func RegisterQUICDealer(sign string, create func(name string, stage *Stage, router *QUICRouter) QUICDealer) {
	_registerComponent1(sign, compQUICDealer, quicDealerCreators, create)
}
func RegisterQUICEditor(sign string, create func(name string, stage *Stage, router *QUICRouter) QUICEditor) {
	_registerComponent1(sign, compQUICEditor, quicEditorCreators, create)
}
func RegisterTCPSDealer(sign string, create func(name string, stage *Stage, router *TCPSRouter) TCPSDealer) {
	_registerComponent1(sign, compTCPSDealer, tcpsDealerCreators, create)
}
func RegisterTCPSEditor(sign string, create func(name string, stage *Stage, router *TCPSRouter) TCPSEditor) {
	_registerComponent1(sign, compTCPSEditor, tcpsEditorCreators, create)
}
func RegisterUDPSDealer(sign string, create func(name string, stage *Stage, router *UDPSRouter) UDPSDealer) {
	_registerComponent1(sign, compUDPSDealer, udpsDealerCreators, create)
}
func RegisterUDPSEditor(sign string, create func(name string, stage *Stage, router *UDPSRouter) UDPSEditor) {
	_registerComponent1(sign, compUDPSEditor, udpsEditorCreators, create)
}
func RegisterStater(sign string, create func(name string, stage *Stage) Stater) {
	_registerComponent0(sign, compStater, staterCreators, create)
}
func RegisterCacher(sign string, create func(name string, stage *Stage) Cacher) {
	_registerComponent0(sign, compCacher, cacherCreators, create)
}
func RegisterHandlet(sign string, create func(name string, stage *Stage, app *App) Handlet) {
	_registerComponent1(sign, compHandlet, handletCreators, create)
}
func RegisterReviser(sign string, create func(name string, stage *Stage, app *App) Reviser) {
	_registerComponent1(sign, compReviser, reviserCreators, create)
}
func RegisterSocklet(sign string, create func(name string, stage *Stage, app *App) Socklet) {
	_registerComponent1(sign, compSocklet, sockletCreators, create)
}
func RegisterServer(sign string, create func(name string, stage *Stage) Server) {
	_registerComponent0(sign, compServer, serverCreators, create)
}
func RegisterCronjob(sign string, create func(name string, stage *Stage) Cronjob) {
	_registerComponent0(sign, compCronjob, cronjobCreators, create)
}

func _registerComponent0[T Component](sign string, comp int16, creators map[string]func(string, *Stage) T, create func(string, *Stage) T) { // runner, backend, stater, cacher, server, cronjob
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[sign]; ok {
		BugExitln("component0 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}
func _registerComponent1[T Component, C Component](sign string, comp int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // dealer, editor, handlet, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[sign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}

func RegisterAppInit(name string, init func(app *App) error) {
	initsLock.Lock()
	appInits[name] = init
	initsLock.Unlock()
}
func RegisterSvcInit(name string, init func(svc *Svc) error) {
	initsLock.Lock()
	svcInits[name] = init
	initsLock.Unlock()
}

// Component is the interface for all components.
type Component interface {
	MakeComp(name string)
	OnShutdown()
	SubDone() // sub components call this

	Name() string

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

// Component_ is the mixin for all components.
type Component_ struct {
	// Mixins
	subsWaiter_
	shutdownable_
	// Assocs
	shell  Component // the concrete Component
	parent Component // the parent component, used by config
	// States
	name  string           // main, ...
	props map[string]Value // name1=value1, ...
	info  any              // extra info about this component, used by config
}

func (c *Component_) MakeComp(name string) {
	c.shutdownable_.init()
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

// createStage creates a new stage which runs alongside existing stage.
func createStage() *Stage {
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
	// Mixins
	Component_
	// Assocs
	clock        *clockFixture         // for fast accessing
	fcache       *fcacheFixture        // for fast accessing
	resolver     *resolverFixture      // for fast accessing
	http1Outgate *HTTP1Outgate         // for fast accessing
	http2Outgate *HTTP2Outgate         // for fast accessing
	http3Outgate *HTTP3Outgate         // for fast accessing
	hwebOutgate  *HWEBOutgate          // for fast accessing
	quicOutgate  *QUICOutgate          // for fast accessing
	tcpsOutgate  *TCPSOutgate          // for fast accessing
	udpsOutgate  *UDPSOutgate          // for fast accessing
	fixtures     compDict[fixture]     // indexed by sign
	runners      compDict[Runner]      // indexed by runnerName
	backends     compDict[Backend]     // indexed by backendName
	quicRouters  compDict[*QUICRouter] // indexed by routerName
	tcpsRouters  compDict[*TCPSRouter] // indexed by routerName
	udpsRouters  compDict[*UDPSRouter] // indexed by routerName
	staters      compDict[Stater]      // indexed by staterName
	cachers      compDict[Cacher]      // indexed by cacherName
	apps         compDict[*App]        // indexed by appName
	svcs         compDict[*Svc]        // indexed by svcName
	servers      compDict[Server]      // indexed by serverName
	cronjobs     compDict[Cronjob]     // indexed by cronjobName
	// States
	logFile string // stage's log file
	logger  *log.Logger
	cpuFile string
	hepFile string
	thrFile string
	grtFile string
	blkFile string
	id      int32
	numCPU  int32
}

func (s *Stage) onCreate() {
	s.MakeComp("stage")

	s.clock = createClock(s)
	s.fcache = createFcache(s)
	s.resolver = createResolver(s)
	s.http1Outgate = createHTTP1Outgate(s)
	s.http2Outgate = createHTTP2Outgate(s)
	s.http3Outgate = createHTTP3Outgate(s)
	s.hwebOutgate = createHWEBOutgate(s)
	s.quicOutgate = createQUICOutgate(s)
	s.tcpsOutgate = createTCPSOutgate(s)
	s.udpsOutgate = createUDPSOutgate(s)

	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFcache] = s.fcache
	s.fixtures[signResolver] = s.resolver
	s.fixtures[signHTTP1Outgate] = s.http1Outgate
	s.fixtures[signHTTP2Outgate] = s.http2Outgate
	s.fixtures[signHTTP3Outgate] = s.http3Outgate
	s.fixtures[signHWEBOutgate] = s.hwebOutgate
	s.fixtures[signQUICOutgate] = s.quicOutgate
	s.fixtures[signTCPSOutgate] = s.tcpsOutgate
	s.fixtures[signUDPSOutgate] = s.udpsOutgate

	s.runners = make(compDict[Runner])
	s.backends = make(compDict[Backend])
	s.quicRouters = make(compDict[*QUICRouter])
	s.tcpsRouters = make(compDict[*TCPSRouter])
	s.udpsRouters = make(compDict[*UDPSRouter])
	s.staters = make(compDict[Stater])
	s.cachers = make(compDict[Cacher])
	s.apps = make(compDict[*App])
	s.svcs = make(compDict[*Svc])
	s.servers = make(compDict[Server])
	s.cronjobs = make(compDict[Cronjob])
}
func (s *Stage) OnShutdown() {
	if IsDebug(2) {
		Debugf("stage id=%d shutdown start!!\n", s.id)
	}

	// cronjobs
	s.IncSub(len(s.cronjobs))
	s.cronjobs.goWalk(Cronjob.OnShutdown)
	s.WaitSubs()

	// servers
	s.IncSub(len(s.servers))
	s.servers.goWalk(Server.OnShutdown)
	s.WaitSubs()

	// svcs & apps
	s.IncSub(len(s.svcs) + len(s.apps))
	s.svcs.goWalk((*Svc).OnShutdown)
	s.apps.goWalk((*App).OnShutdown)
	s.WaitSubs()

	// cachers & staters
	s.IncSub(len(s.cachers) + len(s.staters))
	s.cachers.goWalk(Cacher.OnShutdown)
	s.staters.goWalk(Stater.OnShutdown)
	s.WaitSubs()

	// routers
	s.IncSub(len(s.udpsRouters) + len(s.tcpsRouters) + len(s.quicRouters))
	s.udpsRouters.goWalk((*UDPSRouter).OnShutdown)
	s.tcpsRouters.goWalk((*TCPSRouter).OnShutdown)
	s.quicRouters.goWalk((*QUICRouter).OnShutdown)
	s.WaitSubs()

	// backends
	s.IncSub(len(s.backends))
	s.backends.goWalk(Backend.OnShutdown)
	s.WaitSubs()

	// runners
	s.IncSub(len(s.runners))
	s.runners.goWalk(Runner.OnShutdown)
	s.WaitSubs()

	// fixtures
	s.IncSub(7)
	go s.http1Outgate.OnShutdown() // we don't treat this as goroutine
	go s.http2Outgate.OnShutdown() // we don't treat this as goroutine
	go s.http3Outgate.OnShutdown() // we don't treat this as goroutine
	go s.hwebOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.quicOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.tcpsOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.udpsOutgate.OnShutdown()  // we don't treat this as goroutine
	s.WaitSubs()

	s.IncSub(1)
	s.fcache.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.resolver.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.clock.OnShutdown()
	s.WaitSubs()

	// stage
	if IsDebug(2) {
		Debugln("stage close log file")
	}
	s.logger.Writer().(*os.File).Close()
}

func (s *Stage) OnConfigure() {
	// logFile
	s.ConfigureString("logFile", &s.logFile, func(value string) error {
		if value == "" {
			return errors.New(".logFile has an invalid value")
		}
		return nil
	}, LogsDir()+"/worker.log")

	tempDir := TempDir()

	// cpuFile
	s.ConfigureString("cpuFile", &s.cpuFile, func(value string) error {
		if value == "" {
			return errors.New(".cpuFile has an invalid value")
		}
		return nil
	}, tempDir+"/cpu.prof")

	// hepFile
	s.ConfigureString("hepFile", &s.hepFile, func(value string) error {
		if value == "" {
			return errors.New(".hepFile has an invalid value")
		}
		return nil
	}, tempDir+"/hep.prof")

	// thrFile
	s.ConfigureString("thrFile", &s.thrFile, func(value string) error {
		if value == "" {
			return errors.New(".thrFile has an invalid value")
		}
		return nil
	}, tempDir+"/thr.prof")

	// grtFile
	s.ConfigureString("grtFile", &s.grtFile, func(value string) error {
		if value == "" {
			return errors.New(".grtFile has an invalid value")
		}
		return nil
	}, tempDir+"/grt.prof")

	// blkFile
	s.ConfigureString("blkFile", &s.blkFile, func(value string) error {
		if value == "" {
			return errors.New(".blkFile has an invalid value")
		}
		return nil
	}, tempDir+"/blk.prof")

	// sub components
	s.fixtures.walk(fixture.OnConfigure)
	s.runners.walk(Runner.OnConfigure)
	s.backends.walk(Backend.OnConfigure)
	s.quicRouters.walk((*QUICRouter).OnConfigure)
	s.tcpsRouters.walk((*TCPSRouter).OnConfigure)
	s.udpsRouters.walk((*UDPSRouter).OnConfigure)
	s.staters.walk(Stater.OnConfigure)
	s.cachers.walk(Cacher.OnConfigure)
	s.apps.walk((*App).OnConfigure)
	s.svcs.walk((*Svc).OnConfigure)
	s.servers.walk(Server.OnConfigure)
	s.cronjobs.walk(Cronjob.OnConfigure)
}
func (s *Stage) OnPrepare() {
	for _, file := range []string{s.logFile, s.cpuFile, s.hepFile, s.thrFile, s.grtFile, s.blkFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			EnvExitln(err.Error())
		}
	}
	osFile, err := os.OpenFile(s.logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		EnvExitln(err.Error())
	}
	s.logger = log.New(osFile, "", log.Ldate|log.Ltime)

	// sub components
	s.fixtures.walk(fixture.OnPrepare)
	s.runners.walk(Runner.OnPrepare)
	s.backends.walk(Backend.OnPrepare)
	s.quicRouters.walk((*QUICRouter).OnPrepare)
	s.tcpsRouters.walk((*TCPSRouter).OnPrepare)
	s.udpsRouters.walk((*UDPSRouter).OnPrepare)
	s.staters.walk(Stater.OnPrepare)
	s.cachers.walk(Cacher.OnPrepare)
	s.apps.walk((*App).OnPrepare)
	s.svcs.walk((*Svc).OnPrepare)
	s.servers.walk(Server.OnPrepare)
	s.cronjobs.walk(Cronjob.OnPrepare)
}

func (s *Stage) createRunner(sign string, name string) Runner {
	if s.Runner(name) != nil {
		UseExitf("conflicting runner with a same name '%s'\n", name)
	}
	create, ok := runnerCreators[sign]
	if !ok {
		UseExitln("unknown runner type: " + sign)
	}
	runner := create(name, s)
	runner.setShell(runner)
	s.runners[name] = runner
	return runner
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
func (s *Stage) createQUICRouter(name string) *QUICRouter {
	if s.QUICRouter(name) != nil {
		UseExitf("conflicting quicRouter with a same name '%s'\n", name)
	}
	router := new(QUICRouter)
	router.onCreate(name, s)
	router.setShell(router)
	s.quicRouters[name] = router
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
func (s *Stage) createApp(name string) *App {
	if s.App(name) != nil {
		UseExitf("conflicting app with a same name '%s'\n", name)
	}
	app := new(App)
	app.onCreate(name, s)
	app.setShell(app)
	s.apps[name] = app
	return app
}
func (s *Stage) createSvc(name string) *Svc {
	if s.Svc(name) != nil {
		UseExitf("conflicting svc with a same name '%s'\n", name)
	}
	svc := new(Svc)
	svc.onCreate(name, s)
	svc.setShell(svc)
	s.svcs[name] = svc
	return svc
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

func (s *Stage) Clock() *clockFixture        { return s.clock }
func (s *Stage) Fcache() *fcacheFixture      { return s.fcache }
func (s *Stage) Resolver() *resolverFixture  { return s.resolver }
func (s *Stage) HTTP1Outgate() *HTTP1Outgate { return s.http1Outgate }
func (s *Stage) HTTP2Outgate() *HTTP2Outgate { return s.http2Outgate }
func (s *Stage) HTTP3Outgate() *HTTP3Outgate { return s.http3Outgate }
func (s *Stage) HWEBOutgate() *HWEBOutgate   { return s.hwebOutgate }
func (s *Stage) QUICOutgate() *QUICOutgate   { return s.quicOutgate }
func (s *Stage) TCPSOutgate() *TCPSOutgate   { return s.tcpsOutgate }
func (s *Stage) UDPSOutgate() *UDPSOutgate   { return s.udpsOutgate }

func (s *Stage) fixture(sign string) fixture        { return s.fixtures[sign] }
func (s *Stage) Runner(name string) Runner          { return s.runners[name] }
func (s *Stage) Backend(name string) Backend        { return s.backends[name] }
func (s *Stage) QUICRouter(name string) *QUICRouter { return s.quicRouters[name] }
func (s *Stage) TCPSRouter(name string) *TCPSRouter { return s.tcpsRouters[name] }
func (s *Stage) UDPSRouter(name string) *UDPSRouter { return s.udpsRouters[name] }
func (s *Stage) Stater(name string) Stater          { return s.staters[name] }
func (s *Stage) Cacher(name string) Cacher          { return s.cachers[name] }
func (s *Stage) App(name string) *App               { return s.apps[name] }
func (s *Stage) Svc(name string) *Svc               { return s.svcs[name] }
func (s *Stage) Server(name string) Server          { return s.servers[name] }
func (s *Stage) Cronjob(name string) Cronjob        { return s.cronjobs[name] }

func (s *Stage) Start(id int32) {
	s.id = id
	s.numCPU = int32(runtime.NumCPU())

	if IsDebug(2) {
		Debugf("size of http1Conn = %d\n", unsafe.Sizeof(http1Conn{}))
		Debugf("size of http2Conn = %d\n", unsafe.Sizeof(http2Conn{}))
		Debugf("size of http3Conn = %d\n", unsafe.Sizeof(http3Conn{}))
		Debugf("size of http2Stream = %d\n", unsafe.Sizeof(http2Stream{}))
		Debugf("size of http3Stream = %d\n", unsafe.Sizeof(http3Stream{}))
		Debugf("size of H1Conn = %d\n", unsafe.Sizeof(H1Conn{}))
		Debugf("size of H2Conn = %d\n", unsafe.Sizeof(H2Conn{}))
		Debugf("size of H3Conn = %d\n", unsafe.Sizeof(H3Conn{}))
		Debugf("size of H2Stream = %d\n", unsafe.Sizeof(H2Stream{}))
		Debugf("size of H3Stream = %d\n", unsafe.Sizeof(H3Stream{}))
	}
	if IsDebug(1) {
		Debugf("stageID=%d\n", s.id)
		Debugf("numCPU=%d\n", s.numCPU)
		Debugf("baseDir=%s\n", BaseDir())
		Debugf("logsDir=%s\n", LogsDir())
		Debugf("tempDir=%s\n", TempDir())
		Debugf("varsDir=%s\n", VarsDir())
	}

	// Init running environment
	rand.Seed(time.Now().UnixNano())
	if err := os.Chdir(BaseDir()); err != nil {
		EnvExitln(err.Error())
	}

	// Configure all components
	if err := s.configure(); err != nil {
		UseExitln(err.Error())
	}

	s.linkServerApps()
	s.linkServerSvcs()

	// Prepare all components
	if err := s.prepare(); err != nil {
		EnvExitln(err.Error())
	}

	// Start all components
	s.startFixtures() // go fixture.run()
	s.startRunners()  // go runner.Run()
	s.startBackends() // go backend.maintain()
	s.startRouters()  // go router.serve()
	s.startStaters()  // go stater.Maintain()
	s.startCachers()  // go cacher.Maintain()
	s.startApps()     // go app.maintain()
	s.startSvcs()     // go svc.maintain()
	s.startServers()  // go server.Serve()
	s.startCronjobs() // go cronjob.Schedule()

	s.Logf("stage=%d is ready to serve.\n", s.id)
}
func (s *Stage) Quit() {
	s.OnShutdown()
	if IsDebug(2) {
		Debugf("stage id=%d: quit.\n", s.id)
	}
}

func (s *Stage) linkServerApps() {
	if IsDebug(1) {
		Debugln("link apps to web servers")
	}
	for _, server := range s.servers {
		if webServer, ok := server.(webServer); ok {
			webServer.linkApps()
		}
	}
}
func (s *Stage) linkServerSvcs() {
	if IsDebug(1) {
		Debugln("link svcs to rpc servers")
	}
	for _, server := range s.servers {
		if rpcServer, ok := server.(rpcServer); ok {
			rpcServer.LinkSvcs()
		}
	}
}

func (s *Stage) startFixtures() {
	for _, fixture := range s.fixtures {
		if IsDebug(1) {
			Debugf("fixture=%s go run()\n", fixture.Name())
		}
		go fixture.run()
	}
}
func (s *Stage) startRunners() {
	for _, runner := range s.runners {
		if IsDebug(1) {
			Debugf("runner=%s go Run()\n", runner.Name())
		}
		go runner.Run()
	}
}
func (s *Stage) startBackends() {
	for _, backend := range s.backends {
		if IsDebug(1) {
			Debugf("backend=%s go maintain()\n", backend.Name())
		}
		go backend.Maintain()
	}
}
func (s *Stage) startRouters() {
	for _, quicRouter := range s.quicRouters {
		if IsDebug(1) {
			Debugf("quicRouter=%s go serve()\n", quicRouter.Name())
		}
		go quicRouter.serve()
	}
	for _, tcpsRouter := range s.tcpsRouters {
		if IsDebug(1) {
			Debugf("tcpsRouter=%s go serve()\n", tcpsRouter.Name())
		}
		go tcpsRouter.serve()
	}
	for _, udpsRouter := range s.udpsRouters {
		if IsDebug(1) {
			Debugf("udpsRouter=%s go serve()\n", udpsRouter.Name())
		}
		go udpsRouter.serve()
	}
}
func (s *Stage) startStaters() {
	for _, stater := range s.staters {
		if IsDebug(1) {
			Debugf("stater=%s go Maintain()\n", stater.Name())
		}
		go stater.Maintain()
	}
}
func (s *Stage) startCachers() {
	for _, cacher := range s.cachers {
		if IsDebug(1) {
			Debugf("cacher=%s go Maintain()\n", cacher.Name())
		}
		go cacher.Maintain()
	}
}
func (s *Stage) startApps() {
	for _, app := range s.apps {
		if IsDebug(1) {
			Debugf("app=%s go maintain()\n", app.Name())
		}
		go app.maintain()
	}
}
func (s *Stage) startSvcs() {
	for _, svc := range s.svcs {
		if IsDebug(1) {
			Debugf("svc=%s go maintain()\n", svc.Name())
		}
		go svc.maintain()
	}
}
func (s *Stage) startServers() {
	for _, server := range s.servers {
		if IsDebug(1) {
			Debugf("server=%s go Serve()\n", server.Name())
		}
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	for _, cronjob := range s.cronjobs {
		if IsDebug(1) {
			Debugf("cronjob=%s go Schedule()\n", cronjob.Name())
		}
		go cronjob.Schedule()
	}
}

func (s *Stage) configure() (err error) {
	if IsDebug(1) {
		Debugln("now configure stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if IsDebug(1) {
			Debugln("stage configured")
		}
	}()
	s.OnConfigure()
	return nil
}
func (s *Stage) prepare() (err error) {
	if IsDebug(1) {
		Debugln("now prepare stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if IsDebug(1) {
			Debugln("stage prepared")
		}
	}()
	s.OnPrepare()
	return nil
}

func (s *Stage) ID() int32     { return s.id }
func (s *Stage) NumCPU() int32 { return s.numCPU }

func (s *Stage) Log(str string)                  { s.logger.Print(str) }
func (s *Stage) Logln(str string)                { s.logger.Println(str) }
func (s *Stage) Logf(format string, args ...any) { s.logger.Printf(format, args...) }

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

// fixture component.
//
// Fixtures only exist in internal, and are created by stage.
// Some critical functions, like clock and name resolver, are
// implemented as fixtures.
//
// Fixtures are singletons in stage.
type fixture interface {
	Component
	run() // goroutine
}

// Runner component.
//
// Runners are plugins or addons for Gorox. Users can create their own runners.
type Runner interface {
	Component
	Run() // goroutine
}

// Stater component is the interface to storages of HTTP states. See RFC 6265.
type Stater interface {
	Component
	Maintain() // goroutine
	Set(sid []byte, session *Session)
	Get(sid []byte) (session *Session)
	Del(sid []byte) bool
}

// Stater_
type Stater_ struct {
	// Mixins
	Component_
}

// Session is an HTTP session in stater
type Session struct {
	// TODO
	ID     [40]byte // session id
	Secret [40]byte // secret
	Role   int8     // 0: default, >0: app defined values
	Device int8     // terminal device type
	state1 int8     // app defined state1
	state2 int8     // app defined state2
	state3 int32    // app defined state3
	expire int64    // unix time
	states map[string]string
}

func (s *Session) init() {
	s.states = make(map[string]string)
}

func (s *Session) Get(name string) string        { return s.states[name] }
func (s *Session) Set(name string, value string) { s.states[name] = value }
func (s *Session) Del(name string)               { delete(s.states, name) }

// Cronjob component
type Cronjob interface {
	Component
	Schedule() // goroutine
}

// Cronjob_ is the mixin for all cronjobs.
type Cronjob_ struct {
	// Mixins
	Component_
	// States
}
