package hub

import (
	"testing"
	"time"
)

func TestMinuteAverageSkipsMissingValues(t *testing.T) {
	f := newFixture(t)
	id, _ := f.pair(t)
	at := time.Now().UTC().Truncate(time.Minute)
	for i, value := range []any{nil, 10.0, nil, 30.0} {
		e := sample(id, int64(i+1))
		e.SampledAt = at.Add(time.Duration(i) * time.Second)
		cpu := e.Metrics["cpu"]
		cpu.CollectedAt = e.SampledAt
		if value == nil {
			cpu.Status = "warming_up"
			cpu.Data = nil
		} else {
			cpu.Data["percent"] = value
		}
		e.Metrics["cpu"] = cpu
		if _, err := f.s.Ingest(e); err != nil {
			t.Fatal(err)
		}
	}
	var mean float64
	var n int
	if err := f.s.DB.QueryRow("SELECT cpu,cpu_n FROM minutes WHERE device_id=?", id).Scan(&mean, &n); err != nil {
		t.Fatal(err)
	}
	if mean != 20 || n != 2 {
		t.Fatalf("missing values distorted mean: mean=%v count=%d", mean, n)
	}
}

func TestOutOfOrderDoesNotReplaceLatest(t *testing.T) {
	f := newFixture(t)
	id, _ := f.pair(t)
	newer := sample(id, 2)
	older := sample(id, 1)
	older.SampledAt = newer.SampledAt.Add(-time.Hour)
	if _, err := f.s.Ingest(newer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Ingest(older); err != nil {
		t.Fatal(err)
	}
	devices, err := f.s.Devices()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devices {
		if d.ID == id && d.Latest.Sequence != 2 {
			t.Fatal("backlog replaced current sample")
		}
	}
}

func TestFleet100DistinctDevices(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 100; i++ {
		id, code, err := f.s.NewPair("Example device", "linux")
		if err != nil {
			t.Fatal(err)
		}
		paired, _, err := f.s.Enroll(code)
		if err != nil || paired != id {
			t.Fatal("enrollment")
		}
		e := sample(id, 1)
		if _, err = f.s.Ingest(e); err != nil {
			t.Fatal(err)
		}
	}
	devices, err := f.s.Devices()
	if err != nil || len(devices) != 100 {
		t.Fatal("fleet query")
	}
	var n int
	if err = f.s.DB.QueryRow("SELECT count(*) FROM samples").Scan(&n); err != nil || n != 100 {
		t.Fatal("fleet histories mixed")
	}
}
