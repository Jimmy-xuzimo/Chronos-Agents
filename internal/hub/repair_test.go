package hub

import (
	"testing"
	"time"
)

func TestRePairRevokedDeviceKeepsIdentity(t *testing.T) {
	f := newFixture(t)
	id, oldToken := f.pair(t)
	if err := f.s.Revoke(id); err != nil {
		t.Fatal(err)
	}
	code, err := f.s.RePair(id)
	if err != nil {
		t.Fatal(err)
	}
	nextID, nextToken, err := f.s.Enroll(code)
	if err != nil {
		t.Fatal(err)
	}
	if nextID != id {
		t.Fatal("re-pair changed device identity")
	}
	if _, _, ok := f.s.DeviceToken(oldToken); ok {
		t.Fatal("old credential accepted")
	}
	if _, _, ok := f.s.DeviceToken(nextToken); !ok {
		t.Fatal("new credential rejected")
	}
	if _, _, err := f.s.Enroll(code); err == nil {
		t.Fatal("pair code reused")
	}
}

func TestCleanupUsesIndependentRetentionWindows(t *testing.T) {
	f := newFixture(t)
	id, code, err := f.s.NewPair("Pending", "linux")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, days := range []int{10, 40, 100} {
		at := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
		if _, err = f.s.DB.Exec("INSERT INTO minutes(device_id,minute,n)VALUES(?,?,1)", id, at); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.DB.Exec("INSERT INTO audit(at,event,device_id)VALUES(?, 'test',?)", at, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.DB.Exec("INSERT INTO samples(device_id,session,sequence,sampled,received,body)VALUES(?, 'test',1,?,?, '{}')", id, now.Add(-48*time.Hour).Unix(), now.Unix()); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Cleanup(); err != nil {
		t.Fatal(err)
	}
	var minutes, audits, samples int
	f.s.DB.QueryRow("SELECT count(*) FROM minutes").Scan(&minutes)
	f.s.DB.QueryRow("SELECT count(*) FROM audit WHERE event='test'").Scan(&audits)
	f.s.DB.QueryRow("SELECT count(*) FROM samples").Scan(&samples)
	if minutes != 1 || audits != 2 || samples != 0 {
		t.Fatalf("incorrect retention: minutes=%d audits=%d samples=%d", minutes, audits, samples)
	}
	if _, _, err = f.s.Enroll(code); err != nil {
		t.Fatal("cleanup removed unexpired pairing", err)
	}
}
