Hemi
====

Hemi is the engine of Gorox.


Architecture
============

The logical architecture of a stage in Hemi engine looks like this:

```
   ^     +--------------------------------------------+  shutdown
   |     |                cronjob(*)                  |     |
   |     +--------+---+--------------+----------------+     |
   |     |        |   |     rpc[+]   |     web[+]     |     |
   |     |        | s |     server   |     server     |     |
   |     | router | e | [gate][conn] |  [gate][conn]  |     |
   |     | dealer | r +--------------+----------------+     |
   |     | editor | v |              | app(*) handlet |     |
   |     |  case  | e |              | socklet reviser|     |
   |     |        | r |   svc(*)     |    rule        |     |
   |     |        |(*)|          +---+--+      +------+     |
   |     |        |   |          |stater|      |cacher|     |
   |     +--------+---+----------+------+------+------+     |
   |     |           backend [node] [conn]            |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     | o | u | t | g | a | t | e | s |   runner   |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     |   clock   |     fcache    |    resolver    |     |
prepare  +-----------+---------------+----------------+     v

```

Examples
========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples
