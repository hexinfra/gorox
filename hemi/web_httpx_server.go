// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1 and HTTP/2 server.

package hemi

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/library/system"
)

func init() {
	RegisterServer("httpxServer", func(name string, stage *Stage) Server {
		s := new(httpxServer)
		s.onCreate(name, stage)
		return s
	})
}

const (
	httpModeAdaptive = 0 // must be 0
	httpModeHTTP1    = 1
	httpModeHTTP2    = 2
)

// httpxServer is the HTTP/1 and HTTP/2 server.
type httpxServer struct {
	// Parent
	httpServer_[*httpxGate]
	// States
	httpMode int8 // adaptive, http1, http2
}

func (s *httpxServer) onCreate(name string, stage *Stage) {
	s.httpServer_.onCreate(name, stage)
}

func (s *httpxServer) OnConfigure() {
	s.httpServer_.onConfigure()

	if DebugLevel() >= 2 { // remove this condition after HTTP/2 server has been fully implemented
		// httpMode
		s.ConfigureInt8("httpMode", &s.httpMode, nil, httpModeAdaptive)
	}
}
func (s *httpxServer) OnPrepare() {
	s.httpServer_.onPrepare()

	if s.IsTLS() {
		var nextProtos []string
		switch s.httpMode {
		case httpModeHTTP2:
			nextProtos = []string{"h2"}
		case httpModeHTTP1:
			nextProtos = []string{"http/1.1"}
		default:
			nextProtos = []string{"h2", "http/1.1"}
		}
		s.tlsConfig.NextProtos = nextProtos
	}
}

func (s *httpxServer) Serve() { // runner
	for id := int32(0); id < s.numGates; id++ {
		gate := new(httpxGate)
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub() // gate
		if s.IsTLS() {
			go gate.serveTLS()
		} else if s.IsUDS() {
			go gate.serveUDS()
		} else {
			go gate.serveTCP()
		}
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("httpxServer=%s done\n", s.Name())
	}
	s.stage.DecSub() // server
}

// httpxGate is a gate of httpxServer.
type httpxGate struct {
	// Parent
	Gate_
	// Assocs
	server *httpxServer
	// States
	listener net.Listener // the real gate. set after open
}

func (g *httpxGate) init(id int32, server *httpxServer) {
	g.Gate_.Init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *httpxGate) Server() Server  { return g.server }
func (g *httpxGate) Address() string { return g.server.Address() }
func (g *httpxGate) IsTLS() bool     { return g.server.IsTLS() }
func (g *httpxGate) IsUDS() bool     { return g.server.IsUDS() }

func (g *httpxGate) Open() error {
	var (
		listener net.Listener
		err      error
	)
	if g.IsUDS() {
		address := g.Address()
		// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
		// This affects graceful upgrading, maybe we can implement fd transfer in the future.
		os.Remove(address)
		listener, err = net.Listen("unix", address)
		if err == nil {
			g.listener = listener.(*net.UnixListener)
			if DebugLevel() >= 1 {
				Printf("httpxGate id=%d address=%s opened!\n", g.id, g.Address())
			}
		}
	} else {
		listenConfig := new(net.ListenConfig)
		listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
			if err := system.SetReusePort(rawConn); err != nil {
				return err
			}
			return system.SetDeferAccept(rawConn)
		}
		listener, err = listenConfig.Listen(context.Background(), "tcp", g.Address())
		if err == nil {
			g.listener = listener.(*net.TCPListener)
			if DebugLevel() >= 1 {
				Printf("httpxGate id=%d address=%s opened!\n", g.id, g.Address())
			}
		}
	}
	return err
}
func (g *httpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *httpxGate) serveTLS() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub() // conn
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.server.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			if connState := tlsConn.ConnectionState(); connState.NegotiatedProtocol == "h2" {
				serverConn := getServer2Conn(connID, g, tlsConn, nil)
				go serverConn.serve() // serverConn is put to pool in serve()
			} else {
				serverConn := getServer1Conn(connID, g, tlsConn, nil)
				go serverConn.serve() // serverConn is put to pool in serve()
			}
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TLS done\n", g.id)
	}
	g.server.DecSub() // gate
}
func (g *httpxGate) serveUDS() { // runner
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
	for {
		unixConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub() // conn
		if g.ReachLimit() {
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			if g.server.httpMode == httpModeHTTP2 {
				serverConn := getServer2Conn(connID, g, unixConn, rawConn)
				go serverConn.serve() // serverConn is put to pool in serve()
			} else {
				serverConn := getServer1Conn(connID, g, unixConn, rawConn)
				go serverConn.serve() // serverConn is put to pool in serve()
			}
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub() // gate
}
func (g *httpxGate) serveTCP() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: accept error: %v\n", g.server.name, g.id, err)
				continue
			}
		}
		g.IncSub() // conn
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				//g.stage.Logf("httpxServer[%s] httpxGate[%d]: SyscallConn() error: %v\n", g.server.name, g.id, err)
				continue
			}
			if g.server.httpMode == httpModeHTTP2 {
				serverConn := getServer2Conn(connID, g, tcpConn, rawConn)
				go serverConn.serve() // serverConn is put to pool in serve()
			} else {
				serverConn := getServer1Conn(connID, g, tcpConn, rawConn)
				go serverConn.serve() // serverConn is put to pool in serve()
			}
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("httpxGate=%d TCP done\n", g.id)
	}
	g.server.DecSub() // gate
}

func (g *httpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecConns()
	g.DecSub()
}
