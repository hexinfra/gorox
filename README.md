Welcome
=======

Welcome to Gorox!

Gorox is an advanced Web server, application server, RPC server, and proxy
server. It can be used as:

  * Web Server (HTTP 1/2/3, HWEB, WebSocket, TLS, FCGI, uwsgi, AJP)
  * Application Server for Go (Applications, Frameworks)
  * RPC Server for Go (HRPC Services, gRPC Services)
  * Web Proxy Server (HTTP 1/2/3, HWEB, WebSocket, Forward, Reverse, Caching)
  * Layer 4 Proxy Server (QUIC, TCP/TLS, UDP/DTLS)
  * Service Mesh (Data Plane)
  * ... and more through its highly extensible compoments design!

There is also a controller program called Goops which is distributed with
Gorox and can be used to manage your Gorox cluster optionally.

For more details about Gorox and Goops, please see: https://gorox.io/ .


Motivation
==========

To be written.


Platforms
=========

Gorox should be working on these operating systems:

  * Linux kernel >= 3.9
  * FreeBSD >= 12.0
  * Apple macOS >= Catalina
  * Microsoft Windows >= 10

And these 64-bit CPU architectures:

  * AMD64, also known as x64, x86-64, Intel 64
  * ARM64, also known as AArch64
  * RISC-V 64
  * LoongArch 64

Other platforms are currently not considered and probably don't work.


Quickstart
==========

To start using Gorox, you can download the official binary distributions. If you
need to build from source, please ensure you have Go >= 1.19 installed:

  shell> go version

Then build Gorox with Go (if build failed, set CGO_ENABLED=0 and try again):

  shell> go build

To run Gorox as a background daemon (if not, remove the "-daemon" option):

  shell> ./gorox serve -daemon

Visit http://localhost:3080 to check if it works. To exit server gracefully:

  shell> ./gorox quit

For more actions and options, run:

  shell> ./gorox -h

To install, move the whole Gorox directory to where you like. To uninstall,
remove the whole Gorox directory.


Performance
===========

Gorox is fast. You can use wrk to perform a simple benchmark:

  shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/hello
  shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/hello.html

Change the parameters and/or target URL to match your need.

Generally, the result is about 80% of nginx and slightly faster than fasthttp.


Architecture
============

Deployment
----------

A modest deployment architecture using Gorox might like this:

```
           mobile  pc iot
              |    |   |
              |    |   |           public internet
              |    |   |
              v    v   v
            +------------+
+-----------| edgeProxy1 |------------+
|           +--+---+--+--+            | gorox cluster
|   web        |   |  |       tcp     |
|     +--------+   |  +-------+       |
|     |            |          |       |
|     v           rpc         v       |
|  +------+        |       +------+   |
|  | app1 +----+   |   +---+ srv1 |   |
|  +------+    |   |   |   +---+--+   |
|              |   |   |       |      |
|              v   v   v       v      |
| +------+   +----------+   +------+  |   +------------------+
| | svc1 |<->| sidecar1 |   | app2 +--+-->| php-fpm / tomcat |
| +------+   +----+-----+   +------+  |   +------------------+
|                 |                   |
|                 v                   |
| +------+   +----------+   +------+  |
| | svc2 |<--+ sidecar2 |   | job1 |  |
| +------+   +----------+   +------+  |
|                                     |
+----------+------+---------+---------+
           |      |         |
           v      v         v
+-------------------------------------+
|    +------+  +-----+  +--------+    |
| .. | db1  |  | mq1 |  | cache1 | .. | data layer
|    +------+  +-----+  +--------+    |
+-------------------------------------+

```

In this typical architecture, with various configurations, Gorox can play ALL of
the roles in "gorox cluster":

  * edgeProxy1: This is the Edge Proxy, also works as an API Gateway or WAF,
  * app1: This is a Web application implemented directly on Gorox,
  * srv1: This is a TCP server implemented directly on Gorox,
  * svc1: This is a public RPC service implemented directly on Gorox,
  * sidecar1: This is a Service Mesh sidecar for svc1,
  * app2: This is a reverse proxy passing requests to PHP-FPM or Tomcat server,
  * svc2: This is a private RPC service implemented directly on Gorox,
  * sidecar2: This is a Service Mesh sidecar for svc2,
  * job1: This is a background application doing something periodically.

The whole "gorox cluster" can optionally be managed by a Goops instance. In this
configuration, all Gorox instances in "gorox cluster" are connected to the Goops
instance and under its control.


Processes
---------

A Gorox instance has a pair of processes: a leader, and a worker:

```
                   +----------------+
                   | goops instance |
                   +----------------+
                           ^
                           | goroxConn
                           v
                   +-------+--------+         +----------------+ public traffic
          admConn  |                | cmdPipe |                |<=============>
operator --------->| leader process |<------->| worker process |<=============>
                   |                |         |                |<=============>
                   +----------------+         +----------------+
```

Leader process manages the worker process, which do the real and heavy work.

A Gorox instance can be controlled by operators through the admin interface in
leader process. Alternately, it can connects to a Goops instance and delegate
its administration to Goops. In this way, the admin interface in leader process
is disabled.


Internals
---------

The logical architecture of Gorox internal (called Hemi Engine) looks like this:

```
+--------------------------------------------+     ^    shutdown
|                cronjob(*)                  |     |       |
+--------+---+--------------+----------------+     |       |
|        |   |      rpc     |      web       |     |       |
|        | s |     server   |     server     |     |       |
| mesher | e | [gate][conn] |  [gate][conn]  |     |       |
| filter | r +--------------+----------------+     |       |
| editor | v |              | app(*) reviser |     |       |
|  case  | e |   svc(*)     | socklet handlet|     |       |
|        | r |          +---+--+ rule +------+     |       |
|        |(*)|          |stater|      |cacher|     |       |
+--------+---+----------+------+------+------+     |       |
|           [node] [conn] backend            |     |       |
+---+---+---+---+---+---+---+---+------------+     |       |
| o | u | t | g | a | t | e | s |   uniture  |     |       |
+---+---+---+---+---+---+---+---+------------+     |       |
|   clock   |     fcache    |    resolver    |     |       |
+-----------+---------------+----------------+  prepare    v

```

Components marked as (*) are user programmable. They are placed in these dirs:

  * apps/ - Place your Web applications,
  * jobs/ - Place your Cronjobs,
  * srvs/ - Place your General servers,
  * svcs/ - Place your RPC services.


Documentation
=============

View Gorox documentation online:

  * English version: https://gorox.io/docs
  * Chinese version: https://www.gorox.io/docs

Or view locally (ensure your local server under hemi/gosites is started):

  * English version: http://gorox.net:5080/docs
  * Chinese version: http://www.gorox.net:5080/docs


Community
=========

Currently Github Discussions is used for discussing:

  https://github.com/hexinfra/gorox/discussions


Contact
=======

Gorox is originally written by Zhang Jingcheng <diogin@gmail.com>.
You can also contact him through Twitter: @diogin.

The official website of the Gorox project is at:

  * English version: https://gorox.io/
  * Chinese version: https://www.gorox.io/


License
=======

Gorox is licensed under a BSD License. See LICENSE.md file.


Contributing
============

Gorox is hosted at Github:

  https://github.com/hexinfra/gorox

Fork this repository and contribute your patch through Github Pull Requests.

By contributing to Gorox, you MUST agree to release your code under the BSD
License that you can find in the LICENSE.md file.

