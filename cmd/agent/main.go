package main

import (
	"context"
	"flag"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/agent"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/collector"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/desktop"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/security"
	"github.com/Jimmy-xuzimo/Chronos-Agents/web"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func main() {
	base, _ := os.UserConfigDir()
	dir := flag.String("data", filepath.Join(base, "ChronosAgents"), "private local configuration directory")
	listen := flag.String("listen", "127.0.0.1:17860", "local management bind address")
	originFlag := flag.String("origin", "http://127.0.0.1:17860", "exact management origin")
	dev := flag.Bool("dev-loopback-http", false, "allow loopback HTTP Hub during explicit development")
	scope := flag.String("scope", "host", "host or container metrics")
	volumes := flag.String("volumes", "", "comma-separated accessible volume paths")
	cert := flag.String("tls-cert", "", "management TLS certificate")
	key := flag.String("tls-key", "", "management TLS key")
	proxy := flag.String("trusted-proxy", "", "explicit HTTPS reverse-proxy CIDR")
	headless := flag.Bool("headless", false, "disable desktop tray for services and containers")
	open := flag.Bool("open", false, "open management in the default browser and exit")
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
	origin, e := security.Origin(*originFlag, true)
	if e != nil {
		log.Fatal(e)
	}
	if strings.HasPrefix(origin, "http:") && !security.ListenLoopback(*listen) {
		log.Fatal("plain HTTP management must bind loopback")
	}
	if *open {
		client := &http.Client{Timeout: time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
		running := func() bool {
			r, e := client.Get(origin + "/api/info")
			if e != nil {
				return false
			}
			defer r.Body.Close()
			return r.StatusCode == 200
		}
		if !running() {
			exe, err := os.Executable()
			if err != nil {
				return
			}
			args := []string{}
			for _, arg := range os.Args[1:] {
				if arg != "--open" && arg != "--open=true" {
					args = append(args, arg)
				}
			}
			if exec.Command(exe, args...).Start() != nil {
				return
			}
			for i := 0; i < 30 && !running(); i++ {
				time.Sleep(100 * time.Millisecond)
			}
		}
		desktop.Open(origin)
		return
	}
	var trusted *net.IPNet
	if *proxy != "" {
		_, trusted, e = net.ParseCIDR(*proxy)
		if e != nil {
			log.Fatal("invalid proxy CIDR")
		}
	}
	if strings.HasPrefix(origin, "https:") && *cert == "" && trusted == nil {
		log.Fatal("management HTTPS requires TLS files or a trusted proxy")
	}
	if *scope != "host" && *scope != "container" {
		log.Fatal("invalid metric scope")
	}
	if runtime.GOOS == "linux" {
		if _, e = os.Stat("/.dockerenv"); e == nil && *scope == "host" && (os.Getenv("HOST_PROC") == "" || os.Getenv("HOST_SYS") == "") {
			log.Fatal("container host mode requires explicit read-only HOST_PROC and HOST_SYS mounts; otherwise use --scope container")
		}
		if os.Getenv("HOST_PROC") != "" && *volumes == "" {
			log.Fatal("host-mounted mode needs explicit --volumes; container filesystem must not be called host storage")
		}
	}
	listener, e := net.Listen("tcp", *listen)
	if e != nil {
		log.Fatal("management port already in use; existing agent remains running")
	}
	defer listener.Close()
	a, e := agent.Open(*dir, origin, *scope, collector.CSV(*volumes), *dev, web.Handler())
	if e != nil {
		log.Fatal("cannot initialize agent; inspect protected configuration and credential store")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go a.Run(ctx)
	handler := a.Handler()
	if trusted != nil && *cert == "" {
		next := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if !trusted.Contains(net.ParseIP(ip)) || r.Header.Get("X-Forwarded-Proto") != "https" {
				security.Error(w, 403, "trusted_proxy_required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	s := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.Shutdown(deadline)
	}()
	log.Printf("Chronos Agent management listening on %s; initialize using the private bootstrap-secret file", *listen)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if *cert != "" {
			e = s.ServeTLS(listener, *cert, *key)
		} else {
			e = s.Serve(listener)
		}
		if e != nil && e != http.ErrServerClosed {
			log.Print("management listener stopped")
			stop()
		}
	}()
	desktop.Run(ctx, origin, a.Paused, a.TogglePause, stop, *headless)
	<-done
}
