package collector

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	netstat "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type Options struct {
	Processes bool
	Addresses bool
	Hostname  bool
	Scope     string
	Volumes   []string
}
type processPoint struct {
	seconds float64
	at      time.Time
	created int64
}
type Collector struct {
	previousProcesses map[int32]processPoint
	previousDisk      map[string]disk.IOCountersStat
	diskAt            time.Time
	previousCPU       []cpu.TimesStat
	previousNet       map[string]netstat.IOCountersStat
	netAt             time.Time
	processAt         time.Time
	processData       any
	opts              Options
}

func New(opts Options) *Collector { return &Collector{opts: opts} }
func (c *Collector) Configure(opts Options) {
	c.opts = opts
	if !opts.Processes {
		c.previousProcesses = nil
		c.processData = nil
		c.processAt = time.Time{}
	}
}
func block(status, scope string, at time.Time, data any) protocol.Metric {
	var value map[string]any
	if data != nil {
		raw, _ := json.Marshal(data)
		_ = json.Unmarshal(raw, &value)
	}
	return protocol.Metric{Status: status, Scope: scope, CollectedAt: at.UTC(), Data: value}
}
func failed(scope string, at time.Time, e error) protocol.Metric {
	if errors.Is(e, os.ErrPermission) {
		return block("permission_denied", scope, at, nil)
	}
	return block("error", scope, at, nil)
}
func total(t cpu.TimesStat) (float64, float64) {
	return t.User + t.System + t.Nice + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal, t.Idle + t.Iowait
}
func (c *Collector) Sample(ctx context.Context) map[string]protocol.Metric {
	at := time.Now().UTC()
	scope := c.opts.Scope
	if scope == "" {
		scope = "host"
	}
	out := map[string]protocol.Metric{}
	times, e := cpu.TimesWithContext(ctx, true)
	if e != nil {
		out["cpu"] = failed(scope, at, e)
	} else if len(times) != len(c.previousCPU) || len(times) == 0 {
		out["cpu"] = block("warming_up", scope, at, nil)
	} else {
		cores := []float64{}
		dt, di := 0.0, 0.0
		for i, t := range times {
			all, idle := total(t)
			pa, pi := total(c.previousCPU[i])
			a, b := all-pa, idle-pi
			if a <= 0 {
				if len(cores) < 256 {
					cores = append(cores, 0)
				}
				continue
			}
			dt += a
			di += b
			if len(cores) < 256 {
				cores = append(cores, max(0, min(100, (a-b)/a*100)))
			}
		}
		if dt <= 0 {
			out["cpu"] = block("warming_up", scope, at, nil)
		} else {
			out["cpu"] = block("supported", scope, at, map[string]any{"percent": max(0, min(100, (dt-di)/dt*100)), "per_core_percent": cores, "cores": len(times)})
		}
	}
	c.previousCPU = times
	memory, e := mem.VirtualMemoryWithContext(ctx)
	if e != nil {
		out["memory"] = failed(scope, at, e)
	} else {
		used := memory.Total - min(memory.Total, memory.Available)
		pct := 0.0
		if memory.Total > 0 {
			pct = float64(used) / float64(memory.Total) * 100
		}
		out["memory"] = block("supported", scope, at, map[string]any{"total_bytes": memory.Total, "available_bytes": memory.Available, "used_bytes": used, "percent": pct, "basis": "total_minus_available"})
	}
	swap, e := mem.SwapMemoryWithContext(ctx)
	if e != nil {
		out["swap"] = failed(scope, at, e)
	} else {
		out["swap"] = block("supported", scope, at, map[string]any{"total_bytes": swap.Total, "used_bytes": swap.Used, "percent": swap.UsedPercent})
	}
	volumes := []map[string]any{}
	mounts := c.opts.Volumes
	if len(mounts) == 0 {
		parts, err := disk.PartitionsWithContext(ctx, false)
		if err == nil {
			for _, p := range parts {
				if len(mounts) >= 32 {
					break
				}
				if !slices.Contains(mounts, p.Mountpoint) {
					mounts = append(mounts, p.Mountpoint)
				}
			}
		}
	}
	for _, mount := range mounts {
		if len(volumes) >= 32 {
			break
		}
		d, err := disk.UsageWithContext(ctx, mount)
		if err == nil {
			label := filepath.Base(mount)
			if label == "." || label == string(filepath.Separator) {
				label = "root"
			}
			volumes = append(volumes, map[string]any{"name": label, "total_bytes": d.Total, "used_bytes": d.Used, "available_bytes": d.Free, "percent": d.UsedPercent})
		}
	}
	storageScope := scope
	if os.Getenv("HOST_PROC") != "" {
		storageScope = "accessible_storage"
	}
	if len(volumes) == 0 {
		out["storage"] = block("unsupported", storageScope, at, nil)
	} else {
		out["storage"] = block("supported", storageScope, at, map[string]any{"volumes": volumes})
	}
	if storage, ok := out["storage"]; ok && storage.Status == "supported" {
		storage.Data["io"] = block("supported", scope, at, c.diskIO(ctx, at, scope)).Data
		out["storage"] = storage
	}
	counters, e := network(ctx)
	if e != nil {
		out["network"] = failed(scope, at, e)
	} else {
		interfaces := []map[string]any{}
		current := map[string]netstat.IOCountersStat{}
		rx, tx := 0.0, 0.0
		validRates := 0
		hasRate := !c.netAt.IsZero() && at.After(c.netAt)
		elapsed := at.Sub(c.netAt).Seconds()
		for _, nic := range counters {
			if len(interfaces) >= 64 {
				break
			}
			row := map[string]any{"name": nic.Name, "rx_bytes": nic.BytesRecv, "tx_bytes": nic.BytesSent, "rx_bytes_per_second": nil, "tx_bytes_per_second": nil}
			if old, ok := c.previousNet[nic.Name]; ok && hasRate && nic.BytesRecv >= old.BytesRecv && nic.BytesSent >= old.BytesSent {
				r, t := float64(nic.BytesRecv-old.BytesRecv)/elapsed, float64(nic.BytesSent-old.BytesSent)/elapsed
				row["rx_bytes_per_second"], row["tx_bytes_per_second"] = r, t
				validRates++
				rx += r
				tx += t
			}
			if c.opts.Addresses && os.Getenv("HOST_PROC") == "" {
				row["addresses"] = interfaceAddresses(ctx, nic.Name)
			}
			interfaces = append(interfaces, row)
			current[nic.Name] = nic
		}
		data := map[string]any{"interfaces": interfaces, "rx_bytes_per_second": nil, "tx_bytes_per_second": nil, "addresses_status": "disabled"}
		if c.opts.Addresses {
			data["addresses_status"] = "supported"
			if os.Getenv("HOST_PROC") != "" {
				data["addresses_status"] = "unsupported"
			}
		}
		if hasRate && validRates > 0 {
			data["rx_bytes_per_second"], data["tx_bytes_per_second"] = rx, tx
		}
		out["network"] = block("supported", scope, at, data)
		c.previousNet = current
		c.netAt = at
	}
	info, e := host.InfoWithContext(ctx)
	if e != nil {
		out["system"] = failed(scope, at, e)
	} else {
		data := map[string]any{"uptime_seconds": info.Uptime, "kernel_version": info.KernelVersion, "os_name": info.Platform, "architecture": runtime.GOARCH}
		if os.Getenv("HOST_PROC") != "" {
			data["os_name"] = "Linux host (distribution unavailable)"
		}
		if c.opts.Hostname && os.Getenv("HOST_PROC") == "" {
			data["hostname"] = info.Hostname
		}
		out["system"] = block("supported", scope, at, data)
	}
	if !c.opts.Processes {
		out["processes"] = block("disabled", scope, at, nil)
	} else {
		if c.processAt.IsZero() || at.Sub(c.processAt) >= time.Minute {
			items, err := process.ProcessesWithContext(ctx)
			if err != nil {
				out["processes"] = failed(scope, at, err)
			} else {
				rows := []map[string]any{}
				nextProcesses := map[int32]processPoint{}
				for index, p := range items {
					if index >= 512 || ctx.Err() != nil {
						break
					}
					name, err := p.NameWithContext(ctx)
					if err != nil {
						continue
					}
					if runes := []rune(name); len(runes) > 128 {
						name = string(runes[:128])
					}
					m, err := p.MemoryInfoWithContext(ctx)
					if err != nil {
						continue
					}
					times, terr := p.TimesWithContext(ctx)
					created, cerr := p.CreateTimeWithContext(ctx)
					var pct any
					cpuStatus := "warming_up"
					if terr != nil || cerr != nil {
						cpuStatus = "permission_denied"
					} else {
						seconds := times.User + times.System
						if old, ok := c.previousProcesses[p.Pid]; ok && old.created == created && seconds >= old.seconds && at.After(old.at) {
							pct = max(0, min(100, (seconds-old.seconds)/at.Sub(old.at).Seconds()/float64(max(1, len(c.previousCPU)))*100))
							cpuStatus = "supported"
						}
						nextProcesses[p.Pid] = processPoint{seconds, at, created}
					}
					rows = append(rows, map[string]any{"pid": p.Pid, "name": name, "cpu_percent": pct, "cpu_status": cpuStatus, "memory_bytes": m.RSS})
				}
				c.previousProcesses = nextProcesses
				slices.SortFunc(rows, func(a, b map[string]any) int {
					am, bm := a["memory_bytes"].(uint64), b["memory_bytes"].(uint64)
					if am > bm {
						return -1
					}
					if am < bm {
						return 1
					}
					return 0
				})
				if len(rows) > 20 {
					rows = rows[:20]
				}
				c.processAt = at
				c.processData = map[string]any{"items": rows, "cpu_basis": "machine_capacity", "order": "resident_memory"}
			}
		}
		if _, exists := out["processes"]; !exists {
			out["processes"] = block("supported", scope, c.processAt, c.processData)
		}
	}
	battery, ok := Battery()
	if ok {
		out["battery"] = block("supported", scope, at, battery)
	} else {
		out["battery"] = block("unsupported", scope, at, nil)
	}
	out["sensors"] = block("unsupported", scope, at, nil)
	return out
}
func interfaceAddresses(ctx context.Context, name string) []string {
	items, e := netstat.InterfacesWithContext(ctx)
	if e != nil {
		return []string{}
	}
	for _, nic := range items {
		if nic.Name == name {
			out := []string{}
			for _, a := range nic.Addrs {
				if len(out) < 16 {
					out = append(out, a.Addr)
				}
			}
			return out
		}
	}
	return []string{}
}
func Platform(hostname bool) protocol.Platform {
	p := protocol.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if runtime.GOOS == "darwin" {
		p.OS = "macos"
	}
	if hostname && os.Getenv("HOST_PROC") == "" {
		p.Hostname, _ = os.Hostname()
	}
	return p
}
func CSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// Disk counters are per physical/logical block device, never summed together to
// avoid counting partitions and their parent device twice.
func (c *Collector) diskIO(ctx context.Context, at time.Time, scope string) map[string]any {
	counters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return map[string]any{"status": "unsupported", "scope": scope, "items": []any{}}
	}
	items := []map[string]any{}
	elapsed := at.Sub(c.diskAt).Seconds()
	names := make([]string, 0, len(counters))
	for name := range counters {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if len(items) >= 64 {
			break
		}
		d := counters[name]
		row := map[string]any{"name": name, "read_bytes": d.ReadBytes, "write_bytes": d.WriteBytes, "read_bytes_per_second": nil, "write_bytes_per_second": nil}
		if old, ok := c.previousDisk[name]; ok && !c.diskAt.IsZero() && elapsed > 0 && d.ReadBytes >= old.ReadBytes && d.WriteBytes >= old.WriteBytes {
			row["read_bytes_per_second"] = float64(d.ReadBytes-old.ReadBytes) / elapsed
			row["write_bytes_per_second"] = float64(d.WriteBytes-old.WriteBytes) / elapsed
		}
		items = append(items, row)
	}
	c.previousDisk = counters
	c.diskAt = at
	status := "supported"
	if len(items) == 0 {
		status = "unsupported"
	}
	return map[string]any{"status": status, "scope": scope, "items": items}
}
