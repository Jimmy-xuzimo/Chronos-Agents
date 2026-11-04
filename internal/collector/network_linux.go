package collector

import (
	"context"
	netstat "github.com/shirou/gopsutil/v4/net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func network(ctx context.Context) ([]netstat.IOCountersStat, error) {
	if os.Getenv("HOST_PROC") == "" {
		return netstat.IOCountersWithContext(ctx, true)
	}
	sys := os.Getenv("HOST_SYS")
	items, e := os.ReadDir(filepath.Join(sys, "class/net"))
	if e != nil {
		return nil, e
	}
	result := []netstat.IOCountersStat{}
	for _, item := range items {
		base := filepath.Join(sys, "class/net", item.Name(), "statistics")
		rx, e := os.ReadFile(filepath.Join(base, "rx_bytes"))
		if e != nil {
			continue
		}
		tx, e := os.ReadFile(filepath.Join(base, "tx_bytes"))
		if e != nil {
			continue
		}
		r, e := strconv.ParseUint(strings.TrimSpace(string(rx)), 10, 64)
		if e != nil {
			continue
		}
		t, e := strconv.ParseUint(strings.TrimSpace(string(tx)), 10, 64)
		if e != nil {
			continue
		}
		result = append(result, netstat.IOCountersStat{Name: item.Name(), BytesRecv: r, BytesSent: t})
	}
	return result, nil
}
