// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Component is the configurable component.

package internal

import (
	. "github.com/hexinfra/gorox/hemi/libraries/config"
	"sync"
	"time"
)

var ( // global maps, shared between stages
	fixtureSigns       = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required
	creatorsLock       sync.RWMutex
	optwareCreators    = make(map[string]func(sign string, stage *Stage) Optware) // indexed by sign, same below.
	backendCreators    = make(map[string]func(name string, stage *Stage) backend)
	quicDealetCreators = make(map[string]func(name string, stage *Stage, mesher *QUICMesher) QUICDealet)
	quicEditorCreators = make(map[string]func(name string, stage *Stage, mesher *QUICMesher) QUICEditor)
	tcpsDealetCreators = make(map[string]func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet)
	tcpsEditorCreators = make(map[string]func(name string, stage *Stage, mesher *TCPSMesher) TCPSEditor)
	udpsDealetCreators = make(map[string]func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealet)
	udpsEditorCreators = make(map[string]func(name string, stage *Stage, mesher *UDPSMesher) UDPSEditor)
	staterCreators     = make(map[string]func(name string, stage *Stage) Stater)
	cacherCreators     = make(map[string]func(name string, stage *Stage) Cacher)
	handletCreators    = make(map[string]func(name string, stage *Stage, app *App) Handlet)
	reviserCreators    = make(map[string]func(name string, stage *Stage, app *App) Reviser)
	sockletCreators    = make(map[string]func(name string, stage *Stage, app *App) Socklet)
	serverCreators     = make(map[string]func(name string, stage *Stage) Server)
	cronjobCreators    = make(map[string]func(sign string, stage *Stage) Cronjob)
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
func RegisterOptware(sign string, create func(sign string, stage *Stage) Optware) {
	registerComponent0(sign, compOptware, optwareCreators, create)
}
func registerBackend(sign string, create func(name string, stage *Stage) backend) {
	registerComponent0(sign, compBackend, backendCreators, create)
}
func RegisterQUICDealet(sign string, create func(name string, stage *Stage, mesher *QUICMesher) QUICDealet) {
	registerComponent1(sign, compQUICDealet, quicDealetCreators, create)
}
func RegisterQUICEditor(sign string, create func(name string, stage *Stage, mesher *QUICMesher) QUICEditor) {
	registerComponent1(sign, compQUICEditor, quicEditorCreators, create)
}
func RegisterTCPSDealet(sign string, create func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet) {
	registerComponent1(sign, compTCPSDealet, tcpsDealetCreators, create)
}
func RegisterTCPSEditor(sign string, create func(name string, stage *Stage, mesher *TCPSMesher) TCPSEditor) {
	registerComponent1(sign, compTCPSEditor, tcpsEditorCreators, create)
}
func RegisterUDPSDealet(sign string, create func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealet) {
	registerComponent1(sign, compUDPSDealet, udpsDealetCreators, create)
}
func RegisterUDPSEditor(sign string, create func(name string, stage *Stage, mesher *UDPSMesher) UDPSEditor) {
	registerComponent1(sign, compUDPSEditor, udpsEditorCreators, create)
}
func RegisterStater(sign string, create func(name string, stage *Stage) Stater) {
	registerComponent0(sign, compStater, staterCreators, create)
}
func RegisterCacher(sign string, create func(name string, stage *Stage) Cacher) {
	registerComponent0(sign, compCacher, cacherCreators, create)
}
func RegisterHandlet(sign string, create func(name string, stage *Stage, app *App) Handlet) {
	registerComponent1(sign, compHandlet, handletCreators, create)
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
func RegisterCronjob(sign string, create func(sign string, stage *Stage) Cronjob) {
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
func registerComponent1[T Component, C Component](sign string, comp int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // dealet, editor, handlet, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()
	if _, ok := creators[sign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
}

// Component is the interface for all components.
type Component interface {
	CompInit(name string)

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
	waiter_
	// Assocs
	shell  Component // the concrete Component
	parent Component // the parent component, used by config
	// States
	name  string           // main, ...
	props map[string]Value // name1=value1, ...
	info  any              // extra info about this component, used by config
	Shut  chan struct{}    // notify component to shutdown
}

func (c *Component_) CompInit(name string) {
	c.name = name
	c.props = make(map[string]Value)
	c.Shut = make(chan struct{})
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
