//go:build !headless && (windows || linux || (darwin && cgo))

package desktop

import (
	"context"
	"embed"
	"fyne.io/systray"
	"os"
	"os/exec"
	"runtime"
)

//go:embed icon.ico icon.png
var icons embed.FS

func Run(ctx context.Context, origin string, paused func() bool, toggle func(), stop context.CancelFunc, headless bool) {
	if headless || (runtime.GOOS == "linux" && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "") {
		<-ctx.Done()
		return
	}
	ready := func() {
		name := "icon.png"
		if runtime.GOOS == "windows" {
			name = "icon.ico"
		}
		icon, _ := icons.ReadFile(name)
		systray.SetIcon(icon)
		systray.SetTooltip("Chronos Agent")
		open := systray.AddMenuItem("打开管理界面", "Open local management")
		pause := systray.AddMenuItemCheckbox("暂停采样与上传", "Pause telemetry", paused())
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出采集器", "Exit agent")
		go func() {
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-open.ClickedCh:
					Open(origin)
				case <-pause.ClickedCh:
					toggle()
					if paused() {
						pause.Check()
					} else {
						pause.Uncheck()
					}
				case <-quit.ClickedCh:
					stop()
				}
			}
		}()
	}
	systray.Run(ready, func() { stop() })
}
func Open(origin string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", origin).Start()
	case "darwin":
		_ = exec.Command("/usr/bin/open", origin).Start()
	default:
		_ = exec.Command("xdg-open", origin).Start()
	}
}
