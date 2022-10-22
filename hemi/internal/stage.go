// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Stage component.

package internal

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/logger"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"
	"unsafe"
)

// createStage creates a new stage which runs alongside existing stage.
func createStage() *Stage {
	stage := new(Stage)
	stage.init()
	stage.setShell(stage)
	return stage
}

// Stage represents a running stage in worker process.
//
// A worker process may have many stages in its lifetime, especially
// when new configuration is applied, a new stage is created, or the
// old one is told to shutdown.
type Stage struct {
	// Mixins
	Component_
	// Assocs
	clock       *clockFixture         // for fast accessing
	filesys     *filesysFixture       // for fast accessing
	resolv      *resolvFixture        // for fast accessing
	quic        *QUICOutgate          // for fast accessing
	tcps        *TCPSOutgate          // for fast accessing
	udps        *UDPSOutgate          // for fast accessing
	unix        *UnixOutgate          // for fast accessing
	http1       *HTTP1Outgate         // for fast accessing
	http2       *HTTP2Outgate         // for fast accessing
	http3       *HTTP3Outgate         // for fast accessing
	fixtures    compDict[fixture]     // indexed by sign
	optwares    compDict[Optware]     // indexed by sign
	cronjobs    compDict[Cronjob]     // indexed by sign
	backends    compDict[backend]     // indexed by backendName
	quicRouters compDict[*QUICRouter] // indexed by routerName
	tcpsRouters compDict[*TCPSRouter] // indexed by routerName
	udpsRouters compDict[*UDPSRouter] // indexed by routerName
	cachers     compDict[Cacher]      // indexed by cacherName
	servers     compDict[Server]      // indexed by serverName
	apps        compDict[*App]        // indexed by appName
	svcs        compDict[*Svc]        // indexed by svcName
	// States
	appServers map[string][]string
	svcServers map[string][]string
	logFile    string
	cpuFile    string
	hepFile    string
	thrFile    string
	grtFile    string
	blkFile    string
	alone      bool
	id         int32
	numCPU     int32
	logger     *logger.Logger
}

