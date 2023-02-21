// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 and HTTP/2 server.

package internal

import (
	"context"
	"crypto/tls"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"net"
	"syscall"
)

func init() {
	RegisterServer("httpxServer", func(name string, stage *Stage) Server {
		s := new(httpxServer)
		s.onCreate(name, stage)
		return s
	})
}

// httpxServer is the HTTP/1 and HTTP/2 server.
type httpxServer struct {
	// Mixins
	httpServer_
	// Assocs
	// States
	forceScheme  int8 // scheme that must be used
	adjustScheme bool // use https scheme for TLS and http scheme for TCP?
	enableHTTP2  bool // enable HTTP/2?
	h2cMode      bool // if true, TCP runs HTTP/2 only. TLS is not affected. requires enableHTTP2
}

func (s *httpxServer) onCreate(name string, stage *Stage) {
	s.httpServer_.onCreate(name, stage)
	s.forceScheme = -1 // not force
}
func (s *httpxServer) OnShutdown() {
	// We don't close(s.Shut) here.
	for _, gate := range s.gates {
		gate.shutdown()
	}
}

func (s *httpxServer) OnConfigure() {
	s.httpServer_.onConfigure(s)
	var scheme string
	// forceScheme
	s.ConfigureString("forceScheme", &scheme, func(value string) bool {
		return value == "http" || value == "https"
	}, "")
	if scheme == "http" {
		s.forceScheme = SchemeHTTP
	} else if scheme == "https" {
		s.forceScheme = SchemeHTTPS
	}
	// adjustScheme
	s.ConfigureBool("adjustScheme", &s.adjustScheme, true)
	// enableHTTP2
	s.ConfigureBool("enableHTTP2", &s.enableHTTP2, false) // TODO: change to true after HTTP/2 is fully implemented
	// h2cMode
	s.ConfigureBool("h2cMode", &s.h2cMode, false)
}
func (s *httpxServer) OnPrepare() {
	s.httpServer_.onPrepare(s)
	if s.tlsMode {
		var nextProtos []string
		if s.enableHTTP2 {
			nextProtos = []string{"h2", "http/1.1"}
		} else {
			nextProtos = []string{"http/1.1"}
		}
		s.tlsConfig.NextProtos = nextProtos
	} else if !s.enableHTTP2 {
		s.h2cMode = false
	}
}

func (s *httpxServer) Serve() { // goroutine
	for id := int32(0); id < s.numGates; id++ {
		gate := new(httpxGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		s.IncSub(1)
		if s.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
	if IsDebug(2) {
		Debugf("httpxServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

// httpxGate is a gate of httpxServer.
type httpxGate struct {
	// Mixins
	httpGate_
	// Assocs
	server *httpxServer
	// States
	gate *net.TCPListener
}

func (g *httpxGate) init(server *httpxServer, id int32) {
	g.httpGate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *httpxGate) open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		if err := system.SetReusePort(rawConn); err != nil {
			return err
		}
		return system.SetDeferAccept(rawConn)
	}
	gate, err := listenConfig.Listen(context.Background(), "tcp", g.address)
	if err == nil {
		g.gate = gate.(*net.TCPListener)
	}
	return err
}
func (g *httpxGate) shutdown() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *httpxGate) serveTCP() { // goroutine
	getHTTPConn := getHTTP1Conn
	if g.server.h2cMode {
		getHTTPConn = getHTTP2Conn
	}
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				tcpConn.Close()
				g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			httpConn := getHTTPConn(connID, g.server, g, tcpConn, rawConn)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns
	if IsDebug(2) {
		Debugf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.SubDone()
}
func (g *httpxGate) serveTLS() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.server.tlsConfig)
			// TODO: set deadline
			if err := tlsConn.Handshake(); err != nil {
				g.justClose(tcpConn)
				continue
			}
			connState := tlsConn.ConnectionState()
			getHTTPConn := getHTTP1Conn
			if connState.NegotiatedProtocol == "h2" {
				getHTTPConn = getHTTP2Conn
			}
			httpConn := getHTTPConn(connID, g.server, g, tlsConn, nil)
			go httpConn.serve() // httpConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns
	if IsDebug(2) {
		Debugf("httpxGate=%d TLS done\n", g.id)
	}
	g.server.SubDone()
}

func (g *httpxGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}
