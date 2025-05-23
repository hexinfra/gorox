// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General types for net, rpc, and web.

package hemi

import (
	"crypto/tls"
	"errors"
	"os"
	"time"
)

// holder collects shared methods between Gate and Node.
type holder interface {
	Stage() *Stage
	Address() string
	UDSMode() bool
	TLSMode() bool
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// _holder_ is a mixin.
type _holder_ struct { // for Node_, Server_, and Gate_
	// Assocs
	stage *Stage // current stage
	// States
	address      string        // :port, hostname:port, /path/to/unix.sock
	udsMode      bool          // is address a unix domain socket?
	tlsMode      bool          // use tls to secure the transport?
	tlsConfig    *tls.Config   // set if tls mode is true
	readTimeout  time.Duration // read() timeout
	writeTimeout time.Duration // write() timeout
}

func (h *_holder_) onConfigure(comp Component, defaultRead time.Duration, defaultWrite time.Duration) {
	// .tlsMode
	comp.ConfigureBool("tlsMode", &h.tlsMode, false)
	if h.tlsMode {
		h.tlsConfig = new(tls.Config)
	}

	// .readTimeout
	comp.ConfigureDuration("readTimeout", &h.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, defaultRead)

	// .writeTimeout
	comp.ConfigureDuration("writeTimeout", &h.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, defaultWrite)
}
func (h *_holder_) onPrepare(comp Component) {
}

func (h *_holder_) Stage() *Stage { return h.stage }

func (h *_holder_) Address() string             { return h.address }
func (h *_holder_) UDSMode() bool               { return h.udsMode }
func (h *_holder_) TLSMode() bool               { return h.tlsMode }
func (h *_holder_) TLSConfig() *tls.Config      { return h.tlsConfig }
func (h *_holder_) ReadTimeout() time.Duration  { return h.readTimeout }
func (h *_holder_) WriteTimeout() time.Duration { return h.writeTimeout }

// _accessLogger_ is a mixin.
type _accessLogger_ struct {
	// States
	useLogger string    // "noop", "simple", ...
	logConfig LogConfig // used to configure logger
	logger    Logger    // the logger
}

func (l *_accessLogger_) onConfigure(comp Component) {
	// .useLogger
	comp.ConfigureString("useLogger", &l.useLogger, func(value string) error {
		if loggerRegistered(value) {
			return nil
		}
		return errors.New(".useLogger has an unknown value")
	}, "noop")

	// .logConfig
	if v, ok := comp.Find("logConfig"); ok {
		vLogConfig, ok := v.Dict()
		if !ok {
			UseExitln(".logConfig must be a dict")
		}
		// target
		vTarget, ok := vLogConfig["target"]
		if !ok {
			UseExitln("target is required in .logConfig")
		}
		if target, ok := vTarget.String(); ok {
			l.logConfig.Target = target
		} else {
			UseExitln("target in .logConfig must be a string")
		}
		// bufSize
		vBufSize, ok := vLogConfig["bufSize"]
		if ok {
			if bufSize, ok := vBufSize.Int32(); ok && bufSize >= _1K {
				l.logConfig.BufSize = bufSize
			} else {
				UseExitln("invalid bufSize in .logConfig")
			}
		} else {
			l.logConfig.BufSize = _4K
		}
	}
}
func (l *_accessLogger_) onPrepare(comp Component) {
	logger := createLogger(l.useLogger, &l.logConfig)
	if logger == nil {
		UseExitln("cannot create logger")
	}
	l.logger = logger
}

func (l *_accessLogger_) Logf(f string, v ...any) { l.logger.Logf(f, v...) }
func (l *_accessLogger_) CloseLog()               { l.logger.Close() }

// contentSaver
type contentSaver interface {
	RecvTimeout() time.Duration  // timeout to recv the whole message content. zero means no timeout
	SendTimeout() time.Duration  // timeout to send the whole message. zero means no timeout
	MaxContentSize() int64       // max content size allowed
	SaveContentFilesDir() string // the dir to save content temporarily
}

// _contentSaver_ is a mixin.
type _contentSaver_ struct {
	// States
	recvTimeout         time.Duration // timeout to recv the whole message content. zero means no timeout
	sendTimeout         time.Duration // timeout to send the whole message. zero means no timeout
	maxContentSize      int64         // max content size allowed to receive
	saveContentFilesDir string        // temp content files are placed here
}

func (s *_contentSaver_) onConfigure(comp Component, defaultRecv time.Duration, defaultSend time.Duration, defaultDir string) {
	// .recvTimeout
	comp.ConfigureDuration("recvTimeout", &s.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, defaultRecv)

	// .sendTimeout
	comp.ConfigureDuration("sendTimeout", &s.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, defaultSend)

	// .maxContentSize
	comp.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)

	// .saveContentFilesDir
	comp.ConfigureString("saveContentFilesDir", &s.saveContentFilesDir, func(value string) error {
		if value != "" && len(value) <= 232 {
			return nil
		}
		return errors.New(".saveContentFilesDir has an invalid value")
	}, defaultDir)
}
func (s *_contentSaver_) onPrepare(comp Component, perm os.FileMode) {
	if err := os.MkdirAll(s.saveContentFilesDir, perm); err != nil {
		EnvExitln(err.Error())
	}
	if s.saveContentFilesDir[len(s.saveContentFilesDir)-1] != '/' {
		s.saveContentFilesDir += "/"
	}
}

func (s *_contentSaver_) RecvTimeout() time.Duration  { return s.recvTimeout }
func (s *_contentSaver_) SendTimeout() time.Duration  { return s.sendTimeout }
func (s *_contentSaver_) MaxContentSize() int64       { return s.maxContentSize }
func (s *_contentSaver_) SaveContentFilesDir() string { return s.saveContentFilesDir } // must ends with '/'
