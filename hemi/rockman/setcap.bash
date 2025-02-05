#!/usr/bin/env bash

# Enable rockman to listen on ports < 1024 without root privilege.

# This script is Linux only. Use sudo to execute this shell script.
# You must mount your filesystem with suid option to enable setcap support.

if [ -f rockman ]; then
	setcap cap_net_bind_service=+ep rockman
fi
