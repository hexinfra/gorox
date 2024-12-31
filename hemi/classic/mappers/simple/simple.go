// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// A simple mapper.

package simple

import (
	. "github.com/hexinfra/gorox/hemi"
)

// simpleMapper implements Mapper.
type simpleMapper struct {
	mapping map[string]Handle // for all methods
	gets    map[string]Handle
	posts   map[string]Handle
	puts    map[string]Handle
	deletes map[string]Handle
}

// New creates a simpleMapper.
func New() *simpleMapper {
	m := new(simpleMapper)
	m.mapping = make(map[string]Handle)
	m.gets = make(map[string]Handle)
	m.posts = make(map[string]Handle)
	m.puts = make(map[string]Handle)
	m.deletes = make(map[string]Handle)
	return m
}

func (m *simpleMapper) Map(path string, handle Handle) {
	m.mapping[path] = handle
}

func (m *simpleMapper) GET(path string, handle Handle) {
	m.gets[path] = handle
}
func (m *simpleMapper) POST(path string, handle Handle) {
	m.posts[path] = handle
}
func (m *simpleMapper) PUT(path string, handle Handle) {
	m.puts[path] = handle
}
func (m *simpleMapper) DELETE(path string, handle Handle) {
	m.deletes[path] = handle
}

func (m *simpleMapper) FindHandle(req Request) Handle {
	// TODO
	path := req.Path()
	if handle, ok := m.mapping[path]; ok {
		return handle
	} else if req.IsGET() {
		return m.gets[path]
	} else if req.IsPOST() {
		return m.posts[path]
	} else if req.IsPUT() {
		return m.puts[path]
	} else if req.IsDELETE() {
		return m.deletes[path]
	} else {
		return nil
	}
}
func (m *simpleMapper) HandleName(req Request) string {
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
	return WeakString(name)
}
