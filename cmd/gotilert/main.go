/*
 * MIT License
 *
 * Copyright (c) 2025 Roberto Leinardi
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/leinardi/gotilert/internal/alertmanager"
	"github.com/leinardi/gotilert/internal/config"
	"github.com/leinardi/gotilert/internal/gotify"
	"github.com/leinardi/gotilert/internal/logger"
	"github.com/leinardi/gotilert/internal/metrics"
	"github.com/leinardi/gotilert/internal/server"
)

const exitCodeError = 1

const (
	defaultReadTimeout     = 5 * time.Second
	defaultWriteTimeout    = 10 * time.Second
	defaultIdleTimeout     = 60 * time.Second
	defaultShutdownTimeout = 10 * time.Second

	defaultReadyTimeout = 2 * time.Second
)

type cliOptions struct {
	showVersion bool
	configFile  string

	logFormat string
	logLevel  string
	logTime   bool

	overrides map[string]bool
}

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		logger.L().Error("exiting with error", "err", err)

		os.Exit(exitCodeError)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	options, err := parseCLI(args, stderr)
	if err != nil {
		return err
	}

	// Preliminary logger from CLI defaults/overrides so config errors are emitted consistently.
	logger.Configure(options.logFormat, options.logLevel, options.logTime)

	if options.showVersion {
		err = printVersion(stdout)
		if err != nil {
			return err
		}

		return nil
	}

	logger.L().Info("starting gotilert", "version", version, "commit", commit, "date", date)

	cfg, err := loadConfigOrExit(options.configFile)
	if err != nil {
		if errors.Is(err, ErrConfigFileMissing) {
			// No config provided -> current behavior: do not start server.
			return nil
		}

		return err
	}

	applyLoggingConfig(cfg, options)

	httpServer, shutdownTimeout, err := buildHTTPServer(cfg)
	if err != nil {
		return err
	}

	err = runHTTPServer(httpServer, shutdownTimeout)
	if err != nil {
		return err
	}

	return nil
}

func buildHTTPServer(cfg *config.Config) (*http.Server, time.Duration, error) {
	readTimeout := pickDuration(cfg.Server.ReadTimeout.Duration, defaultReadTimeout)
	writeTimeout := pickDuration(cfg.Server.WriteTimeout.Duration, defaultWriteTimeout)
	idleTimeout := pickDuration(cfg.Server.IdleTimeout.Duration, defaultIdleTimeout)
	shutdownTimeout := pickDuration(cfg.Server.ShutdownTimeout.Duration, defaultShutdownTimeout)

	resolveApp := newResolveAppFunc(cfg)

	amClient, err := newAlertmanagerClient(cfg)
	if err != nil {
		return nil, 0, err
	}

	metricsCollector := metrics.New()

	readyFunc := func() (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultReadyTimeout)
		defer cancel()

		readyErr := amClient.Ready(ctx)
		if readyErr != nil {
			return false, readyErr.Error()
		}

		return true, ""
	}

	forward := newForwarder(cfg, amClient, metricsCollector)

	httpServer, err := server.New(&server.Options{
		Addr:            cfg.Server.ListenAddr,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		IdleTimeout:     idleTimeout,
		ShutdownTimeout: shutdownTimeout,
		MaxBodyBytes:    1 << 20, // 1 MiB

		Health: func() (bool, string) { return true, "" },
		Ready:  readyFunc,

		ResolveApp:     resolveApp,
		ForwardMessage: forward,

		Metrics: metricsCollector,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("create http server: %w", err)
	}

	return httpServer, shutdownTimeout, nil
}

func newResolveAppFunc(cfg *config.Config) server.ResolveAppFunc {
	apps := make(map[string]server.App, len(cfg.Apps))

	for token, app := range cfg.Apps {
		apps[token] = server.App{
			Name:                 app.AppName,
			ID:                   appIDFromName(app.AppName),
			AlertName:            strings.TrimSpace(app.AlertName),
			Labels:               copyLabels(app.Labels),
			SeverityFromPriority: copySeverityMap(app.SeverityFromPriority),
		}
	}

	return func(token string) (server.App, bool) {
		app, ok := apps[token]

		return app, ok
	}
}

func copySeverityMap(input map[int]string) map[int]string {
	out := make(map[int]string, len(input))
	maps.Copy(out, input)

	return out
}

func appIDFromName(appName string) uint32 {
	// Small deterministic hash (FNV-1a 32-bit) without importing hash/fnv here.
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)

	hash := uint32(offset32)
	for i := range len(appName) {
		hash ^= uint32(appName[i])
		hash *= prime32
	}

	return hash
}

func newAlertmanagerClient(cfg *config.Config) (*alertmanager.Client, error) {
	auth := alertmanager.Auth{}

	if cfg.Alertmanager.BasicAuth != nil {
		auth.BasicUsername = cfg.Alertmanager.BasicAuth.Username
		auth.BasicPassword = cfg.Alertmanager.BasicAuth.Password
	}

	auth.BearerToken = cfg.Alertmanager.Bearer

	client, err := alertmanager.New(&alertmanager.Options{
		BaseURL:            cfg.Alertmanager.URL,
		Timeout:            cfg.Alertmanager.Timeout.Duration,
		InsecureSkipVerify: cfg.Alertmanager.TLSConfig.InsecureSkipVerify,
		Auth:               auth,
	})
	if err != nil {
		return nil, fmt.Errorf("create alertmanager client: %w", err)
	}

	return client, nil
}

func newForwarder(
	cfg *config.Config,
	amClient *alertmanager.Client,
	metricsCollector *metrics.Metrics,
) server.ForwardMessageFunc {
	ttl := cfg.Defaults.TTL.Duration
	defaultLabels := copyLabels(cfg.Defaults.Labels)
	defaultSeverityMap := cfg.Defaults.SeverityFromPriority
	defaultAlertName := cfg.Defaults.AlertName

	return func(ctx context.Context, app server.App, msg gotify.MessageRequest, messageIdentifier uint64) error {
		severityMap := defaultSeverityMap
		if len(app.SeverityFromPriority) > 0 {
			severityMap = app.SeverityFromPriority
		}

		alertName := defaultAlertName
		if strings.TrimSpace(app.AlertName) != "" {
			alertName = strings.TrimSpace(app.AlertName)
		}

		severity := severityForPriority(severityMap, msg.Priority)

		// Merge: defaults.labels + app.labels + computed labels (computed wins).
		labels := copyLabels(defaultLabels)
		mergeStringMap(labels, app.Labels)

		labels["alertname"] = alertName
		labels["app"] = app.Name
		labels["severity"] = severity
		labels["priority"] = strconv.Itoa(msg.Priority)
		labels["gotilert_id"] = strconv.FormatUint(messageIdentifier, 10)

		annotations := map[string]string{
			"summary":     pickSummary(app.Name, msg.Title, msg.Message),
			"description": msg.Message,
		}

		mergeStringMap(annotations, gotify.ExtrasAnnotations(msg.Extras))

		now := time.Now().UTC()
		alert := alertmanager.Alert{
			Labels:      labels,
			Annotations: annotations,
			StartsAt:    now,
			EndsAt:      now.Add(ttl),
		}

		forwardCtx, cancel := withBoundedTimeout(ctx, cfg.Alertmanager.Timeout.Duration)
		defer cancel()

		postErr := amClient.PostAlerts(forwardCtx, []alertmanager.Alert{alert})
		if postErr != nil {
			if metricsCollector != nil {
				metricsCollector.IncUpstreamFailure(app.Name)
			}

			// Make auth/upstream issues debuggable (e.g., 401 with WWW-Authenticate).
			logArgs := []any{
				"err", postErr,
				"app", app.Name,
				"upstream", cfg.Alertmanager.URL,
			}

			var stErr alertmanager.HTTPStatusError
			if errors.As(postErr, &stErr) {
				logArgs = append(logArgs,
					"upstream_status", stErr.StatusCode(),
					"upstream_body", stErr.Body(),
				)
			}

			logger.L().Error("forward to alertmanager failed", logArgs...)

			return fmt.Errorf("post alert: %w", postErr)
		}

		if metricsCollector != nil {
			metricsCollector.IncForwarded(app.Name)
		}

		return nil
	}
}

func mergeStringMap(dst, src map[string]string) {
	if len(src) == 0 {
		return
	}

	maps.Copy(dst, src)
}

func severityForPriority(mapping map[int]string, priority int) string {
	if sev, ok := mapping[priority]; ok {
		return sev
	}

	// Choose the closest lower key if possible; otherwise the smallest key.
	bestKey := 0
	bestSet := false

	for key := range mapping {
		if !bestSet {
			bestKey = key
			bestSet = true

			continue
		}

		if key <= priority && bestKey <= priority {
			if key > bestKey {
				bestKey = key
			}

			continue
		}

		if bestKey > priority && key < bestKey {
			bestKey = key
		}
	}

	if sev, ok := mapping[bestKey]; ok {
		return sev
	}

	return "info"
}

func copyLabels(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	maps.Copy(out, input)

	return out
}

func pickSummary(appName, title, message string) string {
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle != "" {
		return trimmedTitle
	}

	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return appName
	}

	const maxLen = 120
	if len(trimmedMessage) <= maxLen {
		return trimmedMessage
	}

	return trimmedMessage[:maxLen] + "…"
}

func runHTTPServer(httpServer *http.Server, shutdownTimeout time.Duration) error {
	errorChan := make(chan error, 1)

	go func() {
		errorChan <- server.ListenAndServe(httpServer)
	}()

	logger.L().Info("http server listening", "addr", httpServer.Addr)

	signalChan := make(chan os.Signal, 1)

	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalChan)

	select {
	case sig := <-signalChan:
		logger.L().Info("shutdown requested", "signal", sig.String())

		ctx := context.Background()

		err := server.Shutdown(ctx, httpServer, shutdownTimeout)
		if err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}

		logger.L().Info("shutdown complete")

		return nil

	case err := <-errorChan:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("http server error: %w", err)
	}
}

func parseCLI(args []string, stderr io.Writer) (cliOptions, error) {
	flagSet := flag.NewFlagSet("gotilert", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	showVersion := flagSet.Bool("version", false, "Print version information and exit.")
	configFile := flagSet.String("config.file", "", "Path to gotilert YAML configuration file.")

	logFormat := flagSet.String("log-format", "plain", "Log format: plain, text, json.")
	logLevel := flagSet.String("log-level", "info", "Log level: debug, info, warn, error.")
	logTime := flagSet.Bool("log-time", false, "Include time field in logs.")

	err := flagSet.Parse(args)
	if err != nil {
		return cliOptions{}, fmt.Errorf("parse flags: %w", err)
	}

	overrides := make(map[string]bool)

	flagSet.Visit(func(f *flag.Flag) {
		overrides[f.Name] = true
	})

	return cliOptions{
		showVersion: *showVersion,
		configFile:  *configFile,
		logFormat:   *logFormat,
		logLevel:    *logLevel,
		logTime:     *logTime,
		overrides:   overrides,
	}, nil
}

func loadConfigOrExit(configFile string) (*config.Config, error) {
	if configFile == "" {
		logger.L().
			Info("no config file provided; cannot start server without config", "flag", "config.file")

		return nil, ErrConfigFileMissing
	}

	cfg, err := config.LoadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	logger.L().Info("configuration loaded", "path", configFile, "apps", len(cfg.Apps))

	return cfg, nil
}

func applyLoggingConfig(cfg *config.Config, options cliOptions) {
	effectiveFormat := options.logFormat
	effectiveLevel := options.logLevel
	effectiveIncludeTime := options.logTime

	if !options.overrides["log-format"] && cfg.Logging.Format != "" {
		effectiveFormat = cfg.Logging.Format
	}

	if !options.overrides["log-level"] && cfg.Logging.Level != "" {
		effectiveLevel = cfg.Logging.Level
	}

	if !options.overrides["log-time"] {
		effectiveIncludeTime = cfg.Logging.IncludeTime
	}

	if effectiveFormat == options.logFormat && effectiveLevel == options.logLevel &&
		effectiveIncludeTime == options.logTime {
		return
	}

	logger.Configure(effectiveFormat, effectiveLevel, effectiveIncludeTime)
	logger.L().Info("logger configured from config (unless overridden by CLI)",
		"format", effectiveFormat,
		"level", effectiveLevel,
		"includeTime", effectiveIncludeTime,
	)
}

func printVersion(writer io.Writer) error {
	if writer == nil {
		return ErrNilStdoutWriter
	}

	_, err := fmt.Fprintf(writer, "gotilert version=%s commit=%s date=%s\n", version, commit, date)
	if err != nil {
		return fmt.Errorf("print version: %w", err)
	}

	return nil
}

func pickDuration(value, fallback time.Duration) time.Duration {
	if value == 0 {
		return fallback
	}

	return value
}

func withBoundedTimeout(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}

	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining <= timeout {
			return context.WithCancel(parent)
		}
	}

	return context.WithTimeout(parent, timeout)
}
