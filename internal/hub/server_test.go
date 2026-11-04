package hub

import (
	"bytes"
	"encoding/json"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/security"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fixture struct {
	s      *Store
	h      *Server
	cookie *http.Cookie
	csrf   string
}

const testOrigin = "https://hub.example.com"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, testOrigin, http.NotFoundHandler())
	f := &fixture{s: s, h: h}
	code, e := os.ReadFile(filepath.Join(s.Directory, "bootstrap-secret"))
	if e != nil {
		t.Fatal(e)
	}
	r := f.req("POST", "/api/setup", map[string]string{"bootstrap": string(code), "password": "fictional-admin-password"}, "")
	if r.Code != 200 {
		t.Fatalf("setup status %d", r.Code)
	}
	cookies := r.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("secure session cookie")
	}
	f.cookie = cookies[0]
	var body struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &body)
	f.csrf = body.CSRF
	return f
}
func (f *fixture) req(method, path string, body any, token string) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, testOrigin+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", testOrigin)
	if f.cookie != nil {
		r.AddCookie(f.cookie)
		r.Header.Set("X-CSRF-Token", f.csrf)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.Handler().ServeHTTP(w, r)
	return w
}
func (f *fixture) pair(t *testing.T) (string, string) {
	t.Helper()
	id, code, e := f.s.NewPair("Example device", "windows")
	if e != nil {
		t.Fatal(e)
	}
	r := f.req("POST", "/api/v1/enroll", map[string]string{"code": code}, "")
	if r.Code != 200 {
		t.Fatalf("enroll %d", r.Code)
	}
	var p map[string]string
	_ = json.Unmarshal(r.Body.Bytes(), &p)
	again := f.req("POST", "/api/v1/enroll", map[string]string{"code": code}, "")
	if again.Code != 401 {
		t.Fatal("pairing code reused")
	}
	return id, p["token"]
}
func sample(id string, seq int64) protocol.Envelope {
	now := time.Now().UTC()
	return protocol.Envelope{SchemaVersion: 1, DeviceID: id, AgentVersion: "test", SessionID: "test_session_123", Sequence: seq, SampledAt: now, ExpectedIntervalSeconds: 10, Platform: protocol.Platform{OS: "windows", Arch: "amd64", Hostname: "PRIVATE-HOST"}, Metrics: map[string]protocol.Metric{"cpu": {Status: "supported", Scope: "host", CollectedAt: now, Data: map[string]any{"percent": 10.0}}, "processes": {Status: "supported", Scope: "host", CollectedAt: now, Data: map[string]any{"items": []any{map[string]any{"name": "private-process"}}}}, "network": {Status: "supported", Scope: "host", CollectedAt: now, Data: map[string]any{"interfaces": []any{map[string]any{"name": "example", "addresses": []any{"192.0.2.10"}}}}}}}
}
func TestAuthIsolationIdempotencyAndRevocation(t *testing.T) {
	f := newFixture(t)
	id, token := f.pair(t)
	other, otherToken := f.pair(t)
	fake := sample(other, 1)
	if r := f.req("POST", "/api/v1/ingest", fake, token); r.Code != 403 {
		t.Fatalf("forged device accepted: %d", r.Code)
	}
	payload := sample(id, 1)
	for i := 0; i < 2; i++ {
		if r := f.req("POST", "/api/v1/ingest", payload, token); r.Code != 202 {
			t.Fatalf("ingest %d", r.Code)
		}
	}
	var n int
	_ = f.s.DB.QueryRow("SELECT count(*) FROM samples").Scan(&n)
	if n != 1 {
		t.Fatal("duplicate samples retained")
	}
	devices, e := f.s.Devices()
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range devices {
		if d.ID == id {
			if d.Latest.Platform.Hostname != "" || d.Latest.Metrics["processes"].Status != "disabled" {
				t.Fatal("center privacy policy bypassed")
			}
			iface := d.Latest.Metrics["network"].Data["interfaces"].([]any)[0].(map[string]any)
			if _, ok := iface["addresses"]; ok {
				t.Fatal("address filter bypassed")
			}
		}
	}
	reader := security.Random(32)
	scope, _ := json.Marshal([]string{id})
	_, e = f.s.DB.Exec("INSERT INTO readers(hash,name,devices,created)VALUES(?,?,?,?)", security.Hash(reader), "example-reader", string(scope), time.Now().Unix())
	if e != nil {
		t.Fatal(e)
	}
	var result struct {
		Items []Device `json:"items"`
	}
	r := f.req("GET", "/api/v1/devices", nil, reader)
	_ = json.Unmarshal(r.Body.Bytes(), &result)
	if r.Code != 200 || len(result.Items) != 1 || result.Items[0].ID != id {
		t.Fatal("reader scope leaked")
	}
	if r = f.req("GET", "/api/v1/history?device="+other, nil, reader); r.Code != 403 {
		t.Fatal("reader accessed unassigned history")
	}
	if r = f.req("POST", "/api/v1/ingest", sample(id, 2), reader); r.Code != 401 {
		t.Fatal("reader wrote telemetry")
	}
	if r = f.req("GET", "/api/v1/devices", nil, otherToken); r.Code != 401 {
		t.Fatal("device read fleet")
	}
	_ = f.s.Revoke(id)
	if r = f.req("POST", "/api/v1/ingest", sample(id, 2), token); r.Code != 401 {
		t.Fatal("revoked credential accepted")
	}
}
func TestCSRFHostAndNoUnauthenticatedData(t *testing.T) {
	f := newFixture(t)
	r := httptest.NewRequest("POST", testOrigin+"/api/admin/pair", bytes.NewBufferString(`{"name":"test","kind":"linux"}`))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	f.h.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF bypassed")
	}
	r = httptest.NewRequest("GET", "https://malicious.example/api/session", nil)
	w = httptest.NewRecorder()
	f.h.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("DNS rebinding host accepted")
	}
	saved := f.cookie
	f.cookie = nil
	if r := f.req("GET", "/api/v1/devices", nil, ""); r.Code != 401 {
		t.Fatal("unauthenticated read")
	}
	f.cookie = saved
}
func TestRotationAndSourcePersistence(t *testing.T) {
	f := newFixture(t)
	id, old := f.pair(t)
	token, e := f.s.Rotate(id)
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{old, token} {
		if _, _, ok := f.s.DeviceToken(key); !ok {
			t.Fatal("rotation grace")
		}
	}
	_ = f.s.Revoke(id)
	for _, key := range []string{old, token} {
		if _, _, ok := f.s.DeviceToken(key); ok {
			t.Fatal("revocation ignored")
		}
	}
	id, _ = f.pair(t)
	p := sample(id, 1)
	if _, e = f.s.Ingest(p); e != nil {
		t.Fatal(e)
	}
	var archived string
	_ = f.s.DB.QueryRow("SELECT body FROM samples WHERE device_id=?", id).Scan(&archived)
	var archivedSample protocol.Envelope
	if json.Unmarshal([]byte(archived), &archivedSample) != nil {
		t.Fatal("archive")
	}
	if _, ok := archivedSample.Metrics["processes"]; ok {
		t.Fatal("process history retained")
	}
	db, e := Open(f.s.Directory)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	items, e := db.Devices()
	if e != nil || len(items) < 2 {
		t.Fatal("data not persistent")
	}
}
func TestMalformedPayloadAndTOTP(t *testing.T) {
	f := newFixture(t)
	id, token := f.pair(t)
	bad := sample(id, 1)
	m := bad.Metrics["cpu"]
	m.Status = "unsupported"
	bad.Metrics["cpu"] = m
	if r := f.req("POST", "/api/v1/ingest", bad, token); r.Code != 422 {
		t.Fatal("unsupported data accepted")
	}
	req := httptest.NewRequest("POST", testOrigin+"/api/v1/ingest", io.LimitReader(bytes.NewReader(make([]byte, 300000)), 300000))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.h.Handler().ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatal("oversize payload accepted")
	}
}
