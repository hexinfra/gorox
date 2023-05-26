// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Component is the configurable component.

package internal

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const ( // component list
	compStage      = 1 + iota // stage
	compFixture               // clock, fcache, resolver, http1Outgate, tcpsOutgate, ...
	compRunner                // ...
	compBackend               // HTTP1Backend, QUICBackend, UDPSBackend, ...
	compQUICRouter            // quicRouter
	compQUICDealer            // quicRelay, ...
	compQUICEditor            // ...
	compTCPSRouter            // tcpsRouter
	compTCPSDealer            // tcpsRelay, ...
	compTCPSEditor            // ...
	compUDPSRouter            // udpsRouter
	compUDPSDealer            // udpsRelay, ...
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
		Printf("stage id=%d shutdown start!!\n", s.id)
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
		Println("stage close log file")
	}
}

func (s *Stage) OnConfigure() {
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
	for _, file := range []string{s.cpuFile, s.hepFile, s.thrFile, s.grtFile, s.blkFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			EnvExitln(err.Error())
		}
	}

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
	if IsDebug(1) {
		Printf("stageID=%d\n", s.id)
		Printf("numCPU=%d\n", s.numCPU)
		Printf("baseDir=%s\n", BaseDir())
		Printf("logsDir=%s\n", LogsDir())
		Printf("tempDir=%s\n", TempDir())
		Printf("varsDir=%s\n", VarsDir())
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

	Printf("[worker] stage=%d is ready to serve.\n", s.id)
}
func (s *Stage) Quit() {
	s.OnShutdown()
	if IsDebug(2) {
		Printf("stage id=%d: quit.\n", s.id)
	}
}

func (s *Stage) linkServerApps() {
	if IsDebug(1) {
		Println("link apps to web servers")
	}
	for _, server := range s.servers {
		if webServer, ok := server.(webServer); ok {
			webServer.linkApps()
		}
	}
}
func (s *Stage) linkServerSvcs() {
	if IsDebug(1) {
		Println("link svcs to rpc servers")
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
			Printf("fixture=%s go run()\n", fixture.Name())
		}
		go fixture.run()
	}
}
func (s *Stage) startRunners() {
	for _, runner := range s.runners {
		if IsDebug(1) {
			Printf("runner=%s go Run()\n", runner.Name())
		}
		go runner.Run()
	}
}
func (s *Stage) startBackends() {
	for _, backend := range s.backends {
		if IsDebug(1) {
			Printf("backend=%s go maintain()\n", backend.Name())
		}
		go backend.Maintain()
	}
}
func (s *Stage) startRouters() {
	for _, quicRouter := range s.quicRouters {
		if IsDebug(1) {
			Printf("quicRouter=%s go serve()\n", quicRouter.Name())
		}
		go quicRouter.serve()
	}
	for _, tcpsRouter := range s.tcpsRouters {
		if IsDebug(1) {
			Printf("tcpsRouter=%s go serve()\n", tcpsRouter.Name())
		}
		go tcpsRouter.serve()
	}
	for _, udpsRouter := range s.udpsRouters {
		if IsDebug(1) {
			Printf("udpsRouter=%s go serve()\n", udpsRouter.Name())
		}
		go udpsRouter.serve()
	}
}
func (s *Stage) startStaters() {
	for _, stater := range s.staters {
		if IsDebug(1) {
			Printf("stater=%s go Maintain()\n", stater.Name())
		}
		go stater.Maintain()
	}
}
func (s *Stage) startCachers() {
	for _, cacher := range s.cachers {
		if IsDebug(1) {
			Printf("cacher=%s go Maintain()\n", cacher.Name())
		}
		go cacher.Maintain()
	}
}
func (s *Stage) startApps() {
	for _, app := range s.apps {
		if IsDebug(1) {
			Printf("app=%s go maintain()\n", app.Name())
		}
		go app.maintain()
	}
}
func (s *Stage) startSvcs() {
	for _, svc := range s.svcs {
		if IsDebug(1) {
			Printf("svc=%s go maintain()\n", svc.Name())
		}
		go svc.maintain()
	}
}
func (s *Stage) startServers() {
	for _, server := range s.servers {
		if IsDebug(1) {
			Printf("server=%s go Serve()\n", server.Name())
		}
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	for _, cronjob := range s.cronjobs {
		if IsDebug(1) {
			Printf("cronjob=%s go Schedule()\n", cronjob.Name())
		}
		go cronjob.Schedule()
	}
}

func (s *Stage) configure() (err error) {
	if IsDebug(1) {
		Println("now configure stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if IsDebug(1) {
			Println("stage configured")
		}
	}()
	s.OnConfigure()
	return nil
}
func (s *Stage) prepare() (err error) {
	if IsDebug(1) {
		Println("now prepare stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if IsDebug(1) {
			Println("stage prepared")
		}
	}()
	s.OnPrepare()
	return nil
}

func (s *Stage) ID() int32     { return s.id }
func (s *Stage) NumCPU() int32 { return s.numCPU }

/*
func (s *Stage) Log(str string)                  { s.logger.Print(str) }
func (s *Stage) Logln(str string)                { s.logger.Println(str) }
func (s *Stage) Logf(format string, args ...any) { s.logger.Printf(format, args...) }
*/

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

// client is the interface for outgates and backends.
type client interface {
	Stage() *Stage
	TLSMode() bool
	WriteTimeout() time.Duration
	ReadTimeout() time.Duration
	AliveTimeout() time.Duration
	nextConnID() int64
}

// client_ is a mixin for outgates and backends.
type client_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	tlsMode      bool          // use TLS?
	tlsConfig    *tls.Config   // TLS config if TLS is enabled
	dialTimeout  time.Duration // dial remote timeout
	writeTimeout time.Duration // write operation timeout
	readTimeout  time.Duration // read operation timeout
	aliveTimeout time.Duration // conn alive timeout
	connID       atomic.Int64  // next conn id
}

