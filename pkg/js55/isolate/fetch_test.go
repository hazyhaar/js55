// SPDX-License-Identifier: BUSL-1.1

package isolate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/hostcall"
)

func TestIsolate_FetchDenyOffList(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = iso.Fetch(context.Background(), "https://evil.example/x")
	if !errors.Is(err, hostcall.ErrAllowlist) {
		t.Fatalf("deny evil.example : %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("deny evil.example a touché le réseau")
	}

	_, err = iso.Fetch(context.Background(), srv.URL)
	if !errors.Is(err, hostcall.ErrAllowlist) {
		t.Fatalf("deny hors liste : %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("deny hors liste a touché le réseau")
	}

	scriptCatch := `
		var caught = null;
		fetch("https://evil.example/x").catch(function(e) {
			caught = e;
		});
	`
	if _, err := iso.EvalContext(context.Background(), scriptCatch); err != nil {
		t.Fatalf("Eval deny catch : %v", err)
	}
	vCaught, err := iso.EvalContext(context.Background(), `caught`)
	if err != nil {
		t.Fatalf("lecture caught : %v", err)
	}
	sCaught := iso.VM().StringOf(vCaught)
	if sCaught == nil || !strings.Contains(sCaught.GoString(), "allowlist") {
		t.Fatalf("rejet deny attendu, obtenu %v (str=%v)", vCaught, sCaught)
	}
	if hits.Load() != 0 {
		t.Fatal("fetch JS deny a touché le réseau")
	}

	scriptThenReject := `
		var rejectedErr = null;
		fetch("` + srv.URL + `").then(function(res) {
			rejectedErr = "should not succeed";
		}, function(err) {
			rejectedErr = err;
		});
	`
	if _, err := iso.EvalContext(context.Background(), scriptThenReject); err != nil {
		t.Fatalf("Eval deny then reject : %v", err)
	}
	vRej, err := iso.EvalContext(context.Background(), `rejectedErr`)
	if err != nil {
		t.Fatalf("lecture rejectedErr : %v", err)
	}
	sRej := iso.VM().StringOf(vRej)
	if sRej == nil || !strings.Contains(sRej.GoString(), "allowlist") {
		t.Fatalf("rejet srv.URL deny attendu, obtenu %v (str=%v)", vRej, sRej)
	}
	if hits.Load() != 0 {
		t.Fatal("fetch JS deny srv.URL a touché le réseau")
	}
}

func TestIsolate_FetchAllowLocal(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	iso, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"127.0.0.1"}}})
	if err != nil {
		t.Fatal(err)
	}

	body, err := iso.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("corps %q", body)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}

	script := `
		var gOk = false;
		var gStatus = 0;
		var gBody = "";
		fetch("` + srv.URL + `").then(function(res) {
			gOk = res.ok;
			gStatus = res.status;
			return res.text();
		}).then(function(txt) {
			gBody = txt;
		});
	`
	if _, err := iso.EvalContext(context.Background(), script); err != nil {
		t.Fatalf("fetch JS : %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits JS=%d", hits.Load())
	}

	vOk, err := iso.EvalContext(context.Background(), `gOk`)
	if err != nil || vOk != engine.True {
		t.Fatalf("gOk attendu true, err=%v got=%v", err, vOk)
	}
	vStatus, err := iso.EvalContext(context.Background(), `gStatus`)
	if err != nil || vStatus.ToInt() != 200 {
		t.Fatalf("gStatus attendu 200, err=%v got=%v", err, vStatus)
	}
	vBody, err := iso.EvalContext(context.Background(), `gBody`)
	if err != nil {
		t.Fatalf("lecture gBody : %v", err)
	}
	got := iso.VM().StringOf(vBody)
	if got == nil || got.GoString() != "ok" {
		t.Fatalf("fetch JS corps %v", vBody)
	}

	isoDeny, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	_, err = isoDeny.Fetch(context.Background(), srv.URL)
	if !errors.Is(err, hostcall.ErrAllowlist) {
		t.Fatalf("hôte local hors liste %v", err)
	}
	if hits.Load() != before {
		t.Fatal("deny après allow a touché le réseau")
	}
}

