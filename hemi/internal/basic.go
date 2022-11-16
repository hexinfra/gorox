// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Basic internal facilities and components.

package internal

import (
	"crypto/tls"
	"fmt"
	. "github.com/hexinfra/gorox/hemi/libraries/config"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ( // global variables shared between stages
	_debug    atomic.Int32 // debug level
	_baseOnce sync.Once    // protects _baseDir
	_baseDir  atomic.Value // directory of the executable
	_logsOnce sync.Once    // protects _logsDir
	_logsDir  atomic.Value // directory of the log files
	_tempOnce sync.Once    // protects _tempDir
	_tempDir  atomic.Value // directory of the run-time files
	_varsOnce sync.Once    // protects _varsDir
	_varsDir  atomic.Value // directory of the run-time datum
)

func Debug(level int32) bool { return _debug.Load() >= level }
func BaseDir() string        { return _baseDir.Load().(string) }
func LogsDir() string        { return _logsDir.Load().(string) }
func TempDir() string        { return _tempDir.Load().(string) }
func VarsDir() string        { return _varsDir.Load().(string) }

func SetDebug(level int32)  { _debug.Store(level) }
func SetBaseDir(dir string) { _baseOnce.Do(func() { _baseDir.Store(dir) }) } // only once
func SetLogsDir(dir string) { _logsOnce.Do(func() { _logsDir.Store(dir) }) } // only once
func SetTempDir(dir string) { _tempOnce.Do(func() { _tempDir.Store(dir) }) } // only once
func SetVarsDir(dir string) { _varsOnce.Do(func() { _varsDir.Store(dir) }) } // only once

// Component is the interface for all components.
type Component interface {
	CompInit(name string)

	SetName(name string)
	Name() string

	OnConfigure()
	Find(name string) (value Value, ok bool)
	Prop(name string) (value Value, ok bool)
	ConfigureBool(name string, prop *bool, defaultValue bool)
	ConfigureInt64(name string, prop *int64, check func(value int64) bool, defaultValue int64)
	ConfigureInt32(name string, prop *int32, check func(value int32) bool, defaultValue int32)
	ConfigureInt16(name string, prop *int16, check func(value int16) bool, defaultValue int16)
	ConfigureInt8(name string, prop *int8, check func(value int8) bool, defaultValue int8)
	ConfigureInt(name string, prop *int, check func(value int) bool, defaultValue int)
	ConfigureString(name string, prop *string, check func(value string) bool, defaultValue string)
	ConfigureDuration(name string, prop *time.Duration, check func(value time.Duration) bool, defaultValue time.Duration)
	ConfigureStringList(name string, prop *[]string, check func(value []string) bool, defaultValue []string)
	ConfigureStringDict(name string, prop *map[string]string, check func(value map[string]string) bool, defaultValue map[string]string)

	OnPrepare()

	OnShutdown()
	SubDone()

	setShell(shell Component)
	setParent(parent Component)
	getParent() Component
	setInfo(info any)
	setProp(name string, value Value)
}

// Component_ is the mixin for all components.
type Component_ struct {
	// Assocs
	shell  Component // the concrete Component
	parent Component // the parent component, used by config
	// States
	name  string           // main, ...
	props map[string]Value // name1=value1, ...
	info  any              // extra info about this component, used by config
	Shut  chan struct{}    // notify component to shutdown
	subs  sync.WaitGroup   // for shutting down sub compoments, if any
}

func (c *Component_) CompInit(name string) {
	c.name = name
	c.props = make(map[string]Value)
	c.Shut = make(chan struct{})
}

func (c *Component_) SetName(name string) { c.name = name }
func (c *Component_) Name() string        { return c.name }

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
	configureProp(c, name, prop, (*Value).Bool, nil, defaultValue)
}
func (c *Component_) ConfigureInt64(name string, prop *int64, check func(value int64) bool, defaultValue int64) {
	configureProp(c, name, prop, (*Value).Int64, check, defaultValue)
}
func (c *Component_) ConfigureInt32(name string, prop *int32, check func(value int32) bool, defaultValue int32) {
	configureProp(c, name, prop, (*Value).Int32, check, defaultValue)
}
func (c *Component_) ConfigureInt16(name string, prop *int16, check func(value int16) bool, defaultValue int16) {
	configureProp(c, name, prop, (*Value).Int16, check, defaultValue)
}
func (c *Component_) ConfigureInt8(name string, prop *int8, check func(value int8) bool, defaultValue int8) {
	configureProp(c, name, prop, (*Value).Int8, check, defaultValue)
}
func (c *Component_) ConfigureInt(name string, prop *int, check func(value int) bool, defaultValue int) {
	configureProp(c, name, prop, (*Value).Int, check, defaultValue)
}
func (c *Component_) ConfigureString(name string, prop *string, check func(value string) bool, defaultValue string) {
	configureProp(c, name, prop, (*Value).String, check, defaultValue)
}
func (c *Component_) ConfigureDuration(name string, prop *time.Duration, check func(value time.Duration) bool, defaultValue time.Duration) {
	configureProp(c, name, prop, (*Value).Duration, check, defaultValue)
}
func (c *Component_) ConfigureStringList(name string, prop *[]string, check func(value []string) bool, defaultValue []string) {
	configureProp(c, name, prop, (*Value).StringList, check, defaultValue)
}
func (c *Component_) ConfigureStringDict(name string, prop *map[string]string, check func(value map[string]string) bool, defaultValue map[string]string) {
	configureProp(c, name, prop, (*Value).StringDict, check, defaultValue)
}
func configureProp[T any](c *Component_, name string, prop *T, conv func(*Value) (T, bool), check func(value T) bool, defaultValue T) {
	if v, ok := c.Find(name); ok {
		if value, ok := conv(&v); ok && (check == nil || check(value)) {
			*prop = value
		} else {
			UseExitln("invalid " + name)
		}
	} else {
		*prop = defaultValue
	}
}

