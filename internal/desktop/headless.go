//go:build headless || (darwin && !cgo)

package desktop

import (
	"context"
	"os/exec"
	"runtime"
)

func Run(ctx context.Context, origin string, paused func() bool, toggle func(), stop context.CancelFunc, headless bool) {
	<-ctx.Done()
}
func Open(origin string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("/usr/bin/open", origin).Start()
	} else {
		_ = exec.Command("xdg-open", origin).Start()
	}
}
