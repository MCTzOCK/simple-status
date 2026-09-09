package prober

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"simple-status/internal/config"
)

// maxBodyBytes limits how much of a response body is read when a
// body_pattern is configured; matches beyond this size are not detected.
const maxBodyBytes = 1 << 20

func init() { Register("http", newHTTPProbe) }

// httpProbe performs an HTTP request against a target URL.
type httpProbe struct {
	url       string
	method    string
	headers   map[string]string
	expected  map[int]bool // nil means "any 2xx status"
	bodyRegex *regexp.Regexp
	client    *http.Client
}

func newHTTPProbe(svc config.Service) (Probe, error) {
	if !strings.Contains(svc.Target, "://") {
		return nil, fmt.Errorf("http target %q must be a full URL including scheme", svc.Target)
	}
	p := &httpProbe{
		url:     svc.Target,
		method:  svc.Method,
		headers: svc.Headers,
		client: &http.Client{
			// The per-check deadline is enforced through the request
			// context supplied by the monitor.
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
	}
	if svc.InsecureTLS {
		p.client.Transport = &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in via insecure_tls
		}
	}
	if len(svc.ExpectedStatuses) > 0 {
		p.expected = make(map[int]bool, len(svc.ExpectedStatuses))
		for _, code := range svc.ExpectedStatuses {
			p.expected[code] = true
		}
	}
	if svc.BodyPattern != "" {
		re, err := regexp.Compile(svc.BodyPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid body_pattern: %w", err)
		}
		p.bodyRegex = re
	}
	return p, nil
}

func (p *httpProbe) Check(ctx context.Context) Result {
	req, err := http.NewRequestWithContext(ctx, p.method, p.url, nil)
	if err != nil {
		return Result{Success: false, Message: fmt.Sprintf("invalid request: %v", err)}
	}
	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	defer resp.Body.Close() //nolint:errcheck // best effort; nothing actionable on failure

	if !p.statusAccepted(resp.StatusCode) {
		return Result{Success: false, Message: fmt.Sprintf("unexpected status %s", resp.Status)}
	}
	if p.bodyRegex != nil {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		if err != nil {
			return Result{Success: false, Message: fmt.Sprintf("read body: %v", err)}
		}
		if !p.bodyRegex.Match(body) {
			return Result{Success: false, Message: fmt.Sprintf("body does not match %q", p.bodyRegex.String())}
		}
	}
	return Result{Success: true, Message: resp.Status}
}

func (p *httpProbe) statusAccepted(code int) bool {
	if p.expected != nil {
		return p.expected[code]
	}
	return code/100 == 2
}
