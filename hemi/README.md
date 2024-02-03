Hemi
====

Hemi is the engine of Gorox.


Layout
======

Hemi uses these directories:

  * common/   - Place general purpose libraries,
  * contrib/  - Place community contributed components,
  * develop/  - A prototype application used to develop Hemi,
  * internal/ - The core of Hemi,
  * procman/  - A process manager for applications using Hemi.

The "export.go" file collects the exported elements of Hemi.


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
   |     | s | [quic] |    server    |     server      |     |
   |     | e | [tcps] | <gate><conn> |  <gate><conn>   |     |
   |     | r | [udps] +--------------+-----------------+     |
   |     | v | router |              |webapp(*) handlet|     |
   |     | e | dealet |              | socklet reviser |     |
   |     | r |  case  |  service(*)  |     rule        |     |
   |     |(*)|        |          +---+--+       +------+     |
   |     |   |        |          |stater|       |cacher|     |
   |     +---+--------+----------+------+-------+------+     |
   |     |            backend <node> <conn>            |     |
   |     +---+---+---+---+---+---+---+---+-------------+     |
   |     | o | u | t | g | a | t | e | s |    addon    |     |
   |     +---+---+---+---+---+---+---+---+-------------+     |
   |     |   clock   |     fcache    |      namer      |     |
prepare  +-----------+---------------+-----------------+     v

```

Dependencies
------------

A program using Hemi typically has following dependencies:

```
  +-------------------------------------------------------------+
  |                           <program>                         |
  +-------------+-------------------------------+----------+----+
                |                               |          |
                v                               v          v
  +------+   +---------------------------+   +------+ +---------+
  | libs |<--+ apps & jobs & srvs & svcs +-->| exts | |<procman>|
  +------+   +--+---------------------+--+   +--+---+ +----+----+
                |                     |         |          |
                v                     v         v          v
  +-----------------------+   +---------------------------------+
  |       <contrib>       |<--+              <hemi>             |
  +-----------+-----------+   +-----------------+---------------+
              |                                 |
              v                                 v
  +-------------------------------------------------------------+
  |                         <internal>                          |
  +-------------------------------------------------------------+
```
