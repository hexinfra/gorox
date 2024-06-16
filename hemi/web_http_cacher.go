// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP caching implementation. See RFC 9111.

package hemi

// Cacher component is the interface to storages of HTTP caching.
type Cacher interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Set(key []byte, hobject *Hobject)
	Get(key []byte) (hobject *Hobject)
	Del(key []byte) bool
}

// Cacher_
type Cacher_ struct {
	// Parent
	Component_
	// Assocs
	// States
}

// Hobject is an HTTP object in Cacher.
type Hobject struct {
	// TODO
	uri      []byte
	headers  any
	content  any
	trailers any
}
