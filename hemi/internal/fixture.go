// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General fixture implementation.

package internal

// fixture component.
//
// Fixtures only exist in internal, and are created by stage.
// Some critical functions, like clock and name resolv, are
// implemented as fixtures.
//
// Fixtures are singletons in stage.
type fixture interface {
	// Imports
	Component
	// Methods
	run() // goroutine
}

// fixture_
type fixture_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
}

func (f *fixture_) onCreate(name string, stage *Stage) {
	f.MakeComp(name)
	f.stage = stage
}
