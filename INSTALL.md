From Binary
===========

  Before installing, please ensure your platform is supported, see README.md.

    shell> tar zxf gorox-x.y.z-os-arch.tar.gz
    shell> cd gorox-x.y.z
    shell> ./gorox serve -daemon
    shell> ./gorox quit

  See http://localhost:3080/ while gorox is running, you'll see a welcome page.
  After gorox exits, move the gorox-x.y.z folder to where you like, and the
  installation is done. To uninstall, simply delete the gorox-x.y.z folder.

From Source
===========

  Before installing, please ensure your platform is supported, see README.md.

    shell> go version                     # ensure Go version >= 1.18
    shell> tar zxf gorox-x.y.z.tar.gz     # or unzip gorox-x.y.z.zip for Windows
    shell> cd gorox-x.y.z                 # switch to source directory
    shell> go build cmds/gomake.go        # build gomake which will builds gorox
    shell> ./gomake all                   # move gomake to $PATH if you like
    shell> ./gorox serve -daemon          # start gorox server in daemon mode
    shell> ./gorox quit                   # tell gorox server to exit gracefully

  See http://localhost:3080/ while gorox is running, you'll see a welcome page.
  After gorox exits, move the gorox-x.y.z folder to where you like, and the
  installation is done. To uninstall, simply delete the gorox-x.y.z folder.
