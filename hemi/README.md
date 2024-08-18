Hemi
====

Hemi is the engine of Gorox.


Layout
======

Hemi contains these directories:

  * contrib/  - Place optional components,
  * hemidev/  - A prototype application used to develop Hemi,
  * library/  - Place general purpose libraries,
  * procmgr/  - A process manager for programs using Hemi,
  * toolkit/  - Place useful commands.


How to use
==========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples.


Architecture
============

Logical
-------

The logical architecture of a stage in Hemi engine looks like this:

```
   ^     +---------------------------------------------+  shutdown
   |     |                  cronjob(*)                 |     |
   |     +---+--------+--------------+-----------------+     |
   |     |   |        |    rpc[+]    |     web[+]      |     |
   |     | s | [quix] |    server    |     server      |     |
   |     | e | [tcpx] | <gate><conn> |  <gate><conn>   |     |
   |     | r | [udpx] +--------------+-----------------+     |
   |     | v | router |              |webapp(*) handlet|     |
   |     | e |        |              | socklet reviser |     |
   |     | r | dealet |  service(*)  |      rule       |     |
   |     |(*)|  case  |              +------+   +------+     |
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

A program (like Gorox) using Hemi engine typically has these dependencies:

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
  |       <contrib>       |-->+              <hemi>             |
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
* tcpoh (tcp over http) server implementation.
* udpoh (udp over http) server implementation.
* ipoh (ip over http) server implementation.
* web application framework implementation.
* documentation.
* official websites.
* logger implementation.
* goben design and implementation.
* more unit tests.
* black/white box tests.
* online parsing algorithm for forms.
* fetch config through url.
* ktls support?
* ...

