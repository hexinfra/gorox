// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Stage components.

package internal

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
	"unsafe"
)

var ( // global maps, shared between stages
	fixtureSigns       = make(map[string]bool) // we guarantee this is not manipulated concurrently, so no lock is required
	creatorsLock       sync.RWMutex
	optwareCreators    = make(map[string]func(sign string, stage *Stage) Optware) // indexed by sign, same below.
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
func registerComponent1[T Component, C Component](sign string, comp int16, creators map[string]func(string, *Stage, C) T, create func(string, *Stage, C) T) { // runner, filter, handler, reviser, socklet
	creatorsLock.Lock()
	defer creatorsLock.Unlock()
	if _, ok := creators[sign]; ok {
		BugExitln("component1 sign conflicted")
	}
	creators[sign] = create
	signComp(sign, comp)
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
	clock       *clockFixture         // for fast accessing
	filesys     *filesysFixture       // for fast accessing
	resolv      *resolvFixture        // for fast accessing
	http1       *HTTP1Outgate         // for fast accessing
	http2       *HTTP2Outgate         // for fast accessing
	http3       *HTTP3Outgate         // for fast accessing
	quic        *QUICOutgate          // for fast accessing
	tcps        *TCPSOutgate          // for fast accessing
	udps        *UDPSOutgate          // for fast accessing
	unix        *UnixOutgate          // for fast accessing
	fixtures    compDict[fixture]     // indexed by sign
	optwares    compDict[Optware]     // indexed by sign
	backends    compDict[backend]     // indexed by backendName
	quicMeshers compDict[*QUICMesher] // indexed by mesherName
	tcpsMeshers compDict[*TCPSMesher] // indexed by mesherName
	udpsMeshers compDict[*UDPSMesher] // indexed by mesherName
	staters     compDict[Stater]      // indexed by staterName
	cachers     compDict[Cacher]      // indexed by cacherName
	apps        compDict[*App]        // indexed by appName
	svcs        compDict[*Svc]        // indexed by svcName
	servers     compDict[Server]      // indexed by serverName
	cronjobs    compDict[Cronjob]     // indexed by sign
	// States
	logFile    string // stage's log file
	logger     *log.Logger
	cpuFile    string
	hepFile    string
	thrFile    string
	grtFile    string
	blkFile    string
	appServers map[string][]string
	svcServers map[string][]string
	id         int32
	numCPU     int32
}

func (s *Stage) onCreate() {
	s.CompInit("stage")

	s.clock = createClock(s)
	s.filesys = createFilesys(s)
	s.resolv = createResolv(s)
	s.http1 = createHTTP1(s)
	s.http2 = createHTTP2(s)
	s.http3 = createHTTP3(s)
	s.quic = createQUIC(s)
	s.tcps = createTCPS(s)
	s.udps = createUDPS(s)
	s.unix = createUnix(s)

	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFilesys] = s.filesys
	s.fixtures[signResolv] = s.resolv
	s.fixtures[signHTTP1] = s.http1
	s.fixtures[signHTTP2] = s.http2
	s.fixtures[signHTTP3] = s.http3
	s.fixtures[signQUIC] = s.quic
	s.fixtures[signTCPS] = s.tcps
	s.fixtures[signUDPS] = s.udps
	s.fixtures[signUnix] = s.unix

	s.optwares = make(compDict[Optware])
	s.backends = make(compDict[backend])
	s.quicMeshers = make(compDict[*QUICMesher])
	s.tcpsMeshers = make(compDict[*TCPSMesher])
	s.udpsMeshers = make(compDict[*UDPSMesher])
	s.staters = make(compDict[Stater])
	s.cachers = make(compDict[Cacher])
	s.apps = make(compDict[*App])
	s.svcs = make(compDict[*Svc])
	s.servers = make(compDict[Server])
	s.cronjobs = make(compDict[Cronjob])

	s.appServers = make(map[string][]string)
	s.svcServers = make(map[string][]string)
}

