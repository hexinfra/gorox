Procmgr
=======

Procmgr implements a leader-worker process model and its control client.

A control client process connects to a leader process, tell or call its cmdui
APIs. The leader process starts and monitors its worker process. It uses a TCP
connection to communicate with its worker process. A leader process has only
one worker process.

If the leader process was connected to Myrox, then its cmdui interface is not
opened. In this case, it is managed by Myrox.


Layout
======

Procmgr uses these directories:

  * client/  - As client process,
  * common/  - Shared elements between client, leader, and worker,
  * leader/  - As leader process,
  * worker/  - As worker process.

