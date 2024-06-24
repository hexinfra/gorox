// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package testee

func Main() {
	http1TestHello()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
