# js55

> **Sovereign Pure Go 1.27 JavaScript Runtime (0-CGO)**

[![Live Demo](https://img.shields.io/badge/Live_Sandbox-js55.hazyhaar.fr-blue?style=flat-square)](https://js55.hazyhaar.fr)
[![License: BSL 1.1](https://img.shields.io/badge/License-BSL_1.1-orange?style=flat-square)](LICENSE)
[![Go Report](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go)](go.mod)
[![Architecture](https://img.shields.io/badge/Isolates-0--CGO_Go_1.27-green?style=flat-square)](pkg/js55)

---

## 1. Overview

`js55` is a sovereign JavaScript execution engine and isolate manager written in 100% pure Go 1.27 (0-CGO). Engineered for autonomous AI agents, multi-tenant workflows, and sandboxed execution. It does not embed V8, Node.js, or any external C++ runtime.

**TypeScript :** Support expérimental TypeScript par effacement syntaxique de types (en cours d'intégration).

### Key Architectural Invariants
- **Pure Go, no foreign runtime:** the engine compiles into a single static binary without CGO, and without V8, Node.js, libuv or external shared objects.
- **Hermetic multi-tenancy:** each isolate is memory-isolated inside one Go process, with per-isolate byte quotas and an instruction (gas) budget that bound runaway scripts.
- **Native ArrayBuffer & TypedArray Semantics:** `ArrayBuffer`, resizable buffers, and TypedArray views are implemented directly in the Go engine.
- **Deterministic resource control:** CPU instruction budgeting and hard wall-clock timeouts prevent infinite loops, ReDoS, and denial of service.

Performance figures for isolate startup latency, idle memory footprint and host-call overhead are still being qualified on named hardware and benchmarks; they are deliberately not published here until they rest on reproducible measurements.

---

## 2. Architecture & Measurement Status

The engine pursues three architectural objectives rather than public benchmark claims:

- **A pure Go execution path with 0-CGO**, so the runtime ships as a single static binary and remains portable across targets supported by the Go toolchain.
- **Strict per-isolate quotas** on memory and CPU, enforced by the isolate manager, so a hostile or faulty script cannot exhaust the host process.
- **Sovereign hosting**, where the engine is embedded directly in a Go host application instead of being proxied through a foreign runtime.

Metrology is in the process of qualification. Measured latency and footprint figures will be published only once they are produced by named, reproducible benchmarks on identified hardware, with their date and conditions.

---

## 3. Quick Start & Code Example

### Installation
```bash
go get github.com/hazyhaar/js55
```

### Running an Isolated Script
```go
package main

import (
	"fmt"

	"github.com/hazyhaar/js55/pkg/js55"
)

func main() {
	// Create an isolate with strict memory and CPU fuel budget
	iso, err := js55.NewIsolate(js55.Config{
		MaxMemoryBytes: 1024 * 1024, // 1 MB quota
		StepLimit:      100_000,     // 100k instructions quota
	})
	if err != nil {
		panic(err)
	}
	defer iso.Close()

	// Evaluate JavaScript code
	val, err := iso.Eval(`
		const numbers = [1, 2, 3, 4, 5];
		numbers.map(x => x * 2).reduce((a, b) => a + b, 0);
	`)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Result: %v\n", val)
	// Output: Result: 30
}
```

> This example is guaranteed by the Go compiler through the `Example_basic`
> test in `pkg/js55/example_test.go`, executed by `go test ./pkg/js55`.

---

## 4. Live Sandbox & Interactive REPL

Experience the interactive isolate sandbox and REPL:
**[https://js55.hazyhaar.fr](https://js55.hazyhaar.fr)**

Run locally:
```bash
go run ./cmd/js55 -port 8557
```

---

## 5. Commercial Licensing & B2B Solutions

`js55` is licensed under the **Business Source License 1.1 (BSL 1.1)**.

- **Non-Commercial, Evaluation, Local Development & Research:** free under the Additional Use Grant.
- **Commercial Production Use:** Commercial hosted platforms, managed cloud serverless execution runtimes, or multi-tenant code-execution sandbox APIs require a commercial license agreement.

### B2B Solutions:
1. **Agentic Code Execution Sandboxes:** Secure JavaScript tool-calling sandboxes for autonomous LLM agents.
2. **Edge Serverless Runtimes:** Embed lightweight JS worker runtimes directly in Go API gateways.
3. **Enterprise Support & Plugins:** DOM/Browser emulation extensions, host integration plugins, and priority support.

For commercial licensing inquiries, contact: [contact@hazyhaar.fr](mailto:contact@hazyhaar.fr).

---

## 6. License

> **The js55 engine is licensed under the Business Source License 1.1 (BSL 1.1). This is not an open-source license at the current date and it is not Apache-2.0. The Apache License, Version 2.0 applies only as the Change License on or after the Change Date of September 21, 2028.**

The repository is not uniformly licensed, and no statement of exclusive BSL coverage applies to it:

- **js55 engine and web front** (`pkg/js55/...`, `cmd/js55/...`, `pkg/c2web/...`) are licensed under the Business Source License 1.1 (SPDX: `BUSL-1.1`). See [LICENSE](LICENSE) for the full terms. BSL 1.1 is the Business Source License published by MariaDB Corporation Ab, distinct from BSL-1.0 (Boost Software License).
- **SIMD and numeric conversion submodules** (`pkg/c2d2s/...`, `pkg/c2strsimd/...`, `pkg/c2strclass/...`) are dual-licensed under Apache-2.0 OR MIT (SPDX: `Apache-2.0 OR MIT`).
- **Third-party components** used as test fixtures and conformance material retain their own licenses and are credited in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

Go source files carry the SPDX identifier matching their component: `BUSL-1.1` for the engine, `Apache-2.0 OR MIT` for the SIMD and numeric submodules.
