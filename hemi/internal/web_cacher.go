// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General cacher implementation.

package internal

// Cacher component is the interface to storages of HTTP caching. See RFC 9111.
type Cacher interface {
	// Imports
	Component
	// Methods
	Maintain() // goroutine
	Set(key []byte, hobject *Hobject)
	Get(key []byte) (hobject *Hobject)
	Del(key []byte) bool
}

// Cacher_
type Cacher_ struct {
	// Mixins
	Component_
}

// Hobject is an HTTP object in cacher
type Hobject struct {
	// TODO
	uri      []byte
	headers  any
	content  any
	trailers any
}
