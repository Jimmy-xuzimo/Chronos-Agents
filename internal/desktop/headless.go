//go:build headless || (darwin && !cgo)

package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Run(ctx context.Context, origin string, paused func() bool, toggle func(), stop context.CancelFunc, headless bool) {
	<-ctx.Done()
}
func Open(origin string) {
	if runtime.GOOS == "darwin" {
		exe, _ := os.Executable()
		_ = exec.Command(filepath.Join(filepath.Dir(exe), "chronos-control")).Start()
	} else {
		_ = exec.Command("xdg-open", origin).Start()
	}
}
