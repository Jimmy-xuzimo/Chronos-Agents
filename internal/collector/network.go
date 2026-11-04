//go:build !linux

package collector

import (
	"context"
	netstat "github.com/shirou/gopsutil/v4/net"
)

func network(ctx context.Context) ([]netstat.IOCountersStat, error) {
	return netstat.IOCountersWithContext(ctx, true)
}
