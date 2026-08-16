package enrich_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/enrich"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// baseOpts returns a deterministic Options for the fixed clock, mirroring opts()
// but usable as a base to layer overwrite/lifecycle fields onto.
func baseOpts() enrich.Options {
	return enrich.Options{Codec: native.New(), Version: "0.1.0", Now: fixedNow}
}

// normalize runs an additive (no-overwrite) enrich over src so every file is
// serialized into the codec's canonical form. Subsequent overwrite runs then
// differ ONLY in the refreshed values, which lets a byte/line comparison prove
// byte-faithfulness without hard-coding the serializer's formatting.
func normalize(t *testing.T, src string) {
	t.Helper()
	if _, err := enrich.Enrich(src, baseOpts()); err != nil {
		t.Fatalf("normalize enrich: %v", err)
	}
}

const richDoc = "---\n" +
	"type: Note\n" +
	"title: Alpha\n" +
	"status: stable\n" +
	"stale_after: 2019-05-05\n" +
	"custom_key: keep me\n" +
	"nested:\n" +
	"  a: 1\n" +
	"  b: 2\n" +
	"tags:\n" +
	"  - x\n" +
	"  - y\n" +
	"---\n" +
	"\n# Alpha\n\nBody text.\n"

// TestOverwriteRefreshesOnlyNamedKeys is the core acceptance test (criterion 3):
// with --overwrite-keys status,stale_after those two keys are refreshed on a file
// that already has them, and every OTHER key, its value, key order, and the body
// bytes are untouched.
func TestOverwriteRefreshesOnlyNamedKeys(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "notes/a.md", richDoc)
	normalize(t, src) // canonicalize (also injects generated)
	canonical := read(t, p)

	o := baseOpts()
	o.StatusMap = map[string]string{"notes": "deprecated"}
	o.StaleAfterMap = map[string]string{"notes": "+6m"}
	o.OverwriteKeys = map[string]bool{"status": true, "stale_after": true}

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "notes/a.md")
	if res.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched", res.Status)
	}

	got := read(t, p)
	if got == canonical {
		t.Fatal("file unchanged; expected status/stale_after refresh")
	}

	// The refreshed values are present.
	if !strings.Contains(got, "status: deprecated") {
		t.Errorf("status not refreshed to deprecated:\n%s", got)
	}
	// fixedNow (2026-08-15) + 6 months = 2027-02-15.
	if !strings.Contains(got, "2027-02-15") {
		t.Errorf("stale_after not refreshed to 2027-02-15:\n%s", got)
	}

	// Line-by-line: only the status and stale_after lines may differ, and they
	// must differ at the SAME index (order preserved). Everything else is
	// byte-identical.
	cl := strings.Split(canonical, "\n")
	gl := strings.Split(got, "\n")
	if len(cl) != len(gl) {
		t.Fatalf("line count changed %d -> %d (order/structure not preserved)\n--- canonical\n%s\n--- got\n%s", len(cl), len(gl), canonical, got)
	}
	for i := range cl {
		if cl[i] == gl[i] {
			continue
		}
		isStatus := strings.HasPrefix(strings.TrimSpace(cl[i]), "status:") && strings.HasPrefix(strings.TrimSpace(gl[i]), "status:")
		isStale := strings.HasPrefix(strings.TrimSpace(cl[i]), "stale_after:") && strings.HasPrefix(strings.TrimSpace(gl[i]), "stale_after:")
		if !isStatus && !isStale {
			t.Errorf("line %d changed but is neither status nor stale_after:\n  canonical: %q\n  got:       %q", i, cl[i], gl[i])
		}
	}
}

