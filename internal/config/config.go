// Package config defines the runtime configuration of simple-status and
// loads it from a single YAML file, layered with CLI flag overrides.
//
// Precedence, from strongest to weakest:
//
//  1. CLI flags (--listen, --interval, --timeout)
//  2. Values set in the configuration file, including per-service values
//     (a service-level interval beats the root interval, which beats a flag)
//  3. Built-in defaults (see the Default* constants)
package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Built-in defaults for every setting left unset by the file and the flags.
const (
	DefaultTitle    = "Status"
	DefaultListen   = ":8080"
	DefaultInterval = 30 * time.Second
	DefaultTimeout  = 10 * time.Second
	DefaultRetries  = 1
	DefaultHistory  = 720
)

// Duration wraps time.Duration so that YAML configuration accepts Go
// duration strings such as "30s" or "1m" instead of raw nanoseconds.
type Duration struct{ time.Duration }

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: duration must be a string such as \"30s\"", node.Line)
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("line %d: invalid duration %q", node.Line, node.Value)
	}
	d.Duration = parsed
	return nil
}

// Config is the root configuration document.
type Config struct {
	// Title is shown in the browser and document title of the status page.
	Title string `yaml:"title"`
	// Listen is the address the status page and API are served on.
	Listen string `yaml:"listen"`
	// Interval is the default time between two checks of a service.
	Interval Duration `yaml:"interval"`
	// Timeout is the default deadline for a single check.
	Timeout Duration `yaml:"timeout"`
	// Retries is the default number of consecutive failed checks before a
	// service is considered down.
	Retries int `yaml:"retries"`
	// History is the number of results kept per service (in memory).
	History int `yaml:"history"`
	// Services is the list of monitored services; order is preserved on the page.
	Services []Service `yaml:"services"`
}

// Service describes one monitored service. Zero-valued optional fields
// inherit the root configuration values during normalization.
type Service struct {
	// ID uniquely identifies the service; it is used in API paths and must
	// be URL-safe ([a-zA-Z0-9][a-zA-Z0-9_-]*).
	ID string `yaml:"id"`
	// Name is the human-readable label shown on the status page; defaults to ID.
	Name   string `yaml:"name"`
	Group  string `yaml:"group"`
	Type   string `yaml:"type"`   // probe type, e.g. http, tcp, dns
	Target string `yaml:"target"` // probe-specific target, e.g. URL or host:port

	// Per-service overrides of the root defaults (0 means "inherit").
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
	Retries  int      `yaml:"retries"`

	// HTTP options (type: http).
	Method           string            `yaml:"method"`            // default GET
	Headers          map[string]string `yaml:"headers"`           // extra request headers
	ExpectedStatuses []int             `yaml:"expected_statuses"` // default: any 2xx
	BodyPattern      string            `yaml:"body_pattern"`      // optional regex the body must match
	InsecureTLS      bool              `yaml:"insecure_tls"`      // skip certificate verification

	// DNS options (type: dns).
	Resolver   string `yaml:"resolver"` // optional DNS server host[:port]
	RecordType string `yaml:"record"`   // A (default), AAAA, CNAME, MX, NS, TXT
}

// Overrides carries CLI flag values that take precedence over root
// configuration values. The zero value applies no overrides. Per-service
// values from the file always win over flags.
type Overrides struct {
	Listen   string
	Interval time.Duration
	Timeout  time.Duration
}

// Load reads the configuration file at path, applies flag overrides, fills
// in defaults and normalizes service inheritance. Structural validation is
// performed by Validate; probe types are validated against knownTypes.
func Load(path string, o Overrides) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.applyOverrides(o)
	c.normalize()
	return &c, nil
}

// applyOverrides layers non-zero flag values over the root configuration.
// It runs before normalization so that services which set their own values
// keep them.
func (c *Config) applyOverrides(o Overrides) {
	if o.Listen != "" {
		c.Listen = o.Listen
	}
	if o.Interval > 0 {
		c.Interval = Duration{o.Interval}
	}
	if o.Timeout > 0 {
		c.Timeout = Duration{o.Timeout}
	}
}

// normalize fills every unset field with its default and resolves
// per-service inheritance of interval, timeout and retries.
func (c *Config) normalize() {
	if c.Title == "" {
		c.Title = DefaultTitle
	}
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if c.Interval.Duration <= 0 {
		c.Interval = Duration{DefaultInterval}
	}
	if c.Timeout.Duration <= 0 {
		c.Timeout = Duration{DefaultTimeout}
	}
	if c.Retries < 1 {
		c.Retries = DefaultRetries
	}
	if c.History < 1 {
		c.History = DefaultHistory
	}
	for i := range c.Services {
		s := &c.Services[i]
		if s.Name == "" {
			s.Name = s.ID
		}
		if s.Interval.Duration <= 0 {
			s.Interval = c.Interval
		}
		if s.Timeout.Duration <= 0 {
			s.Timeout = c.Timeout
		}
		if s.Retries < 1 {
			s.Retries = c.Retries
		}
		s.Method = strings.ToUpper(s.Method)
		if s.Method == "" {
			s.Method = http.MethodGet
		}
		s.RecordType = strings.ToUpper(s.RecordType)
		if s.RecordType == "" {
			s.RecordType = "A"
		}
	}
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Validate checks the configuration for structural errors and returns them
// joined. knownTypes lists the probe type names accepted by the prober
// registry (see prober.Types).
func (c *Config) Validate(knownTypes []string) error {
	var errs []error
	if len(c.Services) == 0 {
		errs = append(errs, errors.New("at least one service is required"))
	}
	known := make(map[string]bool, len(knownTypes))
	for _, t := range knownTypes {
		known[t] = true
	}

	ids := make(map[string]bool, len(c.Services))
	for _, s := range c.Services {
		if !idPattern.MatchString(s.ID) {
			errs = append(errs, fmt.Errorf("service %q: id must match %s", s.ID, idPattern.String()))
			continue
		}
		if ids[s.ID] {
			errs = append(errs, fmt.Errorf("service %q: duplicate id", s.ID))
			continue
		}
		ids[s.ID] = true

		if s.Target == "" {
			errs = append(errs, fmt.Errorf("service %q: target is required", s.ID))
		}
		if !known[s.Type] {
			errs = append(errs, fmt.Errorf("service %q: unknown type %q (known: %s)",
				s.ID, s.Type, strings.Join(knownTypes, ", ")))
		}
		if s.Timeout.Duration > s.Interval.Duration {
			errs = append(errs, fmt.Errorf("service %q: timeout (%s) must not exceed interval (%s)",
				s.ID, s.Timeout.Duration, s.Interval.Duration))
		}
		if s.BodyPattern != "" {
			if _, err := regexp.Compile(s.BodyPattern); err != nil {
				errs = append(errs, fmt.Errorf("service %q: invalid body_pattern: %w", s.ID, err))
			}
		}
		for _, code := range s.ExpectedStatuses {
			if code < 100 || code > 599 {
				errs = append(errs, fmt.Errorf("service %q: expected_statuses entry %d outside 100-599", s.ID, code))
			}
		}
	}
	return errors.Join(errs...)
}
