#!/usr/bin/env bash

# This script is Linux only. Use sudo to execute this shell script.
# You must mount your filesystem with suid option to enable setcap support.

if [ -f goops ]; then
	setcap cap_net_bind_service=+ep goops
fi

if [ -f gorox ]; then
	setcap cap_net_bind_service=+ep gorox
fi
