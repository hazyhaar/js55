// SPDX-License-Identifier: BUSL-1.1
package c2web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/hazyhaar/js55/pkg/js55"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	httpServer *http.Server
	addr       string
	mu         sync.RWMutex
}

func NewServer(addr string) (*Server, error) {
	s := &Server{
		addr: addr,
	}

	mux := http.NewServeMux()

	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded static files: %w", err)
	}
	fileServer := http.FileServer(http.FS(subFS))

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/eval", s.handleEval)
	mux.HandleFunc("/api/v1/bench", s.handleBench)

	mux.Handle("/", fileServer)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	return s, nil
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Close() error {
	return s.httpServer.Close()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"engine":  "js55",
		"version": "1.0",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"engine":         "js55",
		"cgo_enabled":    0,
		"gas_metering":   true,
		"memory_quota":   true,
		"metrics_status": "not_qualified",
		"note":           "latency and memory figures are withheld until produced by named, reproducible benchmarks",
	}
	_ = json.NewEncoder(w).Encode(resp)
}

type EvalRequest struct {
	Code        string `json:"code"`
	MemoryLimit int64  `json:"memory_limit"`
	FuelLimit   uint64 `json:"fuel_limit"`
}

type EvalResponse struct {
	Success     bool    `json:"success"`
	Result      string  `json:"result,omitempty"`
	Error       string  `json:"error,omitempty"`
	ElapsedUs   float64 `json:"elapsed_us"`
	StartupUs   float64 `json:"startup_us"`
	MemoryBytes int     `json:"memory_bytes"`
}

func (s *Server) handleEval(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		req.Code = "const a = 10, b = 20; a + b;"
	}
	if req.MemoryLimit <= 0 {
		req.MemoryLimit = 2 * 1024 * 1024 // 2MB default
	}

	// Microsecond isolate instantiation measurement
	tStart := time.Now()
	iso, err := js55.NewIsolate(js55.Config{
		MaxMemoryBytes: req.MemoryLimit,
		GasLimit:       100_000,
	})
	startupTime := time.Since(tStart)

	if err != nil {
		_ = json.NewEncoder(w).Encode(EvalResponse{
			Success:   false,
			Error:     fmt.Sprintf("Failed to initialize isolate: %v", err),
			StartupUs: float64(startupTime.Microseconds()),
		})
		return
	}

	// Execute inside isolated context with 3 second timeout
	tExecStart := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	val, err := iso.EvalContext(ctx, req.Code)
	execTime := time.Since(tExecStart)

	if err != nil {
		_ = json.NewEncoder(w).Encode(EvalResponse{
			Success:   false,
			Error:     err.Error(),
			ElapsedUs: float64(execTime.Microseconds()),
			StartupUs: float64(startupTime.Microseconds()),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(EvalResponse{
		Success:     true,
		Result:      fmt.Sprintf("%v", val),
		ElapsedUs:   float64(execTime.Microseconds()),
		StartupUs:   float64(startupTime.Microseconds()),
		MemoryBytes: int(iso.AllocatedMemory()),
	})
}

func (s *Server) handleBench(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Measure isolate instantiation cost locally. No cross-runtime comparison and
	// no allocation count is asserted here.
	const iterations = 1000
	t0 := time.Now()
	for i := 0; i < iterations; i++ {
		iso, err := js55.NewIsolate(js55.Config{})
		if err != nil {
			http.Error(w, "isolate instantiation failed", http.StatusInternalServerError)
			return
		}
		_ = iso.Close()
	}
	avgStartupUs := float64(time.Since(t0).Microseconds()) / float64(iterations)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"iterations":     iterations,
		"avg_startup_us": avgStartupUs,
		"metrics_status": "local_measurement_only",
	})
}