func (s *Stage) init() {
	s.SetName("stage")

	s.clock = createClock(s)
	s.filesys = createFilesys(s)
	s.resolv = createResolv(s)
	s.quic = createQUIC(s)
	s.tcps = createTCPS(s)
	s.udps = createUDPS(s)
	s.unix = createUnix(s)
	s.http1 = createHTTP1(s)
	s.http2 = createHTTP2(s)
	s.http3 = createHTTP3(s)

	s.fixtures = make(compDict[fixture])
	s.fixtures[signClock] = s.clock
	s.fixtures[signFilesys] = s.filesys
	s.fixtures[signResolv] = s.resolv
	s.fixtures[signQUIC] = s.quic
	s.fixtures[signTCPS] = s.tcps
	s.fixtures[signUDPS] = s.udps
	s.fixtures[signUnix] = s.unix
	s.fixtures[signHTTP1] = s.http1
	s.fixtures[signHTTP2] = s.http2
	s.fixtures[signHTTP3] = s.http3

	s.optwares = make(compDict[Optware])
	s.cronjobs = make(compDict[Cronjob])
	s.backends = make(compDict[backend])
	s.quicRouters = make(compDict[*QUICRouter])
	s.tcpsRouters = make(compDict[*TCPSRouter])
	s.udpsRouters = make(compDict[*UDPSRouter])
	s.cachers = make(compDict[Cacher])
	s.servers = make(compDict[Server])
	s.apps = make(compDict[*App])
	s.svcs = make(compDict[*Svc])

	s.appServers = make(map[string][]string)
	s.svcServers = make(map[string][]string)
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
func (s *Stage) createQUICRouter(name string) *QUICRouter {
	if s.QUICRouter(name) != nil {
		UseExitf("conflicting quicRouter with a same name '%s'\n", name)
	}
	router := new(QUICRouter)
	router.init(name, s)
	router.setShell(router)
	s.quicRouters[name] = router
	return router
}
func (s *Stage) createTCPSRouter(name string) *TCPSRouter {
	if s.TCPSRouter(name) != nil {
		UseExitf("conflicting tcpsRouter with a same name '%s'\n", name)
	}
	router := new(TCPSRouter)
	router.init(name, s)
	router.setShell(router)
	s.tcpsRouters[name] = router
	return router
}
func (s *Stage) createUDPSRouter(name string) *UDPSRouter {
	if s.UDPSRouter(name) != nil {
		UseExitf("conflicting udpsRouter with a same name '%s'\n", name)
	}
	router := new(UDPSRouter)
	router.init(name, s)
	router.setShell(router)
	s.udpsRouters[name] = router
	return router
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
func (s *Stage) createApp(name string) *App {
	if s.App(name) != nil {
		UseExitf("conflicting app with a same name '%s'\n", name)
	}
	app := new(App)
	app.init(name, s)
	app.setShell(app)
	s.apps[name] = app
	return app
}
func (s *Stage) createSvc(name string) *Svc {
	if s.Svc(name) != nil {
		UseExitf("conflicting svc with a same name '%s'\n", name)
	}
	svc := new(Svc)
	svc.init(name, s)
	svc.setShell(svc)
	s.svcs[name] = svc
	return svc
}

func (s *Stage) fixture(sign string) fixture        { return s.fixtures[sign] }
func (s *Stage) QUIC() *QUICOutgate                 { return s.quic }
func (s *Stage) TCPS() *TCPSOutgate                 { return s.tcps }
func (s *Stage) UDPS() *UDPSOutgate                 { return s.udps }
func (s *Stage) Unix() *UnixOutgate                 { return s.unix }
func (s *Stage) HTTP1() *HTTP1Outgate               { return s.http1 }
func (s *Stage) HTTP2() *HTTP2Outgate               { return s.http2 }
func (s *Stage) HTTP3() *HTTP3Outgate               { return s.http3 }
func (s *Stage) Optware(sign string) Optware        { return s.optwares[sign] }
func (s *Stage) Cronjob(sign string) Cronjob        { return s.cronjobs[sign] }
func (s *Stage) Backend(name string) backend        { return s.backends[name] }
func (s *Stage) QUICRouter(name string) *QUICRouter { return s.quicRouters[name] }
func (s *Stage) TCPSRouter(name string) *TCPSRouter { return s.tcpsRouters[name] }
func (s *Stage) UDPSRouter(name string) *UDPSRouter { return s.udpsRouters[name] }
func (s *Stage) Cacher(name string) Cacher          { return s.cachers[name] }
func (s *Stage) Server(name string) Server          { return s.servers[name] }
func (s *Stage) App(name string) *App               { return s.apps[name] }
func (s *Stage) Svc(name string) *Svc               { return s.svcs[name] }

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
	} else {
		UseExitln("appServers is required for stage")
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
	} else {
		UseExitln("svcServers is required for stage")
	}
	// logFile
	s.ConfigureString("logFile", &s.logFile, func(value string) bool { return value != "" }, LogsDir()+"/stage.log")
	baseDir := BaseDir()
	// cpuFile
	s.ConfigureString("cpuFile", &s.cpuFile, func(value string) bool { return value != "" }, baseDir+"/cpu.prof")
	// hepFile
	s.ConfigureString("hepFile", &s.hepFile, func(value string) bool { return value != "" }, baseDir+"/hep.prof")
	// thrFile
	s.ConfigureString("thrFile", &s.thrFile, func(value string) bool { return value != "" }, baseDir+"/thr.prof")
	// grtFile
	s.ConfigureString("grtFile", &s.grtFile, func(value string) bool { return value != "" }, baseDir+"/grt.prof")
	// blkFile
	s.ConfigureString("blkFile", &s.blkFile, func(value string) bool { return value != "" }, baseDir+"/blk.prof")

	// sub components
	s.fixtures.walk(fixture.OnConfigure)
	s.optwares.walk(Optware.OnConfigure)
	s.backends.walk(backend.OnConfigure)
	s.quicRouters.walk((*QUICRouter).OnConfigure)
	s.tcpsRouters.walk((*TCPSRouter).OnConfigure)
	s.udpsRouters.walk((*UDPSRouter).OnConfigure)
	s.cachers.walk(Cacher.OnConfigure)
	s.servers.walk(Server.OnConfigure)
	s.apps.walk((*App).OnConfigure)
	s.svcs.walk((*Svc).OnConfigure)
	s.cronjobs.walk(Cronjob.OnConfigure)
}
func (s *Stage) OnPrepare() {
	// logger
	if err := os.MkdirAll(filepath.Dir(s.logFile), 0755); err != nil {
		EnvExitln(err.Error())
	}
	//s.logger = newLogger(s.logFile, "") // dividing not needed

	// sub components
	s.fixtures.walk(fixture.OnPrepare)
	s.optwares.walk(Optware.OnPrepare)
	s.backends.walk(backend.OnPrepare)
	s.quicRouters.walk((*QUICRouter).OnPrepare)
	s.tcpsRouters.walk((*TCPSRouter).OnPrepare)
	s.udpsRouters.walk((*UDPSRouter).OnPrepare)
	s.cachers.walk(Cacher.OnPrepare)
	s.servers.walk(Server.OnPrepare)
	s.apps.walk((*App).OnPrepare)
	s.svcs.walk((*Svc).OnPrepare)
	s.cronjobs.walk(Cronjob.OnPrepare)
}
func (s *Stage) OnShutdown() {
	// sub components
	s.cronjobs.walk(Cronjob.OnShutdown)
	s.servers.walk(Server.OnShutdown)
	s.svcs.walk((*Svc).OnShutdown)
	s.apps.walk((*App).OnShutdown)
	s.cachers.walk(Cacher.OnShutdown)
	s.udpsRouters.walk((*UDPSRouter).OnShutdown)
	s.tcpsRouters.walk((*TCPSRouter).OnShutdown)
	s.quicRouters.walk((*QUICRouter).OnShutdown)
	s.backends.walk(backend.OnShutdown)
	s.optwares.walk(Optware.OnShutdown)
	s.fixtures.walk(fixture.OnShutdown)

	// TODO: shutdown stage
}

