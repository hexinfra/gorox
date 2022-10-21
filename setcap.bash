#!/bin/bash

# This script is Linux only. Use sudo to execute this shell script.
# You must mount your filesystem with suid options to enable setcap support.

setcap cap_net_bind_service=+ep ./gocmc
setcap cap_net_bind_service=+ep ./gorox
