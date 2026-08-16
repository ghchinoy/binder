package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestNormalizeVersion pins the single normalization funnel that both the
// ldflags-injected value and the debug.ReadBuildInfo() fallback pass through in
// init(). The canonical form is NO leading "v"; the two source paths used to
// disagree on exactly that "v", which is the defect this funnel closes.
func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"v-prefixed semver is stripped", "v0.3.0", "0.3.0"},
		{"bare semver is unchanged", "0.3.0", "0.3.0"},
		{"prerelease v-prefixed is stripped", "v1.2.3-rc1", "1.2.3-rc1"},
		{"dev default is unchanged", "dev", "dev"},
		{"build-info devel sentinel is unchanged", "(devel)", "(devel)"},
		{"empty is unchanged", "", ""},
		{"v followed by non-digit is not a semver, untouched", "version-x", "version-x"},
		{"bare v is untouched", "v", "v"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeVersion(tc.in); got != tc.want {
				t.Errorf("normalizeVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTrustStampMatchesVersion is the trust-integrity assertion: the three
// surfaces that carry the binder version — `binder --version`, the JSON
// envelope "binder" field, and the generated.by trust stamp written into
// concept frontmatter — must all report the SAME canonical value. The defect
// was that install method could make these diverge (v-prefixed vs not); this
// test fails if any surface reintroduces a divergent or v-prefixed value.
//
// It drives a v-prefixed raw version through normalizeVersion (the same funnel
// init() uses) so the assertion exercises the production path, not a copy.
func TestTrustStampMatchesVersion(t *testing.T) {
	old := Version
	Version = normalizeVersion("v0.3.0")
	defer func() { Version = old }()

	const want = "binder/0.3.0"
	if strings.Contains("binder/"+Version, "binder/v") {
		t.Fatalf("funnel left a leading v: Version = %q", Version)
	}

	// Surface 1: `binder --version`.
	verOut, code := runCLI(t, "--version")
	if code != clijson.ExitSuccess {
		t.Fatalf("--version exit = %d, want 0", code)
	}
	if strings.TrimSpace(verOut) != want {
		t.Errorf("--version = %q, want %q", strings.TrimSpace(verOut), want)
	}

	// Surfaces 2 and 3: run a real convert so the generated.by stamp is written
	// to disk, and capture the JSON envelope in the same run.
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	src := mkCorpus(t)
	out := t.TempDir()
	jsonOut, code := runCLI(t, "convert", src, "-o", out, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("convert exit = %d, want 0; output:\n%s", code, jsonOut)
	}

	// Surface 2: JSON envelope "binder" field.
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(jsonOut), &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\n%s", err, jsonOut)
	}
	if env.Binder != want {
		t.Errorf("envelope binder = %q, want %q", env.Binder, want)
	}

	// Surface 3: generated.by stamp in the converted concept's frontmatter.
	b, err := os.ReadFile(filepath.Join(out, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !contains(got, "generated:") || !contains(got, "by: "+want) {
		t.Errorf("a.md generated.by stamp is not %q:\n%s", want, got)
	}
	if contains(got, "binder/v") {
		t.Errorf("a.md carries a v-prefixed stamp (divergence reintroduced):\n%s", got)
	}
}
