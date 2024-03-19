Hemi
====

Hemi is the engine of Gorox.


Layout
======

Hemi contains these directories:

  * addons/   - Place optional addons,
  * common/   - Place general purpose libraries,
  * develop/  - A prototype application used to develop Hemi,
  * procmgr/  - A process manager for programs using Hemi,
  * program/  - The recommended directory hierarchy for programs using Hemi,
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
   |     | e | [tcps] | <gate><conn> |  <gate><conn>   |     |
   |     | r | [udps] +--------------+-----------------+     |
   |     | v | router |              |webapp(*) handlet|     |
   |     | e |        |  service(*)  | socklet reviser |     |
   |     | r | dealet |              |     rule        |     |
   |     |(*)|  case  |          +---+--+       +------+     |
   |     |   |        |          |stater|       |cacher|     |
   |     +---+--------+----------+------+-------+------+     |
   |     |                  backend                    |     |
   |     |                   node                      |     |
   |     |                  <conn>                     |     |
   |     +-----------+---------------+-----------------+     |
   |     |   clock   |     fcache    |      namer      |     |
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
  |       <addons>        |-->+              <hemi>             |
  +-----------------------+   +---------------------------------+
```


Roadmap
=======

  * [TODO] net system design and implementation.
  * [TODO] rpc system design and implementation.
  * [TODO] web rewrite handlet design and implementation.
  * [TODO] web reviser design and implementation.
  * [TODO] websocket implementation.
  * [TODO] http/2 implementation.
  * [TODO] quic and http/3 implementation.
  * [TODO] cacher implementation.
  * [TODO] tcpoh (tcp over http) server implementation.
  * [TODO] udpoh (udp over http) server implementation.
  * [TODO] ipoh (ip over http) server implementation.
  * [TODO] web application framework implementation.
  * [TODO] documentation.
  * [TODO] official websites.
  * [TODO] logger implementation.
  * [TODO] goben design and implementation.
  * [TODO] more unit tests.
  * [TODO] black/white box tests.
  * [TODO] online parsing algorithm for forms.
  * [TODO] fetch config through url.
  * [TODO] ktls support?
  * [TODO] ...