func (c *Component_) Shutdown() { c.Shut <- struct{}{} }

func (c *Component_) IncSub(n int) { c.subs.Add(n) }
func (c *Component_) WaitSubs()    { c.subs.Wait() }
func (c *Component_) SubDone()     { c.subs.Done() }

func (c *Component_) setShell(shell Component)         { c.shell = shell }
func (c *Component_) setParent(parent Component)       { c.parent = parent }
func (c *Component_) getParent() Component             { return c.parent }
func (c *Component_) setInfo(info any)                 { c.info = info }
func (c *Component_) setProp(name string, value Value) { c.props[name] = value }

// compList
type compList[T Component] []T

func (list compList[T]) walk(method func(T)) {
	for _, component := range list {
		method(component)
	}
}
func (list compList[T]) goWalk(method func(T)) {
	for _, component := range list {
		go method(component)
	}
}

// compDict
type compDict[T Component] map[string]T

func (dict compDict[T]) walk(method func(T)) {
	for _, component := range dict {
		method(component)
	}
}
func (dict compDict[T]) goWalk(method func(T)) {
	for _, component := range dict {
		go method(component)
	}
}

var ( // global maps, shared between stages
	fixtureSigns       = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required
	creatorsLock       sync.RWMutex
	optwareCreators    = make(map[string]func(name string, stage *Stage) Optware) // indexed by sign, same below.
	backendCreators    = make(map[string]func(name string, stage *Stage) backend)
	quicRunnerCreators = make(map[string]func(name string, stage *Stage, mesher *QUICMesher) QUICRunner)
	quicFilterCreators = make(map[string]func(name string, stage *Stage, mesher *QUICMesher) QUICFilter)
	tcpsRunnerCreators = make(map[string]func(name string, stage *Stage, mesher *TCPSMesher) TCPSRunner)
	tcpsFilterCreators = make(map[string]func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter)
	udpsRunnerCreators = make(map[string]func(name string, stage *Stage, mesher *UDPSMesher) UDPSRunner)
	udpsFilterCreators = make(map[string]func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter)
	staterCreators     = make(map[string]func(name string, stage *Stage) Stater)
	cacherCreators     = make(map[string]func(name string, stage *Stage) Cacher)
	handlerCreators    = make(map[string]func(name string, stage *Stage, app *App) Handler)
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
func RegisterOptware(sign string, create func(name string, stage *Stage) Optware) {
	registerComponent0(sign, compOptware, optwareCreators, create)
}
func registerBackend(sign string, create func(name string, stage *Stage) backend) {
	registerComponent0(sign, compBackend, backendCreators, create)
}
func RegisterQUICRunner(sign string, create func(name string, stage *Stage, mesher *QUICMesher) QUICRunner) {
	registerComponent1(sign, compQUICRunner, quicRunnerCreators, create)
}
func RegisterQUICFilter(sign string, create func(name string, stage *Stage, mesher *QUICMesher) QUICFilter) {
	registerComponent1(sign, compQUICFilter, quicFilterCreators, create)
}
func RegisterTCPSRunner(sign string, create func(name string, stage *Stage, mesher *TCPSMesher) TCPSRunner) {
	registerComponent1(sign, compTCPSRunner, tcpsRunnerCreators, create)
}
func RegisterTCPSFilter(sign string, create func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter) {
	registerComponent1(sign, compTCPSFilter, tcpsFilterCreators, create)
}
func RegisterUDPSRunner(sign string, create func(name string, stage *Stage, mesher *UDPSMesher) UDPSRunner) {
	registerComponent1(sign, compUDPSRunner, udpsRunnerCreators, create)
}
func RegisterUDPSFilter(sign string, create func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter) {
	registerComponent1(sign, compUDPSFilter, udpsFilterCreators, create)
}
func RegisterStater(sign string, create func(name string, stage *Stage) Stater) {
	registerComponent0(sign, compStater, staterCreators, create)
}
func RegisterCacher(sign string, create func(name string, stage *Stage) Cacher) {
	registerComponent0(sign, compCacher, cacherCreators, create)
}
func RegisterHandler(sign string, create func(name string, stage *Stage, app *App) Handler) {
	registerComponent1(sign, compHandler, handlerCreators, create)
}
func RegisterReviser(sign string, create func(name string, stage *Stage, app *App) Reviser) {
	registerComponent1(sign, compReviser, reviserCreators, create)
}
func RegisterSocklet(sign string, create func(name string, stage *Stage, app *App) Socklet) {
	registerComponent1(sign, compSocklet, sockletCreators, create)
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
func RegisterServer(sign string, create func(name string, stage *Stage) Server) {
	registerComponent0(sign, compServer, serverCreators, create)
}
func RegisterCronjob(sign string, create func(name string, stage *Stage) Cronjob) {
	registerComponent0(sign, compCronjob, cronjobCreators, create)
}

func registerComponent0[T Component](sign string, comp int16, creators map[string]func(string, *Stage) T, create func(string, *Stage) T) { // optware, backend, stater, cacher, server, cronjob
	creatorsLock.Lock()
	defer creatorsLock.Unlock()
	if _, ok := creators[sign]; ok {
		BugExitln("component0 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}
func registerComponent1[T Component, C Component](sign string, comp int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // runner, filter, handler, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()
	if _, ok := creators[sign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}

const ( // exit codes. keep sync with ../hemi.go
	CodeBug = 20
	CodeUse = 21
	CodeEnv = 22
)

func BugExitln(args ...any) { exitln(CodeBug, "[BUG] ", args...) }
func UseExitln(args ...any) { exitln(CodeUse, "[USE] ", args...) }
func EnvExitln(args ...any) { exitln(CodeEnv, "[ENV] ", args...) }

func BugExitf(format string, args ...any) { exitf(CodeBug, "[BUG] ", format, args...) }
func UseExitf(format string, args ...any) { exitf(CodeUse, "[USE] ", format, args...) }
func EnvExitf(format string, args ...any) { exitf(CodeEnv, "[ENV] ", format, args...) }

func exitln(exitCode int, prefix string, args ...any) {
	fmt.Fprint(os.Stderr, prefix)
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCode)
}
func exitf(exitCode int, prefix, format string, args ...any) {
	fmt.Fprintf(os.Stderr, prefix+format, args...)
	os.Exit(exitCode)
}

// fixture component.
//
// Fixtures only exist in internal, and are created by stage.
// Some critical functions, like clock and name resolver, are
// implemented as fixtures.
type fixture interface {
	Component
	run() // goroutine
}

// Optware component.
//
// Optwares behave like fixtures except that they are optional
// and extendible, so users can create their own optwares.
type Optware interface {
	Component
	Run() // goroutine
}

// office_ is a mixin for meshers and servers.
type office_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	address         string      // hostname:port
	tlsMode         bool        // tls mode?
	numGates        int32       // number of gates
	maxConnsPerGate int32       // max concurrent connections allowed per gate
	tlsConfig       *tls.Config // set if is tls mode
}

func (o *office_) init(name string, stage *Stage) {
	o.CompInit(name)
	o.stage = stage
}

func (o *office_) onConfigure() {
	// address
	if v, ok := o.Find("address"); ok {
		if address, ok := v.String(); ok {
			if p := strings.IndexByte(address, ':'); p == -1 || p == len(address)-1 {
				UseExitln("bad address: " + address)
			} else {
				o.address = address
			}
		} else {
			UseExitln("address should be of string type")
		}
	} else {
		UseExitln("address is required for servers")
	}
	// tlsMode
	o.ConfigureBool("tlsMode", &o.tlsMode, false)
	if o.tlsMode {
		o.tlsConfig = new(tls.Config)
	}
	// numGates
	o.ConfigureInt32("numGates", &o.numGates, func(value int32) bool { return value > 0 }, o.stage.NumCPU())
	// maxConnsPerGate
	o.ConfigureInt32("maxConnsPerGate", &o.maxConnsPerGate, func(value int32) bool { return value > 0 }, 100000)
}
func (o *office_) onPrepare() {
}

func (o *office_) Stage() *Stage          { return o.stage }
func (o *office_) Address() string        { return o.address }
func (o *office_) TLSMode() bool          { return o.tlsMode }
func (o *office_) NumGates() int32        { return o.numGates }
func (o *office_) MaxConnsPerGate() int32 { return o.maxConnsPerGate }

// Gate_ is a mixin for mesher gates and server gates.
type Gate_ struct {
	// Assocs
	stage *Stage // current stage
	// States
	id       int32
	address  string
	shut     atomic.Bool
	maxConns int32
	numConns atomic.Int32 // TODO: false sharing
}

func (g *Gate_) Init(stage *Stage, id int32, address string, maxConns int32) {
	g.stage = stage
	g.id = id
	g.address = address
	g.maxConns = maxConns
	g.numConns.Store(0)
}

func (g *Gate_) Stage() *Stage   { return g.stage }
func (g *Gate_) Address() string { return g.address }

func (g *Gate_) Shutdown()        { g.shut.Store(true) }
func (g *Gate_) IsShutdown() bool { return g.shut.Load() }

func (g *Gate_) DecConns() int32 { return g.numConns.Add(-1) }
func (g *Gate_) ReachLimit() bool {
	return g.numConns.Add(1) > g.maxConns
}

// proxy_ is a mixin for proxies.
type proxy_ struct {
	// Assocs
	stage   *Stage  // current stage
	backend backend // if works as forward proxy, this is nil
	// States
	proxyMode string // forward, reverse
}

func (p *proxy_) init(stage *Stage) {
	p.stage = stage
}

func (p *proxy_) onConfigure(c Component) {
	// proxyMode
	if v, ok := c.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			p.proxyMode = mode
		} else {
			UseExitln("invalid proxyMode")
		}
	} else {
		p.proxyMode = "reverse"
	}
	// toBackend
	if v, ok := c.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := p.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				p.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if p.proxyMode == "reverse" {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (p *proxy_) onPrepare() {
}

// Cronjob component
type Cronjob interface {
	Component
	Run() // goroutine
}

// Cronjob_ is the mixin for all cronjobs.
type Cronjob_ struct {
	// Mixins
	Component_
	// States
}
