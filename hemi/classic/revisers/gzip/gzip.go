// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Gzip revisers can gzip response content.

package gzip

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/classic/revisers"
)

func init() {
	RegisterReviser("gzipReviser", func(compName string, stage *Stage, webapp *Webapp) Reviser {
		r := new(gzipReviser)
		r.onCreate(compName, stage, webapp)
		return r
	})
}

// gzipReviser
type gzipReviser struct {
	// Parent
	Reviser_
	// States
	compressLevel  int
	minLength      int64
	onContentTypes []string
}

func (r *gzipReviser) onCreate(compName string, stage *Stage, webapp *Webapp) {
	r.Reviser_.OnCreate(compName, stage, webapp)
}
func (r *gzipReviser) OnShutdown() {
	r.Webapp().DecSub() // reviser
}

func (r *gzipReviser) OnConfigure() {
	// compressLevel
	r.ConfigureInt("compressLevel", &r.compressLevel, nil, 1)

	// minLength
	r.ConfigureInt64("minLength", &r.minLength, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".minLength has an invalid value")
	}, 0)

	// onContentTypes
	r.ConfigureStringList("onContentTypes", &r.onContentTypes, nil, []string{"text/html"})
}
func (r *gzipReviser) OnPrepare() {
	// TODO
}

func (r *gzipReviser) Rank() int8 { return RankGzip }

func (r *gzipReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
	return
}
func (r *gzipReviser) OnInput(req Request, resp Response, input *Chain) bool {
	// TODO
	return true
}
func (r *gzipReviser) BeforeDraw(req Request, resp Response) { // vague
	// TODO
	return
}
func (r *gzipReviser) FinishDraw(req Request, resp Response) { // vague
	// TODO
	return
}

func (r *gzipReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *gzipReviser) BeforeEcho(req Request, resp Response) { // vague
	// TODO
}
func (r *gzipReviser) OnOutput(req Request, resp Response, output *Chain) {
	// TODO
}
func (r *gzipReviser) FinishEcho(req Request, resp Response) { // vague
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
