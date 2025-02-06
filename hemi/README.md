Hemi
====

Hemi is the engine of Gorox. It's a plain Go module that only depends on Go's
standard library. It can be used as a module by your programs.


How to use
==========

To use the Hemi Engine module, add a "require" line to your "go.mod" file:

    require github.com/hexinfra/gorox vx.y.z

Where x.y.z is the version of Hemi. Then import it in your "main.go":

    import "github.com/hexinfra/gorox/hemi"

If you would like to use the standard components too, import them with:

    import _ "github.com/hexinfra/gorox/hemi/builtin"

For examples showing how to use the Hemi Engine in your programs, please see our
examples in the "example/" sub directory.


Layout
======

In addition to the *.go files in current directory that implement the core of
the Hemi Engine, we also have these sub directories that supplement Hemi:

  * builtin/  - Place standard Hemi components,
  * contrib/  - Place community contributed Hemi components,
  * library/  - Place general purpose libraries,
  * procman/  - A process manager for programs using Hemi.

The following sub directories are some programs that use the Hemi Engine:

  * goroxio/  - A program that implements the official website of Gorox,
  * hemicar/  - A prototype program that is used to develop Hemi itself,
  * rockman/  - A program that can optionally be used to manage gorox clusters.

And something useful:

  * example/  - Place example programs showing how to use the Hemi Engine,
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
  | libs |<--+        apps & svcs        +-->| exts | |<procman>|
  +------+   +--+---------------------+--+   +--+---+ +----+----+
                |                     |         |          |
                v                     v         v          v
  +-----------------------+   +---------------------------------+
  | <builtin> & <contrib> +-->|              <hemi>             |
  +-----------------------+   +---------------------------------+
```

Processes
---------

A Hemi powered program that is managed by procman normally has two processes
when started - a leader process, and a worker process:

```
                  +----------------+         +----------------+ business traffic
         cmdConn  |                | admConn |                |<===============>
operator<-------->| leader process |<------->| worker process |<===============>
                  |                |         |                |<===============>
                  +----------------+         +----------------+
```

The leader process manages the worker process, which do the real and heavy work.

A program instance can be controlled by operators through the cmdui interface of
the leader process. Using gorox as a controlling agent, operators can connect to
the leader, send commands, then the leader executes the commands. Some commands
are delivered to the worker process through admConn which is a TCP connection
between the leader process and the worker process.

If operators prefer to use the Web UI, there is a webui interface in the leader
process too. Run your program with "help" action and you will see there is a
"-webui" option which controls what ip:port it should use. Start your program
with that option and you'll get a Web UI to control your program instance.

Alternatively, leaders of program instances can choose to connect to a Rockman
instance and delegate their administration to Rockman. In this way, the cmdui
and the webui interface in the leader process are disabled.

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
   |     +--------+--------------+-----------------+---+     |
   |     |        |    rpc[+]    |     web[+]      |   |     |
   |     | [quix] |    server    |     server      | s |     |
   |     | [tcpx] | <gate><conn> |  <gate><conn>   | e |     |
   |     | [udpx] +--------------+-----------------+ r |     |
   |     | router |              | webapp(*)  rule | v |     |
   |     |        |              | handlet reviser | e |     |
   |     |  case  |  service(*)  |     socklet     | r |     |
   |     | dealet |              +------+   +------+(*)|     |
   |     |        |              |hstate|   |hcache|   |     |
   |     +--------+--------------+------+---+------+---+     |
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
* goroxio.
* rperf design and implementation.
* more unit tests.
* black/white box tests.
* online parsing algorithm for forms.
* fetch config through url.
* ktls support?
* ...
