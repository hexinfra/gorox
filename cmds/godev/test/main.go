// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Tests.

package test

func Main() {
	http1TestHello()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
