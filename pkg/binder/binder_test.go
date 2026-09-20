package binder_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/binder/render"
	"github.com/ghchinoy/binder/pkg/okf/native"
)

// fixedNow pins the determinism instant so the corpus-derived report is stable.
var fixedNow = time.Unix(1700000000, 0).UTC() // 2023-11-14 UTC

const (
	testCorpus = "../../testdata/corpus-basic"
	testToday  = "2023-11-14"
)

func newReq() binder.LintRequest {
	return binder.LintRequest{
		Src:     testCorpus,
		Now:     fixedNow,
		Today:   testToday,
		Version: "test",
	}
}

// TestLintResultComplete proves the service returns a COMPLETE result: Src is filled
// inside the service (no adapter patch) and the gating definition lives on the Result.
func TestLintResultComplete(t *testing.T) {
	svc := binder.New(native.New())
	res, err := svc.Lint(context.Background(), newReq())
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.Report == nil {
		t.Fatal("Report is nil")
	}
	if res.Report.Src != testCorpus {
		t.Errorf("Report.Src = %q, want %q (service must own the fill)", res.Report.Src, testCorpus)
	}
	if got := res.GatingFindings(); got != res.Report.NumFindings() {
		t.Errorf("GatingFindings() = %d, want NumFindings() = %d", got, res.Report.NumFindings())
	}
	// Bare lint never gates; --strict gates iff there is a gating finding.
	if err := res.Gate(false); err != nil {
		t.Errorf("Gate(false) = %v, want nil (bare lint never gates)", err)
	}
	strictErr := res.Gate(true)
	if (res.GatingFindings() > 0) != (strictErr != nil) {
		t.Errorf("Gate(true) = %v but GatingFindings() = %d", strictErr, res.GatingFindings())
	}
}

// TestTodayDefaultsToNow proves the empty-Today rule is owned by the service: an empty
// Today resolves to Now's date, matching what cmd/mcp did before the collapse.
func TestTodayDefaultsToNow(t *testing.T) {
	svc := binder.New(native.New())
	req := newReq()
	req.Today = ""
	res, err := svc.Lint(context.Background(), req)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	reqExplicit := newReq()
	reqExplicit.Today = fixedNow.Format("2006-01-02")
	resExplicit, err := svc.Lint(context.Background(), reqExplicit)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	var gotEmpty, gotExplicit bytes.Buffer
	if err := res.EncodeJSON(&gotEmpty); err != nil {
		t.Fatal(err)
	}
	if err := resExplicit.EncodeJSON(&gotExplicit); err != nil {
		t.Fatal(err)
	}
	if gotEmpty.String() != gotExplicit.String() {
		t.Errorf("empty Today did not default to Now's date:\n%s\n---\n%s", gotEmpty.String(), gotExplicit.String())
	}
}

// TestResolveNow covers the pure determinism rule and its edge cases.
func TestResolveNow(t *testing.T) {
	fallback := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

	if got, err := binder.ResolveNow("", fallback); err != nil || !got.Equal(fallback) {
		t.Errorf("empty epoch: got %v, %v; want %v, nil", got, err, fallback)
	}
	if got, err := binder.ResolveNow("1700000000", fallback); err != nil || !got.Equal(fixedNow) {
		t.Errorf("valid epoch: got %v, %v; want %v, nil", got, err, fixedNow)
	}
	// Malformed epoch: returns the fallback AND an error (adapters ignore the error
	// to preserve the historical silent-fallback behavior).
	got, err := binder.ResolveNow("not-a-number", fallback)
	if err == nil {
		t.Error("malformed epoch: want error, got nil")
	}
	if !got.Equal(fallback) {
		t.Errorf("malformed epoch: got %v, want fallback %v", got, fallback)
	}
}

// TestLintConcurrent is the design's Residual Risk 5 harness: many goroutines drive
// Service.Lint over the same corpus under -race. The service holds only the injected
// codec and no mutable state, and the underlying package-level tables/regexps are
// read-only — this proves it, rather than assuming it. Every goroutine must produce
// the byte-identical envelope + prose the single-threaded run produces.
func TestLintConcurrent(t *testing.T) {
	svc := binder.New(native.New())

	// Reference output from a single-threaded run.
	ref, err := svc.Lint(context.Background(), newReq())
	if err != nil {
		t.Fatalf("reference Lint: %v", err)
	}
	var refJSON bytes.Buffer
	if err := ref.EncodeJSON(&refJSON); err != nil {
		t.Fatal(err)
	}
	wantJSON := refJSON.String()
	wantProse := render.Lint(ref)

	const goroutines = 32
	const iterations = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				res, err := svc.Lint(context.Background(), newReq())
				if err != nil {
					errs <- err
					return
				}
				var buf bytes.Buffer
				if err := res.EncodeJSON(&buf); err != nil {
					errs <- err
					return
				}
				if buf.String() != wantJSON {
					t.Errorf("concurrent JSON diverged:\n%s", buf.String())
					return
				}
				if render.Lint(res) != wantProse {
					t.Errorf("concurrent prose diverged:\n%s", render.Lint(res))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Lint error: %v", err)
	}
}
