// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Component is the configurable component.

package internal

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

const ( // component list
	compStage      = 1 + iota // stage
	compFixture               // clock, fcache, resolv, http1Outgate, tcpsOutgate, ...
	compAddon                 // ...
	compBackend               // HTTP1Backend, QUICBackend, UDPSBackend, ...
	compQUICMesher            // quicMesher
	compQUICFilter            // quicProxy, ...
	compTCPSMesher            // tcpsMesher
	compTCPSFilter            // tcpsProxy, ...
	compUDPSMesher            // udpsMesher
	compUDPSFilter            // udpsProxy, ...
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
	"quicMesher": compQUICMesher,
	"tcpsMesher": compTCPSMesher,
	"udpsMesher": compUDPSMesher,
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

var fixtureSigns = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required

func registerFixture(sign string) {
	if _, ok := fixtureSigns[sign]; ok {
		BugExitln("fixture sign conflicted")
	}
	fixtureSigns[sign] = true
	signComp(sign, compFixture)
}

var (
	creatorsLock       sync.RWMutex
	addonCreators      = make(map[string]func(name string, stage *Stage) Addon) // indexed by sign, same below.
	backendCreators    = make(map[string]func(name string, stage *Stage) Backend)
	quicFilterCreators = make(map[string]func(name string, stage *Stage, mesher *QUICMesher) QUICFilter)
	tcpsFilterCreators = make(map[string]func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter)
	udpsFilterCreators = make(map[string]func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter)
	staterCreators     = make(map[string]func(name string, stage *Stage) Stater)
	cacherCreators     = make(map[string]func(name string, stage *Stage) Cacher)
	handletCreators    = make(map[string]func(name string, stage *Stage, app *App) Handlet)
	reviserCreators    = make(map[string]func(name string, stage *Stage, app *App) Reviser)
	sockletCreators    = make(map[string]func(name string, stage *Stage, app *App) Socklet)
	serverCreators     = make(map[string]func(name string, stage *Stage) Server)
	cronjobCreators    = make(map[string]func(name string, stage *Stage) Cronjob)
)

func RegisterAddon(sign string, create func(name string, stage *Stage) Addon) {
	_registerComponent0(sign, compAddon, addonCreators, create)
}
func RegisterBackend(sign string, create func(name string, stage *Stage) Backend) {
	_registerComponent0(sign, compBackend, backendCreators, create)
}
func RegisterQUICFilter(sign string, create func(name string, stage *Stage, mesher *QUICMesher) QUICFilter) {
	_registerComponent1(sign, compQUICFilter, quicFilterCreators, create)
}
func RegisterTCPSFilter(sign string, create func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter) {
	_registerComponent1(sign, compTCPSFilter, tcpsFilterCreators, create)
}
func RegisterUDPSFilter(sign string, create func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter) {
	_registerComponent1(sign, compUDPSFilter, udpsFilterCreators, create)
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

func _registerComponent0[T Component](sign string, comp int16, creators map[string]func(string, *Stage) T, create func(string, *Stage) T) { // addon, backend, stater, cacher, server, cronjob
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[sign]; ok {
		BugExitln("component0 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}
func _registerComponent1[T Component, C Component](sign string, comp int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // filter, handlet, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()

	if _, ok := creators[sign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}

var (
	initsLock sync.RWMutex
	appInits  = make(map[string]func(app *App) error) // indexed by app name.
	svcInits  = make(map[string]func(svc *Svc) error) // indexed by svc name.
)

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
	resolv       *resolvFixture        // for fast accessing
	quicOutgate  *QUICOutgate          // for fast accessing
	qudsOutgate  *QUDSOutgate          // for fast accessing
	tcpsOutgate  *TCPSOutgate          // for fast accessing
	tudsOutgate  *TUDSOutgate          // for fast accessing
	udpsOutgate  *UDPSOutgate          // for fast accessing
	uudsOutgate  *UUDSOutgate          // for fast accessing
	hrpcOutgate  *HRPCOutgate          // for fast accessing
	http1Outgate *HTTP1Outgate         // for fast accessing
	http2Outgate *HTTP2Outgate         // for fast accessing
	http3Outgate *HTTP3Outgate         // for fast accessing
	hwebOutgate  *HWEBOutgate          // for fast accessing
	fixtures     compDict[fixture]     // indexed by sign
	addons       compDict[Addon]       // indexed by addonName
	backends     compDict[Backend]     // indexed by backendName
	quicMeshers  compDict[*QUICMesher] // indexed by mesherName
	tcpsMeshers  compDict[*TCPSMesher] // indexed by mesherName
	udpsMeshers  compDict[*UDPSMesher] // indexed by mesherName
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
	s.resolv = createResolv(s)
	s.quicOutgate = createQUICOutgate(s)
	s.qudsOutgate = createQUDSOutgate(s)
	s.tcpsOutgate = createTCPSOutgate(s)
	s.tudsOutgate = createTUDSOutgate(s)
	s.udpsOutgate = createUDPSOutgate(s)
	s.uudsOutgate = createUUDSOutgate(s)
	s.hrpcOutgate = createHRPCOutgate(s)
	s.http1Outgate = createHTTP1Outgate(s)
	s.http2Outgate = createHTTP2Outgate(s)
	s.http3Outgate = createHTTP3Outgate(s)
	s.hwebOutgate = createHWEBOutgate(s)

	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFcache] = s.fcache
	s.fixtures[signResolv] = s.resolv
	s.fixtures[signQUICOutgate] = s.quicOutgate
	s.fixtures[signQUDSOutgate] = s.qudsOutgate
	s.fixtures[signTCPSOutgate] = s.tcpsOutgate
	s.fixtures[signTUDSOutgate] = s.tudsOutgate
	s.fixtures[signUDPSOutgate] = s.udpsOutgate
	s.fixtures[signUUDSOutgate] = s.uudsOutgate
	s.fixtures[signHRPCOutgate] = s.hrpcOutgate
	s.fixtures[signHTTP1Outgate] = s.http1Outgate
	s.fixtures[signHTTP2Outgate] = s.http2Outgate
	s.fixtures[signHTTP3Outgate] = s.http3Outgate
	s.fixtures[signHWEBOutgate] = s.hwebOutgate

	s.addons = make(compDict[Addon])
	s.backends = make(compDict[Backend])
	s.quicMeshers = make(compDict[*QUICMesher])
	s.tcpsMeshers = make(compDict[*TCPSMesher])
	s.udpsMeshers = make(compDict[*UDPSMesher])
	s.staters = make(compDict[Stater])
	s.cachers = make(compDict[Cacher])
	s.apps = make(compDict[*App])
	s.svcs = make(compDict[*Svc])
	s.servers = make(compDict[Server])
	s.cronjobs = make(compDict[Cronjob])
}
func (s *Stage) OnShutdown() {
	if Debug() >= 2 {
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

	// meshers
	s.IncSub(len(s.udpsMeshers) + len(s.tcpsMeshers) + len(s.quicMeshers))
	s.udpsMeshers.goWalk((*UDPSMesher).OnShutdown)
	s.tcpsMeshers.goWalk((*TCPSMesher).OnShutdown)
	s.quicMeshers.goWalk((*QUICMesher).OnShutdown)
	s.WaitSubs()

	// backends
	s.IncSub(len(s.backends))
	s.backends.goWalk(Backend.OnShutdown)
	s.WaitSubs()

	// addons
	s.IncSub(len(s.addons))
	s.addons.goWalk(Addon.OnShutdown)
	s.WaitSubs()

	// fixtures
	s.IncSub(11)
	go s.quicOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.qudsOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.tcpsOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.tudsOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.udpsOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.uudsOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.hrpcOutgate.OnShutdown()  // we don't treat this as goroutine
	go s.http1Outgate.OnShutdown() // we don't treat this as goroutine
	go s.http2Outgate.OnShutdown() // we don't treat this as goroutine
	go s.http3Outgate.OnShutdown() // we don't treat this as goroutine
	go s.hwebOutgate.OnShutdown()  // we don't treat this as goroutine
	s.WaitSubs()

	s.IncSub(1)
	s.fcache.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.resolv.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.clock.OnShutdown()
	s.WaitSubs()

	// stage
	if Debug() >= 2 {
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
	s.addons.walk(Addon.OnConfigure)
	s.backends.walk(Backend.OnConfigure)
	s.quicMeshers.walk((*QUICMesher).OnConfigure)
	s.tcpsMeshers.walk((*TCPSMesher).OnConfigure)
	s.udpsMeshers.walk((*UDPSMesher).OnConfigure)
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
	s.addons.walk(Addon.OnPrepare)
	s.backends.walk(Backend.OnPrepare)
	s.quicMeshers.walk((*QUICMesher).OnPrepare)
	s.tcpsMeshers.walk((*TCPSMesher).OnPrepare)
	s.udpsMeshers.walk((*UDPSMesher).OnPrepare)
	s.staters.walk(Stater.OnPrepare)
	s.cachers.walk(Cacher.OnPrepare)
	s.apps.walk((*App).OnPrepare)
	s.svcs.walk((*Svc).OnPrepare)
	s.servers.walk(Server.OnPrepare)
	s.cronjobs.walk(Cronjob.OnPrepare)
}

func (s *Stage) createAddon(sign string, name string) Addon {
	if s.Addon(name) != nil {
		UseExitf("conflicting addon with a same name '%s'\n", name)
	}
	create, ok := addonCreators[sign]
	if !ok {
		UseExitln("unknown addon type: " + sign)
	}
	addon := create(name, s)
	addon.setShell(addon)
	s.addons[name] = addon
	return addon
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
func (s *Stage) createQUICMesher(name string) *QUICMesher {
	if s.QUICMesher(name) != nil {
		UseExitf("conflicting quicMesher with a same name '%s'\n", name)
	}
	mesher := new(QUICMesher)
	mesher.onCreate(name, s)
	mesher.setShell(mesher)
	s.quicMeshers[name] = mesher
	return mesher
}
func (s *Stage) createTCPSMesher(name string) *TCPSMesher {
	if s.TCPSMesher(name) != nil {
		UseExitf("conflicting tcpsMesher with a same name '%s'\n", name)
	}
	mesher := new(TCPSMesher)
	mesher.onCreate(name, s)
	mesher.setShell(mesher)
	s.tcpsMeshers[name] = mesher
	return mesher
}
func (s *Stage) createUDPSMesher(name string) *UDPSMesher {
	if s.UDPSMesher(name) != nil {
		UseExitf("conflicting udpsMesher with a same name '%s'\n", name)
	}
	mesher := new(UDPSMesher)
	mesher.onCreate(name, s)
	mesher.setShell(mesher)
	s.udpsMeshers[name] = mesher
	return mesher
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
func (s *Stage) Resolv() *resolvFixture      { return s.resolv }
func (s *Stage) QUICOutgate() *QUICOutgate   { return s.quicOutgate }
func (s *Stage) QUDSOutgate() *QUDSOutgate   { return s.qudsOutgate }
func (s *Stage) TCPSOutgate() *TCPSOutgate   { return s.tcpsOutgate }
func (s *Stage) TUDSOutgate() *TUDSOutgate   { return s.tudsOutgate }
func (s *Stage) UDPSOutgate() *UDPSOutgate   { return s.udpsOutgate }
func (s *Stage) UUDSOutgate() *UUDSOutgate   { return s.uudsOutgate }
func (s *Stage) HRPCOutgate() *HRPCOutgate   { return s.hrpcOutgate }
func (s *Stage) HTTP1Outgate() *HTTP1Outgate { return s.http1Outgate }
func (s *Stage) HTTP2Outgate() *HTTP2Outgate { return s.http2Outgate }
func (s *Stage) HTTP3Outgate() *HTTP3Outgate { return s.http3Outgate }
func (s *Stage) HWEBOutgate() *HWEBOutgate   { return s.hwebOutgate }

func (s *Stage) fixture(sign string) fixture        { return s.fixtures[sign] }
func (s *Stage) Addon(name string) Addon            { return s.addons[name] }
func (s *Stage) Backend(name string) Backend        { return s.backends[name] }
func (s *Stage) QUICMesher(name string) *QUICMesher { return s.quicMeshers[name] }
func (s *Stage) TCPSMesher(name string) *TCPSMesher { return s.tcpsMeshers[name] }
func (s *Stage) UDPSMesher(name string) *UDPSMesher { return s.udpsMeshers[name] }
func (s *Stage) Stater(name string) Stater          { return s.staters[name] }
func (s *Stage) Cacher(name string) Cacher          { return s.cachers[name] }
func (s *Stage) App(name string) *App               { return s.apps[name] }
func (s *Stage) Svc(name string) *Svc               { return s.svcs[name] }
func (s *Stage) Server(name string) Server          { return s.servers[name] }
func (s *Stage) Cronjob(name string) Cronjob        { return s.cronjobs[name] }

func (s *Stage) Start(id int32) {
	s.id = id
	s.numCPU = int32(runtime.NumCPU())

	if Debug() >= 2 {
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
	if Debug() >= 1 {
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

	// Configure all components
	if err := s.configure(); err != nil {
		UseExitln(err.Error())
	}

	s.bindServerApps()
	s.bindServerSvcs()

	// Prepare all components
	if err := s.prepare(); err != nil {
		EnvExitln(err.Error())
	}

	// Start all components
	s.startFixtures() // go fixture.run()
	s.startAddons()   // go addon.Run()
	s.startBackends() // go backend.maintain()
	s.startMeshers()  // go mesher.serve()
	s.startStaters()  // go stater.Maintain()
	s.startCachers()  // go cacher.Maintain()
	s.startApps()     // go app.maintain()
	s.startSvcs()     // go svc.maintain()
	s.startServers()  // go server.Serve()
	s.startCronjobs() // go cronjob.Schedule()
}
func (s *Stage) Quit() {
	s.OnShutdown()
	if Debug() >= 2 {
		Printf("stage id=%d: quit.\n", s.id)
	}
}

func (s *Stage) bindServerApps() {
	if Debug() >= 1 {
		Println("bind apps to web servers")
	}
	for _, server := range s.servers {
		if webServer, ok := server.(webServer); ok {
			webServer.bindApps()
		}
	}
}
func (s *Stage) bindServerSvcs() {
	if Debug() >= 1 {
		Println("bind svcs to rpc servers")
	}
	for _, server := range s.servers {
		if rpcServer, ok := server.(rpcServer); ok {
			rpcServer.BindSvcs()
		}
	}
}

func (s *Stage) startFixtures() {
	for _, fixture := range s.fixtures {
		if Debug() >= 1 {
			Printf("fixture=%s go run()\n", fixture.Name())
		}
		go fixture.run()
	}
}
func (s *Stage) startAddons() {
	for _, addon := range s.addons {
		if Debug() >= 1 {
			Printf("addon=%s go Run()\n", addon.Name())
		}
		go addon.Run()
	}
}
func (s *Stage) startBackends() {
	for _, backend := range s.backends {
		if Debug() >= 1 {
			Printf("backend=%s go maintain()\n", backend.Name())
		}
		go backend.Maintain()
	}
}
func (s *Stage) startMeshers() {
	for _, quicMesher := range s.quicMeshers {
		if Debug() >= 1 {
			Printf("quicMesher=%s go serve()\n", quicMesher.Name())
		}
		go quicMesher.serve()
	}
	for _, tcpsMesher := range s.tcpsMeshers {
		if Debug() >= 1 {
			Printf("tcpsMesher=%s go serve()\n", tcpsMesher.Name())
		}
		go tcpsMesher.serve()
	}
	for _, udpsMesher := range s.udpsMeshers {
		if Debug() >= 1 {
			Printf("udpsMesher=%s go serve()\n", udpsMesher.Name())
		}
		go udpsMesher.serve()
	}
}
func (s *Stage) startStaters() {
	for _, stater := range s.staters {
		if Debug() >= 1 {
			Printf("stater=%s go Maintain()\n", stater.Name())
		}
		go stater.Maintain()
	}
}
func (s *Stage) startCachers() {
	for _, cacher := range s.cachers {
		if Debug() >= 1 {
			Printf("cacher=%s go Maintain()\n", cacher.Name())
		}
		go cacher.Maintain()
	}
}
func (s *Stage) startApps() {
	for _, app := range s.apps {
		if Debug() >= 1 {
			Printf("app=%s go maintain()\n", app.Name())
		}
		go app.maintain()
	}
}
func (s *Stage) startSvcs() {
	for _, svc := range s.svcs {
		if Debug() >= 1 {
			Printf("svc=%s go maintain()\n", svc.Name())
		}
		go svc.maintain()
	}
}
func (s *Stage) startServers() {
	for _, server := range s.servers {
		if Debug() >= 1 {
			Printf("server=%s go Serve()\n", server.Name())
		}
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	for _, cronjob := range s.cronjobs {
		if Debug() >= 1 {
			Printf("cronjob=%s go Schedule()\n", cronjob.Name())
		}
		go cronjob.Schedule()
	}
}

func (s *Stage) configure() (err error) {
	if Debug() >= 1 {
		Println("now configure stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if Debug() >= 1 {
			Println("stage configured")
		}
	}()
	s.OnConfigure()
	return nil
}
func (s *Stage) prepare() (err error) {
	if Debug() >= 1 {
		Println("now prepare stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if Debug() >= 1 {
			Println("stage prepared")
		}
	}()
	s.OnPrepare()
	return nil
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

// Addon component.
//
// Addons are addons for Hemi. Users can create their own addons.
type Addon interface {
	// Imports
	Component
	// Methods
	Run() // goroutine
}

// Stater component is the interface to storages of Web/RPC states.
type Stater interface {
	// Imports
	Component
	// Methods
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

// Cronjob component
type Cronjob interface {
	// Imports
	Component
	// Methods
	Schedule() // goroutine
}

// Cronjob_ is the mixin for all cronjobs.
type Cronjob_ struct {
	// Mixins
	Component_
	// States
}