func (c *client_) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}

func (c *client_) onConfigure() {
	// tlsMode
	c.ConfigureBool("tlsMode", &c.tlsMode, false)
	if c.tlsMode {
		c.tlsConfig = new(tls.Config)
	}

	// dialTimeout
	c.ConfigureDuration("dialTimeout", &c.dialTimeout, func(value time.Duration) error {
		if value > time.Second {
			return nil
		}
		return errors.New(".dialTimeout has an invalid value")
	}, 10*time.Second)

	// writeTimeout
	c.ConfigureDuration("writeTimeout", &c.writeTimeout, func(value time.Duration) error {
		if value > time.Second {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 30*time.Second)

	// readTimeout
	c.ConfigureDuration("readTimeout", &c.readTimeout, func(value time.Duration) error {
		if value > time.Second {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 30*time.Second)

	// aliveTimeout
	c.ConfigureDuration("aliveTimeout", &c.aliveTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 5*time.Second)
}
func (c *client_) onPrepare() {
	// Currently nothing.
}

func (c *client_) OnShutdown() {
	close(c.Shut)
}

func (c *client_) Stage() *Stage               { return c.stage }
func (c *client_) TLSMode() bool               { return c.tlsMode }
func (c *client_) WriteTimeout() time.Duration { return c.writeTimeout }
func (c *client_) ReadTimeout() time.Duration  { return c.readTimeout }
func (c *client_) AliveTimeout() time.Duration { return c.aliveTimeout }

func (c *client_) nextConnID() int64 { return c.connID.Add(1) }

// outgate
type outgate interface {
	servedConns() int64
	servedStreams() int64
}

// outgate_ is the mixin for outgates.
type outgate_ struct {
	// Mixins
	client_
	// States
	nServedStreams atomic.Int64
	nServedExchans atomic.Int64
}

func (o *outgate_) onCreate(name string, stage *Stage) {
	o.client_.onCreate(name, stage)
}

func (o *outgate_) onConfigure() {
	o.client_.onConfigure()
}
func (o *outgate_) onPrepare() {
	o.client_.onPrepare()
}

func (o *outgate_) servedStreams() int64 { return o.nServedStreams.Load() }
func (o *outgate_) incServedStreams()    { o.nServedStreams.Add(1) }

func (o *outgate_) servedExchans() int64 { return o.nServedExchans.Load() }
func (o *outgate_) incServedExchans()    { o.nServedExchans.Add(1) }

// Backend is a group of nodes.
type Backend interface {
	Component
	client

	Maintain() // goroutine
}

// Backend_ is the mixin for backends.
type Backend_[N Node] struct {
	// Mixins
	client_
	// Assocs
	creator interface {
		createNode(id int32) N
	} // if Go's generic supports new(N) then this is not needed.
	nodes []N // nodes of this backend
	// States
}

func (b *Backend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.client_.onCreate(name, stage)
	b.creator = creator
}

func (b *Backend_[N]) onConfigure() {
	b.client_.onConfigure()
	// nodes
	v, ok := b.Find("nodes")
	if !ok {
		UseExitln("nodes is required for backends")
	}
	vNodes, ok := v.List()
	if !ok {
		UseExitln("nodes must be a list")
	}
	for id, elem := range vNodes {
		vNode, ok := elem.Dict()
		if !ok {
			UseExitln("node in nodes must be a dict")
		}
		node := b.creator.createNode(int32(id))

		// address
		vAddress, ok := vNode["address"]
		if !ok {
			UseExitln("address is required in node")
		}
		if address, ok := vAddress.String(); ok && address != "" {
			node.setAddress(address)
		}

		// weight
		vWeight, ok := vNode["weight"]
		if !ok {
			node.setWeight(1)
		} else if weight, ok := vWeight.Int32(); ok && weight > 0 {
			node.setWeight(weight)
		} else {
			UseExitln("bad weight in node")
		}

		// keepConns
		vKeepConns, ok := vNode["keepConns"]
		if !ok {
			node.setKeepConns(10)
		} else if keepConns, ok := vKeepConns.Int32(); ok && keepConns > 0 {
			node.setKeepConns(keepConns)
		} else {
			UseExitln("bad keepConns in node")
		}

		b.nodes = append(b.nodes, node)
	}
}
func (b *Backend_[N]) onPrepare() {
	b.client_.onPrepare()
}

func (b *Backend_[N]) Maintain() { // goroutine
	for _, node := range b.nodes {
		b.IncSub(1)
		go node.Maintain()
	}
	<-b.Shut

	// Backend is told to shutdown. Tell its nodes to shutdown too
	for _, node := range b.nodes {
		node.shut()
	}
	b.WaitSubs() // nodes
	if IsDebug(2) {
		Printf("backend=%s done\n", b.Name())
	}
	b.stage.SubDone()
}

// Node is a member of backend.
type Node interface {
	setAddress(address string)
	setWeight(weight int32)
	setKeepConns(keepConns int32)
	Maintain() // goroutine
	shut()
}

// Node_ is a mixin for backend nodes.
type Node_ struct {
	// Mixins
	subsWaiter_ // usually for conns
	shutdownable_
	// States
	id        int32       // the node id
	address   string      // hostname:port, /path/to/unix.sock
	weight    int32       // 1, 22, 333, ...
	keepConns int32       // max conns to keep alive
	down      atomic.Bool // TODO: false-sharing
	freeList  struct {    // free list of conns in this node
		sync.Mutex
		head conn // head element
		tail conn // tail element
		qnty int  // size of the list
	}
}

func (n *Node_) init(id int32) {
	n.shutdownable_.init()
	n.id = id
}

func (n *Node_) setAddress(address string)    { n.address = address }
func (n *Node_) setWeight(weight int32)       { n.weight = weight }
func (n *Node_) setKeepConns(keepConns int32) { n.keepConns = keepConns }

func (n *Node_) markDown()    { n.down.Store(true) }
func (n *Node_) markUp()      { n.down.Store(false) }
func (n *Node_) isDown() bool { return n.down.Load() }

func (n *Node_) pullConn() conn {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.getNext()
	conn.setNext(nil)
	list.qnty--
	return conn
}
func (n *Node_) pushConn(conn conn) {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		list.head = conn
		list.tail = conn
	} else { // >= 1
		list.tail.setNext(conn)
		list.tail = conn
	}
	list.qnty++
}

func (n *Node_) closeFree() int {
	list := &n.freeList
	list.Lock()
	defer list.Unlock()

	for conn := list.head; conn != nil; conn = conn.getNext() {
		conn.closeConn()
	}
	qnty := list.qnty
	list.qnty = 0
	list.head, list.tail = nil, nil
	return qnty
}

func (n *Node_) shut() {
	close(n.Shut)
}

var errNodeDown = errors.New("node is down")

// conn is the client conns.
type conn interface {
	getNext() conn
	setNext(next conn)
	isAlive() bool
	closeConn()
}

// conn_ is the mixin for client conns.
type conn_ struct {
	// Conn states (non-zeros)
	next   conn      // the link
	id     int64     // the conn id
	client client    // associated client
	expire time.Time // when the conn is considered expired
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

func (c *conn_) onGet(id int64, client client) {
	c.id = id
	c.client = client
	c.expire = time.Now().Add(client.AliveTimeout())
}
func (c *conn_) onPut() {
	c.client = nil
	c.expire = time.Time{}
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *conn_) getNext() conn     { return c.next }
func (c *conn_) setNext(next conn) { c.next = next }

func (c *conn_) isAlive() bool { return time.Now().Before(c.expire) }

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

// Server component.
type Server interface {
	Component
	Serve() // goroutine

	Stage() *Stage
	TLSMode() bool
	ColonPort() string
	ColonPortBytes() []byte
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// Server_ is the mixin for all servers.
type Server_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	address         string        // hostname:port
	colonPort       string        // like: ":9876"
	colonPortBytes  []byte        // like: []byte(":9876")
	tlsMode         bool          // tls mode?
	tlsConfig       *tls.Config   // set if is tls mode
	readTimeout     time.Duration // read() timeout
	writeTimeout    time.Duration // write() timeout
	numGates        int32         // number of gates
	maxConnsPerGate int32         // max concurrent connections allowed per gate
}

func (s *Server_) OnCreate(name string, stage *Stage) { // exported
	s.MakeComp(name)
	s.stage = stage
}

func (s *Server_) OnConfigure() {
	// address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok {
			if p := strings.IndexByte(address, ':'); p == -1 || p == len(address)-1 {
				UseExitln("bad address: " + address)
			} else {
				s.address = address
				s.colonPort = address[p:]
				s.colonPortBytes = []byte(s.colonPort)
			}
		} else {
			UseExitln("address should be of string type")
		}
	} else {
		UseExitln("address is required for servers")
	}

	// tlsMode
	s.ConfigureBool("tlsMode", &s.tlsMode, false)
	if s.tlsMode {
		s.tlsConfig = new(tls.Config)
	}

	// readTimeout
	s.ConfigureDuration("readTimeout", &s.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 60*time.Second)

	// writeTimeout
	s.ConfigureDuration("writeTimeout", &s.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 60*time.Second)

	// numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".numGates has an invalid value")
	}, s.stage.NumCPU())

	// maxConnsPerGate
	s.ConfigureInt32("maxConnsPerGate", &s.maxConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConnsPerGate has an invalid value")
	}, 100000)
}
func (s *Server_) OnPrepare() {
	// Currently nothing.
}

