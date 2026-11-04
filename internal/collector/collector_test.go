package collector

import (
	"context"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"testing"
	"time"
)

func TestRealSampleHasHonestCapabilitiesAndNoDefaultPrivateFields(t *testing.T) {
	c := New(Options{Scope: "host"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	metrics := c.Sample(ctx)
	e := protocol.Envelope{SchemaVersion: 1, DeviceID: "test_device", AgentVersion: "test", SessionID: "test_session", Sequence: 1, SampledAt: time.Now().UTC(), ExpectedIntervalSeconds: 10, Platform: Platform(false), Metrics: metrics}
	if err := e.Validate(); err != nil {
		t.Fatalf("collector violated own protocol: %v", err)
	}
	if metrics["cpu"].Status != "warming_up" && metrics["cpu"].Status != "error" {
		t.Fatal("CPU rate requires baseline")
	}
	if metrics["processes"].Status != "disabled" || metrics["processes"].Data != nil {
		t.Fatal("process consent default")
	}
	if e.Platform.Hostname != "" {
		t.Fatal("hostname disclosed")
	}
	for _, m := range metrics {
		if _, ok := m.Data["hostname"]; ok {
			t.Fatal("system hostname disclosed")
		}
	}
	if n := metrics["network"]; n.Status == "supported" && n.Data["rx_bytes_per_second"] != nil {
		t.Fatal("first network sample has fabricated rate")
	}
}
