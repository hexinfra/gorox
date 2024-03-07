Hemi
====

Hemi is the engine of Gorox.


Layout
======

Hemi uses these directories:

  * addons/   - Place optional addons,
  * common/   - Place general purpose libraries,
  * develop/  - A prototype application used to develop Hemi,
  * procmgr/  - A process manager for applications using Hemi,
  * toolkit/  - Place useful commands.


How to use
==========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples.


Architecture
============

Logical
-------

The logical architecture of a stage in Hemi looks like this:

```
   ^     +---------------------------------------------+  shutdown
   |     |                  cronjob(*)                 |     |
   |     +---+--------+--------------+-----------------+     |
   |     |   |        |    rpc[+]    |     web[+]      |     |
   |     | s | [quix] |    server    |     server      |     |
   |     | e | [tcps] | <gate><conn> |  <gate><conn>   |     |
   |     | r | [udps] +--------------+-----------------+     |
   |     | v | router |              |webapp(*) handlet|     |
   |     | e | dealet |  service(*)  | socklet reviser |     |
   |     | r |  case  |              |     rule        |     |
   |     |(*)|        |          +---+--+       +------+     |
   |     |   |        |          |stater|       |cacher|     |
   |     +---+--------+----------+------+-------+------+     |
   |     |                  backend                    |     |
   |     |                   node        +-------------+     |
   |     |                  <conn>       |   complet   |     |
   |     +-----------+---------------+---+-------------+     |
   |     |   clock   |     fcache    |      namer      |     |
prepare  +-----------+---------------+-----------------+     v

```

Dependencies
------------

A program (like Gorox) using Hemi typically has the following dependencies:

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