func (s *Server_) Stage() *Stage               { return s.stage }
func (s *Server_) Address() string             { return s.address }
func (s *Server_) ColonPort() string           { return s.colonPort }
func (s *Server_) ColonPortBytes() []byte      { return s.colonPortBytes }
func (s *Server_) TLSMode() bool               { return s.tlsMode }
func (s *Server_) ReadTimeout() time.Duration  { return s.readTimeout }
func (s *Server_) WriteTimeout() time.Duration { return s.writeTimeout }
func (s *Server_) NumGates() int32             { return s.numGates }
func (s *Server_) MaxConnsPerGate() int32      { return s.maxConnsPerGate }

// Gate is the interface for all gates.
type Gate interface {
	ID() int32
	IsShut() bool

	shut() error
}

// Gate_ is a mixin for router gates and server gates.
type Gate_ struct {
	// Mixins
	subsWaiter_ // for conns
	// Assocs
	stage *Stage // current stage
	// States
	id       int32        // gate id
	address  string       // listening address
	isShut   atomic.Bool  // is gate shut?
	maxConns int32        // max concurrent conns allowed
	numConns atomic.Int32 // TODO: false sharing
}

func (g *Gate_) Init(stage *Stage, id int32, address string, maxConns int32) {
	g.stage = stage
	g.id = id
	g.address = address
	g.isShut.Store(false)
	g.maxConns = maxConns
	g.numConns.Store(0)
}

func (g *Gate_) Stage() *Stage   { return g.stage }
func (g *Gate_) ID() int32       { return g.id }
func (g *Gate_) Address() string { return g.address }

func (g *Gate_) MarkShut()    { g.isShut.Store(true) }
func (g *Gate_) IsShut() bool { return g.isShut.Load() }

func (g *Gate_) DecConns() int32  { return g.numConns.Add(-1) }
func (g *Gate_) ReachLimit() bool { return g.numConns.Add(1) > g.maxConns }

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
