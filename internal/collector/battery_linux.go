package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func Battery() (map[string]any, bool) {
	base := "/sys/class/power_supply"
	if s := os.Getenv("HOST_SYS"); s != "" {
		base = filepath.Join(s, "class/power_supply")
	}
	items, e := os.ReadDir(base)
	if e != nil {
		return nil, false
	}
	for _, item := range items {
		dir := filepath.Join(base, item.Name())
		typ, _ := os.ReadFile(filepath.Join(dir, "type"))
		if strings.TrimSpace(string(typ)) != "Battery" {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(dir, "capacity"))
		if e != nil {
			continue
		}
		n, e := strconv.Atoi(strings.TrimSpace(string(raw)))
		if e != nil || n < 0 || n > 100 {
			continue
		}
		status, _ := os.ReadFile(filepath.Join(dir, "status"))
		return map[string]any{"percent": n, "on_battery": strings.TrimSpace(string(status)) == "Discharging"}, true
	}
	return nil, false
}
