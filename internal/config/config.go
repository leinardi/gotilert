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

// Package config provides YAML configuration loading and validation for Gotilert.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultAlertName = "GotilertNotification"

	// Logging formats.
	logFormatPlain = "plain"
	logFormatText  = "text"
	logFormatJSON  = "json"

	// Logging levels.
	logLevelDebug   = "debug"
	logLevelInfo    = "info"
	logLevelWarn    = "warn"
	logLevelWarning = "warning"
	logLevelError   = "error"
	logLevelFatal   = "fatal"
	logLevelPanic   = "panic"

	// Severity canonical values.
	severityInfo     = "info"
	severityWarning  = "warning"
	severityCritical = "critical"

	// Severity aliases accepted in config.
	severityAliasWarn = "warn"
	severityAliasCrit = "crit"
)

var (
	ErrConfigFilePathEmpty          = errors.New("config file path is empty")
	ErrConfigNil                    = errors.New("config is nil")
	ErrDurationNilNode              = errors.New("duration yaml node is nil")
	ErrDurationExpectedScalar       = errors.New("duration yaml node must be a scalar")
	ErrAlertmanagerURLRequired      = errors.New("alertmanager.url is required")
	ErrAlertmanagerURLParse         = errors.New("alertmanager.url parse failed")
	ErrAlertmanagerURLInvalidScheme = errors.New("alertmanager.url must use http or https scheme")
	ErrAlertmanagerURLMissingHost   = errors.New("alertmanager.url must include host")
	ErrAlertmanagerBasicAuthUser    = errors.New(
		"alertmanager.basicAuth.username is required when basicAuth is set",
	)
	ErrAlertmanagerBasicAuthPass = errors.New(
		"alertmanager.basicAuth.password is required when basicAuth is set",
	)
	ErrAlertmanagerAuthExclusive = errors.New(
		"alertmanager.basicAuth and alertmanager.bearerToken are mutually exclusive",
	)
	ErrAlertmanagerTimeoutNegative = errors.New("alertmanager.timeout must be >= 0")

	ErrDefaultsSeverityMapRequired = errors.New(
		"defaults.severityFromPriority is required and must be non-empty",
	)
	ErrDefaultsTTLNonPositive = errors.New("defaults.ttl must be > 0")
	ErrPriorityNegative       = errors.New("priority must be >= 0")
	ErrInvalidSeverity        = errors.New(
		"invalid severity (allowed: info, warning, critical)",
	)

	ErrAppsEmptyTokenKey   = errors.New("apps contains an empty token key")
	ErrAppsAppNameRequired = errors.New("apps appName is required")

	ErrLoggingLevelInvalid  = errors.New("logging.level is invalid")
	ErrLoggingFormatInvalid = errors.New("logging.format is invalid (allowed: plain, text, json)")

	ErrServerTimeoutNegative = errors.New("server timeouts must be >= 0")
)

type Config struct {
	Server       ServerConfig         `yaml:"server"`
	Logging      LoggingConfig        `yaml:"logging"`
	Alertmanager AlertmanagerConfig   `yaml:"alertmanager"`
	Defaults     DefaultsConfig       `yaml:"defaults"`
	Apps         map[string]AppConfig `yaml:"apps"`
}

type ServerConfig struct {
	ListenAddr      string   `yaml:"listenAddr"`
	ReadTimeout     Duration `yaml:"readTimeout"`
	WriteTimeout    Duration `yaml:"writeTimeout"`
	IdleTimeout     Duration `yaml:"idleTimeout"`
	ShutdownTimeout Duration `yaml:"shutdownTimeout"`
}

type LoggingConfig struct {
	Format      string `yaml:"format"`
	Level       string `yaml:"level"`
	IncludeTime bool   `yaml:"includeTime"`
}

type AlertmanagerConfig struct {
	URL       string     `yaml:"url"`
	BasicAuth *BasicAuth `yaml:"basicAuth"`
	Bearer    string     `yaml:"bearerToken"`
	TLSConfig TLSConfig  `yaml:"tlsConfig"`
	Timeout   Duration   `yaml:"timeout"`
}

type TLSConfig struct {
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type DefaultsConfig struct {
	AlertName            string            `yaml:"alertname"`
	TTL                  Duration          `yaml:"ttl"`
	SeverityFromPriority map[int]string    `yaml:"severityFromPriority"`
	Labels               map[string]string `yaml:"labels"`
}

type AppConfig struct {
	AppName              string            `yaml:"appName"`
	AlertName            string            `yaml:"alertname"`
	Labels               map[string]string `yaml:"labels"`
	SeverityFromPriority map[int]string    `yaml:"severityFromPriority"`
}

type Duration struct {
	time.Duration
}

func (duration *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		return ErrDurationNilNode
	}

	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%w: kind=%d", ErrDurationExpectedScalar, node.Kind)
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(node.Value))
	if err != nil {
		return fmt.Errorf("duration parse %q: %w", node.Value, err)
	}

	duration.Duration = parsed

	return nil
}

// LoadFile loads, validates, and returns configuration from a YAML file.
func LoadFile(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrConfigFilePathEmpty
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	err = cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("validate config file %q: %w", path, err)
	}

	return &cfg, nil
}

func (cfg *Config) Validate() error {
	if cfg == nil {
		return ErrConfigNil
	}

	err := cfg.validateServer()
	if err != nil {
		return err
	}

	err = cfg.validateLogging()
	if err != nil {
		return err
	}

	err = cfg.validateAlertmanager()
	if err != nil {
		return err
	}

	err = cfg.validateDefaults()
	if err != nil {
		return err
	}

	err = cfg.validateApps()
	if err != nil {
		return err
	}

	return nil
}

func (cfg *Config) validateServer() error {
	if cfg.Server.ReadTimeout.Duration < 0 {
		return ErrServerTimeoutNegative
	}

	if cfg.Server.WriteTimeout.Duration < 0 {
		return ErrServerTimeoutNegative
	}

	if cfg.Server.IdleTimeout.Duration < 0 {
		return ErrServerTimeoutNegative
	}

	if cfg.Server.ShutdownTimeout.Duration < 0 {
		return ErrServerTimeoutNegative
	}

	return nil
}

func (cfg *Config) validateLogging() error {
	// All logging fields are optional; when set, validate.
	format := strings.TrimSpace(cfg.Logging.Format)
	if format != "" {
		switch strings.ToLower(format) {
		case logFormatPlain, logFormatText, logFormatJSON:
			// ok
		default:
			return fmt.Errorf("%w: %q", ErrLoggingFormatInvalid, cfg.Logging.Format)
		}
	}

	level := strings.TrimSpace(cfg.Logging.Level)
	if level != "" {
		switch strings.ToLower(level) {
		case logLevelDebug,
			logLevelInfo,
			logLevelWarn,
			logLevelWarning,
			logLevelError,
			logLevelFatal,
			logLevelPanic:
			// ok (fatal/panic will be normalized by logger package later)
		default:
			return fmt.Errorf("%w: %q", ErrLoggingLevelInvalid, cfg.Logging.Level)
		}
	}

	return nil
}

func (cfg *Config) validateAlertmanager() error {
	if strings.TrimSpace(cfg.Alertmanager.URL) == "" {
		return ErrAlertmanagerURLRequired
	}

	parsed, err := url.Parse(cfg.Alertmanager.URL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAlertmanagerURLParse, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: %q", ErrAlertmanagerURLInvalidScheme, parsed.Scheme)
	}

	if strings.TrimSpace(parsed.Host) == "" {
		return ErrAlertmanagerURLMissingHost
	}

	// Auth is optional (may be absent entirely).
	if cfg.Alertmanager.BasicAuth != nil {
		if strings.TrimSpace(cfg.Alertmanager.BasicAuth.Username) == "" {
			return ErrAlertmanagerBasicAuthUser
		}

		if strings.TrimSpace(cfg.Alertmanager.BasicAuth.Password) == "" {
			return ErrAlertmanagerBasicAuthPass
		}
	}

	if cfg.Alertmanager.BasicAuth != nil && strings.TrimSpace(cfg.Alertmanager.Bearer) != "" {
		return ErrAlertmanagerAuthExclusive
	}

	if cfg.Alertmanager.Timeout.Duration < 0 {
		return ErrAlertmanagerTimeoutNegative
	}

	return nil
}

func (cfg *Config) validateDefaults() error {
	if len(cfg.Defaults.SeverityFromPriority) == 0 {
		return ErrDefaultsSeverityMapRequired
	}

	if strings.TrimSpace(cfg.Defaults.AlertName) == "" {
		cfg.Defaults.AlertName = DefaultAlertName
	}

	for priority, severity := range cfg.Defaults.SeverityFromPriority {
		if priority < 0 {
			return fmt.Errorf(
				"defaults.severityFromPriority: %w: %d",
				ErrPriorityNegative,
				priority,
			)
		}

		err := validateSeverity(severity)
		if err != nil {
			return fmt.Errorf("defaults.severityFromPriority[%d]: %w", priority, err)
		}

		cfg.Defaults.SeverityFromPriority[priority] = canonicalSeverity(severity)
	}

	if cfg.Defaults.TTL.Duration <= 0 {
		return ErrDefaultsTTLNonPositive
	}

	return nil
}

func (cfg *Config) validateApps() error {
	for token, app := range cfg.Apps {
		if strings.TrimSpace(token) == "" {
			return ErrAppsEmptyTokenKey
		}

		if strings.TrimSpace(app.AppName) == "" {
			return fmt.Errorf("%w: %s", ErrAppsAppNameRequired, tokenKeyForError(token))
		}

		if strings.TrimSpace(app.AlertName) == "" {
			// Optional; leave empty so runtime can fall back to defaults.
			app.AlertName = ""
		}

		err := normalizeSeverityMap(app.SeverityFromPriority, "apps", tokenKeyForError(token))
		if err != nil {
			return err
		}

		cfg.Apps[token] = app
	}

	return nil
}

func normalizeSeverityMap(
	mapping map[int]string,
	section string,
	tokenRedaction string,
) error {
	if len(mapping) == 0 {
		return nil
	}

	for prio, sev := range mapping {
		if prio < 0 {
			return fmt.Errorf(
				"%s[%s].severityFromPriority: %w: %d",
				section,
				tokenRedaction,
				ErrPriorityNegative,
				prio,
			)
		}

		err := validateSeverity(sev)
		if err != nil {
			return fmt.Errorf(
				"%s[%s].severityFromPriority[%d]: %w",
				section,
				tokenRedaction,
				prio,
				err,
			)
		}

		mapping[prio] = canonicalSeverity(sev)
	}

	return nil
}

func canonicalSeverity(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case severityAliasWarn, severityWarning:
		return severityWarning
	case severityAliasCrit, severityCritical:
		return severityCritical
	default:
		return severityInfo
	}
}

func validateSeverity(input string) error {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case severityInfo,
		severityAliasWarn, severityWarning,
		severityAliasCrit, severityCritical:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSeverity, input)
	}
}

func tokenKeyForError(token string) string {
	// Don’t echo secrets (tokens) in errors. Use a stable redaction.
	return fmt.Sprintf("token(len=%d)", len(token))
}
