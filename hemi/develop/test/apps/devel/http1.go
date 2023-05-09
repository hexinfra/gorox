// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package devel

import (
	"fmt"
	"net"
)

func http1TestHello() {
	conn, err := net.Dial("tcp", "127.0.0.1:4080")
	must(err)
	defer conn.Close()
	req := `GET / HTTP/1.1
Host: f.com

`
	n, err := conn.Write([]byte(req))
	must(err)
	if n != len(req) {
		panic("n != len(req)")
	}
	p := make([]byte, 1024)
	n, err = conn.Read(p)
	must(err)
	fmt.Println(string(p[:n]))
}
