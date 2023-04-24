// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gzip revisers can gzip response content.

package gzip

import (
	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
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
	compressLevel  int
	minLength      int64
	onContentTypes []string
}

func (r *gzipReviser) onCreate(name string, stage *Stage, app *App) {
	r.MakeComp(name)
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
	// onContentTypes
	r.ConfigureStringList("onContentTypes", &r.onContentTypes, nil, []string{"text/html"})
}
func (r *gzipReviser) OnPrepare() {
	// TODO
}

func (r *gzipReviser) Rank() int8 { return RankGzip }

func (r *gzipReviser) BeforeRecv(req Request, resp Response) { // sized
	return
}
func (r *gzipReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) { // sized
	return chain, true
}

func (r *gzipReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *gzipReviser) OnSend(req Request, resp Response, content *Chain) { // sized
	// TODO
}

func (r *gzipReviser) BeforeDraw(req Request, resp Response) { // unsized
	return
}
func (r *gzipReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) { // unsized
	return chain, true
}
func (r *gzipReviser) FinishDraw(req Request, resp Response) { // unsized
	return
}

func (r *gzipReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
}
func (r *gzipReviser) OnEcho(req Request, resp Response, chunks *Chain) { // unsized
	// TODO
}
func (r *gzipReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
}

var (
	gzipReviserBytesVary = []byte("vary")
	gzipReviserBytesGzip = []byte("gzip")
)

var gzipBadContentTypes = []string{
	"image/jpeg",
	"image/png",
}
