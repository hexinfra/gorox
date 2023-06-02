Hemi
====

Hemi is the engine of Gorox.


Architecture
============

Logical
-------

The logical architecture of a stage in Hemi engine looks like this:

```
   ^     +--------------------------------------------+  shutdown
   |     |                cronjob(*)                  |     |
   |     +---+--------+--------------+----------------+     |
   |     |   |        |     rpc[+]   |     web[+]     |     |
   |     | s | [quic] |     server   |     server     |     |
   |     | e | [tcps] | <gate><conn> |  <gate><conn>  |     |
   |     | r | [udps] +--------------+----------------+     |
   |     | v | mesher |              | app(*) handlet |     |
   |     | e | dealer |              | socklet reviser|     |
   |     | r | editor |   svc(*)     |    rule        |     |
   |     |(*)|  case  |          +---+--+      +------+     |
   |     |   |        |          |stater|      |storer|     |
   |     +---+--------+----------+------+------+------+     |
   |     |           backend <node> <conn>            |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     | o | u | t | g | a | t | e | s |   runner   |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     |   clock   |     fcache    |     resolv     |     |
prepare  +-----------+---------------+----------------+     v

```

Dependencies
------------

A program (i.e. Gorox) using Hemi typically has following dependencies:

```
  +-------------------------------------------------------------+
  |                            <gorox>                          |
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
