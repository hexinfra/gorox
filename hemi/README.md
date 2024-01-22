Hemi
====

Hemi is the engine of Gorox.


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
   |     | v | mesher |              |webapp(*) handlet|     |
   |     | e | filter |              | socklet reviser |     |
   |     | r |  case  |  service(*)  |     rule        |     |
   |     |(*)|        |          +---+--+       +------+     |
   |     |   |        |          |stater|       |cacher|     |
   |     +---+--------+----------+------+-------+------+     |
   |     |            backend <node> <conn>            |     |
   |     +---+---+---+---+---+---+---+---+-------------+     |
   |     | o | u | t | g | a | t | e | s |    addon    |     |
   |     +---+---+---+---+---+---+---+---+-------------+     |
   |     |   clock   |     fcache    |     resolv      |     |
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


Examples
========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples
