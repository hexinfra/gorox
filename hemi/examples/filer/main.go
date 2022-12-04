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
        app "filer" {
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
        "filer": ("main"),
    ]
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
	SetLogsDir(baseDir + "/logs")
	SetTempDir(baseDir + "/temp")
	SetVarsDir(baseDir + "/vars")

	stage, err := ApplyText(config)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	stage.Start(0)

	select {}
}
