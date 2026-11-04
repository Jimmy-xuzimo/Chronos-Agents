package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/collector"

	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/privatefile"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/secret"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/security"
)

type Config struct {
	Hub          string `json:"hub"`
	DeviceID     string `json:"device_id"`
	Interval     int    `json:"interval"`
	Timeout      int    `json:"timeout"`
	Processes    bool   `json:"processes"`
	Addresses    bool   `json:"addresses"`
	Hostname     bool   `json:"hostname"`
	Paused       bool   `json:"paused"`
	PauseBattery bool   `json:"pause_on_battery"`
	CAPEM        string `json:"ca_pem"`
	RotatedAt    int64  `json:"rotated_at"`
}

func (c Config) Validate(dev bool) error {
	if c.Interval < 10 || c.Interval > 300 || c.Timeout < 5 || c.Timeout > 60 || len(c.CAPEM) > 64*1024 {
		return errors.New("invalid configuration")
	}
	if c.Hub != "" {
		if _, e := security.Origin(c.Hub, dev); e != nil {
			return e
		}
	}
	return nil
}

type State struct {
	Config        Config `json:"config"`
	AdminHash     string `json:"admin_hash"`
	BootstrapHash string `json:"bootstrap_hash"`
}
type Status struct {
	State       string             `json:"state"`
	LastAttempt int64              `json:"last_attempt"`
	LastSuccess int64              `json:"last_success"`
	LastError   string             `json:"last_error"`
	Queued      int                `json:"queued"`
	Dropped     int                `json:"dropped"`
	Successes   int                `json:"successes"`
	Failures    int                `json:"failures"`
	Storage     string             `json:"credential_storage"`
	Policy      protocol.Policy    `json:"center_policy"`
	Sample      *protocol.Envelope `json:"sample"`
}
type Agent struct {
	mu                 sync.Mutex
	state              State
	status             Status
	dir, origin, scope string
	volumes            []string
	dev                bool
	token              string
	wake               chan struct{}
	gate               *security.Gate
	assets             http.Handler
	client             *http.Client
	policyAt           time.Time
	session            string
	sequence           int64
	queue              []protocol.Envelope
	queueBytes         int
	consecutive        int
	retryNotBefore     time.Time
	lastCollection     time.Time
	collector          *collector.Collector
	runMu              sync.Mutex
}

