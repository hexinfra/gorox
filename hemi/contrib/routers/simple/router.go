// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A simple router.

package simple

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
)

// simpleRouter implements Router.
type simpleRouter struct {
	routes  map[string]Handle // for all methods
	gets    map[string]Handle
	posts   map[string]Handle
	puts    map[string]Handle
	deletes map[string]Handle
}

// New creates a simpleRouter.
func New() *simpleRouter {
	r := new(simpleRouter)
	r.routes = make(map[string]Handle)
	r.gets = make(map[string]Handle)
	r.posts = make(map[string]Handle)
	r.puts = make(map[string]Handle)
	r.deletes = make(map[string]Handle)
	return r
}

func (r *simpleRouter) Link(path string, handle Handle) {
	r.routes[path] = handle
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
	path := req.Path()
	if handle, ok := r.routes[path]; ok {
		return handle
	} else if req.IsGET() {
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
func (r *simpleRouter) HandleName(req Request) string {
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
