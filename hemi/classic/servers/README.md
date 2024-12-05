listen socket options:

  Linux:   SO_REUSEPORT
  FreeBSD: SO_REUSEPORT_LB
  macOS:   SO_REUSEPORT (but the last socket wins?)
  Windows: SO_REUSEADDR (but the first socket wins?)
