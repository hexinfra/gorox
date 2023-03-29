Welcome
=======

  Welcome to Gorox!

  Gorox is an advanced HTTP server, application server, RPC server, and proxy
  server. It can be used as:

    * HTTP Server (HTTP 1/2/3, WebSocket, TLS, FastCGI, uwsgi)
    * HTTP Proxy Server (Forward, Reverse, Caching, Tunneling)
    * Application Server for Go (Applications, Frameworks)
    * RPC Server for Go (HRPC Services, gRPC Services)
    * API Gateway (HTTP APIs, HRPC APIs, gRPC APIs, WAF)
    * Service Mesh (Data Plane)
    * ... and more through its highly extensible compoments design!

  There is also a control plane program called Goops which is distributed
  with Gorox and can be used to manage your Gorox cluster optionally.

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

  And these CPU architectures:

    * AMD64, also known as x64, x86-64, Intel 64
    * ARM64, also known as AArch64
    * RISC-V
    * LoongArch

  Other platforms are currently not considered and probably don't work.


Quickstart
==========

  To start using Gorox, you can download the official binary distributions. If
  you need to build from source, please ensure you have Go >= 1.19 installed:

    shell> go version

  Then build Gorox with Go (set CGO_ENABLED=0 if failed):

    shell> go build

  To run Gorox as a daemon:

    shell> ./gorox serve -daemon

  To check if it works, visit: http://localhost:3080. To exit server gracefully:

    shell> ./gorox quit

  To install, move the whole Gorox directory to where you like. To uninstall,
  remove the whole Gorox directory.

  If you are a developer and need to rebuild Gorox frequently, try using gomake:

    shell> go build cmds/gomake.go
    shell> ./gomake -h

  Move gomake to your $PATH if you like.


Performance
===========

  Gorox is fast. You can use wrk to perform a simple benchmark:

    shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/hello
    shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/hello.html

  Change the parameters and/or target URL to match your need.

  Generally, the result is about 80% of nginx and slightly better than fasthttp.


Documentation
=============

  View documentation online:

    English version: https://gorox.io/docs
    Chinese version: https://www.gorox.io/docs

  Or view locally (ensure your local official server is started):

    English version: http://gorox.net:5080/docs
    Chinese version: http://www.gorox.net:5080/docs


Community
=========

  Currently Github Discussions is used for discussing:

    https://github.com/hexinfra/gorox/discussions


Contact
=======

  Gorox is originally written by Zhang Jingcheng <diogin@gmail.com>.
  You can also contact him through Twitter: @diogin.

  The official website of the Gorox project is at:

    English version: https://gorox.io/
    Chinese version: https://www.gorox.io/


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