func Open(dir, origin, scope string, volumes []string, dev bool, assets http.Handler) (*Agent, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	if e := os.Chmod(dir, 0700); e != nil {
		return nil, e
	}
	a := &Agent{dir: dir, origin: origin, scope: scope, volumes: volumes, dev: dev, wake: make(chan struct{}, 1), session: security.Random(16), assets: assets}
	a.state.Config = Config{Interval: 10, Timeout: 12}
	raw, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e == nil {
		if json.Unmarshal(raw, &a.state) != nil {
			return nil, errors.New("invalid private configuration")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if a.state.Config.Validate(dev) != nil {
		return nil, errors.New("invalid saved configuration")
	}
	if a.state.AdminHash == "" && a.state.BootstrapHash == "" {
		code := security.Random(32)
		a.state.BootstrapHash = security.Hash(code)
		if e = privatefile.Write(filepath.Join(dir, "bootstrap-secret"), []byte(code)); e != nil {
			return nil, e
		}
		if e = a.persist(); e != nil {
			return nil, e
		}
	}
	a.token, e = secret.Read(dir)
	if e != nil && a.state.Config.DeviceID != "" {
		a.status.LastError = "credential_store_unavailable"
	}
	a.status.Storage = secret.Backend()
	a.status.State = "unpaired"
	a.collector = collector.New(collector.Options{Scope: scope, Volumes: volumes})
	a.gate = &security.Gate{Origin: origin, Secure: strings.HasPrefix(origin, "https:"), Initialized: func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.state.AdminHash != "" }, Check: func(password, totp string) bool {
		a.mu.Lock()
		hash := a.state.AdminHash
		a.mu.Unlock()
		return security.VerifyPassword(hash, password)
	}, Setup: a.setup}
	return a, nil
}
func (a *Agent) persist() error { return privatefile.JSON(filepath.Join(a.dir, "state.json"), a.state) }
func (a *Agent) setup(code, password string) error {
	hash, e := security.Password(password)
	if e != nil {
		return e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.AdminHash != "" || !security.Equal(security.Hash(code), a.state.BootstrapHash) {
		return errors.New("setup refused")
	}
	old := a.state
	a.state.AdminHash = hash
	a.state.BootstrapHash = ""
	if e = a.persist(); e != nil {
		a.state = old
		return e
	}
	_ = os.Remove(filepath.Join(a.dir, "bootstrap-secret"))
	return nil
}
func (a *Agent) notify() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}
func (a *Agent) Snapshot() (Config, Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state.Config, a.status
}
func (a *Agent) Handler() http.Handler { return security.Headers(a.origin, a) }
func (a *Agent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		security.JSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.URL.Path == "/api/info" {
		security.JSON(w, 200, map[string]string{"role": "agent", "version": "0.1.0"})
		return
	}
	if a.gate.Route(w, r) {
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Method != "GET" {
			security.Error(w, 405, "method")
			return
		}
		a.assets.ServeHTTP(w, r)
		return
	}
	if !a.gate.Admin(w, r) {
		return
	}
	switch r.URL.Path {
	case "/api/local/status":
		if r.Method != "GET" {
			security.Error(w, 405, "method")
			return
		}
		c, s := a.Snapshot()
		security.JSON(w, 200, map[string]any{"config": c, "status": s})
	case "/api/local/config":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var c Config
		if security.Decode(w, r, &c) != nil || c.Validate(a.dev) != nil {
			security.Error(w, 400, "invalid_config")
			return
		}
		a.runMu.Lock()
		defer a.runMu.Unlock()
		a.mu.Lock()
		old := a.state.Config
		if c.Hub != old.Hub || c.DeviceID != old.DeviceID || c.CAPEM != old.CAPEM {
			a.mu.Unlock()
			security.Error(w, 400, "pairing_required_to_change_hub")
			return
		}
		if (old.Processes && !c.Processes) || (old.Addresses && !c.Addresses) || (old.Hostname && !c.Hostname) {
			a.queue = nil
			a.queueBytes = 0
			a.status.Sample = nil
			a.status.Queued = 0
		}
		c.RotatedAt = old.RotatedAt
		a.state.Config = c
		e := a.persist()
		if e != nil {
			a.state.Config = old
		}
		a.mu.Unlock()
		if e != nil {
			security.Error(w, 500, "save_failed")
			return
		}
		a.notify()
		security.JSON(w, 200, map[string]bool{"ok": true})
	case "/api/local/pair":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			Hub  string `json:"hub"`
			Code string `json:"code"`
			CA   string `json:"ca_pem"`
		}
		if security.Decode(w, r, &req) != nil || len(req.Code) > 100 || len(req.CA) > 64*1024 {
			security.Error(w, 400, "invalid_request")
			return
		}
		origin, e := security.Origin(req.Hub, a.dev)
		if e != nil {
			security.Error(w, 400, "https_origin_required")
			return
		}
		a.runMu.Lock()
		defer a.runMu.Unlock()
		client, e := newClient(req.CA)
		if e != nil {
			security.Error(w, 400, "invalid_ca")
			return
		}
		defer client.CloseIdleConnections()
		var paired struct {
			DeviceID string `json:"device_id"`
			Token    string `json:"token"`
		}
		code, _, e := call(r.Context(), client, origin, "/api/v1/enroll", "POST", "", map[string]string{"code": req.Code}, &paired, 12)
		if e != nil || code != 200 || !protocol.ID.MatchString(paired.DeviceID) || len(paired.Token) < 32 {
			security.Error(w, 400, "pairing_failed_check_https_and_code")
			return
		}
		if e = secret.Save(a.dir, paired.Token); e != nil {
			security.Error(w, 500, "credential_save_failed_repair_required")
			return
		}
		a.mu.Lock()
		a.state.Config.Hub = origin
		a.state.Config.DeviceID = paired.DeviceID
		a.state.Config.CAPEM = req.CA
		a.state.Config.RotatedAt = time.Now().Unix()
		a.token = paired.Token
		a.status.State = "paired"
		a.status.LastError = ""
		a.status.Sample = nil
		a.status.Queued = 0
		a.status.Policy = protocol.Policy{}
		a.queue = nil
		a.queueBytes = 0
		a.policyAt = time.Time{}
		a.retryNotBefore = time.Time{}
		a.lastCollection = time.Time{}
		a.session = security.Random(16)
		a.sequence = 0
		e = a.persist()
		if a.client != nil {
			a.client.CloseIdleConnections()
		}
		a.client = nil
		a.mu.Unlock()
		if e != nil {
			security.Error(w, 500, "config_save_failed_repair_required")
			return
		}
		a.notify()
		security.JSON(w, 200, map[string]bool{"ok": true})
	case "/api/local/rotate":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		a.runMu.Lock()
		e := a.rotate(r.Context())
		a.runMu.Unlock()
		if e != nil {
			security.Error(w, 400, "rotation_failed")
			return
		}
		security.JSON(w, 200, map[string]bool{"ok": true})
	case "/api/local/upload":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		a.notify()
		security.JSON(w, 202, map[string]bool{"scheduled": true})
	case "/api/local/forget":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		a.runMu.Lock()
		defer a.runMu.Unlock()
		a.mu.Lock()
		e := secret.Delete(a.dir)
		if e != nil && !os.IsNotExist(e) {
			a.mu.Unlock()
			security.Error(w, 500, "credential_delete_failed")
			return
		}
		a.token = ""
		a.state.Config.Hub = ""
		a.state.Config.DeviceID = ""
		a.state.Config.CAPEM = ""
		a.status = Status{State: "unpaired", Storage: secret.Backend()}
		a.queue = nil
		a.queueBytes = 0
		e = a.persist()
		a.mu.Unlock()
		if e != nil {
			security.Error(w, 500, "save_failed")
			return
		}
		a.notify()
		security.JSON(w, 200, map[string]bool{"ok": true})
	default:
		security.Error(w, 404, "not_found")
	}
}
func newClient(ca string) (*http.Client, error) {
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if ca != "" && !roots.AppendCertsFromPEM([]byte(ca)) {
		return nil, errors.New("invalid CA")
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 90 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
func call(ctx context.Context, client *http.Client, origin, path, method, token string, body, out any, timeout int) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	var b io.Reader
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			return 0, 0, e
		}
		b = bytes.NewReader(raw)
	}
	r, e := http.NewRequestWithContext(ctx, method, origin+path, b)
	if e != nil {
		return 0, 0, e
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	response, e := client.Do(r)
	if e != nil {
		return 0, 0, errors.New("network_or_tls_error")
	}
	defer response.Body.Close()
	retry := security.Int(response.Header.Get("Retry-After"), 0, 0, 3600)
	limited := io.LimitReader(response.Body, 512*1024)
	if out != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		if json.NewDecoder(limited).Decode(out) != nil {
			return response.StatusCode, retry, errors.New("invalid_server_response")
		}
	} else {
		_, _ = io.Copy(io.Discard, limited)
	}
	return response.StatusCode, retry, nil
}
func (a *Agent) rotate(ctx context.Context) error {
	a.mu.Lock()
	c, t := a.state.Config, a.token
	a.mu.Unlock()
	if t == "" {
		return errors.New("unpaired")
	}
	if a.client == nil {
		var e error
		a.client, e = newClient(c.CAPEM)
		if e != nil {
			return e
		}
	}
	var result struct {
		Token string `json:"token"`
	}
	code, _, e := call(ctx, a.client, c.Hub, "/api/v1/rotate", "POST", t, map[string]bool{}, &result, c.Timeout)
	if e != nil || code != 200 || len(result.Token) < 32 {
		return errors.New("rotation refused")
	}
	if e = secret.Save(a.dir, result.Token); e != nil {
		return e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = result.Token
	a.state.Config.RotatedAt = time.Now().Unix()
	return a.persist()
}
func (a *Agent) Run(ctx context.Context) {
	for {
		a.runMu.Lock()
		wait := a.step(ctx)
		a.runMu.Unlock()
		if wait < 0 {
			wait = 24 * time.Hour
		}
		timer := time.NewTimer(max(wait, time.Second))
		select {
		case <-ctx.Done():
			timer.Stop()
			if a.client != nil {
				a.client.CloseIdleConnections()
			}
			return
		case <-a.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}
func (a *Agent) step(ctx context.Context) time.Duration {
	a.mu.Lock()
	c, t := a.state.Config, a.token
	a.mu.Unlock()
	if c.Paused {
		a.setStatus("paused", "")
		return -1
	}
	if t == "" || c.Hub == "" {
		a.setStatus("unpaired", "")
		return -1
	}
	if until := time.Until(a.retryNotBefore); until > 0 {
		return until
	}
	if since := time.Since(a.lastCollection); since < 10*time.Second {
		return 10*time.Second - since
	}
	if c.PauseBattery {
		if b, ok := collector.Battery(); ok && b["on_battery"] == true {
			a.setStatus("battery_paused", "")
			return time.Minute
		}
	}
	if a.client == nil {
		var e error
		a.client, e = newClient(c.CAPEM)
		if e != nil {
			a.setStatus("configuration_error", "invalid_ca")
			return -1
		}
	}
	if time.Since(a.policyAt) >= time.Minute {
		var policy protocol.Policy
		code, retry, e := call(ctx, a.client, c.Hub, "/api/v1/policy", "GET", t, nil, &policy, c.Timeout)
		if code == 401 || code == 403 {
			a.setStatus("credential_revoked", "reauthenticate")
			return -1
		}
		if e != nil || code != 200 || !policy.Validate() {
			return a.failure(code, retry)
		}
		a.mu.Lock()
		a.status.Policy = policy
		a.mu.Unlock()
		a.policyAt = time.Now()
	}
	a.mu.Lock()
	policy := a.status.Policy
	a.mu.Unlock()
	if policy.Paused {
		a.setStatus("center_paused", "")
		return time.Minute
	}
	interval := max(c.Interval, policy.Interval)
	a.collector.Configure(collector.Options{Scope: a.scope, Volumes: a.volumes, Processes: c.Processes && policy.Processes, Addresses: c.Addresses && policy.Addresses, Hostname: c.Hostname && policy.Hostname})
	sampleCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
	a.lastCollection = time.Now()
	metrics := a.collector.Sample(sampleCtx)
	cancel()
	a.sequence++
	sample := protocol.Envelope{SchemaVersion: 1, DeviceID: c.DeviceID, AgentVersion: "0.1.0", SessionID: a.session, Sequence: a.sequence, SampledAt: time.Now().UTC(), ExpectedIntervalSeconds: int64(interval), Platform: collector.Platform(c.Hostname && policy.Hostname), Metrics: metrics}
	a.mu.Lock()
	a.status.Sample = &sample
	a.status.LastAttempt = time.Now().Unix()
	a.mu.Unlock()
	a.enqueue(sample)
	// Upload newest first; older buffered samples keep their original timestamps.
	accepted := 0
	for i := len(a.queue) - 1; i >= 0 && accepted < 3; i-- {
		item := a.queue[i]
		var result struct {
			Policy protocol.Policy `json:"policy"`
		}
		code, retry, e := call(ctx, a.client, c.Hub, "/api/v1/ingest", "POST", t, item, &result, c.Timeout)
		if code == 401 || code == 403 {
			a.setStatus("credential_revoked", "reauthenticate")
			return -1
		}
		if e != nil || code != 202 {
			return a.failure(code, retry)
		}
		a.queue = append(a.queue[:i], a.queue[i+1:]...)
		accepted++
		a.mu.Lock()
		a.status.Policy = result.Policy
		a.status.LastSuccess = time.Now().Unix()
		a.status.Successes++
		a.consecutive = 0
		a.status.LastError = ""
		a.status.State = "online"
		a.mu.Unlock()
	}
	a.updateQueue()
	if c.RotatedAt > 0 && time.Since(time.Unix(c.RotatedAt, 0)) > 30*24*time.Hour {
		_ = a.rotate(ctx)
	}
	return time.Duration(interval) * time.Second
}
func (a *Agent) enqueue(sample protocol.Envelope) {
	a.queue = append(a.queue, sample)
	a.queueBytes = 0
	for _, s := range a.queue {
		b, _ := json.Marshal(s)
		a.queueBytes += len(b)
	}
	for len(a.queue) > 60 || a.queueBytes > 1024*1024 {
		b, _ := json.Marshal(a.queue[0])
		a.queueBytes -= len(b)
		a.queue = a.queue[1:]
		a.mu.Lock()
		a.status.Dropped++
		a.mu.Unlock()
	}
	a.updateQueue()
}
func (a *Agent) updateQueue() { a.mu.Lock(); a.status.Queued = len(a.queue); a.mu.Unlock() }
func (a *Agent) setStatus(state, code string) {
	a.mu.Lock()
	a.status.State = state
	a.status.LastError = code
	a.mu.Unlock()
}
func (a *Agent) failure(code, retry int) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.State = "retrying"
	a.status.LastError = "network_or_tls_error"
	if code > 0 {
		a.status.LastError = http.StatusText(code)
	}
	a.status.Failures++
	a.consecutive++
	n := min(a.consecutive, 5)
	wait := max(time.Duration(retry)*time.Second, time.Duration(min(300, 10<<n))*time.Second)
	a.retryNotBefore = time.Now().Add(wait)
	return wait
}
