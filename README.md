Welcome
=======

  Welcome to Gorox!

  Gorox is a high-performance HTTP server, application server, microservice
  server, and proxy server. It can be used as:

    * HTTP Server (HTTP 1/2/3, WebSocket, FCGI, uwsgi)
    * Application Server for Go (Applications, Frameworks)
    * Microservice Server for Go (HRPC Services, gRPC Services)
    * HTTP Proxy Server (Forward & Reverse, Caching, Tunneling)
    * API Gateway (HTTP APIs & gRPC APIs, WAF)
    * Service Mesh (Data Plane & Control Plane)
    * ... and more through its highly extensible compoments design!

  There is also a Goops which can optionally manage your Gorox cluster.

  For more details about Gorox, please see: https://gorox.io/


Motivation
==========

  To be written.


Platforms
=========

  Gorox has been tested to be working on these platforms:

    * Linux kernel >= 3.9, AMD64 & ARM64
    * FreeBSD >= 12.0, AMD64
    * Apple macOS >= Catalina, AMD64 & ARM64
    * Microsoft Windows >= 10, AMD64

  Other platforms are currently not tested and probably don't work.


Performance
===========

  Gorox is fast. You can use wrk to perform a simple benchmark:

    shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/benchmark
    shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/benchmark.html

  Change the parameters and/or target URL to match your need.

  Generally, the result is about 80% of nginx and slightly better than fasthttp.


Quickstart
==========

  See INSTALL.md file.


Documentation
=============

  View documentation online:

    English version: https://gorox.io/docs
    Chinese version: https://www.gorox.io/docs

  Or view locally (ensure your local gorox server is started):

    English version: http://gorox.net:3080/docs
    Chinese version: http://www.gorox.net:3080/docs


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

