// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Goben is a simple HTTP benchmarking tool.

package main

import (
	"flag"
	"fmt"
	"net/url"
	"sync"
)

var (
	C int      // concurrent connections
	R int      // requests per connection
	S int      // streams per connection
	U *url.URL // target url
	M string   // http method
	H string   // http headers
	B string   // http content
)

// goben -c 240 -r 1000 -u http1://localhost:3080/hello
// goben -c 240 -r 1000 -u http1s://localhost:3080/hello

// goben -c 240 -r 1000 -s 100 -u http2://localhost:3080/hello
// goben -c 240 -r 1000 -s 100 -u http2s://localhost:3080/hello

// goben -c 240 -r 1000 -s 100 -u http3://localhost:3080/hello

func main() {
	var (
		u string
		e error
	)
	flag.IntVar(&C, "c", 240, "concurrent connections")
	flag.IntVar(&R, "r", 1000, "requests per connection")
	flag.IntVar(&S, "s", 100, "streams per connection")
	flag.StringVar(&u, "u", "http1://localhost:3080/hello", "target url")
	flag.StringVar(&M, "m", "GET", "http method")
	flag.StringVar(&H, "h", "", "http headers")
	flag.StringVar(&B, "b", "", "http content")
	flag.Parse()
	if U, e = url.Parse(u); e != nil {
		fmt.Println(e.Error())
		return
	}
	switch U.Scheme {
	case "http1":
		http1()
	case "http1s":
		http1s()
	case "http2":
		http2()
	case "http2s":
		http2s()
	case "http3":
		http3()
	default:
		fmt.Println("unknown url scheme")
		return
	}
}

var (
	benchmark sync.WaitGroup
	clients   []client
)

type client interface {
	bench()
}
