package main

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi"
	"os"
	"path/filepath"
	"runtime"
)

var config = `
stage {
    apps {
        app "filex" {
            hostnames = ("*")
            webRoot   = @baseDir
            rules {
                rule %path == "/favicon.ico" {
                    faviconHandler {}
                }
                rule {
                    static {
                        autoIndex = true
                    }
                }
            }
        }
    }
    appServers = [
        "filex": ("main"),
    ]
    svcs {}
    svcServers = []
    servers {
        httpxServer "main" {
            address = ":3080"
        }
    }
}
`

func main() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	baseDir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		baseDir = filepath.ToSlash(baseDir)
	}

	SetBaseDir(baseDir)
	SetDataDir(baseDir + "/data")
	SetLogsDir(baseDir + "/logs")
	SetTempDir(baseDir + "/temp")

	stage, err := ApplyText(config)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	stage.StartAlone()

	select {}
}
