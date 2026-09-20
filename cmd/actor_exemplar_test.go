package cmd

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/version"
)

// pinVersion publishes a fake resolved release version for one test, exactly as
// cmd's init() publishes the real one, and restores the prior state afterwards.
//
// The value is deliberately NOT the repository's current version: a test that
// asserts "the exemplar says 0.5.3" would still pass against a hard-coded
// "binder/0.5.3" literal, which is the defect under test. 9.9.9 can only appear
// if the string was genuinely derived.
func pinVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Current()
	version.PinUnresolved()
	version.Set(v)
	t.Cleanup(func() {
		version.PinUnresolved()
		version.Set(old)
	})
}

// TestActorExemplarTracksLiveVersion is #60's core assertion: with a resolved
// release version published, every shipped surface shows that version in its
// worked `<producer>/<version>` example, and none shows a hand-written literal.
//
// The subtests are an explicit inventory of the sites #60 enumerated, not a loop
// over whatever happens to be registered, so losing one fails loudly instead of
// silently reducing coverage — the same reasoning as the #169 gate's
// minimum-coverage assertion. Three of the four are reachable from this package;
// the fourth (the MCP convert tool's invalid-actor error) is covered by
// internal/mcp's TestInvalidActorExemplarTracksLiveVersion.
func TestActorExemplarTracksLiveVersion(t *testing.T) {
	pinVersion(t, "9.9.9")
	const want = "binder/9.9.9"

	t.Run("sites 1+3: convert --verified-by help", func(t *testing.T) {
		out, _ := runCLI(t, "convert", "--help")
		if !strings.Contains(out, want) {
			t.Errorf("convert --help does not carry %q:\n%s", want, out)
		}
	})

	t.Run("sites 1+4: enrich --verified-by help", func(t *testing.T) {
		out, _ := runCLI(t, "enrich", "--help")
		if !strings.Contains(out, want) {
			t.Errorf("enrich --help does not carry %q:\n%s", want, out)
		}
	})

	// The error path is the one #60 calls out as more than cosmetic: a stale
	// exemplar reached users here, not only in --help.
	// The root silences errors and main.go prints what Execute returns, so the
	// message a user sees is the error value itself (runCLIErr), not a buffer.
	t.Run("site 2: invalid-actor usage error", func(t *testing.T) {
		isolateConfig(t)
		src := mkCorpus(t)
		err := runCLIErr(t, "convert", src, "-o", t.TempDir(), "--verified-by", "agent:bot")
		if err == nil {
			t.Fatalf("invalid actor must be a usage error, got nil")
		}
		if code := clijson.ExitCode(err); code != clijson.ExitUsage {
			t.Errorf("invalid actor exit = %d, want %d", code, clijson.ExitUsage)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("invalid-actor error does not carry %q:\n%s", want, err)
		}
	})
}

// TestNoHardCodedExemplarSurvives is the regression lock proper. It re-runs the
// same surfaces under a DIFFERENT published version and asserts the previous
// version's string is gone. A site that reverted to a literal would keep showing
// the old value and fail here, whereas a "contains the current version" check
// alone would not notice a literal that happens to match today's release.
func TestNoHardCodedExemplarSurvives(t *testing.T) {
	pinVersion(t, "1.2.3-rc1")

	for _, tc := range []struct{ name, cmd string }{
		{"convert", "convert"},
		{"enrich", "enrich"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runCLI(t, tc.cmd, "--help")
			if !strings.Contains(out, "binder/1.2.3-rc1") {
				t.Errorf("%s --help did not track the published version:\n%s", tc.cmd, out)
			}
			// Any other binder/<semver> in the help text is a literal that did
			// not follow the derivation.
			for _, stale := range []string{"binder/0.3.0", "binder/0.4.0", "binder/0.5.3"} {
				if strings.Contains(out, stale) {
					t.Errorf("%s --help still carries the hard-coded literal %q", tc.cmd, stale)
				}
			}
		})
	}
}

// TestUnresolvedVersionNeverRendersEmpty guards the failure mode #60 names as
// worse than the stale literal it replaces: an exemplar that renders with an
// EMPTY version because the value was read before it was resolved. An unstamped
// build must degrade to the version-neutral placeholder, never to "binder/".
func TestUnresolvedVersionNeverRendersEmpty(t *testing.T) {
	old := version.Current()
	version.PinUnresolved()
	t.Cleanup(func() { version.PinUnresolved(); version.Set(old) })

	for _, c := range []string{"convert", "enrich"} {
		out, _ := runCLI(t, c, "--help")
		if strings.Contains(out, `binder/"`) || strings.Contains(out, "binder/)") {
			t.Errorf("%s --help rendered an empty version:\n%s", c, out)
		}
		if !strings.Contains(out, "binder/<version>") {
			t.Errorf("%s --help did not fall back to the placeholder exemplar:\n%s", c, out)
		}
	}
}

// TestExemplarUsesTheNormalizedVersion ties the exemplar to the single
// normalization funnel in init(): the canonical form has no leading "v" (PR
// #52). A raw v-prefixed tag routed through normalizeVersion — the production
// path — must not produce "binder/v0.5.3".
func TestExemplarUsesTheNormalizedVersion(t *testing.T) {
	pinVersion(t, normalizeVersion("v0.5.3"))
	out, _ := runCLI(t, "convert", "--help")
	if strings.Contains(out, "binder/v") {
		t.Errorf("convert --help carries a v-prefixed exemplar:\n%s", out)
	}
	if !strings.Contains(out, "binder/0.5.3") {
		t.Errorf("convert --help lost the normalized exemplar:\n%s", out)
	}
}

// TestInitPublishesTheResolvedVersion pins the init-order contract that makes
// all of the above safe: cmd's init() publishes binder.Version to
// internal/version, so by the time any test (or main()) runs, the two agree. A
// future edit that drops the version.Set call, or that publishes before
// normalizeVersion, fails here.
func TestInitPublishesTheResolvedVersion(t *testing.T) {
	// In a `go test` build binder.Version is the unstamped "dev", which Set
	// rejects, so the published value is "" and the exemplar is the placeholder.
	// Assert the relationship rather than a literal, so this holds for a stamped
	// build too.
	if cur := version.Current(); cur != "" && cur != binder.Version {
		t.Errorf("internal/version has %q but binder.Version is %q; init() must publish "+
			"the normalized value", cur, binder.Version)
	}
	if strings.Contains(version.ActorExemplar(), "binder/v") {
		t.Errorf("published exemplar %q is v-prefixed", version.ActorExemplar())
	}
}
