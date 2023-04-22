Hemi
====

Hemi is the engine of Gorox.


Architecture
============

The logical architecture of a Stage in Hemi Engine looks like this:

```
   ^     +--------------------------------------------+  shutdown
   |     |                cronjob(*)                  |     |
   |     +--------+---+--------------+----------------+     |
   |     |        |   |      rpc     |      web       |     |
   |     |        | s |     server   |     server     |     |
   |     | mesher | e | [gate][conn] |  [gate][conn]  |     |
   |     | dealer | r +--------------+----------------+     |
   |     | editor | v |              | app(*) reviser |     |
   |     |  case  | e |   svc(*)     | socklet handlet|     |
   |     |        | r |          +---+--+ rule +------+     |
   |     |        |(*)|          |stater|      |cacher|     |
   |     +--------+---+----------+------+------+------+     |
   |     |           [node] [conn] backend            |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     | o | u | t | g | a | t | e | s |   uniture  |     |
   |     +---+---+---+---+---+---+---+---+------------+     |
   |     |   clock   |     fcache    |    resolver    |     |
prepare  +-----------+---------------+----------------+     v

```

Examples
========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples
