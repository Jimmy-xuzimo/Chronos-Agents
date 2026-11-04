package collector

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

var power = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

func Battery() (map[string]any, bool) {
	var s struct {
		AC, Flags, Percent, Reserved byte
		Life, Full                   uint32
	}
	ok, _, _ := power.Call(uintptr(unsafe.Pointer(&s)))
	if ok == 0 || s.Flags == 128 || s.Percent == 255 {
		return nil, false
	}
	return map[string]any{"percent": int(s.Percent), "on_battery": s.AC == 0}, true
}
