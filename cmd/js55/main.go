// SPDX-License-Identifier: BUSL-1.1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hazyhaar/js55/pkg/c2web"
	"github.com/hazyhaar/js55/pkg/js55"
)

var (
	version = "1.0.0"
)

func main() {
	port := flag.Int("port", 8557, "HTTP port for web server and sandbox API")
	evalCode := flag.String("eval", "", "Evaluate a JavaScript expression and exit")
	runFile := flag.String("run", "", "Run a JavaScript file and exit")
	v := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *v {
		fmt.Printf("js55 version %s (Go 1.27, 0-CGO, microsecond isolates)\n", version)
		os.Exit(0)
	}

	if *evalCode != "" {
		iso, err := js55.NewIsolate(js55.Config{})
		if err != nil {
			log.Fatalf("[js55] Failed to initialize isolate: %v", err)
		}

		res, err := iso.EvalContext(context.Background(), *evalCode)
		if err != nil {
			log.Fatalf("[js55] Eval error: %v", err)
		}
		fmt.Printf("%v\n", res)
		return
	}

	if *runFile != "" {
		content, err := os.ReadFile(*runFile)
		if err != nil {
			log.Fatalf("[js55] Failed to read file %s: %v", *runFile, err)
		}

		iso, err := js55.NewIsolate(js55.Config{})
		if err != nil {
			log.Fatalf("[js55] Failed to initialize isolate: %v", err)
		}

		res, err := iso.EvalContext(context.Background(), string(content))
		if err != nil {
			log.Fatalf("[js55] Execution error: %v", err)
		}
		if !res.IsUndefined() {
			fmt.Printf("%v\n", res)
		}
		return
	}

	log.Printf("[js55] Starting runtime showcase on port :%d...", *port)

	addr := fmt.Sprintf(":%d", *port)
	srv, err := c2web.NewServer(addr)
	if err != nil {
		log.Fatalf("[js55] Failed to initialize web server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[js55] Web showcase & isolate sandbox listening on http://127.0.0.1:%d", *port)
		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("[js55] Web server error: %v", err)
		}
	}()

	<-sigCh
	log.Println("[js55] Shutting down gracefully...")
	_ = srv.Close()
	log.Println("[js55] Server stopped.")
}
