// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP proxy handlet and WebSocket proxy socklet implementation.

/*

--http[1-3]Stream-->                        ----H[1-3]Stream--->
[REVISERS]      ^    -------pass/post----->                 ^
                |                                           |
httpInMessage_  | stream                    httpOutMessage_ | stream
^ - stream -----+                           ^ - stream -----+
| - shell ------+                           | - shell ------+
|               | httpInMessage             |               | httpOutMessage
+-httpRequest_  |                           +-hRequest_     |
  ^ - httpConn -+---> http[1-3]Conn           ^ - hConn ----+---> H[1-3]Conn
  |             |                             |             |
  |             v                             |             v
  +-http[1-3]Request                          +-H[1-3]Request
                          \           /
                \          \         /
      1/2/3      \          \       /             1/2/3
   [httpServer]  [Handlet]/[httpProxy]         [httpClient]
      1/2/3      /          /       \             1/2/3
                /          /         \
                          /           \
<--http[1-3]Stream--                        <---H[1-3]Stream----
[REVISERS]       ^   <------pass/post------                ^
                 |                                         |
httpOutMessage_  | stream                   httpInMessage_ | stream
^ - stream ------+                          ^ - stream ----+
| - shell -------+                          | - shell -----+
|                | httpOutMessage           |              | httpInMessage
+-httpResponse_  |                          +-hResponse_   |
  ^ - httpConn --+---> http[1-3]Conn          ^ - hConn ---+---> H[1-3]Conn
  |              |                            |            |
  |              v                            |            v
  +-http[1-3]Response                         +-H[1-3]Response


NOTES:

  * messages are composed of control, headers, [content, [trailers]].
  * control & headers is called head, and it must be small.
  * contents, if exist (perhaps of zero size), may be large/small, counted/chunked.
  * trailers must be small, and only exist when contents exist and are chunked.
  * incoming messages need parsing.
  * outgoing messages need building.

*/

package internal

// httpProxy_ is the mixin for http[1-3]Proxy.
type httpProxy_ struct {
	// Mixins
	Handlet_
	proxy_
	// Assocs
	app    *App   // the app to which the proxy belongs
	cacher Cacher // the cache server which is used by this proxy
	// States
	bufferClientContent bool        // buffer client content into TempFile?
	bufferServerContent bool        // buffer server content into TempFile?
	delRequestHeaders   [][]byte    // client request headers to delete
	addRequestHeaders   [][2][]byte // headers appended to client request
	delResponseHeaders  [][]byte    // server response headers to delete
	addResponseHeaders  [][2][]byte // headers appended to server response
}

func (h *httpProxy_) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.proxy_.onCreate(stage)
	h.app = app
}

func (h *httpProxy_) onConfigure(c Component) {
	h.proxy_.onConfigure(c)
	if h.proxyMode == "forward" && !h.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}
	// withCacher
	if v, ok := h.Find("withCacher"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCacher")
		}
	}
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	var headers []string
	// delRequestHeaders
	h.ConfigureStringList("delRequestHeaders", &headers, nil, []string{})
	for _, header := range headers {
		h.delRequestHeaders = append(h.delRequestHeaders, []byte(header))
	}
	// delResponseHeaders
	h.ConfigureStringList("delResponseHeaders", &headers, nil, []string{})
	for _, header := range headers {
		h.delResponseHeaders = append(h.delResponseHeaders, []byte(header))
	}
}
func (h *httpProxy_) onPrepare() {
	h.proxy_.onPrepare()
}

func (h *httpProxy_) IsProxy() bool { return true }
func (h *httpProxy_) IsCache() bool { return h.cacher != nil }

// sockProxy_ is the mixin for sock[1-3]Proxy.
type sockProxy_ struct {
	// Mixins
	Socklet_
	proxy_
	// Assocs
	app *App // the app to which the proxy belongs
	// States
}

func (s *sockProxy_) onCreate(name string, stage *Stage, app *App) {
	s.CompInit(name)
	s.proxy_.onCreate(stage)
	s.app = app
}

func (s *sockProxy_) onConfigure(c Component) {
	s.proxy_.onConfigure(c)
	if s.proxyMode == "forward" && !s.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}
}
func (s *sockProxy_) onPrepare() {
	s.proxy_.onPrepare()
}

func (s *sockProxy_) IsProxy() bool { return true }
