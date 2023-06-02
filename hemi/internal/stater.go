// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General stater implementation.

package internal

// Stater component is the interface to storages of HTTP states. See RFC 6265.
type Stater interface {
	// Imports
	Component
	// Methods
	Maintain() // goroutine
	Set(sid []byte, session *Session)
	Get(sid []byte) (session *Session)
	Del(sid []byte) bool
}

// Stater_
type Stater_ struct {
	// Mixins
	Component_
}

// Session is an HTTP session in stater
type Session struct {
	// TODO
	ID     [40]byte // session id
	Secret [40]byte // secret
	Role   int8     // 0: default, >0: app defined values
	Device int8     // terminal device type
	state1 int8     // app defined state1
	state2 int8     // app defined state2
	state3 int32    // app defined state3
	expire int64    // unix time
	states map[string]string
}

func (s *Session) init() {
	s.states = make(map[string]string)
}

func (s *Session) Get(name string) string        { return s.states[name] }
func (s *Session) Set(name string, value string) { s.states[name] = value }
func (s *Session) Del(name string)               { delete(s.states, name) }
