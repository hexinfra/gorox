// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gzip revisers can gzip response content.

package gzip

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	. "github.com/hexinfra/gorox/hemi/standard/revisers"
)

func init() {
	RegisterReviser("gzipReviser", func(name string, stage *Stage, app *App) Reviser {
		r := new(gzipReviser)
		r.onCreate(name, stage, app)
		return r
	})
}

// gzipReviser
type gzipReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage *Stage
	app   *App
	// States
	compressLevel int
	minLength     int64
	contentTypes  []string
}

func (r *gzipReviser) onCreate(name string, stage *Stage, app *App) {
	r.CompInit(name)
	r.stage = stage
	r.app = app
}
func (r *gzipReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *gzipReviser) OnConfigure() {
	// compressLevel
	r.ConfigureInt("compressLevel", &r.compressLevel, nil, 1)
	// minLength
	r.ConfigureInt64("minLength", &r.minLength, func(value int64) bool { return value > 0 }, 0)
	// contentTypes
	r.ConfigureStringList("contentTypes", &r.contentTypes, nil, []string{})
}
func (r *gzipReviser) OnPrepare() {
}

func (r *gzipReviser) Rank() int8 { return RankGzip }

func (r *gzipReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *gzipReviser) BeforePull(req Request, resp Response) { // chunked
	// TODO
}
func (r *gzipReviser) FinishPull(req Request, resp Response) { // chunked
	// TODO
}
func (r *gzipReviser) OnInput(req Request, resp Response, chain Chain) Chain {
	return chain
}

func (r *gzipReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *gzipReviser) BeforePush(req Request, resp Response) { // chunked
	// TODO
}
func (r *gzipReviser) FinishPush(req Request, resp Response) { // chunked
	// TODO
}
func (r *gzipReviser) OnOutput(req Request, resp Response, chain Chain) Chain {
	return chain
}

var (
	gzipReviserBytesVary = []byte("vary")
	gzipReviserBytesGzip = []byte("gzip")
)

var gzipBadContentTypes = []string{
	"image/jpeg",
	"image/png",
}
