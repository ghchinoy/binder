package version

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// setCurrent swaps the package state for one test and restores it after, so the
// tests do not leak into each other (or into the process-wide value cmd's
// init() published).
func setCurrent(t *testing.T, v string) {
	t.Helper()
	old := current
	current = v
	t.Cleanup(func() { current = old })
}

// TestSetRejectsUnresolvedVersions is the anti-empty-exemplar lock. #60's stated
// failure mode is a derivation that renders an EMPTY or sentinel version in
// shipped text — worse than the stale literal it replaces. Set must swallow
// every non-release value so the exemplar falls back to the placeholder.
func TestSetRejectsUnresolvedVersions(t *testing.T) {
	for _, in := range []string{"", "dev", "(devel)", "v0.5.3", "version-x", "unknown"} {
		t.Run(in, func(t *testing.T) {
			setCurrent(t, "")
			Set(in)
			if current != "" {
				t.Errorf("Set(%q) stored %q; unresolved values must be ignored", in, current)
			}
			if got := ActorExemplar(); got != unresolvedExemplar {
				t.Errorf("after Set(%q), ActorExemplar() = %q, want %q", in, got, unresolvedExemplar)
			}
		})
	}
}

// TestSetAcceptsResolvedVersions covers the shapes normalizeVersion can produce
// for a real release: a plain semver and a prerelease, both already v-stripped.
func TestSetAcceptsResolvedVersions(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"0.5.3", "binder/0.5.3"},
		{"1.2.3-rc1", "binder/1.2.3-rc1"},
		{"0.1.0", "binder/0.1.0"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			setCurrent(t, "")
			Set(tc.in)
			if got := ActorExemplar(); got != tc.want {
				t.Errorf("after Set(%q), ActorExemplar() = %q, want %q", tc.in, got, tc.want)
			}
			if Current() != tc.in {
				t.Errorf("Current() = %q, want %q", Current(), tc.in)
			}
		})
	}
}

// TestExemplarNeverRendersEmptyOrVPrefixed pins the two malformed renderings
// that would be user-visible regressions: "binder/" (the empty-version trap)
// and "binder/v0.5.3" (the leading-v divergence PR #52 closed).
func TestExemplarNeverRendersEmptyOrVPrefixed(t *testing.T) {
	for _, in := range []string{"", "dev", "(devel)", "v0.5.3", "0.5.3", "1.2.3-rc1"} {
		setCurrent(t, "")
		Set(in)
		got := ActorExemplar()
		if got == Producer+"/" || strings.HasSuffix(got, "/") {
			t.Errorf("Set(%q) -> ActorExemplar() = %q: empty version rendered", in, got)
		}
		if strings.Contains(got, "binder/v") {
			t.Errorf("Set(%q) -> ActorExemplar() = %q: leading v reintroduced", in, got)
		}
	}
}

// TestExemplarIsAlwaysAValidActor holds #60's explicit constraint: whichever
// form the exemplar takes, a user who copies it verbatim into --verified-by must
// not be handed a usage error. Both the resolved and the placeholder rendering
// are checked against the real okf.IsValidActor, not a restatement of it.
func TestExemplarIsAlwaysAValidActor(t *testing.T) {
	for _, in := range []string{"", "dev", "(devel)", "0.5.3", "1.2.3-rc1"} {
		setCurrent(t, "")
		Set(in)
		if got := ActorExemplar(); !okf.IsValidActor(got) {
			t.Errorf("Set(%q) -> ActorExemplar() = %q is not a valid actor", in, got)
		}
	}
}
