package protocol

import "testing"

func TestNumericRangesAndExplicitTemperatureException(t *testing.T) {
	for _, data := range []map[string]any{
		{"percent": 101.0}, {"used_bytes": -1.0},
		{"per_core_percent": []any{10.0, 200.0}},
	} {
		if values(data, 0) == nil {
			t.Fatal("invalid numeric range accepted")
		}
	}
	if err := values(map[string]any{"temperature_celsius": -10.0, "rx_bytes_per_second": nil}, 0); err != nil {
		t.Fatal(err)
	}
}