func (s *Stage) OnConfigure() {
	// appServers
	if v, ok := s.Find("appServers"); ok {
		appServers, ok := v.Dict()
		if !ok {
			UseExitln("invalid appServers")
		}
		for app, vServers := range appServers {
			servers, ok := vServers.StringList()
			if !ok {
				UseExitln("invalid servers in appServers")
			}
			s.appServers[app] = servers
		}
	}
	// svcServers
	if v, ok := s.Find("svcServers"); ok {
		svcServers, ok := v.Dict()
		if !ok {
			UseExitln("invalid svcServers")
		}
		for svc, vServers := range svcServers {
			servers, ok := vServers.StringList()
			if !ok {
				UseExitln("invalid servers in svcServers")
			}
			s.svcServers[svc] = servers
		}
	}
	// logFile
	s.ConfigureString("logFile", &s.logFile, func(value string) bool { return value != "" }, LogsDir()+"/worker.log")
	tempDir := TempDir()
	// cpuFile
	s.ConfigureString("cpuFile", &s.cpuFile, func(value string) bool { return value != "" }, tempDir+"/cpu.prof")
	// hepFile
	s.ConfigureString("hepFile", &s.hepFile, func(value string) bool { return value != "" }, tempDir+"/hep.prof")
	// thrFile
	s.ConfigureString("thrFile", &s.thrFile, func(value string) bool { return value != "" }, tempDir+"/thr.prof")
	// grtFile
	s.ConfigureString("grtFile", &s.grtFile, func(value string) bool { return value != "" }, tempDir+"/grt.prof")
	// blkFile
	s.ConfigureString("blkFile", &s.blkFile, func(value string) bool { return value != "" }, tempDir+"/blk.prof")

	// sub components
	s.fixtures.walk(fixture.OnConfigure)
	s.optwares.walk(Optware.OnConfigure)
	s.backends.walk(backend.OnConfigure)
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
	for _, file := range []string{s.logFile, s.cpuFile, s.hepFile, s.thrFile, s.grtFile, s.blkFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			EnvExitln(err.Error())
		}
	}
	logFile, err := os.OpenFile(s.logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		EnvExitln(err.Error())
	}
	s.logger = log.New(logFile, "stage", log.Ldate|log.Ltime)

	// sub components
	s.fixtures.walk(fixture.OnPrepare)
	s.optwares.walk(Optware.OnPrepare)
	s.backends.walk(backend.OnPrepare)
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

func (s *Stage) OnShutdown() {
	if Debug(2) {
		fmt.Printf("stage id=%d shutdown start!!\n", s.id)
	}

	s.IncSub(len(s.cronjobs))
	s.cronjobs.goWalk(Cronjob.OnShutdown)
	s.WaitSubs()

	s.IncSub(len(s.servers))
	s.servers.goWalk(Server.OnShutdown)
	s.WaitSubs()

	s.IncSub(len(s.svcs) + len(s.apps))
	s.svcs.goWalk((*Svc).OnShutdown)
	s.apps.goWalk((*App).OnShutdown)
	s.WaitSubs()

	s.IncSub(len(s.cachers) + len(s.staters))
	s.cachers.goWalk(Cacher.OnShutdown)
	s.staters.goWalk(Stater.OnShutdown)
	s.WaitSubs()

	s.IncSub(len(s.udpsMeshers) + len(s.tcpsMeshers) + len(s.quicMeshers))
	s.udpsMeshers.goWalk((*UDPSMesher).OnShutdown)
	s.tcpsMeshers.goWalk((*TCPSMesher).OnShutdown)
	s.quicMeshers.goWalk((*QUICMesher).OnShutdown)
	s.WaitSubs()

	s.IncSub(len(s.backends))
	s.backends.goWalk(backend.OnShutdown)
	s.WaitSubs()

	s.IncSub(len(s.optwares))
	s.optwares.goWalk(Optware.OnShutdown)
	s.WaitSubs()

	s.IncSub(7)
	go s.http1.OnShutdown()
	go s.http2.OnShutdown()
	go s.http3.OnShutdown()
	go s.quic.OnShutdown()
	go s.tcps.OnShutdown()
	go s.udps.OnShutdown()
	go s.unix.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.filesys.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.resolv.OnShutdown()
	s.WaitSubs()

	s.IncSub(1)
	s.clock.OnShutdown()
	s.WaitSubs()

	// TODO: shutdown s
	if Debug(2) {
		fmt.Println("stage close log file")
	}
	s.logger.Writer().(*os.File).Close()
}