func (s *Stage) StartAlone() { // single worker process mode
	s.alone = true
	s.id = 0
	s.numCPU = int32(runtime.NumCPU())
	if IsDebug() {
		fmt.Printf("mode=alone numcpu=%d\n", s.numCPU)
	}
	s.start()
}
func (s *Stage) StartShard(id int32) { // multiple worker processes mode
	s.alone = false
	s.id = id
	s.numCPU = 1
	runtime.GOMAXPROCS(1)
	if IsDebug() {
		fmt.Println("mode=shard numcpu=1")
	}
	s.start()
}
func (s *Stage) start() {
	if IsDevel() {
		fmt.Printf("size of http1Conn = %d\n", unsafe.Sizeof(http1Conn{}))
		fmt.Printf("size of h1Conn = %d\n", unsafe.Sizeof(H1Conn{}))
		fmt.Printf("size of http2Conn = %d\n", unsafe.Sizeof(http2Conn{}))
		fmt.Printf("size of http2Stream = %d\n", unsafe.Sizeof(http2Stream{}))
		fmt.Printf("size of http3Conn = %d\n", unsafe.Sizeof(http3Conn{}))
		fmt.Printf("size of http3Stream = %d\n", unsafe.Sizeof(http3Stream{}))
		fmt.Printf("size of Block = %d\n", unsafe.Sizeof(Block{}))
	}
	if IsDebug() {
		fmt.Printf("baseDir=%s\n", BaseDir())
		fmt.Printf("dataDir=%s\n", DataDir())
		fmt.Printf("logsDir=%s\n", LogsDir())
		fmt.Printf("tempDir=%s\n", TempDir())
	}
	if BaseDir() == "" || DataDir() == "" || LogsDir() == "" || TempDir() == "" {
		UseExitln("baseDir, dataDir, logsDir and tempDir must all be set")
	}
	// Configure all components
	if err := s.configure(); err != nil {
		UseExitln(err.Error())
	}
	// Check config requirements
	if len(s.servers) == 0 && len(s.quicRouters) == 0 && len(s.tcpsRouters) == 0 && len(s.udpsRouters) == 0 {
		UseExitln("no server/router provided, nothing to serve")
	}
	// Link apps to http servers
	s.linkAppServers()
	// Link svcs to http servers
	s.linkSvcServers()
	// Prepare all components
	if err := s.prepare(); err != nil {
		EnvExitln(err.Error())
	}
	// Setup running environment
	if err := os.Chdir(BaseDir()); err != nil {
		EnvExitln(err.Error())
	}
	// TODO: use memory barriers
	s.startFixtures()
	s.startOptwares()
	s.startBackends()
	s.startRouters()
	s.startCachers()
	s.startApps()
	s.startSvcs()
	s.startServers()
	s.startCronjobs()
	// TODO: change user here
	s.Logln("stage is ready to serve.")
}

