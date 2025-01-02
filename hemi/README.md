Hemi
====

Hemi is the engine of Gorox. It is a Go module that can be used independently.


Layout
======

In addition to the *.go files in current directory that implement the core of
the Hemi Engine, we also use these sub directories to supplement Hemi:

  * classic/  - Place standard Hemi components,
  * contrib/  - Place community contributed Hemi components,
  * hemicar/  - A prototype program that is used to develop and test Hemi,
  * library/  - Place general purpose libraries,
  * procmgr/  - A process manager for programs using Hemi,
  * toolkit/  - Place useful commands,
  * website/  - A program that hosts the Gorox official website.


How to use
==========

For examples showing how to use the Hemi Engine in your programs, please see our
examples at https://github.com/hexinfra/examples.


Architecture
============

Logical
-------

The logical architecture of a stage in Hemi Engine looks like this:

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
   |     |   |        |              |stater|   |cacher|     |
   |     +---+--------+--------------+------+---+------+     |
   |     |                  backend                    |     |
   |     |                   node                      |     |
   |     |                  <conn>                     |     |
   |     +-----------+---------------+-----------------+     |
   |     |   clock   |     fcache    |     resolv      |     |
prepare  +-----------+---------------+-----------------+     v
                              stage

```

Dependencies
------------

A program (like Gorox) using Hemi Engine typically has these dependencies:

```
  +-------------------------------------------------------------+
  |                           <program>                         |
  +-------------+-------------------------------+----------+----+
                |                               |          |
                v                               v          v
  +------+   +---------------------------+   +------+ +---------+
  | libs |<--+        apps & svcs        +-->| exts | |<procmgr>|
  +------+   +--+---------------------+--+   +--+---+ +----+----+
                |                     |         |          |
                v                     v         v          v
  +-----------------------+   +---------------------------------+
  | <classic> & <contrib> |-->+              <hemi>             |
  +-----------------------+   +---------------------------------+
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
* cacher implementation.
* http tunnel proxy (tcp, udp, ip) implementation.
* web application framework implementation.
* documentation.
* official websites.
* logger implementation.
* rperf design and implementation.
* more unit tests.
* black/white box tests.
* online parsing algorithm for forms.
* fetch config through url.
* ktls support?
* ...

