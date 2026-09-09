// Command simple-status is a self-hosted status page with automated
// service checks. Configuration comes from a single YAML file; CLI flags
// override the root settings. There is no web management interface.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"simple-status/internal/api"
	"simple-status/internal/config"
	"simple-status/internal/monitor"
	"simple-status/internal/prober"
	"simple-status/internal/store"
)

// version is overridden at build time via -ldflags "-X main.version=…".
var version = "dev"

const defaultConfigPath = "simple-status.yml"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "simple-status: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath = flag.String("config", defaultConfigPath, "path to the YAML configuration file")
		listen     = flag.String("listen", "", "listen address, overrides the config file (default \":8080\")")
		interval   = flag.Duration("interval", 0, "default check interval, overrides the config file (e.g. 30s)")
		timeout    = flag.Duration("timeout", 0, "default check timeout, overrides the config file (e.g. 10s)")
		validate   = flag.Bool("validate", false, "validate the configuration and exit")
		once       = flag.Bool("once", false, "check every service once, print the results and exit")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"simple-status %s — a status page with automated service checks\n\n"+
				"Usage: simple-status [flags]\n\nFlags:\n", version)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Printf("simple-status %s\n", version)
		return nil
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath, config.Overrides{
		Listen:   *listen,
		Interval: *interval,
		Timeout:  *timeout,
	})
	if err != nil {
		return err
	}
	if err := cfg.Validate(prober.Types()); err != nil {
		return fmt.Errorf("invalid configuration in %s: %w", *configPath, err)
	}
	if *validate {
		fmt.Println("configuration OK")
		return nil
	}

	st := store.New(cfg)
	mon, err := monitor.New(cfg, st, log)
	if err != nil {
		return err
	}

	if *once {
		return runOnce(mon, st)
	}
	return serve(cfg, st, mon, log)
}

// runOnce probes all services a single time and prints a result table.
// The exit code reflects the check outcomes (not the retry-confirmed
// status): nil only if every check succeeded.
func runOnce(mon *monitor.Monitor, st *store.Store) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	mon.CheckOnce(ctx)

	sum := st.Summary(time.Now())
	var failed []string
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	for _, svc := range sum.Services {
		mark := "✗"
		if lastCheckSucceeded(svc) {
			mark = "✓"
		} else {
			failed = append(failed, svc.ID)
		}
		detail := svc.LastError
		if detail == "" && len(svc.History) > 0 {
			detail = svc.History[len(svc.History)-1].Message
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t(%s)\n", mark, svc.ID, svc.LastLatency.Round(time.Millisecond), detail)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(failed) > 0 {
		return fmt.Errorf("checks failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// lastCheckSucceeded reports whether the most recent recorded check passed;
// a service without any history counts as failed.
func lastCheckSucceeded(svc store.ServiceSnapshot) bool {
	n := len(svc.History)
	return n > 0 && svc.History[n-1].Success
}

// serve runs the check scheduler and the HTTP server until a termination
// signal arrives, then shuts both down gracefully.
func serve(cfg *config.Config, st *store.Store, mon *monitor.Monitor, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		mon.Run(ctx)
	}()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(cfg, st),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("status page listening", "addr", cfg.Listen, "services", len(cfg.Services))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("bye")
	return nil
}
