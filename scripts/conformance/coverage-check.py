#!/usr/bin/env python3
"""coverage-check.py — assert the #171 conformance run is NON-VACUOUS, locally.

The cross-language conformance suite is only meaningful if the shared corpus
actually exercises the things it claims to bind. Before this check, that was an
EMERGENT property: the error strings stayed bound only because the prose check
happened to require the two named structural cases, and the count guard in
cross-language-conformance.sh merely counted cases, not error-producing ones.
A refactor that relaxed the prose check would have left the suite able to pass
while binding nothing — the exact "reports success having examined nothing"
defect class this batch exists to kill (cf. #169 / #172).

This script makes the guarantee LOCAL. Reading only the Go-DERIVED golden, it
asserts directly that:

  1. the corpus reproduces BOTH structural error strings Go emits (the golden's
     Go-observed `canonical` block is the reference — no literal is hard-coded
     here), and
  2. each trust predicate (isHumanActor, isValidActor) yields BOTH true and
     false across the actor corpus.

Something now fails the moment either stops being true, rather than depending on
another check's case list.

Recursion guard: "both polarities were seen" is itself vacuous over an empty
set. Every assertion below requires POSITIVE evidence (a specific string must be
present; both booleans must be observed), so an empty golden fails every one of
them rather than passing by iterating nothing.

Usage: coverage-check.py <golden.json>
Exit 0 if the run is non-vacuous; 1 (naming what was missing) otherwise.
"""
import json
import sys


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: coverage-check.py <golden.json>", file=sys.stderr)
        return 2
    with open(sys.argv[1], encoding="utf-8") as fh:
        golden = json.load(fh)

    frontmatter = golden.get("frontmatter", [])
    actors = golden.get("actors", [])
    canonical = golden.get("canonical", {})

    fails = []
    oks = []

    # --- Structural error coverage -----------------------------------------
    # The set of NON-EMPTY error strings the corpus actually produced.
    produced = {c.get("error", "") for c in frontmatter if c.get("error", "")}

    structural = [
        ("unterminated-fence", canonical.get("unterminatedError", "")),
        ("non-mapping-top-level", canonical.get("nonmappingError", "")),
    ]
    seen_canon = []
    for label, want in structural:
        if not want:
            fails.append(
                f"Go produced no canonical {label} error — the generator or the "
                f"Go source of truth is broken (expected a non-empty error)."
            )
            continue
        seen_canon.append(want)
        if want in produced:
            oks.append(f"corpus reproduces the {label} structural error")
        else:
            fails.append(
                f"no corpus case produced the {label} structural error {want!r}; "
                f"the error-string conformance would pass vacuously for it — add "
                f"a corpus case that triggers it."
            )

    # The two structural strings must be DISTINCT; if they collapse to one, a
    # single case could satisfy both checks above and coverage would be a lie.
    if len(seen_canon) == 2 and seen_canon[0] == seen_canon[1]:
        fails.append(
            "the two canonical structural errors are identical — cannot prove "
            "both paths are exercised separately."
        )

    # --- Trust-predicate polarity coverage ---------------------------------
    # Each predicate must be observed yielding BOTH true and false, so the
    # conformance actually tests the discriminating case rather than a corpus
    # that happens to be all-valid or all-invalid.
    for name, key in (("isHumanActor", "isHuman"), ("isValidActor", "isValid")):
        vals = [bool(a.get(key)) for a in actors]
        saw_true = True in vals
        saw_false = False in vals
        if saw_true and saw_false:
            oks.append(f"{name} exercised with both true and false")
        else:
            fails.append(
                f"{name} was not exercised with both true and false "
                f"(saw true={saw_true}, false={saw_false}); its conformance "
                f"could pass without testing the discriminating case — add "
                f"actor(s) of the missing polarity."
            )

    # --- Wrapped-YAML prefix + TrustTier + IsStale coverage (#175) ---------
    wrapped_prefix = canonical.get("wrappedPrefix", "")
    prefix_cases = [c for c in frontmatter if c.get("prefixOnly") and c.get("error", "").startswith(wrapped_prefix)]
    if wrapped_prefix and prefix_cases:
        oks.append(f"wrapped-YAML error prefix {wrapped_prefix!r} exercised")
    else:
        fails.append("no corpus case exercised the wrapped-YAML error prefix (#175).")

    tiers_seen = {t.get("tier", "") for t in golden.get("tiers", [])}
    want_tiers = {"unverified", "machine-confirmed", "human-reviewed"}
    if want_tiers.issubset(tiers_seen):
        oks.append("deriveTier exercised across all three OKF trust tiers")
    else:
        fails.append(f"deriveTier missing tier(s): {sorted(want_tiers - tiers_seen)} (#175).")

    stale_vals = [bool(s.get("isStale")) for s in golden.get("staleness", [])]
    if True in stale_vals and False in stale_vals:
        oks.append("isStale exercised with both true and false")
    else:
        fails.append("isStale was not exercised with both true and false (#175).")

    for ok in oks:
        print(f"  ok   coverage: {ok}")
    for f in fails:
        print(f"  FAIL coverage: {f}")

    if fails:
        print(
            f"COVERAGE FAILED: {len(fails)} non-vacuity assertion(s) did not hold.",
            file=sys.stderr,
        )
        return 1
    print("  PASS coverage: corpus exercises both structural errors and both predicate polarities.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
