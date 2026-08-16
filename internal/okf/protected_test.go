package okf

import (
	"reflect"
	"sort"
	"testing"
)

// TestProtectedTrustKeysDerivation pins ProtectedTrustKeys() to its DERIVATION
// from the authoritative trust vocabulary (SpecRules.TrustFields) rather than to
// a hard-coded key literal (issue #22). The refusal set is the load-bearing
// guardrail for --overwrite-keys — the one v0.3.0 verb that overwrites existing
// user frontmatter — and the failure mode (clobbering a human attestation) is
// unrecoverable. A test that hard-codes today's keys cannot notice tomorrow's:
// so the expected set here is reconstructed INDEPENDENTLY from TrustFields, and
// this test fails the moment the production derivation drifts from it.
func TestProtectedTrustKeysDerivation(t *testing.T) {
	rules, ok := Rules(DefaultSpecVersion)
	if !ok {
		t.Fatalf("no rules for default spec version %q", DefaultSpecVersion)
	}

	// Independent derivation: EVERY trust-vocabulary key is protected EXCEPT the
	// two refreshable lifecycle stamps; plus the verified_by flag/config alias
	// (config.KeyVerifiedBy) that writes into the `verified` attestation list.
	// These small literals are the derivation the production code must match,
	// stated separately from it — NOT the ten-key protected list — so a drift in
	// either the vocabulary or the subtraction surfaces right here.
	refreshable := map[string]bool{"status": true, "stale_after": true}
	want := map[string]bool{"verified_by": true}
	for _, k := range rules.TrustFields {
		if refreshable[k] {
			continue
		}
		want[k] = true
	}

	got := ProtectedTrustKeys()

	gotSet := map[string]bool{}
	for _, k := range got {
		gotSet[k] = true
	}
	if !reflect.DeepEqual(gotSet, want) {
		t.Errorf("ProtectedTrustKeys() set mismatch:\n got:  %v\n want: %v\n(derivation drifted from TrustFields − {status,stale_after} + {verified_by})", gotSet, want)
	}

	// The two refreshable lifecycle keys must NOT be protected — that is the whole
	// point of the flag (they are its intended targets).
	for k := range refreshable {
		if gotSet[k] {
			t.Errorf("refreshable lifecycle key %q must NOT be protected", k)
		}
	}

	// Sorted + stable across calls: the CLI refusal error text joins this slice,
	// so its order is part of the observable contract.
	if !sort.StringsAreSorted(got) {
		t.Errorf("ProtectedTrustKeys() is not sorted: %v", got)
	}
	if again := ProtectedTrustKeys(); !reflect.DeepEqual(got, again) {
		t.Errorf("ProtectedTrustKeys() not stable across calls:\n first:  %v\n second: %v", got, again)
	}

	// IsProtectedTrustKey agrees with the slice for every protected key, and does
	// not protect a refreshable lifecycle key.
	for _, k := range got {
		if !IsProtectedTrustKey(k) {
			t.Errorf("IsProtectedTrustKey(%q) = false, want true", k)
		}
	}
	for k := range refreshable {
		if IsProtectedTrustKey(k) {
			t.Errorf("IsProtectedTrustKey(%q) = true, want false (lifecycle key)", k)
		}
	}
}