func (s *Stage) createOptware(sign string) Optware {
	create, ok := optwareCreators[sign]
	if !ok {
		UseExitln("unknown optware type: " + sign)
	}
	if s.Optware(sign) != nil {
		UseExitf("conflicting optware with a same sign '%s'\n", sign)
	}
	optware := create(sign, s)
	optware.setShell(optware)
	s.optwares[sign] = optware
	return optware
}
func (s *Stage) createBackend(sign string, name string) backend {
	create, ok := backendCreators[sign]
	if !ok {
		UseExitln("unknown backend type: " + sign)
	}
	if s.Backend(name) != nil {
		UseExitf("conflicting backend with a same name '%s'\n", name)
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
	create, ok := staterCreators[sign]
	if !ok {
		UseExitln("unknown stater type: " + sign)
	}
	if s.Stater(name) != nil {
		UseExitf("conflicting stater with a same name '%s'\n", name)
	}
	stater := create(name, s)
	stater.setShell(stater)
	s.staters[name] = stater
	return stater
}
func (s *Stage) createCacher(sign string, name string) Cacher {
	create, ok := cacherCreators[sign]
	if !ok {
		UseExitln("unknown cacher type: " + sign)
	}
	if s.Cacher(name) != nil {
		UseExitf("conflicting cacher with a same name '%s'\n", name)
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
	create, ok := serverCreators[sign]
	if !ok {
		UseExitln("unknown server type: " + sign)
	}
	if s.Server(name) != nil {
		UseExitf("conflicting server with a same name '%s'\n", name)
	}
	server := create(name, s)
	server.setShell(server)
	s.servers[name] = server
	return server
}
func (s *Stage) createCronjob(sign string) Cronjob {
	create, ok := cronjobCreators[sign]
	if !ok {
		UseExitln("unknown cronjob type: " + sign)
	}
	if s.Cronjob(sign) != nil {
		UseExitf("conflicting cronjob with a same sign '%s'\n", sign)
	}
	cronjob := create(sign, s)
	cronjob.setShell(cronjob)
	s.cronjobs[sign] = cronjob
	return cronjob
}

func (s *Stage) Filesys() *filesysFixture           { return s.filesys }
func (s *Stage) HTTP1() *HTTP1Outgate               { return s.http1 }
func (s *Stage) HTTP2() *HTTP2Outgate               { return s.http2 }
func (s *Stage) HTTP3() *HTTP3Outgate               { return s.http3 }
func (s *Stage) QUIC() *QUICOutgate                 { return s.quic }
func (s *Stage) TCPS() *TCPSOutgate                 { return s.tcps }
func (s *Stage) UDPS() *UDPSOutgate                 { return s.udps }
func (s *Stage) Unix() *UnixOutgate                 { return s.unix }
func (s *Stage) fixture(sign string) fixture        { return s.fixtures[sign] }
func (s *Stage) Optware(sign string) Optware        { return s.optwares[sign] }
func (s *Stage) Backend(name string) backend        { return s.backends[name] }
func (s *Stage) QUICMesher(name string) *QUICMesher { return s.quicMeshers[name] }
func (s *Stage) TCPSMesher(name string) *TCPSMesher { return s.tcpsMeshers[name] }
func (s *Stage) UDPSMesher(name string) *UDPSMesher { return s.udpsMeshers[name] }
func (s *Stage) Stater(name string) Stater          { return s.staters[name] }
func (s *Stage) Cacher(name string) Cacher          { return s.cachers[name] }
func (s *Stage) App(name string) *App               { return s.apps[name] }
func (s *Stage) Svc(name string) *Svc               { return s.svcs[name] }
func (s *Stage) Server(name string) Server          { return s.servers[name] }
func (s *Stage) Cronjob(sign string) Cronjob        { return s.cronjobs[sign] }

func (s *Stage) Start(id int32) {
	s.id = id
	s.numCPU = int32(runtime.NumCPU())

	if Debug(2) {
		fmt.Printf("size of http1Conn = %d\n", unsafe.Sizeof(http1Conn{}))
		fmt.Printf("size of h1Conn = %d\n", unsafe.Sizeof(H1Conn{}))
		fmt.Printf("size of http2Conn = %d\n", unsafe.Sizeof(http2Conn{}))
		fmt.Printf("size of http2Stream = %d\n", unsafe.Sizeof(http2Stream{}))
		fmt.Printf("size of http3Conn = %d\n", unsafe.Sizeof(http3Conn{}))
		fmt.Printf("size of http3Stream = %d\n", unsafe.Sizeof(http3Stream{}))
		fmt.Printf("size of Block = %d\n", unsafe.Sizeof(Block{}))
	}
	if Debug(1) {
		fmt.Printf("stageID=%d\n", s.id)
		fmt.Printf("numCPU=%d\n", s.numCPU)
		fmt.Printf("baseDir=%s\n", BaseDir())
		fmt.Printf("logsDir=%s\n", LogsDir())
		fmt.Printf("tempDir=%s\n", TempDir())
		fmt.Printf("varsDir=%s\n", VarsDir())
	}
	if BaseDir() == "" || LogsDir() == "" || TempDir() == "" || VarsDir() == "" {
		UseExitln("baseDir, logsDir, tempDir, and varsDir must all be set")
	}
	// Configure all components
	if err := s.configure(); err != nil {
		UseExitln(err.Error())
	}
	// Check config requirements
	if len(s.servers) == 0 && len(s.quicMeshers) == 0 && len(s.tcpsMeshers) == 0 && len(s.udpsMeshers) == 0 {
		UseExitln("no server/mesher provided, nothing to serve")
	}
	// Link apps to http servers
	s.linkAppServers()
	// Link svcs to http servers
	s.linkSvcServers()
	// Prepare all components
	if err := s.prepare(); err != nil {
		EnvExitln(err.Error())
	}
	// Init running environment
	if err := os.Chdir(BaseDir()); err != nil {
		EnvExitln(err.Error())
	}

	s.startFixtures() // go fixture.run()
	s.startOptwares() // go optware.Run()
	s.startBackends() // go backend.maintain()
	s.startMeshers()  // go mesher.serve()
	s.startStaters()  // go stater.Maintain()
	s.startCachers()  // go cacher.Maintain()
	s.startApps()     // go app.maintain()
	s.startSvcs()     // go svc.maintain()
	s.startServers()  // go server.Serve()
	s.startCronjobs() // go cronjob.Run()

	s.Logf("stage=%d is ready to serve.\n", s.id)
}
func (s *Stage) Quit() {
	s.OnShutdown()
	if Debug(2) {
		fmt.Printf("stage id=%d: graced.\n", s.id)
	}
}

func (s *Stage) linkAppServers() {
	if Debug(1) {
		fmt.Println("link apps to http servers")
	}
	for _, app := range s.apps {
		serverNames, ok := s.appServers[app.Name()]
		if !ok {
			if Debug(1) {
				fmt.Printf("no server is provided for app '%s'\n", app.name)
			}
			continue
		}
		for _, serverName := range serverNames {
			server := s.servers[serverName]
			if server == nil {
				UseExitf("no server named '%s'", serverName)
			}
			if httpServer, ok := server.(httpServer); ok {
				httpServer.linkApp(app)
			} else {
				UseExitf("server '%s' is not an http server, cannot link app to it", serverName)
			}
			if Debug(1) {
				fmt.Printf("app %s is linked to http server %s\n", app.name, serverName)
			}
		}
	}
}
func (s *Stage) linkSvcServers() {
	if Debug(1) {
		fmt.Println("link svcs to http servers")
	}
	for _, svc := range s.svcs {
		serverNames, ok := s.svcServers[svc.Name()]
		if !ok {
			if Debug(1) {
				fmt.Printf("no server is provided for svc '%s'\n", svc.name)
			}
			continue
		}
		for _, serverName := range serverNames {
			server := s.servers[serverName]
			if server == nil {
				UseExitf("no server named '%s'", serverName)
			}
			if grpcServer, ok := server.(GRPCServer); ok {
				grpcServer.LinkSvc(svc)
			} else if httpServer, ok := server.(httpServer); ok {
				httpServer.linkSvc(svc)
			} else {
				UseExitf("server '%s' is not an http server nor grpc server, cannot link svc to it", serverName)
			}
			if Debug(1) {
				fmt.Printf("svc %s is linked to rpc server %s\n", svc.name, serverName)
			}
		}
	}
}

func (s *Stage) startFixtures() {
	for _, fixture := range s.fixtures {
		if Debug(1) {
			fmt.Printf("fixture=%s go run()\n", fixture.Name())
		}
		go fixture.run()
	}
}
func (s *Stage) startOptwares() {
	for _, optware := range s.optwares {
		if Debug(1) {
			fmt.Printf("optware=%s go Run()\n", optware.Name())
		}
		go optware.Run()
	}
}
func (s *Stage) startBackends() {
	for _, backend := range s.backends {
		if Debug(1) {
			fmt.Printf("backend=%s go maintain()\n", backend.Name())
		}
		go backend.maintain()
	}
}
func (s *Stage) startMeshers() {
	for _, quicMesher := range s.quicMeshers {
		if Debug(1) {
			fmt.Printf("quicMesher=%s go serve()\n", quicMesher.Name())
		}
		go quicMesher.serve()
	}
	for _, tcpsMesher := range s.tcpsMeshers {
		if Debug(1) {
			fmt.Printf("tcpsMesher=%s go serve()\n", tcpsMesher.Name())
		}
		go tcpsMesher.serve()
	}
	for _, udpsMesher := range s.udpsMeshers {
		if Debug(1) {
			fmt.Printf("udpsMesher=%s go serve()\n", udpsMesher.Name())
		}
		go udpsMesher.serve()
	}
}
func (s *Stage) startStaters() {
	for _, stater := range s.staters {
		if Debug(1) {
			fmt.Printf("stater=%s go Maintain()\n", stater.Name())
		}
		go stater.Maintain()
	}
}
func (s *Stage) startCachers() {
	for _, cacher := range s.cachers {
		if Debug(1) {
			fmt.Printf("cacher=%s go Maintain()\n", cacher.Name())
		}
		go cacher.Maintain()
	}
}
func (s *Stage) startApps() {
	for _, app := range s.apps {
		if Debug(1) {
			fmt.Printf("app=%s go maintain()\n", app.Name())
		}
		go app.maintain()
	}
}
func (s *Stage) startSvcs() {
	for _, svc := range s.svcs {
		if Debug(1) {
			fmt.Printf("svc=%s go maintain()\n", svc.Name())
		}
		go svc.maintain()
	}
}
func (s *Stage) startServers() {
	for _, server := range s.servers {
		if Debug(1) {
			fmt.Printf("server=%s go Serve()\n", server.Name())
		}
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	for _, cronjob := range s.cronjobs {
		if Debug(1) {
			fmt.Printf("cronjob=%s go Run()\n", cronjob.Name())
		}
		go cronjob.Run()
	}
}

func (s *Stage) configure() (err error) {
	if Debug(1) {
		fmt.Println("now configure stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if Debug(1) {
			fmt.Println("stage successfully configured")
		}
	}()
	s.OnConfigure()
	return nil
}
func (s *Stage) prepare() (err error) {
	if Debug(1) {
		fmt.Println("now prepare stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if Debug(1) {
			fmt.Println("stage successfully prepared")
		}
	}()
	s.OnPrepare()
	return nil
}

func (s *Stage) ID() int32     { return s.id }
func (s *Stage) NumCPU() int32 { return s.numCPU }

func (s *Stage) Log(str string) {
	s.logger.Print(str)
}
func (s *Stage) Logln(str string) {
	s.logger.Println(str)
}
func (s *Stage) Logf(format string, args ...any) {
	s.logger.Printf(format, args...)
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
