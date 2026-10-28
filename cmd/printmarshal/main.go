// Command printmarshal runs the PrintMarshal server: API, web UI, device
// engine, scheduler and trap listener in one binary.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"printmarshal/internal/alerts"
	"printmarshal/internal/api"
	"printmarshal/internal/auth"
	"printmarshal/internal/config"
	"printmarshal/internal/configmgmt"
	"printmarshal/internal/driver/escl"
	"printmarshal/internal/driver/fwsim"
	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/events"
	"printmarshal/internal/firmware"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins"
	"printmarshal/internal/secrets"
	"printmarshal/internal/security"
	"printmarshal/internal/store"
	"printmarshal/internal/trap"
	"printmarshal/internal/web"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", os.Getenv("PM_CONFIG"), "path to config JSON (optional)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("printmarshal", version)
		return
	}
	if err := run(*cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("PM_LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	if strings.ToLower(os.Getenv("PM_LOG_FORMAT")) == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func run(cfgPath string) error {
	log := newLogger()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	log.Info("printmarshal starting", "version", version, "listen", cfg.Listen, "database", cfg.Database, "dsn", cfg.DSN)

	key, err := secrets.LoadOrCreateKey(os.Getenv("PM_SECRET_KEY"), filepath.Join(cfg.DataDir, "secret.key"))
	if err != nil {
		return err
	}
	box, err := secrets.New(key)
	if err != nil {
		return err
	}

	var st store.Store
	switch cfg.Database {
	case "", "sqlite":
		st, err = store.OpenSQLite(cfg.DSN, box)
	default:
		err = fmt.Errorf("datastore %q is not implemented yet (sqlite is the default)", cfg.Database)
	}
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if n, err := st.MarkInterruptedJobs(ctx); err == nil && n > 0 {
		log.Warn("marked jobs interrupted by previous shutdown", "count", n)
	}

	// Device engine: standards drivers. Vendor plugins register here too.
	reg := engine.NewRegistry()
	snmpOpts := snmp.DefaultOptions()
	if cfg.Poll.SNMPPort > 0 {
		snmpOpts.Port = uint16(cfg.Poll.SNMPPort)
	}
	snmpOpts.Timeout = cfg.Poll.SNMPTimeout.D()
	snmpOpts.Retries = cfg.Poll.SNMPRetries
	snmpDial := snmp.NewDialer(snmpOpts)
	reg.Register(snmp.New(snmpDial))
	ippOpts := ipp.DefaultOptions()
	ippOpts.Timeout = cfg.Poll.IPPTimeout.D()
	ippOpts.InsecureTLS = cfg.Poll.IPPInsecureTLS
	ippOpts.Endpoints = cfg.Poll.IPPEndpoints
	reg.Register(ipp.New(ipp.NewTransport(ippOpts), ippOpts))
	esclOpts := escl.DefaultOptions()
	esclOpts.Timeout = cfg.Poll.IPPTimeout.D()
	esclOpts.InsecureTLS = cfg.Poll.IPPInsecureTLS
	reg.Register(escl.New(escl.NewTransport(esclOpts), esclOpts))
	if v := os.Getenv("PM_FIRMWARE_SIM_TARGETS"); v != "" { // simulator-only firmware delivery (docs/firmware.md)
		reg.Register(fwsim.New(strings.Split(v, ","), fwsim.Options{}))
	}
	plugins.RegisterAll(reg, log) // vendor plugins: enrichment at the edge (spec §6)
	eng := engine.New(reg, log, cfg.Poll.Concurrency)

	bus := events.NewBus() // single event contract: live stream + durable notifications (docs/events.md)
	bus.Unknown = func(t string) { log.Warn("event type is not in the catalogue", "type", t) }
	hub := api.NewHub()
	hub.Origins = cfg.Auth.AllowedOrigins
	bus.Subscribe(hub)
	notifier := alerts.NewNotifier(cfg.Notifiers, st, log)
	notifier.Attach(bus)
	go notifier.Run(ctx)
	runner := jobs.New(st, log, cfg.Jobs.Workers, bus.Publish)
	defer runner.Stop()

	// Cheap liveness probe for active scans: one SNMP system-group read.
	probeOpts := snmp.Options{Port: snmpOpts.Port, Timeout: 1 * time.Second, Retries: 1}
	probeDial := snmp.NewDialer(probeOpts)
	probe := func(ctx context.Context, addr string) bool {
		creds, _ := st.ResolveCreds(ctx, "", nil)
		s, _, err := probeDial(ctx, addr, creds)
		if err != nil {
			return false
		}
		_ = s.Close()
		return true
	}

	fl := fleet.New(st, eng, runner, cfg, log, bus.Publish, probe)
	deps := fl.DepsOf()
	cfgSvc := configmgmt.New(deps)
	secSvc := security.New(deps)
	fwSvc := firmware.New(deps)

	authSvc := auth.New(st, cfg.Auth.SessionTTL.D(), cfg.Auth.SecureCookies, log)
	authSvc.PasswordFile = filepath.Join(cfg.DataDir, "initial-admin-password")
	if !cfg.Auth.SecureCookies && !isLoopback(cfg.Listen) {
		log.Warn("auth.secureCookies is off while listening on a non-loopback address: put PrintMarshal behind TLS and set PM_SECURE_COOKIES=true (docs/security.md)", "listen", cfg.Listen)
	}
	if err := authSvc.Bootstrap(ctx, os.Getenv("PM_ADMIN_PASSWORD")); err != nil {
		return err
	}

	if cfg.Trap.Enabled {
		tl := trap.New(cfg.Trap.Listen, fl, log)
		allowed, err := trap.ParseSources(cfg.Trap.AllowedSources)
		if err != nil {
			return err
		}
		tl.Allowed, tl.RatePerMinute = allowed, cfg.Trap.RatePerMinute
		go func() {
			if err := tl.Run(ctx); err != nil {
				log.Error("trap listener stopped", "err", err)
			}
		}()
	}
	go fl.RunScheduler(ctx)
	go func() { // certificate expiry check: at start and then daily (docs/security.md)
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			secSvc.CheckExpiry(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	srv := &api.Server{Store: st, Fleet: fl, ConfigMgmt: cfgSvc, Security: secSvc, Firmware: fwSvc, Auth: authSvc, Hub: hub, Bus: bus, Notifier: notifier, Cfg: cfg, Log: log, UI: web.Handler(), Version: version}
	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", cfg.Listen)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	bus.Publish(model.Event{Type: "server.stopping"})
	return nil
}

// isLoopback reports whether a listen address binds only to localhost.
func isLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
