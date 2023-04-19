Hemi
====

Hemi is the engine of Gorox.


Dependencies
============

Programs (for example, Gorox) using Hemi Engine have following dependencies:

```
  +-------------------------------------------------------------+
  |                            <gorox>                          |
  +-------------+-------------------------------+----------+----+
                |                               |          |
                v                               v          v
  +------+   +---------------------------+   +------+ +---------+
  | libs |<--+ apps & jobs & srvs & svcs +-->| exts | |<process>|
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


Architecture
============

The logical architecture of Hemi Engine looks like this:

```
+--------------------------------------------+     ^    shutdown
|                cronjob(*)                  |     |       |
+--------+---+--------------+----------------+     |       |
|        |   |      rpc     |      web       |     |       |
|        | s |     server   |     server     |     |       |
| mesher | e | [gate][conn] |  [gate][conn]  |     |       |
| filter | r +--------------+----------------+     |       |
| editor | v |              | app(*) reviser |     |       |
|  case  | e |   svc(*)     | socklet handlet|     |       |
|        | r |          +---+--+ rule +------+     |       |
|        |(*)|          |stater|      |cacher|     |       |
+--------+---+----------+------+------+------+     |       |
|           [node] [conn] backend            |     |       |
+---+---+---+---+---+---+---+---+------------+     |       |
| o | u | t | g | a | t | e | s |   uniture  |     |       |
+---+---+---+---+---+---+---+---+------------+     |       |
|   clock   |     fcache    |    resolver    |     |       |
+-----------+---------------+----------------+  prepare    v

```

Examples
========

For examples showing how to use Hemi, see: https://github.com/hexinfra/examples
