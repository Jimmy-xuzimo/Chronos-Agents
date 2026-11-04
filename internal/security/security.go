package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

func Random(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic("secure entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Hash(v string) string   { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func Password(p string) (string, error) {
	if len(p) < 12 || len(p) > 256 {
		return "", errors.New("password must be 12–256 bytes")
	}
	s := make([]byte, 16)
	if _, e := rand.Read(s); e != nil {
		return "", e
	}
	h := argon2.IDKey([]byte(p), s, 2, 32*1024, 1, 32)
	return base64.RawStdEncoding.EncodeToString(s) + ":" + base64.RawStdEncoding.EncodeToString(h), nil
}
func VerifyPassword(stored, p string) bool {
	if len(p) > 256 {
		return false
	}
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return false
	}
	s, e := base64.RawStdEncoding.DecodeString(parts[0])
	if e != nil || len(s) != 16 {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(p), s, 2, 32*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func TOTPSecret() string {
	b := make([]byte, 20)
	if _, e := rand.Read(b); e != nil {
		panic("entropy unavailable")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}
func VerifyTOTP(secret, code string, t time.Time) bool {
	if len(code) != 6 {
		return false
	}
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if e != nil || len(key) < 20 || len(key) > 64 {
		return false
	}
	for d := -1; d <= 1; d++ {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(t.Unix()/30+int64(d)))
		m := hmac.New(sha1.New, key)
		m.Write(b[:])
		h := m.Sum(nil)
		o := h[len(h)-1] & 15
		n := (binary.BigEndian.Uint32(h[o:o+4]) & 0x7fffffff) % 1000000
		if Equal(fmt.Sprintf("%06d", n), code) {
			return true
		}
	}
	return false
}

func Origin(raw string, allowHTTP bool) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("HTTPS origin required")
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http" && Loopback(u.Hostname())) {
		return "", errors.New("HTTPS required; development HTTP is loopback-only")
	}
	return u.Scheme + "://" + u.Host, nil
}
func Loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
func ListenLoopback(addr string) bool {
	h, _, e := net.SplitHostPort(addr)
	return e == nil && Loopback(h)
}
func Decode(w http.ResponseWriter, r *http.Request, out any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("application/json required")
	}
	if r.Header.Get("Content-Encoding") != "" {
		return errors.New("encoded requests are not accepted")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return errors.New("invalid or oversized JSON")
	}
	var extra any
	if d.Decode(&extra) == io.EOF {
		return nil
	}
	return errors.New("one JSON object required")
}
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func Error(w http.ResponseWriter, status int, code string) {
	JSON(w, status, map[string]string{"error": code})
}

type Bucket struct {
	mu     sync.Mutex
	values map[string][]time.Time
}

func (b *Bucket) Allow(key string, n int, window time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.values == nil {
		b.values = map[string][]time.Time{}
	}
	now := time.Now()
	if len(b.values) > 4096 {
		for k, v := range b.values {
			if len(v) == 0 || v[len(v)-1].Before(now.Add(-window)) {
				delete(b.values, k)
			}
		}
		if len(b.values) > 4096 {
			return false
		}
	}
	v := b.values[key]
	i := 0
	for i < len(v) && v[i].Before(now.Add(-window)) {
		i++
	}
	v = v[i:]
	if len(v) >= n {
		b.values[key] = v
		return false
	}
	b.values[key] = append(v, now)
	return true
}

type Session struct {
	CSRF  string
	Until time.Time
}
type Gate struct {
	Origin      string
	Secure      bool
	mu          sync.Mutex
	sessions    map[string]Session
	Rate        Bucket
	Check       func(string, string) bool
	Initialized func() bool
	Setup       func(string, string) error
}

func (g *Gate) session(r *http.Request) (Session, bool) {
	c, e := r.Cookie(g.cookieName())
	if e != nil {
		return Session{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.sessions[Hash(c.Value)]
	if ok && time.Now().After(s.Until) {
		delete(g.sessions, Hash(c.Value))
		return Session{}, false
	}
	return s, ok
}
func (g *Gate) Admin(w http.ResponseWriter, r *http.Request) bool {
	s, ok := g.session(r)
	if !ok {
		Error(w, 401, "authentication_required")
		return false
	}
	if r.Method != "GET" && (r.Header.Get("Origin") != g.Origin || !Equal(r.Header.Get("X-CSRF-Token"), s.CSRF)) {
		Error(w, 403, "csrf_failed")
		return false
	}
	return true
}
func (g *Gate) Route(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/session":
		if r.Method != "GET" {
			Error(w, 405, "method")
			return true
		}
		s, ok := g.session(r)
		JSON(w, 200, map[string]any{"authenticated": ok, "initialized": g.Initialized(), "csrf": s.CSRF})
		return true
	case "/api/setup", "/api/login":
		if r.Method != "POST" {
			Error(w, 405, "method")
			return true
		}
		if r.Header.Get("Origin") != g.Origin {
			Error(w, 403, "origin_failed")
			return true
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !g.Rate.Allow(host+":login", 5, time.Minute) {
			w.Header().Set("Retry-After", "60")
			Error(w, 429, "rate_limited")
			return true
		}
		var req struct {
			Password  string `json:"password"`
			Bootstrap string `json:"bootstrap"`
			TOTP      string `json:"totp"`
		}
		if Decode(w, r, &req) != nil {
			Error(w, 400, "invalid_request")
			return true
		}
		if r.URL.Path == "/api/setup" {
			if g.Setup(req.Bootstrap, req.Password) != nil {
				Error(w, 403, "setup_failed")
				return true
			}
		} else if !g.Check(req.Password, req.TOTP) {
			Error(w, 401, "invalid_credentials")
			return true
		}
		token := Random(32)
		s := Session{CSRF: Random(32), Until: time.Now().Add(12 * time.Hour)}
		g.mu.Lock()
		if g.sessions == nil {
			g.sessions = map[string]Session{}
		}
		for k, v := range g.sessions {
			if time.Now().After(v.Until) {
				delete(g.sessions, k)
			}
		}
		if len(g.sessions) >= 128 {
			g.mu.Unlock()
			Error(w, 429, "session_limit")
			return true
		}
		g.sessions[Hash(token)] = s
		g.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: g.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: g.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
		JSON(w, 200, map[string]any{"ok": true, "csrf": s.CSRF})
		return true
	case "/api/logout":
		if r.Method != "POST" {
			Error(w, 405, "method")
			return true
		}
		if !g.Admin(w, r) {
			return true
		}
		c, _ := r.Cookie(g.cookieName())
		g.mu.Lock()
		delete(g.sessions, Hash(c.Value))
		g.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: g.cookieName(), Value: "", Path: "/", HttpOnly: true, Secure: g.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		JSON(w, 200, map[string]bool{"ok": true})
		return true
	}
	return false
}
func (g *Gate) RevokeSessions() { g.mu.Lock(); g.sessions = map[string]Session{}; g.mu.Unlock() }
func Headers(origin string, next http.Handler) http.Handler {
	u, _ := url.Parse(origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if !strings.EqualFold(r.Host, u.Host) {
			Error(w, 403, "host_not_allowed")
			return
		}
		if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin {
			Error(w, 403, "origin_not_allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func Int(v string, def, min, max int) int {
	n, e := strconv.Atoi(v)
	if e != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func (g *Gate) cookieName() string { return "chronos_session_" + Hash(g.Origin)[:16] }
