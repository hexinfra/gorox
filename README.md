Welcome
=======

Welcome to Gorox!

Gorox is a pragmatic Webapp Server, RPC Framework, and Network Proxy. It can be
used as a:

  * Web Server (HTTP, TLS, WebSocket, static, CGI, FCGI, SCGI)
  * Go Web Application Server (Frameworks, Applications)
  * RPC Framework for Go (HRPC)
  * Web Reverse Proxy (HTTP, TLS, WebSocket, Caching, Load Balancing)
  * Layer 7 Reverse Proxy (Various Protocols, with or without Load Balancing)
  * Layer 4 Reverse Proxy (TCP, TLS, UDP, QUIC, UDS, Load Balancing)
  * Forward Proxy (SOCKS, HTTP Tunnel)
  * ... and more through its highly extensible compoments design!

Gorox is under heavy development, *CURRENTLY ALL FEATURES ARE EXPERIMENTAL!* For
more details about Gorox, please see our project site: https://gorox.io/.


Platforms
=========

Gorox works on these operating systems:

  * Linux kernel >= 3.9
  * Microsoft Windows >= 10
  * Apple macOS >= Catalina
  * FreeBSD >= 12.0

And these 64-bit CPU architectures:

  * AMD64, a.k.a. x86-64
  * ARM64, a.k.a. AArch64
  * RISCV64, a.k.a. RV64
  * Loong64, a.k.a. LoongArch64

Other platforms are currently not tested and probably don't work.


Quickstart
==========

Using Gorox as a Network Proxy
------------------------------

If you would like to use Gorox as a Network Proxy, you can download the official
binary distribution and read the "Start and stop Gorox" section below. But if
you prefer to build it from source, please read on.

Using Gorox as a Webapp Server or RPC Framework
-----------------------------------------------

When using Gorox as a Webapp Server or RPC Framework, you have to build it from
source. Before building, please ensure you have Go >= 1.22 installed:

    shell> go version

Then download the source code tarball, uncompress it, and build it:

    shell> cd gorox-x.y.z
    shell> go build

If build failed, set CGO_ENABLED to 0 and try again:

    shell> go env -w CGO_ENABLED=0
    shell> go build

On succeed, a single "gorox" or "gorox.exe" binary will be generated.

Gorox is a normal program built on the Hemi Engine which resides in "hemi/" sub
directory. Most of the time, you can develop your Web applications and RPC
services directly on Gorox, but if you prefer to use a program name other than
"gorox", or prefer to organize your program layout differently with the
convention of Gorox, or need to use the Hemi Engine as a plain Go module, please
refer to the "How to use" section in "hemi/README.md" file.

Start and stop Gorox
--------------------

