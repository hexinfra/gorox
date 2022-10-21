Welcome
=======

  Welcome to Gorox!

  Gorox is an HTTP server, application server, microservice server, and proxy
  server. It can be used as:

    * HTTP Server (HTTP 1/2/3, WebSocket, FCGI, uWSGI, AJP)
    * Application Server for Go (Applications & Frameworks)
    * Microservice Server for Go (gRPC Services & HRPC Services)
    * HTTP Proxy Server (Forward & Reverse, Caching, Tunneling)
    * Service Mesh (Data Plane & Control Plane)
    * API Gateway (HTTP APIs & gRPC APIs)
    * Web Application Firewall
    * ... and MORE!

  There is also a Gocmc which can optionally manage your Gorox cluster.

  For more details about Gorox, please see: https://gorox.io/


Platforms
=========

  Gorox has been tested to be working on these platforms:

    * Linux kernel >= 3.9, AMD64 & ARM64
    * FreeBSD >= 12.0, AMD64
    * Apple macOS >= Catalina, AMD64 & ARM64
    * Microsoft Windows >= 10, AMD64

  Other platforms are not tested and probably don't work.


Quick Start
===========

  See INSTALL.md file.


Performance
===========

  Gorox is fast. You can use wrk to perform a simple benchmark:

    shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/benchmark
    shell> wrk -d 8s -c 240 -t 12 http://localhost:3080/benchmark.html

  Change the parameters to match your computer's configuration.


Documentation
=============

  View documentation online:

    English version: https://gorox.io/docs/gorox/
    Chinese version: https://www.gorox.io/docs/gorox/

  Or view locally (ensure your local gorox server is started):

    English version: http://gorox.net:3080/docs/gorox/
    Chinese version: http://www.gorox.net:3080/docs/gorox/


License
=======

  Gorox is licensed under a BSD License. See LICENSE.md file.


Contact
=======

  Gorox is originally written by Jingcheng Zhang <diogin@gmail.com>.
  You can also contact him through Twitter: @diogin.

  The official website of the Gorox project is at:

    English version: https://gorox.io/
    Chinese version: https://www.gorox.io/


Contributing
============

  Gorox is currently hosted at Github:

    https://github.com/hexinfra/gorox

  Fork this repository and contribute your patch through Github Pull Requests.

  By contributing to Gorox, you agree to release your code under the BSD License
  that you can find in the LICENSE.md file.