// TestDefaultNeverClobbers is the regression guard for criterion 2: WITHOUT
// --overwrite-keys, a pre-existing key is preserved even when a lifecycle map
// would otherwise set a different value, and the file is byte-identical.
func TestDefaultNeverClobbers(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "notes/a.md", richDoc)
	normalize(t, src)
	canonical := read(t, p)

	o := baseOpts()
	o.StatusMap = map[string]string{"notes": "deprecated"}
	o.StaleAfterMap = map[string]string{"notes": "+6m"}
	// No OverwriteKeys → additive/never-clobber.

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if res := find(t, rep, "notes/a.md"); res.Status != enrich.StatusUnchanged {
		t.Fatalf("status = %q, want unchanged (never-clobber default)", res.Status)
	}
	if got := read(t, p); got != canonical {
		t.Errorf("default run mutated a file with all keys present:\n--- want\n%s\n--- got\n%s", canonical, got)
	}
}

// TestOverwriteSkipUnchanged is criterion 6: refreshing a key to its EXISTING
// value does not rewrite the file and is not counted as modified.
func TestOverwriteSkipUnchanged(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "notes/a.md", richDoc)
	normalize(t, src)

	info0, _ := os.Stat(p)
	before := read(t, p)
	time.Sleep(10 * time.Millisecond)

	o := baseOpts()
	// Map status to its current value "stable" → no net change.
	o.StatusMap = map[string]string{"notes": "stable"}
	o.OverwriteKeys = map[string]bool{"status": true}

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if res := find(t, rep, "notes/a.md"); res.Status != enrich.StatusUnchanged {
		t.Fatalf("status = %q, want unchanged (refresh to same value)", res.Status)
	}
	if rep.NumEnriched != 0 {
		t.Errorf("NumEnriched = %d, want 0", rep.NumEnriched)
	}
	if read(t, p) != before {
		t.Error("file rewritten despite no net change")
	}
	if info1, _ := os.Stat(p); !info0.ModTime().Equal(info1.ModTime()) {
		t.Error("mtime changed: skip-unchanged violated under --overwrite-keys")
	}
}

// TestOverwriteNoSourceNotClobbered: naming a key in --overwrite-keys for which
// the run supplies no value (e.g. status with no --status-map) leaves the
// authored value untouched rather than clobbering it to empty.
func TestOverwriteNoSourceNotClobbered(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "notes/a.md", richDoc)
	normalize(t, src)
	before := read(t, p)

	o := baseOpts()
	o.OverwriteKeys = map[string]bool{"status": true} // no StatusMap/StatusDefault

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if res := find(t, rep, "notes/a.md"); res.Status != enrich.StatusUnchanged {
		t.Fatalf("status = %q, want unchanged (no source to overwrite with)", res.Status)
	}
	got := read(t, p)
	if got != before {
		t.Errorf("authored status clobbered when no value source configured:\n--- want\n%s\n--- got\n%s", before, got)
	}
	if !strings.Contains(got, "status: stable") {
		t.Errorf("authored status value lost:\n%s", got)
	}
}

// TestOverwriteDeterministic is criterion 7: two overwrite runs over the same
// corpus produce identical files and identical report prose.
func TestOverwriteDeterministic(t *testing.T) {
	run := func() (string, string) {
		src := t.TempDir()
		p := writeFile(t, src, "notes/a.md", richDoc)
		o := baseOpts()
		o.StatusMap = map[string]string{"notes": "deprecated"}
		o.StaleAfterMap = map[string]string{"notes": "+6m"}
		o.OverwriteKeys = map[string]bool{"status": true, "stale_after": true}
		rep, err := enrich.Enrich(src, o)
		if err != nil {
			t.Fatal(err)
		}
		return read(t, p), rep.String()
	}
	// dropSrc strips the leading "enrich <src>" line, which carries the (per-run)
	// temp path; everything after it is the corpus-derived, deterministic body.
	dropSrc := func(s string) string {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			return s[i+1:]
		}
		return s
	}
	f1, r1 := run()
	f2, r2 := run()
	if f1 != f2 {
		t.Errorf("non-deterministic file output:\n--- run1\n%s\n--- run2\n%s", f1, f2)
	}
	if dropSrc(r1) != dropSrc(r2) {
		t.Errorf("non-deterministic report:\n--- run1\n%s\n--- run2\n%s", r1, r2)
	}
}

