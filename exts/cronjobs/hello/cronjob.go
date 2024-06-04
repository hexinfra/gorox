// This is a hello cronjob showing how to use Gorox to host a cronjob.

package hello

import (
	"fmt"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterCronjob("helloCronjob", func(name string, stage *Stage) Cronjob {
		j := new(helloCronjob)
		j.onCreate(name, stage)
		return j
	})
}

// helloCronjob
type helloCronjob struct {
	// Parent
	Cronjob_
	// Assocs
	stage *Stage
	// States
}

func (j *helloCronjob) onCreate(name string, stage *Stage) {
	j.MakeComp(name)
	j.stage = stage
}
func (j *helloCronjob) OnShutdown() {
	close(j.ShutChan) // notifies Schedule()
}

func (j *helloCronjob) OnConfigure() {
}
func (j *helloCronjob) OnPrepare() {
}

func (j *helloCronjob) Schedule() { // runner
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-j.ShutChan:
			break loop
		case now := <-ticker.C:
			fmt.Printf("hello, gorox! time=%s\n", now.String())
		}
	}
	if DebugLevel() >= 2 {
		Printf("helloCronjob=%s done\n", j.Name())
	}
	j.stage.DecSub()
}
