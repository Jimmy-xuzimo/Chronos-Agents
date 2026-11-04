package agent

import (
	"context"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/protocol"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPauseAndBackoffCannotBeBypassedByWake(t *testing.T) {
	a, err := Open(t.TempDir(), "http://127.0.0.1:17860", "host", nil, true, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	a.token = "fictional"
	a.state.Config.Hub = "http://127.0.0.1:1"
	a.state.Config.Paused = true
	if wait := a.step(context.Background()); wait >= 0 {
		t.Fatal("paused agent scheduled network")
	}
	a.state.Config.Paused = false
	wait := a.failure(429, 90)
	if wait < 90*time.Second {
		t.Fatal("Retry-After ignored")
	}
	if remaining := a.step(context.Background()); remaining < 89*time.Second {
		t.Fatal("manual wake bypassed retry deadline")
	}
	if a.sequence != 0 {
		t.Fatal("sampled during network backoff")
	}
}

func TestTLSValidationAndNoRedirectCredentialLeak(t *testing.T) {
	reached := false
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer sink.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	secure, err := newClient("")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = call(context.Background(), secure, redirect.URL, "/", "GET", "fictional-token", nil, nil, 5); err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
	client := redirect.Client()
	client.CheckRedirect = secure.CheckRedirect
	code, _, err := call(context.Background(), client, redirect.URL, "/", "GET", "fictional-token", nil, nil, 5)
	if err != nil || code != 307 || reached {
		t.Fatal("redirect policy leaked or failed")
	}
}

func TestRAMQueueIsBoundedAndSamplesAreNotPersisted(t *testing.T) {
	a, err := Open(t.TempDir(), "http://127.0.0.1:17860", "host", nil, true, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		a.enqueue(protocol.Envelope{Sequence: int64(i), Metrics: map[string]protocol.Metric{}})
	}
	if len(a.queue) != 60 || a.status.Dropped != 40 || a.status.Queued != 60 {
		t.Fatal("unbounded or miscounted queue")
	}
	if a.queue[0].Sequence != 40 {
		t.Fatal("oldest queue item not discarded")
	}
	a2, err := Open(a.dir, a.origin, "host", nil, true, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	if len(a2.queue) != 0 || a2.status.Sample != nil {
		t.Fatal("business sample persisted")
	}
}
