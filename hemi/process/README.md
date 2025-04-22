Process
=======

Package process implements a leader-worker process model & its control client.

A control client process connects to a leader process, tells or calls its cmdui
APIs. The leader process starts and monitors its worker process. It uses a TCP
connection to communicate with its worker process. A leader process has one
worker process only.

If the webui interface of the leader process is open, you can access it through
a web browser too.

If the leader process is connected to a Rockman, then its cmdui interface and
webui interface will not be opened. In this case, it is managed by the Rockman.


Layout
======

Process uses these directories:

  * client/  - As client process,
  * common/  - Shared elements between client, leader, and worker,
  * leader/  - As leader process,
  * worker/  - As worker process.

