package main

import (
	. "github.com/hexinfra/gorox/hemi"
	"os"
	"path/filepath"
	"runtime"
)

var config = `
stage {
    app "example" {
        .hostnames = ("*")
        .webRoot   = %baseDir
        rule $path == "/favicon.ico" {
            favicon {}
        }
        rule {
            static {
                .autoIndex = true
            }
        }
    }
    httpxServer "main" {
        .forApps = ("example")
        .address = ":3080"
    }
}
`

func main() {
	exePath, err := os.Executable()
	must(err)
	baseDir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		baseDir = filepath.ToSlash(baseDir)
	}

	// Begin
	SetBaseDir(baseDir)
	SetDataDir(baseDir + "/data")
	SetLogsDir(baseDir + "/logs")
	SetTempDir(baseDir + "/temp")

	stage, err := ApplyText(config)
	must(err)
	stage.Start(0)
	// End

	select {}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
