// SPDX-License-Identifier: Apache-2.0 OR MIT

package hostcall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrAllowlist = errors.New("js55: host hors allowlist")

type FetchPolicy struct {
	AllowHosts []string
	MaxBytes   int64
	Timeout    time.Duration
}

func (p FetchPolicy) Allowed(host string) bool {
	host = strings.ToLower(host)
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	for _, a := range p.AllowHosts {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if host == a {
			return true
		}
	}
	return false
}

// Response représente la réponse HTTP renvoyée par un appel Fetch.
type Response struct {
	Status     int
	StatusText string
	OK         bool
	Body       []byte
	Headers    map[string]string
}

// FetchResponse exécute une requête HTTP GET si l'hôte est dans l'allowlist et renvoie une Response.
func FetchResponse(ctx context.Context, raw string, p FetchPolicy) (*Response, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("js55: url invalide")
	}
	if !p.Allowed(u.Hostname()) {
		return nil, fmt.Errorf("%w: %s", ErrAllowlist, u.Hostname())
	}
	if p.Timeout <= 0 {
		p.Timeout = 3 * time.Second
	}
	if p.MaxBytes <= 0 {
		p.MaxBytes = 1 << 20
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: p.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, p.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	var lenErr error
	if int64(len(b)) > p.MaxBytes {
		b = b[:p.MaxBytes]
		lenErr = fmt.Errorf("js55: fetch trop long")
	}
	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return &Response{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
		Body:       b,
		Headers:    headers,
	}, lenErr
}

func Fetch(ctx context.Context, raw string, p FetchPolicy) ([]byte, error) {
	resp, err := FetchResponse(ctx, raw, p)
	if err != nil && (resp == nil || len(resp.Body) == 0) {
		return nil, err
	}
	if resp != nil {
		return resp.Body, err
	}
	return nil, err
}
