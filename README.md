Welcome
=======

Welcome to Gorox!

Gorox is an advanced Web server, App server, RPC framework, and proxy server. It
can be used as:

  * Web Server (HTTP 1/2/3, WebSocket, TLS, HWEB, FCGI, uwsgi, AJP)
  * App Server for Go (Applications, Frameworks)
  * RPC Framework for Go (HRPC, gRPC, Thrift)
  * Web Proxy Server (HTTP, WebSocket, Tunneling, Forward, Reverse, Caching)
  * Layer 4 Proxy Server (QUIC, TCP/TLS, UDP/DTLS)
  * ... and more through its highly extensible compoments design!

There is also a controller program called Myrox which is distributed with Gorox
and can be used to manage your Gorox cluster optionally.

For more details about Gorox and Myrox, please see: https://gorox.io/ .


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
  * RISCV64, also known as RV64, RISC-V 64
  * LOONG64, also known as LA64, LoongArch 64

Other platforms are currently not tested and probably don't work.


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


Documentation
=============

View Gorox documentation online:

  * English version: https://gorox.io/docs
  * Chinese version: https://www.gorox.io/docs

Or view locally (ensure your local server under hemi/gosites is started):

  * English version: http://gorox.net:5080/docs
  * Chinese version: http://www.gorox.net:5080/docs


Layout
======

By default, Gorox uses these dirs:

  * apps/ - Place your Web applications,
  * cmds/ - Place your auxiliary commands,
  * conf/ - Place configs for Gorox and your commands,
  * data/ - Place static shared files of your project,
  * docs/ - Place docs of your project,
  * exts/ - Place extended components written for your project,
  * hemi/ - The Hemi Engine,
  * idls/ - Place your Interface Description Language files,
  * jobs/ - Place your Cronjobs written in Go and scheduled by Gorox,
  * libs/ - Place libs written by you for your project,
  * misc/ - Place misc resource of your project,
  * srvs/ - Place your General Servers, like Chat server, SMS server, and so on,
  * svcs/ - Place your RPC services,
  * test/ - Place tests for your project.

After Gorox is started, 3 extra dirs are created:

  * logs/ - Place running logs,
  * temp/ - Place temp files which are safe to remove after Gorox is shutdown,
  * vars/ - Place dynamic data files used by Gorox.


Architecture
============

Deployment
----------

A typical deployment architecture using Gorox might looks like this:

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

  * edgeProxy1: The Edge Proxy, also works as an API Gateway or WAF,
  * app1: A Web application implemented directly on Gorox,
  * srv1: A TCP server implemented directly on Gorox,
  * svc1: A public RPC service implemented directly on Gorox,
  * sidecar1: A sidecar for svc1,
  * app2: A gateway passing requests to PHP-FPM / Tomcat server,
  * svc2: A private RPC service implemented directly on Gorox,
  * sidecar2: A sidecar for svc2,
  * job1: A background application in Gorox doing something periodically.

The whole "gorox cluster" can optionally be managed by a Myrox instance. In this
configuration, all Gorox instances in "gorox cluster" are connected to the Myrox
instance and under its control.

Process
-------

A Gorox instance has two processes: a leader process, and a worker process:

```
                  +----------------+         +----------------+ business traffic
         admConn  |                | cmdConn |                |<===============>
operator--------->| leader process |<------->| worker process |<===============>
                  |                |         |                |<===============>
                  +----------------+         +----------------+
```

Leader process manages the worker process, which do the real and heavy work.

A Gorox instance can be controlled by operators through the admin interface of
leader process. Alternately, it can connects to a Myrox instance and delegates
its administration to Myrox. In this way, the admin interface in leader process
is disabled.

Dependencies
------------

Gorox has following dependencies:

```
  +-------------------------------------------------------------+
  |                            <gorox>                          |
  +-------------+-------------------------------+----------+----+
                |                               |          |
                v                               v          v
  +------+   +---------------------------+   +------+ +---------+
  | libs |<--+ apps & jobs & srvs & svcs +-->| exts | |<procman>|
  +------+   +--+---------------------+--+   +--+---+ +----+----+
                |                     |         |          |
                v                     v         v          v
  +-----------------------+   +---------------------------------+
  |       <contrib>       |<--+              <hemi>             |
  +-----------+-----------+   +-----------------+---------------+
              |                                 |
              v                                 v
  +-------------------------------------------------------------+
  |                         <internal>                          |
  +-------------------------------------------------------------+
```


Community
=========

Currently Github Discussions is used for discussing:

    https://github.com/hexinfra/gorox/discussions


Contact
=======

Gorox is originally written by Zhang Jingcheng <diogin@gmail.com>. You can also
contact him through Twitter: @diogin.

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