// TestOverwriteReportDistinguishes is criterion 8: the report separates keys
// ADDED (were absent) from keys OVERWRITTEN (named and refreshed in place).
func TestOverwriteReportDistinguishes(t *testing.T) {
	src := t.TempDir()
	// status present (will be overwritten); stale_after ABSENT (will be added).
	doc := "---\ntype: Note\ntitle: Alpha\nstatus: stable\ngenerated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n---\n\n# Alpha\n\nBody.\n"
	writeFile(t, src, "notes/a.md", doc)

	o := baseOpts()
	o.StatusMap = map[string]string{"notes": "deprecated"}
	o.StaleAfterMap = map[string]string{"notes": "+6m"}
	o.OverwriteKeys = map[string]bool{"status": true, "stale_after": true}

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "notes/a.md")
	if got := strings.Join(res.Overwritten, ","); got != "status" {
		t.Errorf("Overwritten = %v, want [status]", res.Overwritten)
	}
	// stale_after was absent → additive add, not an overwrite.
	if got := strings.Join(res.Added, ","); got != "stale_after" {
		t.Errorf("Added = %v, want [stale_after]", res.Added)
	}
}

// TestOverwriteDryRun is criterion 5: --dry-run with --overwrite-keys writes
// nothing but reports what WOULD change.
func TestOverwriteDryRun(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "notes/a.md", richDoc)
	normalize(t, src)
	before := read(t, p)
	info0, _ := os.Stat(p)
	time.Sleep(10 * time.Millisecond)

	o := baseOpts()
	o.DryRun = true
	o.StatusMap = map[string]string{"notes": "deprecated"}
	o.OverwriteKeys = map[string]bool{"status": true}

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "notes/a.md")
	if res.Status != enrich.StatusWouldEnrich {
		t.Fatalf("status = %q, want would-enrich", res.Status)
	}
	if strings.Join(res.Overwritten, ",") != "status" {
		t.Errorf("Overwritten = %v, want [status]", res.Overwritten)
	}
	if read(t, p) != before {
		t.Error("dry-run wrote to the file")
	}
	if info1, _ := os.Stat(p); !info0.ModTime().Equal(info1.ModTime()) {
		t.Error("dry-run changed mtime")
	}
}

// TestParseOverwriteKeys covers parsing and the trust-key refusal (criterion 4
// at the parse layer): each protected trust key is refused with a message that
// names it; a normal list parses to a set; empty/blank input is off.
func TestParseOverwriteKeys(t *testing.T) {
	if m, err := enrich.ParseOverwriteKeys(""); err != nil || m != nil {
		t.Errorf("empty: got (%v, %v), want (nil, nil)", m, err)
	}
	if m, err := enrich.ParseOverwriteKeys("  ,  "); err != nil || m != nil {
		t.Errorf("blank entries: got (%v, %v), want (nil, nil)", m, err)
	}
	m, err := enrich.ParseOverwriteKeys(" status , stale_after ,type")
	if err != nil {
		t.Fatalf("valid list: %v", err)
	}
	for _, k := range []string{"status", "stale_after", "type"} {
		if !m[k] {
			t.Errorf("key %q missing from parsed set %v", k, m)
		}
	}
	for _, trust := range []string{"verified", "verified_by", "sources", "generated", "usage_window", "runtime", "attester", "executor", "computation", "parameters"} {
		_, err := enrich.ParseOverwriteKeys("status," + trust)
		if err == nil {
			t.Errorf("trust key %q was not refused", trust)
			continue
		}
		if !strings.Contains(err.Error(), trust) {
			t.Errorf("refusal for %q does not name the key: %v", trust, err)
		}
	}
}
