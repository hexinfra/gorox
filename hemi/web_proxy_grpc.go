// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// gRPC reverse proxy implementation.

package hemi

func init() {
	RegisterHandlet("grpcProxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(grpcProxy)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// grpcProxy handlet passes web requests to backend grpc servers.
type grpcProxy struct {
	// Parent
	Handlet_
	// Assocs
	stage   *Stage     // current stage
	webapp  *Webapp    // the webapp to which the proxy belongs
	backend WebBackend // the backend to pass to
	// States
}

func (h *grpcProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *grpcProxy) OnShutdown() {
	h.webapp.DecSub()
}

func (h *grpcProxy) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				h.backend = backend.(WebBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for grpc proxy")
	}
}
func (h *grpcProxy) OnPrepare() {
}

func (h *grpcProxy) IsProxy() bool { return true }

func (h *grpcProxy) Handle(req Request, resp Response) (handled bool) {
	// TODO
	return true
}
