// SPDX-License-Identifier: Apache-2.0 OR MIT

package hostcall

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFetchDenyOffList(t *testing.T) {
	_, err := Fetch(context.Background(), "https://evil.example/x", FetchPolicy{})
	if !errors.Is(err, ErrAllowlist) {
		t.Fatalf("deny %v", err)
	}
	_, err = FetchResponse(context.Background(), "https://evil.example/x", FetchPolicy{})
	if !errors.Is(err, ErrAllowlist) {
		t.Fatalf("deny FetchResponse %v", err)
	}
}

func TestFetchAllowLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "val")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	body, err := Fetch(context.Background(), srv.URL, FetchPolicy{AllowHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body %q", body)
	}

	resp, err := FetchResponse(context.Background(), srv.URL, FetchPolicy{AllowHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("resp.OK attendu true")
	}
	if resp.Status != 200 {
		t.Fatalf("resp.Status = %d, attendu 200", resp.Status)
	}
	if string(resp.Body) != "ok" {
		t.Fatalf("resp.Body = %q, attendu ok", string(resp.Body))
	}
	if resp.Headers["X-Custom"] != "val" {
		t.Fatalf("header X-Custom = %q", resp.Headers["X-Custom"])
	}

	_, err = Fetch(context.Background(), srv.URL, FetchPolicy{AllowHosts: []string{"example.com"}})
	if !errors.Is(err, ErrAllowlist) {
		t.Fatalf("hôte local hors liste %v", err)
	}
	_, err = FetchResponse(context.Background(), srv.URL, FetchPolicy{AllowHosts: []string{"example.com"}})
	if !errors.Is(err, ErrAllowlist) {
		t.Fatalf("hôte local hors liste FetchResponse %v", err)
	}
}

func TestFetchDenyNoDial(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "leak")
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, FetchPolicy{})
	if !errors.Is(err, ErrAllowlist) {
		t.Fatalf("deny Fetch %v", err)
	}
	_, err = FetchResponse(context.Background(), srv.URL, FetchPolicy{})
	if !errors.Is(err, ErrAllowlist) {
		t.Fatalf("deny FetchResponse %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("deny a touché le réseau: hits=%d", hits.Load())
	}
}

func TestFetchPool_Parallel_DenyNoDial(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "leak")
	}))
	defer srv.Close()

	const N = 32
	var wg sync.WaitGroup
	errs := make(chan error, N*2)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Fetch(context.Background(), srv.URL, FetchPolicy{})
			if !errors.Is(err, ErrAllowlist) {
				errs <- err
				return
			}
			_, err = FetchResponse(context.Background(), srv.URL, FetchPolicy{})
			if !errors.Is(err, ErrAllowlist) {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("deny concurrent %v", err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("deny concurrent a touché le réseau: hits=%d", hits.Load())
	}
}
