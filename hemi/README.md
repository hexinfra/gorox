Hemi
====

Hemi is the engine of Gorox. It's a common Go module that only depends on Go's
standard library. It can be used as a module by your programs.


How to use
==========

To use the Hemi Engine, add a "require" line to your "go.mod" file:

    require github.com/hexinfra/gorox vx.y.z

Here, x.y.z is the version of Hemi. Then import it to your "main.go":

    import . "github.com/hexinfra/gorox/hemi"

If you would like to use the standard components too, import them with:

    import _ "github.com/hexinfra/gorox/hemi/builtin"

For examples showing how to use the Hemi Engine in your programs, please see our
examples repository at: https://github.com/hexinfra/examples.


Layout
======

In addition to the *.go files in current directory that implement the core of
the Hemi Engine, we also have these sub directories that supplement Hemi:

  * builtin/  - Place standard Hemi components,
  * contrib/  - Place community contributed Hemi components,
  * library/  - Place general purpose libraries,
  * procmgr/  - A process manager for programs using Hemi.

The following sub directories are programs that use the Hemi Engine:

  * mulecar/  - A prototype program that is used to develop Hemi itself,
  * hemiweb/  - A program that implements the official website of Gorox.

And something useful:

  * toolkit/  - Place auxiliary commands that are sometimes useful.


Architecture
============

Dependencies
------------

A program (like Gorox) using Hemi Engine typically has these dependencies:

```
  +-------------------------------------------------------------+
  |                          <program>                          |
  +-------------+-------------------------------+----------+----+
                |                               |          |
                v                               v          v
  +------+   +---------------------------+   +------+ +---------+
  | libs |<--+        apps & svcs        +-->| exts | |<procmgr>|
  +------+   +--+---------------------+--+   +--+---+ +----+----+
                |                     |         |          |
                v                     v         v          v
  +-----------------------+   +---------------------------------+
  | <builtin> & <contrib> +-->|              <hemi>             |
  +-----------------------+   +---------------------------------+
```

Processes
---------

A program instance managed by procmgr has two processes: a leader process, and a
worker process:

```
                  +----------------+         +----------------+ business traffic
         cmdConn  |                | admConn |                |<===============>
operator<-------->| leader process |<------->| worker process |<===============>
                  |                |         |                |<===============>
                  +----------------+         +----------------+
```

The leader process manages the worker process, which do the real and heavy work.

A program instance can be controlled by operators through the cmdui interface of
the leader process. Operators connect to the leader, send commands, and the
leader executes the commands. Some commands are delivered to the worker process
through admConn which is a TCP connection between the leader process and the
worker process.

Alternatively, program instances can choose to connect to a Myrox instance and
delegate their administration to Myrox. In this way, the cmdui interface in the
leader process is disabled.

Stages
------

A worker process has one stage most of the time. When an old stage is about to
retire and a new stage is about to start, there may be many stages running at
the same time. This happens when you have changed the config and is telling the
worker to reload the config. The logical architecture of a stage in a worker
process might looks like this:

```
   ^     +---------------------------------------------+  shutdown
   |     |                  cronjob(*)                 |     |
   |     +---+--------+--------------+-----------------+     |
   |     |   |        |    rpc[+]    |     web[+]      |     |
   |     | s | [quix] |    server    |     server      |     |
   |     | e | [tcpx] | <gate><conn> |  <gate><conn>   |     |
   |     | r | [udpx] +--------------+-----------------+     |
   |     | v | router |              | webapp(*)  rule |     |
   |     | e |        |              | handlet reviser |     |
   |     | r |  case  |  service(*)  |     socklet     |     |
   |     |(*)| dealet |              +------+   +------+     |
   |     |   |        |              |hstate|   |hcache|     |
   |     +---+--------+--------------+------+---+------+     |
   |     |                  backend                    |     |
   |     |                   node                      |     |
   |     |                  <conn>                     |     |
   |     +-----------+---------------+-----------------+     |
   |     |   clock   |     fcache    |     resolv      |     |
prepare  +-----------+---------------+-----------------+     v
                              stage

```


Hacking
=======

To be written.


TODO
====

* net system design and implementation.
* rpc system design and implementation.
* web rewrite handlet design and implementation.
* web reviser design and implementation.
* webSocket implementation.
* http/2 implementation.
* quic and http/3 implementation.
* hcache implementation.
* hstate implementation.
* socks 5 proxy implementation.
* http tunnel proxy (tcp, udp, ip) implementation.
* web application framework implementation.
* documentation.
* hemiweb.
* logger design and implementation.
* rperf design and implementation.
* more unit tests.
* black/white box tests.
* online parsing algorithm for forms.
* fetch config through url.
* ktls support?
* ...
