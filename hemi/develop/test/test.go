package test

import (
	"os"

	"github.com/hexinfra/gorox/hemi/develop/test/apps/diogin"
	"github.com/hexinfra/gorox/hemi/develop/test/apps/fengve"
	"github.com/hexinfra/gorox/hemi/develop/test/apps/testee"
)

func Main() {
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
