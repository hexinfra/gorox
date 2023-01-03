// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Router routes HTTP requests to handles in handlets.

package internal

import (
	"github.com/hexinfra/gorox/hemi/libraries/risky"
)

// Router performs request routing.
type Router interface {
	FindHandle(req Request) Handle
	CreateName(req Request) string
}

// simpleRouter implements Router.
type simpleRouter struct {
	gets    map[string]Handle
	posts   map[string]Handle
	puts    map[string]Handle
	deletes map[string]Handle
}

// NewSimpleRouter creates a simpleRouter.
func NewSimpleRouter() *simpleRouter {
	r := new(simpleRouter)
	r.gets = make(map[string]Handle)
	r.posts = make(map[string]Handle)
	r.puts = make(map[string]Handle)
	r.deletes = make(map[string]Handle)
	return r
}

func (r *simpleRouter) GET(path string, handle Handle) {
	r.gets[path] = handle
}
func (r *simpleRouter) POST(path string, handle Handle) {
	r.posts[path] = handle
}
func (r *simpleRouter) PUT(path string, handle Handle) {
	r.puts[path] = handle
}
func (r *simpleRouter) DELETE(path string, handle Handle) {
	r.deletes[path] = handle
}

func (r *simpleRouter) FindHandle(req Request) Handle {
	// TODO
	if path := req.Path(); req.IsGET() {
		return r.gets[path]
	} else if req.IsPOST() {
		return r.posts[path]
	} else if req.IsPUT() {
		return r.puts[path]
	} else if req.IsDELETE() {
		return r.deletes[path]
	} else {
		return nil
	}
}
func (r *simpleRouter) CreateName(req Request) string {
	method := req.UnsafeMethod()
	path := req.UnsafePath() // always starts with '/'
	name := req.UnsafeMake(len(method) + len(path))
	n := copy(name, method)
	copy(name[n:], path)
	for i := n; i < len(name); i++ {
		if name[i] == '/' {
			name[i] = '_'
		}
	}
	return risky.WeakString(name)
}
