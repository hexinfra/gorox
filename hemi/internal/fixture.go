// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Fixtures and Unitures.

package internal

// fixture component.
//
// Fixtures only exist in internal, and are created by stage.
// Some critical functions, like clock and name resolver, are
// implemented as fixtures.
type fixture interface {
	Component
	run() // goroutine
}

// Uniture component.
//
// Unitures behave like fixtures except that they are optional
// and extendible, so users can create their own unitures.
type Uniture interface {
	Component
	Run() // goroutine
}