After you have downloaded and uncompressed the official binary distribution, or
have successfully built the Gorox binary from source, you can run it as a daemon
(simply remove the "-daemon" option if you don't like to run it as a daemon):

    shell> ./gorox -daemon

Then ensure the leader process and the worker process have both been started:

    shell> ./gorox pids

Now visit http://localhost:3080/ to see if it works correctly. To exit the
server gracefully:

    shell> ./gorox quit

Or exit it immediately:

    shell> ./gorox stop

For more actions and options:

    shell> ./gorox help

If you are running Gorox on Linux and need to listen on ports < 1024 without
root privilege, you can run the linux "setcap" command with "sudo" to grant suid
option to your gorox binary (make sure your filesystem was mounted with "suid"
option for this command to work):

    shell> sudo setcap cap_net_bind_service=+ep ./gorox

To install Gorox, simply move the whole Gorox directory to where you like. You
may also add the directory to your $PATH so you can run "gorox" without "./".

To uninstall, simply remove the whole Gorox directory and remove it from $PATH.

Configuration examples
----------------------

We provide some example configs for Gorox to use, see them under conf/examples.
For example, if you would like to use Gorox as an HTTP reverse proxy, there is a
demo config in conf/examples/http_proxy.conf, you can modify it and start Gorox
like:

    shell> ./gorox -config conf/examples/http_proxy.conf


Why Gorox?
==========

To be written.


Performance
===========

Gorox is fast. You can use your favorite HTTP benchmarking tool (like wrk) to
perform a benchmark against these URLs (ensure your local gorox is running):

  * http://localhost:3080/bench
  * http://localhost:3080/bench.html

If you have two machines connected with a fast network, you can run gorox at one
machine and run wrk at the other.

Generally, the result is about 80% of nginx and slightly faster than fasthttp.


Documentation
=============

View Gorox documentation online:

  * English version: https://gorox.io/docs
  * Chinese version: https://www.gorox.io/docs

Or view locally (ensure your local goroxio program is started):

  * English version: http://localhost:5080/docs
  * Chinese version: http://127.0.0.1:5080/docs


Layout
======

By default, Gorox uses these directories:

  * apps/ - Place your Web applications,
  * bins/ - Place source code of your auxiliary commands,
  * conf/ - Place configs for Gorox and your commands,
  * docs/ - Place docs of your project,
  * exts/ - Place extended components written specifically for your project,
  * hemi/ - The Hemi Engine,
  * libs/ - Place libs written or generated by you for your project,
  * misc/ - Place misc resource of your project,
  * svcs/ - Place your RPC services,
  * test/ - Place tests for your project.

After Gorox is started, an extra directory called "data/" will be created, with
3 sub directories in it:

  * data/log/ - Place running logs,
  * data/tmp/ - Place files which are safe to remove after Gorox is shutdown,
  * data/var/ - Place dynamic data files used by Gorox.


Deployment
==========

A typical deployment architecture using Gorox might looks like this:

```
            mobile  pc iot
               |    |   |
               |    |   |           public internet
               |    |   |
               v    v   v
             +------------+
+------------| edgeProxy1 |--------------+ gorox cluster
|            +--+---+--+--+              | (can be managed by Rockman)
|   http        |   |  |        tcp      |
|      +--------+   |  +--------+        |
|      |            |           |        |
|      v           rpc          v        |
|   +------+        |      +---------+   |
|   | app1 +----+   |   +--+ server1 |   |
|   +------+    |   |   |  +----+----+   |
|               |   |   |       |        | stateless layer
|               v   v   v       v        |
|  +------+   +-----------+  +--------+  |   +------------------+
|  | svc1 |<->| svcProxy1 |  | proxy2 |--+-->| php-fpm / tomcat |
|  +------+   +-----+-----+  +--------+  |   +------------------+
|                   |                    |
|                   v                    |
|  +------+   +-----------+  +--------+  |
|  | svc2 |<--+ svcProxy2 |  |cronjob1|  |
|  +------+   +-----------+  +--------+  |
|                                        |
+-----------+------+---------+-----------+
            |      |         |
            v      v         v
+----------------------------------------+
|     +-------+  +-----+  +--------+     |
| ... |  db1  |  | mq1 |  | cache1 | ... | stateful layer
|     +-------+  +-----+  +--------+     |
+----------------------------------------+

```

In this typical architecture, with various configurations, Gorox can play *ALL*
of the roles in "gorox cluster":

  * edgeProxy1: The Edge Proxy, also works as an API Gateway or WAF,
  * app1      : A Web application implemented directly on Gorox,
  * server1   : A TCP server implemented directly on Gorox,
  * svc1      : A public RPC service implemented directly on Gorox,
  * svcProxy1 : A sidecar proxy for svc1,
  * proxy2    : A gateway proxy passing requests to PHP-FPM or Tomcat server,
  * svc2      : A private RPC service implemented directly on Gorox,
  * svcProxy2 : A sidecar proxy for svc2,
  * cronjob1  : A background application in Gorox doing something periodically.

The whole Gorox cluster can alternatively be managed by a Rockman instance,
which behaves like the control plane in Service Mesh. In this configuration, all
Gorox instances in the cluster connect to Rockman and are under its control.


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

Gorox is licensed under a 2-clause BSD License. See LICENSE file.


Contributing
============

Gorox is hosted at Github:

    https://github.com/hexinfra/gorox

Fork this repository and contribute your patch through Github Pull Requests.

By contributing to Gorox, you MUST agree to release your code under the BSD
License that you can find in the LICENSE file.


Security
========

Please report any security issue or crash report to:

  Zhang Jingcheng <diogin@gmail.com>

Your issue will be triaged and coped with appropriately.

Thank you in advance for helping to keep Gorox secure!