func TestIsolate_FetchJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"service":"js55","code":42,"active":true}`)
	}))
	defer srv.Close()

	iso, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"127.0.0.1"}}})
	if err != nil {
		t.Fatal(err)
	}

	script := `
		var gService = "";
		var gCode = 0;
		var gActive = false;
		fetch("` + srv.URL + `").then(function(res) {
			return res.json();
		}).then(function(obj) {
			gService = obj.service;
			gCode = obj.code;
			gActive = obj.active;
		});
	`
	if _, err := iso.EvalContext(context.Background(), script); err != nil {
		t.Fatalf("fetch json eval : %v", err)
	}

	vSvc, _ := iso.EvalContext(context.Background(), `gService`)
	if s := iso.VM().StringOf(vSvc); s == nil || s.GoString() != "js55" {
		t.Fatalf("gService attendu 'js55', obtenu %v", vSvc)
	}

	vCode, _ := iso.EvalContext(context.Background(), `gCode`)
	if vCode.ToInt() != 42 {
		t.Fatalf("gCode attendu 42, obtenu %v", vCode)
	}

	vAct, _ := iso.EvalContext(context.Background(), `gActive`)
	if vAct != engine.True {
		t.Fatalf("gActive attendu true, obtenu %v", vAct)
	}
}

func TestIsolate_FetchHTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "non trouve")
	}))
	defer srv.Close()

	iso, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"127.0.0.1"}}})
	if err != nil {
		t.Fatal(err)
	}

	script := `
		var gOk = null;
		var gStatus = 0;
		var gBody = "";
		fetch("` + srv.URL + `").then(function(res) {
			gOk = res.ok;
			gStatus = res.status;
			return res.text();
		}).then(function(txt) {
			gBody = txt;
		});
	`
	if _, err := iso.EvalContext(context.Background(), script); err != nil {
		t.Fatalf("fetch 404 eval : %v", err)
	}

	vOk, _ := iso.EvalContext(context.Background(), `gOk`)
	if vOk != engine.False {
		t.Fatalf("gOk attendu false pour 404, obtenu %v", vOk)
	}

	vStatus, _ := iso.EvalContext(context.Background(), `gStatus`)
	if vStatus.ToInt() != 404 {
		t.Fatalf("gStatus attendu 404, obtenu %v", vStatus)
	}

	vBody, _ := iso.EvalContext(context.Background(), `gBody`)
	if s := iso.VM().StringOf(vBody); s == nil || s.GoString() != "non trouve" {
		t.Fatalf("gBody attendu 'non trouve', obtenu %v", vBody)
	}
}

func TestIsolate_FetchPool_Parallel_Race(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		_, _ = io.WriteString(w, fmt.Sprintf("resp-%d", n))
	}))
	defer srv.Close()

	const N = 32
	var wg sync.WaitGroup
	errs := make(chan error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			iso, err := New(Config{
				FetchPolicy: hostcall.FetchPolicy{
					AllowHosts: []string{"127.0.0.1"},
				},
			})
			if err != nil {
				errs <- fmt.Errorf("isolat %d creation: %w", id, err)
				return
			}
			script := fmt.Sprintf(`
				var out = "";
				fetch("%s").then(function(r) {
					return r.text();
				}).then(function(t) {
					out = t + "-%d";
				});
			`, srv.URL, id)

			if _, err := iso.EvalContext(context.Background(), script); err != nil {
				errs <- fmt.Errorf("isolat %d eval: %w", id, err)
				return
			}

			vOut, err := iso.EvalContext(context.Background(), `out`)
			if err != nil {
				errs <- fmt.Errorf("isolat %d lecture out: %w", id, err)
				return
			}
			s := iso.VM().StringOf(vOut)
			if s == nil || !strings.Contains(s.GoString(), fmt.Sprintf("-%d", id)) {
				errs <- fmt.Errorf("isolat %d resultat inattendu: %v", id, vOut)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if count.Load() != N {
		t.Fatalf("attendu %d requêtes réseau, obtenu %d", N, count.Load())
	}
}

func TestIsolate_FetchPool_Parallel_DenyNoDial(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "leak")
	}))
	defer srv.Close()

	const N = 32
	var wg sync.WaitGroup
	errs := make(chan error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			iso, err := New(Config{})
			if err != nil {
				errs <- fmt.Errorf("isolat %d creation: %w", id, err)
				return
			}
			script := fmt.Sprintf(`
				var rejectedErr = null;
				fetch("%s").then(function(res) {
					rejectedErr = "should not succeed";
				}, function(err) {
					rejectedErr = err;
				});
			`, srv.URL)
			if _, err := iso.EvalContext(context.Background(), script); err != nil {
				errs <- fmt.Errorf("isolat %d eval: %w", id, err)
				return
			}
			vRej, err := iso.EvalContext(context.Background(), `rejectedErr`)
			if err != nil {
				errs <- fmt.Errorf("isolat %d lecture rejectedErr: %w", id, err)
				return
			}
			sRej := iso.VM().StringOf(vRej)
			if sRej == nil || !strings.Contains(sRej.GoString(), "allowlist") {
				errs <- fmt.Errorf("isolat %d rejet allowlist attendu, obtenu %v (str=%v)", id, vRej, sRej)
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("deny concurrent a touché le réseau: hits=%d", hits.Load())
	}
}

func TestIsolate_FetchJSON_InvalidSyntaxCatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "<html><body>not json</body></html>")
	}))
	defer srv.Close()

	iso, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"127.0.0.1"}}})
	if err != nil {
		t.Fatal(err)
	}

	script := `
		var gCaught = null;
		fetch("` + srv.URL + `").then(function(res) {
			return res.json();
		}).catch(function(err) {
			gCaught = err;
		});
	`
	if _, err := iso.EvalContext(context.Background(), script); err != nil {
		t.Fatalf("fetch invalid json eval : %v", err)
	}

	vCaught, _ := iso.EvalContext(context.Background(), `gCaught`)
	s := iso.VM().StringOf(vCaught)
	if s == nil || !strings.Contains(s.GoString(), "SyntaxError") {
		t.Fatalf("gCaught attendu SyntaxError, obtenu %v (str=%v)", vCaught, s)
	}
}

func TestIsolate_FetchHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Horos-Custom", "val-42")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	iso, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"127.0.0.1"}}})
	if err != nil {
		t.Fatal(err)
	}

	script := `
		var gHeaderCustom = "";
		var gHeaderCT = "";
		fetch("` + srv.URL + `").then(function(res) {
			gHeaderCustom = res.headers.get("X-Horos-Custom");
			gHeaderCT = res.headers.get("content-type");
		});
	`
	if _, err := iso.EvalContext(context.Background(), script); err != nil {
		t.Fatalf("fetch headers eval : %v", err)
	}

	vCustom, _ := iso.EvalContext(context.Background(), `gHeaderCustom`)
	if s := iso.VM().StringOf(vCustom); s == nil || s.GoString() != "val-42" {
		t.Fatalf("gHeaderCustom attendu 'val-42', obtenu %v", vCustom)
	}

	vCT, _ := iso.EvalContext(context.Background(), `gHeaderCT`)
	if s := iso.VM().StringOf(vCT); s == nil || s.GoString() != "application/json" {
		t.Fatalf("gHeaderCT attendu 'application/json', obtenu %v", vCT)
	}
}

func TestIsolate_FetchJSON_ComplexStructures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[10, 20, 30], "meta":{"total":3, "valid":true, "name":null}}`)
	}))
	defer srv.Close()

	iso, err := New(Config{FetchPolicy: hostcall.FetchPolicy{AllowHosts: []string{"127.0.0.1"}}})
	if err != nil {
		t.Fatal(err)
	}

	script := `
		var gFirst = 0;
		var gTotal = 0;
		var gValid = false;
		var gName = "not-null";
		fetch("` + srv.URL + `").then(function(res) {
			return res.json();
		}).then(function(data) {
			gFirst = data.items[0];
			gTotal = data.meta.total;
			gValid = data.meta.valid;
			gName = data.meta.name;
		});
	`
	if _, err := iso.EvalContext(context.Background(), script); err != nil {
		t.Fatalf("fetch json complex eval : %v", err)
	}

	vFirst, _ := iso.EvalContext(context.Background(), `gFirst`)
	if vFirst.ToInt() != 10 {
		t.Fatalf("gFirst attendu 10, obtenu %v", vFirst)
	}

	vTotal, _ := iso.EvalContext(context.Background(), `gTotal`)
	if vTotal.ToInt() != 3 {
		t.Fatalf("gTotal attendu 3, obtenu %v", vTotal)
	}

	vValid, _ := iso.EvalContext(context.Background(), `gValid`)
	if vValid != engine.True {
		t.Fatalf("gValid attendu true, obtenu %v", vValid)
	}

	vName, _ := iso.EvalContext(context.Background(), `gName`)
	if !vName.IsNull() {
		t.Fatalf("gName attendu null, obtenu %v", vName)
	}
}
