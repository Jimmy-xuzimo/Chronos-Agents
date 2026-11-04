package hub

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/security"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB           *sql.DB
	Directory    string
	spaceMu      sync.Mutex
	spaceAt      time.Time
	spaceOK      bool
	minFreeBytes uint64
}

func Open(dir string) (*Store, error) { return OpenWithLimits(dir, 2048, 128) }
func OpenWithLimits(dir string, databaseMiB, minFreeMiB int) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	if e := os.Chmod(dir, 0700); e != nil {
		return nil, e
	}
	path := filepath.Join(dir, "hub.sqlite")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS devices (id TEXT PRIMARY KEY,name TEXT NOT NULL,kind TEXT NOT NULL,created INTEGER NOT NULL,revoked INTEGER NOT NULL DEFAULT 0,token_hash TEXT NOT NULL DEFAULT '',previous_hash TEXT NOT NULL DEFAULT '',previous_until INTEGER NOT NULL DEFAULT 0,policy TEXT NOT NULL DEFAULT '{}');
CREATE TABLE IF NOT EXISTS pairs (hash TEXT PRIMARY KEY,device_id TEXT NOT NULL REFERENCES devices(id),expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS readers (hash TEXT PRIMARY KEY,name TEXT NOT NULL,devices TEXT NOT NULL,created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS latest (device_id TEXT PRIMARY KEY REFERENCES devices(id),sampled INTEGER NOT NULL,received INTEGER NOT NULL,body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS samples (device_id TEXT NOT NULL,session TEXT NOT NULL,sequence INTEGER NOT NULL,sampled INTEGER NOT NULL,received INTEGER NOT NULL,body TEXT NOT NULL,PRIMARY KEY(device_id,session,sequence));
CREATE INDEX IF NOT EXISTS sample_time ON samples(device_id,sampled);
CREATE TABLE IF NOT EXISTS minutes (device_id TEXT NOT NULL,minute INTEGER NOT NULL,cpu REAL,mem REAL,rx REAL,tx REAL,n INTEGER NOT NULL,cpu_n INTEGER NOT NULL DEFAULT 0,mem_n INTEGER NOT NULL DEFAULT 0,rx_n INTEGER NOT NULL DEFAULT 0,tx_n INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(device_id,minute));
CREATE TABLE IF NOT EXISTS audit (id INTEGER PRIMARY KEY,at INTEGER NOT NULL,event TEXT NOT NULL,device_id TEXT NOT NULL);
`)
	if e != nil {
		db.Close()
		return nil, e
	} // Add counters for databases created by early development builds.
	for _, column := range []string{"cpu", "mem", "rx", "tx"} {
		var count int
		rows, err := db.Query("PRAGMA table_info(minutes)")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var def any
			_ = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk)
			if name == column+"_n" {
				count++
			}
		}
		rows.Close()
		if count == 0 {
			if _, err = db.Exec("ALTER TABLE minutes ADD COLUMN " + column + "_n INTEGER NOT NULL DEFAULT 0"); err != nil {
				return nil, err
			}
			_, _ = db.Exec("UPDATE minutes SET " + column + "_n=CASE WHEN " + column + " IS NULL THEN 0 ELSE n END")
		}
	}
	_ = os.Chmod(path, 0600)
	s := &Store{DB: db, Directory: dir}
	if err := s.SetStorageLimits(databaseMiB, minFreeMiB); err != nil {
		db.Close()
		return nil, err
	}
	if s.Get("admin_password") == "" && s.Get("bootstrap_hash") == "" {
		bootstrap := security.Random(32)
		if e = s.Set("bootstrap_hash", security.Hash(bootstrap)); e != nil {
			return nil, e
		}
		if e = os.WriteFile(filepath.Join(dir, "bootstrap-secret"), []byte(bootstrap), 0600); e != nil {
			return nil, e
		}
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) Get(key string) string {
	var v string
	_ = s.DB.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&v)
	return v
}
func (s *Store) Set(key, value string) error {
	_, e := s.DB.Exec("INSERT INTO settings(key,value)VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return e
}
func (s *Store) SetMany(values map[string]string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range values {
		if _, err = tx.Exec("INSERT INTO settings(key,value)VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) Setup(code, password string) error {
	hash, e := security.Password(password)
	if e != nil {
		return e
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var old, bootstrap string
	_ = tx.QueryRow("SELECT value FROM settings WHERE key='admin_password'").Scan(&old)
	_ = tx.QueryRow("SELECT value FROM settings WHERE key='bootstrap_hash'").Scan(&bootstrap)
	if old != "" || !security.Equal(bootstrap, security.Hash(code)) {
		return errors.New("setup refused")
	}
	if _, e = tx.Exec("INSERT INTO settings(key,value)VALUES('admin_password',?); DELETE FROM settings WHERE key='bootstrap_hash'", hash); e != nil {
		return e
	}
	if e = tx.Commit(); e == nil {
		_ = os.Remove(filepath.Join(s.Directory, "bootstrap-secret"))
	}
	return e
}

type Device struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Kind     string             `json:"kind"`
	Created  int64              `json:"created"`
	Revoked  bool               `json:"revoked"`
	Policy   Policy             `json:"policy"`
	Latest   *protocol.Envelope `json:"latest"`
	Received int64              `json:"received_at"`
	State    string             `json:"state"`
}
type Policy = protocol.Policy

func (s *Store) NewPair(name, kind string) (string, string, error) {
	id := security.Random(16)
	code := security.Random(24)
	tx, e := s.DB.Begin()
	if e != nil {
		return "", "", e
	}
	defer tx.Rollback()
	policy, _ := json.Marshal(Policy{Interval: 10})
	_, e = tx.Exec("INSERT INTO devices(id,name,kind,created,policy)VALUES(?,?,?,?,?)", id, name, kind, time.Now().Unix(), string(policy))
	if e != nil {
		return "", "", e
	}
	_, e = tx.Exec("INSERT INTO pairs(hash,device_id,expires)VALUES(?,?,?)", security.Hash(code), id, time.Now().Add(10*time.Minute).Unix())
	if e != nil {
		return "", "", e
	}
	return id, code, tx.Commit()
}
func (s *Store) RePair(id string) (string, error) {
	code := security.Random(24)
	tx, e := s.DB.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var n int
	if e = tx.QueryRow("SELECT count(*) FROM devices WHERE id=?", id).Scan(&n); e != nil || n != 1 {
		return "", errors.New("device missing")
	}
	_, e = tx.Exec("DELETE FROM pairs WHERE device_id=?;INSERT INTO pairs(hash,device_id,expires)VALUES(?,?,?)", id, security.Hash(code), id, time.Now().Add(10*time.Minute).Unix())
	if e != nil {
		return "", e
	}
	return code, tx.Commit()
}
func (s *Store) Enroll(code string) (string, string, error) {
	token := security.Random(32)
	tx, e := s.DB.Begin()
	if e != nil {
		return "", "", e
	}
	defer tx.Rollback()
	var id string
	if e = tx.QueryRow("DELETE FROM pairs WHERE hash=? AND expires>? RETURNING device_id", security.Hash(code), time.Now().Unix()).Scan(&id); e != nil {
		return "", "", errors.New("invalid pairing")
	}
	_, e = tx.Exec("UPDATE devices SET token_hash=?,previous_hash='',previous_until=0,revoked=0 WHERE id=?", security.Hash(token), id)
	if e != nil {
		return "", "", e
	}
	return id, token, tx.Commit()
}
func (s *Store) DeviceToken(token string) (string, Policy, bool) {
	var id, raw string
	h := security.Hash(token)
	e := s.DB.QueryRow("SELECT id,policy FROM devices WHERE revoked=0 AND (token_hash=? OR (previous_hash=? AND previous_until>?))", h, h, time.Now().Unix()).Scan(&id, &raw)
	var p Policy
	if e != nil || json.Unmarshal([]byte(raw), &p) != nil {
		return "", p, false
	}
	return id, p, true
}
func (s *Store) Rotate(id string) (string, error) {
	token := security.Random(32)
	_, e := s.DB.Exec("UPDATE devices SET previous_hash=token_hash,previous_until=?,token_hash=? WHERE id=? AND revoked=0", time.Now().Add(10*time.Minute).Unix(), security.Hash(token), id)
	return token, e
}
func (s *Store) Revoke(id string) error {
	_, e := s.DB.Exec("UPDATE devices SET revoked=1,token_hash='',previous_hash='',previous_until=0 WHERE id=?;DELETE FROM pairs WHERE device_id=?", id, id)
	return e
}
func (s *Store) Reader(token string) ([]string, bool) {
	var raw string
	e := s.DB.QueryRow("SELECT devices FROM readers WHERE hash=?", security.Hash(token)).Scan(&raw)
	var ids []string
	if e != nil || json.Unmarshal([]byte(raw), &ids) != nil {
		return nil, false
	}
	return ids, len(ids) > 0
}
func (s *Store) Devices() ([]Device, error) {
	rows, e := s.DB.Query("SELECT d.id,d.name,d.kind,d.created,d.revoked,d.policy,coalesce(l.body,''),coalesce(l.received,0) FROM devices d LEFT JOIN latest l ON l.device_id=d.id ORDER BY d.created")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []Device{}
	for rows.Next() {
		var d Device
		var policy, body string
		if e = rows.Scan(&d.ID, &d.Name, &d.Kind, &d.Created, &d.Revoked, &policy, &body, &d.Received); e != nil {
			return nil, e
		}
		_ = json.Unmarshal([]byte(policy), &d.Policy)
		if body != "" {
			d.Latest = &protocol.Envelope{}
			_ = json.Unmarshal([]byte(body), d.Latest)
		}
		d.State = "awaiting_enrollment"
		if d.Revoked {
			d.State = "revoked"
		} else if d.Latest != nil {
			d.State = "recent"
			grace := int64(90)
			if d.Latest.ExpectedIntervalSeconds*3 > grace {
				grace = d.Latest.ExpectedIntervalSeconds * 3
			}
			if time.Now().Unix()-d.Received > grace {
				d.State = "stale"
			}
			if d.Kind == "android" && d.State == "stale" {
				d.State = "sync_delayed"
			}
			if d.Policy.Paused {
				d.State = "paused"
			}
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
func metric(e *protocol.Envelope, name, key string) any {
	m, ok := e.Metrics[name]
	if !ok || m.Status != "supported" {
		return nil
	}
	v, ok := m.Data[key].(float64)
	if !ok {
		return nil
	}
	return v
}
func (s *Store) Ingest(e protocol.Envelope) (bool, error) {
	if err := s.checkStorage(); err != nil {
		return false, err
	}
	full, _ := json.Marshal(e)
	history := e
	history.Metrics = map[string]protocol.Metric{}
	for k, v := range e.Metrics {
		if k != "processes" {
			history.Metrics[k] = v
		}
	}
	raw, _ := json.Marshal(history)
	now := time.Now().Unix()
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT OR IGNORE INTO samples(device_id,session,sequence,sampled,received,body)VALUES(?,?,?,?,?,?)", e.DeviceID, e.SessionID, e.Sequence, e.SampledAt.Unix(), now, string(raw))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return true, tx.Commit()
	}
	_, err = tx.Exec("INSERT INTO latest(device_id,sampled,received,body)VALUES(?,?,?,?) ON CONFLICT(device_id) DO UPDATE SET sampled=excluded.sampled,received=excluded.received,body=excluded.body WHERE excluded.sampled>=latest.sampled", e.DeviceID, e.SampledAt.Unix(), now, string(full))
	if err != nil {
		return false, err
	}
	cpu, mem, rx, txRate := metric(&e, "cpu", "percent"), metric(&e, "memory", "percent"), metric(&e, "network", "rx_bytes_per_second"), metric(&e, "network", "tx_bytes_per_second")
	count := func(v any) int {
		if v == nil {
			return 0
		}
		return 1
	}
	_, err = tx.Exec(`INSERT INTO minutes(device_id,minute,cpu,mem,rx,tx,n,cpu_n,mem_n,rx_n,tx_n)VALUES(?,?,?,?,?,?,1,?,?,?,?) ON CONFLICT(device_id,minute) DO UPDATE SET cpu=CASE WHEN excluded.cpu IS NULL THEN minutes.cpu WHEN minutes.cpu_n=0 THEN excluded.cpu ELSE (minutes.cpu*minutes.cpu_n+excluded.cpu)/(minutes.cpu_n+1) END,cpu_n=minutes.cpu_n+excluded.cpu_n,mem=CASE WHEN excluded.mem IS NULL THEN minutes.mem WHEN minutes.mem_n=0 THEN excluded.mem ELSE (minutes.mem*minutes.mem_n+excluded.mem)/(minutes.mem_n+1) END,mem_n=minutes.mem_n+excluded.mem_n,rx=CASE WHEN excluded.rx IS NULL THEN minutes.rx WHEN minutes.rx_n=0 THEN excluded.rx ELSE (minutes.rx*minutes.rx_n+excluded.rx)/(minutes.rx_n+1) END,rx_n=minutes.rx_n+excluded.rx_n,tx=CASE WHEN excluded.tx IS NULL THEN minutes.tx WHEN minutes.tx_n=0 THEN excluded.tx ELSE (minutes.tx*minutes.tx_n+excluded.tx)/(minutes.tx_n+1) END,tx_n=minutes.tx_n+excluded.tx_n,n=minutes.n+1`, e.DeviceID, e.SampledAt.Unix()/60*60, cpu, mem, rx, txRate, count(cpu), count(mem), count(rx), count(txRate))
	if err != nil {
		return false, err
	}
	return false, tx.Commit()
}
func (s *Store) Audit(event, id string) {
	_, _ = s.DB.Exec("INSERT INTO audit(at,event,device_id)VALUES(?,?,?)", time.Now().Unix(), event, id)
}
func (s *Store) Cleanup() error {
	now := time.Now()
	raw := security.Int(s.Get("raw_hours"), 24, 1, 168)
	minutes := security.Int(s.Get("minute_days"), 30, 1, 365)
	audit := security.Int(s.Get("audit_days"), 90, 1, 365)
	_, e := s.DB.Exec("DELETE FROM samples WHERE sampled<?;DELETE FROM minutes WHERE minute<?;DELETE FROM audit WHERE at<?;DELETE FROM pairs WHERE expires<?", now.Add(-time.Duration(raw)*time.Hour).Unix(), now.Add(-time.Duration(minutes)*24*time.Hour).Unix(), now.Add(-time.Duration(audit)*24*time.Hour).Unix(), now.Unix())
	return e
}