func (s *Stage) linkAppServers() {
	if IsDebug() {
		fmt.Println("link apps to http servers")
	}
	for _, app := range s.apps {
		serverNames := s.appServers[app.Name()]
		if serverNames == nil {
			if IsDebug() {
				fmt.Printf("no server provided for app '%s'\n", app.name)
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
			if IsDebug() {
				fmt.Printf("app %s is linked to http server %s\n", app.name, serverName)
			}
		}
	}
}
func (s *Stage) linkSvcServers() {
	if IsDebug() {
		fmt.Println("link svcs to http servers")
	}
	for _, svc := range s.svcs {
		serverNames := s.svcServers[svc.Name()]
		if serverNames == nil {
			if IsDebug() {
				fmt.Printf("no server provided for svc '%s'\n", svc.name)
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
			if IsDebug() {
				fmt.Printf("svc %s is linked to rpc server %s\n", svc.name, serverName)
			}
		}
	}
}

func (s *Stage) startFixtures() {
	if IsDebug() {
		fmt.Println("start fixtures")
	}
	for _, fixture := range s.fixtures {
		go fixture.run()
	}
}
func (s *Stage) startOptwares() {
	if IsDebug() {
		fmt.Println("start optwares")
	}
	for _, optware := range s.optwares {
		go optware.Run()
	}
}
func (s *Stage) startBackends() {
	if IsDebug() {
		fmt.Println("start backends")
	}
	for _, backend := range s.backends {
		go backend.maintain()
	}
}
func (s *Stage) startRouters() {
	if IsDebug() {
		fmt.Println("start routers")
	}
	for _, quicRouter := range s.quicRouters {
		go quicRouter.serve()
	}
	for _, tcpsRouter := range s.tcpsRouters {
		go tcpsRouter.serve()
	}
	for _, udpsRouter := range s.udpsRouters {
		go udpsRouter.serve()
	}
}
func (s *Stage) startCachers() {
	if IsDebug() {
		fmt.Println("start cachers")
	}
	for _, cacher := range s.cachers {
		go cacher.Maintain()
	}
}
func (s *Stage) startApps() {
	if IsDebug() {
		fmt.Println("start apps")
	}
	for _, app := range s.apps {
		app.start()
	}
}
func (s *Stage) startSvcs() {
	if IsDebug() {
		fmt.Println("start svcs")
	}
	for _, svc := range s.svcs {
		svc.start()
	}
}
func (s *Stage) startServers() {
	if IsDebug() {
		fmt.Println("start servers")
	}
	for _, server := range s.servers {
		go server.Serve()
	}
}
func (s *Stage) startCronjobs() {
	if IsDebug() {
		fmt.Println("start cronjobs")
	}
	for _, cronjob := range s.cronjobs {
		go cronjob.Run()
	}
}

func (s *Stage) configure() (err error) {
	if IsDebug() {
		fmt.Println("now configure stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if IsDebug() {
			fmt.Println("stage successfully configured")
		}
	}()
	s.OnConfigure()
	return nil
}
func (s *Stage) prepare() (err error) {
	if IsDebug() {
		fmt.Println("now prepare stage")
	}
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
		if IsDebug() {
			fmt.Println("stage successfully prepared")
		}
	}()
	s.OnPrepare()
	return nil
}
func (s *Stage) Shutdown() {
	// TODO
	s.Logln("stage shutdown")
	os.Exit(0)
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

func (s *Stage) ID() int32     { return s.id }
func (s *Stage) NumCPU() int32 { return s.numCPU }

func (s *Stage) Log(str string) {
	//s.logger.log(str)
}
func (s *Stage) Logln(str string) {
	//s.logger.logln(str)
}
func (s *Stage) Logf(format string, args ...any) {
	//s.logger.logf(format, args...)
}
