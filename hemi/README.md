Hemi
====

Hemi is the engine of Gorox.


Architecture
============

The logical architecture of a stage in Hemi engine looks like this:

```
   ^     +--------------------------------------------+  shutdown
   |     |                cronjob(*)                  |     |
   |     +---+--------+--------------+----------------+     |
   |     |   |        |     rpc[+]   |     web[+]     |     |
   |     | s | [quic] |     server   |     server     |     |
   |     | e | [tcps] | <gate><conn> |  <gate><conn>  |     |
   |     | r | [udps] +--------------+----------------+     |
   |     | v | router |              | app(*) handlet |     |
   |     | e | dealer |              | socklet reviser|     |
   |     | r | editor |   svc(*)     |    rule        |     |
   |     |(*)|  case  |          +---+--+      +------+     |
   |     |   |        |          |stater|      |cacher|     |
   |     +---+--------+----------+------+------+------+     |
   |     |           backend <node> <conn>            |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     | o | u | t | g | a | t | e | s |   runner   |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     |   clock   |     fcache    |    resolver    |     |
prepare  +-----------+---------------+----------------+     v

```

Examples
========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples
