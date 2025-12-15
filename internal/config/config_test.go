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

package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/leinardi/gotilert/internal/config"
)

func TestValidateDefaultsSeverityMapRequired(t *testing.T) {
	t.Parallel()

	cfg := minimalValidConfig()
	cfg.Defaults.SeverityFromPriority = nil

	err := cfg.Validate()
	if !errors.Is(err, config.ErrDefaultsSeverityMapRequired) {
		t.Fatalf("expected ErrDefaultsSeverityMapRequired, got: %v", err)
	}
}

func TestValidateSetsDefaultAlertName(t *testing.T) {
	t.Parallel()

	cfg := minimalValidConfig()
	cfg.Defaults.AlertName = ""

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Defaults.AlertName != config.DefaultAlertName {
		t.Fatalf(
			"expected defaults.alertname=%q, got %q",
			config.DefaultAlertName,
			cfg.Defaults.AlertName,
		)
	}
}

func TestValidateAppsAppNameRequired(t *testing.T) {
	t.Parallel()

	cfg := minimalValidConfig()
	cfg.Apps = map[string]config.AppConfig{
		"TOKEN": {AppName: ""},
	}

	err := cfg.Validate()
	if !errors.Is(err, config.ErrAppsAppNameRequired) {
		t.Fatalf("expected ErrAppsAppNameRequired, got: %v", err)
	}
}

func TestValidateAppsNormalizesSeverityMap(t *testing.T) {
	t.Parallel()

	cfg := minimalValidConfig()
	cfg.Apps = map[string]config.AppConfig{
		"TOKEN": {
			AppName: "truenas",
			SeverityFromPriority: map[int]string{
				0: "INFO",
				2: "warn",
				5: "CRIT",
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	app := cfg.Apps["TOKEN"]

	if got := app.SeverityFromPriority[0]; got != "info" {
		t.Fatalf("expected priority 0 severity %q, got %q", "info", got)
	}

	if got := app.SeverityFromPriority[2]; got != "warning" {
		t.Fatalf("expected priority 2 severity %q, got %q", "warning", got)
	}

	if got := app.SeverityFromPriority[5]; got != "critical" {
		t.Fatalf("expected priority 5 severity %q, got %q", "critical", got)
	}
}

func TestValidateDefaultsTTLMustBePositive(t *testing.T) {
	t.Parallel()

	cfg := minimalValidConfig()
	cfg.Defaults.TTL = config.Duration{Duration: 0}

	err := cfg.Validate()
	if !errors.Is(err, config.ErrDefaultsTTLNonPositive) {
		t.Fatalf("expected ErrDefaultsTTLNonPositive, got: %v", err)
	}
}

func minimalValidConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			ListenAddr: "0.0.0.0:8008",
		},
		Alertmanager: config.AlertmanagerConfig{
			URL: "http://alertmanager.example.local",
		},
		Defaults: config.DefaultsConfig{
			TTL: config.Duration{Duration: 1 * time.Hour},
			SeverityFromPriority: map[int]string{
				0: "info",
				1: "warning",
				2: "critical",
			},
			Labels: map[string]string{
				"source": "gotilert",
			},
		},
		Apps: map[string]config.AppConfig{},
	}
}
