package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/hub"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/security"
	"github.com/Jimmy-xuzimo/Chronos-Agents/web"
)

func main() {
	dir := flag.String("data", ".data/hub", "private data directory")
	listen := flag.String("listen", "127.0.0.1:17870", "bind address")
	originFlag := flag.String("origin", "https://hub.example.com", "exact public origin")
	dev := flag.Bool("dev-loopback-http", false, "explicit local development only")
	cert := flag.String("tls-cert", "", "TLS certificate file")
	key := flag.String("tls-key", "", "TLS key file")
	proxy := flag.String("trusted-proxy", "", "CIDR of the explicit HTTPS reverse proxy")
	check := flag.Bool("healthcheck", false, "check local listening socket and exit")
	flag.Parse()
	if *check {
		address := *listen
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			os.Exit(1)
		}
		if host == "0.0.0.0" || host == "::" || host == "" {
			host = "127.0.0.1"
		}
		connection, e := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
		if e != nil {
			os.Exit(1)
		}
		connection.Close()
		return
	}
	origin, e := security.Origin(*originFlag, *dev)
	if e != nil {
		log.Fatal(e)
	}
	if strings.HasPrefix(origin, "http:") && (!*dev || !security.ListenLoopback(*listen)) {
		log.Fatal("development HTTP requires loopback bind")
	}
	var trusted *net.IPNet
	if *proxy != "" {
		_, trusted, e = net.ParseCIDR(*proxy)
		if e != nil {
			log.Fatal("invalid trusted proxy CIDR")
		}
	}
	if *cert == "" && strings.HasPrefix(origin, "https:") && trusted == nil {
		log.Fatal("TLS files or an explicit trusted HTTPS proxy are required")
	}
	s, e := hub.Open(*dir)
	if e != nil {
		log.Fatal("cannot open private hub storage")
	}
	defer s.Close()
	h := hub.New(s, origin, web.Handler())
	handler := h.Handler()
	if *cert == "" && trusted != nil {
		next := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if !trusted.Contains(net.ParseIP(ip)) || r.Header.Get("X-Forwarded-Proto") != "https" {
				security.Error(w, 403, "trusted_tls_proxy_required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(deadline)
	}()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.Cleanup()
			}
		}
	}()
	log.Printf("Chronos Hub listening on %s; first-run setup secret is in the private data directory, not logs", *listen)
	if *cert != "" {
		e = server.ListenAndServeTLS(*cert, *key)
	} else {
		e = server.ListenAndServe()
	}
	if e != nil && e != http.ErrServerClosed {
		log.Fatal("hub stopped unexpectedly")
	}
}
