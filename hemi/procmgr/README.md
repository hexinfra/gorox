Procmgr implements a leader-worker process model and its control client.

A control client connects to a leader, tell or call its cmdui APIs. The leader
process starts and monitors its worker process. It uses a TCP connection to
communicate with its worker process.

If the leader was connected to Myrox, its cmdui interface is not opened. In this
case, it is managed by Myrox.
