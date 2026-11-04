package protocol

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"time"
)

var ID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)
var Names = map[string]bool{"cpu": true, "memory": true, "swap": true, "storage": true, "network": true, "system": true, "processes": true, "battery": true, "sensors": true, "agent": true}

func (e *Envelope) Validate() error {
	if e.SchemaVersion != 1 || !ID.MatchString(e.DeviceID) || !ID.MatchString(e.SessionID) || e.Sequence < 1 || len(e.AgentVersion) > 40 || e.SampledAt.IsZero() || e.ExpectedIntervalSeconds < 10 || e.ExpectedIntervalSeconds > 86400 {
		return errors.New("invalid envelope")
	}
	if len(e.Platform.Hostname) > 128 || len(e.Platform.OS) > 30 || len(e.Platform.Arch) > 30 || e.Platform.OS == "" {
		return errors.New("invalid platform")
	}
	if len(e.Metrics) < 1 || len(e.Metrics) > len(Names) {
		return errors.New("invalid metrics")
	}
	for name, m := range e.Metrics {
		if !Names[name] || m.CollectedAt.IsZero() || m.CollectedAt.After(e.SampledAt.Add(time.Minute)) || m.CollectedAt.Before(e.SampledAt.Add(-24*time.Hour)) {
			return errors.New("invalid metric time")
		}
		switch m.Scope {
		case "host", "container", "app", "accessible_storage":
		default:
			return errors.New("invalid scope")
		}
		switch m.Status {
		case "supported":
			if m.Data == nil {
				return errors.New("missing supported data")
			}
		case "warming_up", "unsupported", "disabled", "permission_denied", "error":
			if m.Data != nil {
				return errors.New("non-supported data must be null")
			}
		default:
			return errors.New("invalid status")
		}
		if err := values(m.Data, 0); err != nil {
			return err
		}
		raw, _ := json.Marshal(m.Data)
		if len(raw) > 96*1024 {
			return errors.New("metric too large")
		}
	}
	return nil
}
func values(v any, depth int) error {
	if depth > 8 {
		return errors.New("nested metrics")
	}
	switch x := v.(type) {
	case map[string]any:
		if len(x) > 64 {
			return errors.New("too many fields")
		}
		for k, v := range x {
			if len(k) > 64 {
				return errors.New("long key")
			}
			if n, ok := v.(float64); ok {
				if k == "temperature_celsius" {
					if math.IsNaN(n) || math.IsInf(n, 0) || n < -273.15 || n > 1000 {
						return errors.New("invalid temperature")
					}
					continue
				}
				if (k == "percent" || k == "cpu_percent") && (n < 0 || n > 100) {
					return errors.New("percentage out of range")
				}
			}
			if k == "per_core_percent" {
				if list, ok := v.([]any); ok {
					for _, item := range list {
						n, ok := item.(float64)
						if !ok || n < 0 || n > 100 {
							return errors.New("invalid core percentage")
						}
					}
				}
			}

			if err := values(v, depth+1); err != nil {
				return err
			}
		}
	case []any:
		if len(x) > 256 {
			return errors.New("too many items")
		}
		for _, v := range x {
			if err := values(v, depth+1); err != nil {
				return err
			}
		}
	case string:
		if len(x) > 256 {
			return errors.New("long value")
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 || x > 9e18 {
			return errors.New("invalid numeric value")
		}
	case nil, bool, int, int64, uint64:
	default:
		return errors.New("non-JSON metric value")
	}
	return nil
}
