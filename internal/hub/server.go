package hub

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/security"
	"github.com/skip2/go-qrcode"
)

type Server struct {
	Store  *Store
	Gate   *security.Gate
	Rate   security.Bucket
	Origin string
	Assets http.Handler
}

func New(s *Store, origin string, assets http.Handler) *Server {
	h := &Server{Store: s, Origin: origin, Assets: assets}
	h.Gate = &security.Gate{Origin: origin, Secure: strings.HasPrefix(origin, "https:"), Initialized: func() bool { return s.Get("admin_password") != "" }, Setup: s.Setup, Check: func(password, code string) bool {
		return security.VerifyPassword(s.Get("admin_password"), password) && (s.Get("totp") == "" || security.VerifyTOTP(s.Get("totp"), code, time.Now()))
	}}
	return h
}
func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") || len(v) > 512 {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}
func (h *Server) Handler() http.Handler { return security.Headers(h.Origin, h) }
func (h *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		security.JSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	if r.URL.Path == "/api/info" {
		security.JSON(w, 200, map[string]string{"role": "hub", "version": "1.0.0"})
		return
	}
	if h.Gate.Route(w, r) {
		return
	}
	if r.URL.Path == "/api/v1/enroll" {
		h.enroll(w, r)
		return
	}
	if r.URL.Path == "/api/v1/ingest" || r.URL.Path == "/api/v1/policy" || r.URL.Path == "/api/v1/rotate" {
		h.device(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		h.read(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/admin/") {
		if h.Gate.Admin(w, r) {
			h.admin(w, r)
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		security.Error(w, 404, "not_found")
		return
	}
	if r.Method != "GET" {
		security.Error(w, 405, "method")
		return
	}
	h.Assets.ServeHTTP(w, r)
}
func (h *Server) enroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		security.Error(w, 405, "method")
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !h.Rate.Allow("enroll:"+ip, 10, time.Minute) {
		security.Error(w, 429, "rate_limited")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if security.Decode(w, r, &req) != nil || len(req.Code) > 100 {
		security.Error(w, 400, "invalid_request")
		return
	}
	id, token, e := h.Store.Enroll(req.Code)
	if e != nil {
		security.Error(w, 401, "invalid_pairing")
		return
	}
	h.Store.Audit("enrolled", id)
	security.JSON(w, 200, map[string]string{"device_id": id, "token": token})
}
func (h *Server) device(w http.ResponseWriter, r *http.Request) {
	id, policy, ok := h.Store.DeviceToken(bearer(r))
	if !ok {
		security.Error(w, 401, "invalid_device_credential")
		return
	}
	switch r.URL.Path {
	case "/api/v1/policy":
		if r.Method != "GET" {
			security.Error(w, 405, "method")
			return
		}
		security.JSON(w, 200, policy)
	case "/api/v1/rotate":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		if !h.Rate.Allow("rotate:"+id, 3, time.Hour) {
			security.Error(w, 429, "rate_limited")
			return
		}
		token, e := h.Store.Rotate(id)
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		h.Store.Audit("credential_rotated", id)
		security.JSON(w, 200, map[string]string{"token": token})
	case "/api/v1/ingest":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		if !h.Rate.Allow("ingest:"+id, 20, time.Minute) {
			w.Header().Set("Retry-After", "10")
			security.Error(w, 429, "rate_limited")
			return
		}
		var sample protocol.Envelope
		if security.Decode(w, r, &sample) != nil || sample.Validate() != nil {
			security.Error(w, 422, "invalid_sample")
			return
		}
		if sample.DeviceID != id {
			security.Error(w, 403, "device_mismatch")
			return
		}
		if sample.SampledAt.Before(time.Now().Add(-24*time.Hour)) || sample.SampledAt.After(time.Now().Add(10*time.Minute)) {
			security.Error(w, 422, "clock_out_of_range")
			return
		}
		if !policy.Processes {
			if m, ok := sample.Metrics["processes"]; ok && m.Status == "supported" {
				m.Status = "disabled"
				m.Data = nil
				sample.Metrics["processes"] = m
			}
		}
		if !policy.Hostname {
			sample.Platform.Hostname = ""
			if system, ok := sample.Metrics["system"]; ok && system.Data != nil {
				delete(system.Data, "hostname")
				sample.Metrics["system"] = system
			}
		}
		if !policy.Addresses {
			if m, ok := sample.Metrics["network"]; ok && m.Data != nil {
				delete(m.Data, "addresses")
				if m.Data["addresses_status"] == "supported" {
					m.Data["addresses_status"] = "disabled"
				}
				if items, ok := m.Data["interfaces"].([]any); ok {
					for _, item := range items {
						if row, ok := item.(map[string]any); ok {
							delete(row, "addresses")
						}
					}
				}
				sample.Metrics["network"] = m
			}
		}
		duplicate, e := h.Store.Ingest(sample)
		if storagePressure(e) {
			w.Header().Set("Retry-After", "60")
			security.Error(w, 503, "storage_pressure")
			return
		}
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		security.JSON(w, 202, map[string]any{"accepted": true, "duplicate": duplicate, "received_at": time.Now().UTC(), "policy": policy})
	}
}
func (h *Server) read(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		security.Error(w, 405, "read_only")
		return
	}
	var allowed []string
	if bearer(r) != "" {
		var ok bool
		allowed, ok = h.Store.Reader(bearer(r))
		if !ok {
			security.Error(w, 401, "invalid_reader")
			return
		}
	} else if !h.Gate.Admin(w, r) {
		return
	}
	switch r.URL.Path {
	case "/api/v1/devices":
		items, e := h.Store.Devices()
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		if allowed != nil {
			items = slices.DeleteFunc(items, func(d Device) bool { return !slices.Contains(allowed, d.ID) })
		}
		security.JSON(w, 200, map[string]any{"items": items, "server_time": time.Now().UTC()})
	case "/api/v1/history":
		id := r.URL.Query().Get("device")
		if !protocol.ID.MatchString(id) || (allowed != nil && !slices.Contains(allowed, id)) {
			security.Error(w, 403, "device_scope")
			return
		}
		span := r.URL.Query().Get("range")
		duration, bucket := time.Hour, int64(60)
		switch span {
		case "1h", "":
		case "24h":
			duration = 24 * time.Hour
		case "7d":
			duration = 7 * 24 * time.Hour
			bucket = 600
		case "30d":
			duration = 30 * 24 * time.Hour
			bucket = 1800
		default:
			security.Error(w, 400, "invalid_range")
			return
		}
		rows, e := h.Store.DB.Query("SELECT (minute/?)*?,avg(cpu),avg(mem),avg(rx),avg(tx) FROM minutes WHERE device_id=? AND minute>=? GROUP BY minute/? ORDER BY minute LIMIT 2000", bucket, bucket, id, time.Now().Add(-duration).Unix(), bucket)
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		defer rows.Close()
		points := []map[string]any{}
		for rows.Next() {
			var at int64
			var cpu, mem, rx, tx *float64
			if rows.Scan(&at, &cpu, &mem, &rx, &tx) != nil {
				security.Error(w, 500, "storage_error")
				return
			}
			points = append(points, map[string]any{"ts": at, "cpu": cpu, "mem": mem, "rx": rx, "tx": tx})
		}
		security.JSON(w, 200, map[string]any{"points": points, "resolution_seconds": bucket, "process_history": false})
	default:
		security.Error(w, 404, "not_found")
	}
}
func (h *Server) admin(w http.ResponseWriter, r *http.Request) {
	s := h.Store
	switch r.URL.Path {
	case "/api/admin/pair":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			ID   string `json:"id"`
		}
		if security.Decode(w, r, &req) != nil || len(req.Name) > 80 {
			security.Error(w, 400, "invalid_request")
			return
		}
		id, code := "", ""
		var e error
		if req.ID != "" {
			id = req.ID
			code, e = s.RePair(id)
		} else {
			if req.Name == "" || !slices.Contains([]string{"windows", "linux", "nas", "macos", "android"}, req.Kind) {
				security.Error(w, 400, "invalid_device")
				return
			}
			id, code, e = s.NewPair(req.Name, req.Kind)
		}
		if e != nil {
			security.Error(w, 400, "pair_failed")
			return
		}
		payload, _ := json.Marshal(map[string]any{"origin": h.Origin, "code": code, "expires_at": time.Now().Add(10 * time.Minute).UTC()})
		png, _ := qrcode.Encode(string(payload), qrcode.Medium, 256)
		s.Audit("pair_created", id)
		security.JSON(w, 200, map[string]any{"device_id": id, "code": code, "expires_in": 600, "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
	case "/api/admin/revoke":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if security.Decode(w, r, &req) != nil || !protocol.ID.MatchString(req.ID) {
			security.Error(w, 400, "invalid_device")
			return
		}
		if s.Revoke(req.ID) != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		s.Audit("device_revoked", req.ID)
		security.JSON(w, 200, map[string]bool{"ok": true})
	case "/api/admin/policy":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			ID     string `json:"id"`
			Policy Policy `json:"policy"`
		}
		if security.Decode(w, r, &req) != nil || !protocol.ID.MatchString(req.ID) || !req.Policy.Validate() {
			security.Error(w, 400, "invalid_policy")
			return
		}
		raw, _ := json.Marshal(req.Policy)
		res, e := s.DB.Exec("UPDATE devices SET policy=? WHERE id=?", string(raw), req.ID)
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			security.Error(w, 404, "device_missing")
			return
		}
		s.Audit("policy_updated", req.ID)
		security.JSON(w, 200, map[string]bool{"ok": true})
	case "/api/admin/readers":
		if r.Method == "GET" {
			rows, e := s.DB.Query("SELECT substr(hash,1,16),name,devices,created FROM readers")
			if e != nil {
				security.Error(w, 500, "storage_error")
				return
			}
			defer rows.Close()
			items := []map[string]any{}
			for rows.Next() {
				var id, name, raw string
				var at int64
				_ = rows.Scan(&id, &name, &raw, &at)
				var devices []string
				_ = json.Unmarshal([]byte(raw), &devices)
				items = append(items, map[string]any{"id": id, "name": name, "devices": devices, "created": at})
			}
			security.JSON(w, 200, map[string]any{"items": items})
			return
		}
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			Name    string   `json:"name"`
			Devices []string `json:"devices"`
			Revoke  string   `json:"revoke"`
		}
		if security.Decode(w, r, &req) != nil {
			security.Error(w, 400, "invalid_request")
			return
		}
		if req.Revoke != "" {
			_, e := s.DB.Exec("DELETE FROM readers WHERE substr(hash,1,16)=?", req.Revoke)
			if e != nil {
				security.Error(w, 500, "storage_error")
				return
			}
			s.Audit("reader_revoked", "")
			security.JSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if len(req.Devices) < 1 || len(req.Devices) > 100 || len(req.Name) < 1 || len(req.Name) > 80 {
			security.Error(w, 400, "invalid_scope")
			return
		}
		for _, id := range req.Devices {
			var n int
			_ = s.DB.QueryRow("SELECT count(*) FROM devices WHERE id=?", id).Scan(&n)
			if n != 1 {
				security.Error(w, 400, "invalid_device")
				return
			}
		}
		token := security.Random(32)
		raw, _ := json.Marshal(req.Devices)
		_, e := s.DB.Exec("INSERT INTO readers(hash,name,devices,created)VALUES(?,?,?,?)", security.Hash(token), req.Name, string(raw), time.Now().Unix())
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		s.Audit("reader_created", "")
		security.JSON(w, 200, map[string]string{"token": token})
	case "/api/admin/settings":
		if r.Method == "GET" {
			security.JSON(w, 200, map[string]any{"raw_hours": security.Int(s.Get("raw_hours"), 24, 1, 168), "minute_days": security.Int(s.Get("minute_days"), 30, 1, 365), "audit_days": security.Int(s.Get("audit_days"), 90, 1, 365), "totp_enabled": s.Get("totp") != ""})
			return
		}
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			Raw     int `json:"raw_hours"`
			Minutes int `json:"minute_days"`
			Audit   int `json:"audit_days"`
		}
		if security.Decode(w, r, &req) != nil || req.Raw < 1 || req.Raw > 168 || req.Minutes < 1 || req.Minutes > 365 || req.Audit < 1 || req.Audit > 365 {
			security.Error(w, 400, "invalid_retention")
			return
		}
		values := map[string]string{}
		for key, value := range map[string]int{"raw_hours": req.Raw, "minute_days": req.Minutes, "audit_days": req.Audit} {
			raw, _ := json.Marshal(value)
			values[key] = string(raw)
		}
		if s.SetMany(values) != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		s.Audit("retention_updated", "")
		security.JSON(w, 200, map[string]bool{"ok": true})
	case "/api/admin/audit":
		if r.Method != "GET" {
			security.Error(w, 405, "method")
			return
		}
		rows, e := s.DB.Query("SELECT at,event,device_id FROM audit ORDER BY id DESC LIMIT 100")
		if e != nil {
			security.Error(w, 500, "storage_error")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var at int64
			var event, id string
			_ = rows.Scan(&at, &event, &id)
			items = append(items, map[string]any{"at": at, "event": event, "device_id": id})
		}
		security.JSON(w, 200, map[string]any{"items": items})
	case "/api/admin/account":
		if r.Method != "POST" {
			security.Error(w, 405, "method")
			return
		}
		var req struct {
			Password    string `json:"password"`
			NewPassword string `json:"new_password"`
			TOTP        string `json:"totp"`
			Action      string `json:"action"`
			Secret      string `json:"secret"`
		}
		if security.Decode(w, r, &req) != nil || !h.Gate.Check(req.Password, req.TOTP) {
			security.Error(w, 403, "reauthentication_failed")
			return
		}
		switch req.Action {
		case "password":
			hash, e := security.Password(req.NewPassword)
			if e != nil {
				security.Error(w, 400, "password_length")
				return
			}
			if s.Set("admin_password", hash) != nil {
				security.Error(w, 500, "storage_error")
				return
			}
			h.Gate.RevokeSessions()
		case "totp_prepare":
			security.JSON(w, 200, map[string]string{"secret": security.TOTPSecret()})
			return
		case "totp_enable":
			if !security.VerifyTOTP(req.Secret, req.NewPassword, time.Now()) {
				security.Error(w, 400, "totp_verification_failed")
				return
			}
			if s.Set("totp", req.Secret) != nil {
				security.Error(w, 500, "storage_error")
				return
			}
			h.Gate.RevokeSessions()
		case "totp_disable":
			if s.Set("totp", "") != nil {
				security.Error(w, 500, "storage_error")
				return
			}
			h.Gate.RevokeSessions()
		default:
			security.Error(w, 400, "invalid_action")
			return
		}
		s.Audit("account_updated", "")
		security.JSON(w, 200, map[string]bool{"ok": true})
	default:
		security.Error(w, 404, "not_found")
	}
}
