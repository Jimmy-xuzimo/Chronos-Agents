package collector

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func Battery() (map[string]any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, "/usr/bin/pmset", "-g", "batt").Output()
	if e != nil {
		return nil, false
	}
	m := regexp.MustCompile(`(\d+)%`).FindSubmatch(out)
	if len(m) != 2 {
		return nil, false
	}
	n, e := strconv.Atoi(string(m[1]))
	if e != nil {
		return nil, false
	}
	return map[string]any{"percent": n, "on_battery": strings.Contains(string(out), "Battery Power")}, true
}
