package main

import (
	"os"

	"github.com/hexinfra/gorox/hemi/hemidev/test/apps/diogin"
	"github.com/hexinfra/gorox/hemi/hemidev/test/apps/fengve"
	"github.com/hexinfra/gorox/hemi/hemidev/test/apps/testee"
)

func main() {
	tests := ""
	if len(os.Args) >= 3 {
		tests = os.Args[2]
	}
	switch tests {
	case "fengve":
		fengve.Main()
	case "diogin":
		diogin.Main()
	default:
		testee.Main()
	}
}
