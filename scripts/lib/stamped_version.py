#!/usr/bin/env python3
"""Certification for a stamped binder build — the single answer to "is this
binary fit to gate against?" (issues #169, #60).

# Why this file exists

`scripts/lib/stamped-binder.sh` already made the ldflags BUILD shared. That was
not enough: each caller still decided separately whether to TRUST the result, and
the two callers had silently diverged.

    #169's gate:  fullmatch(binder/\\d+\\.\\d+\\.\\d+)     -- no suffix permitted
    #60's gate:   release match WITH prerelease allowed,
                  minus pseudo-versions, minus build metadata

A helper that builds the binary while every caller re-decides whether to believe
it is not a shared precondition; it is a shared subroutine with the real
precondition still duplicated. The thing that builds it should be the thing that
certifies it, so certification lives here and the build calls it.

# The decomposition, and why NOT to merge the two predicates

#169's stricter rule happens to be immune to the pseudo-version bug that cost #60
a CI round trip — but only as a SIDE EFFECT: `fullmatch` on three numeric
components rejects `0.5.4-0.20260914164646-656b05e16068` because it rejects every
suffix, not because it knows what a pseudo-version is. Unifying onto #60's
predicate, which must permit prerelease because goreleaser can ship `v1.2.3-rc1`,
would therefore LOOSEN #169 and hand it the exact bug #60 already paid to find.

So the predicates are not merged, they are decomposed into two independent
questions:

  1. IS THIS BUILD STAMPED AT ALL?  Absolute, identical for every caller, not
     negotiable. A VCS pseudo-version or `+build` metadata means the binary was
     NOT ldflag-stamped: `debug.ReadBuildInfo()` synthesised that string from the
     checkout. Gating against it pins literals to a commit hash while reporting
     green.

  2. WHICH RELEASE SHAPES DOES THIS PROJECT SHIP?  A policy question, genuinely
     per-caller, so it is an argument with a default rather than a constant.
     #60 permits prerelease (goreleaser ships `-rc1`); #169 does not.

Question 1 can only ever get stricter for a caller adopting this module.
Question 2 is passed explicitly. That is what makes the unification safe.

Usage as a library:   certify(version_line, allow_prerelease=True) -> reason|None
Usage from shell:     stamped_version.py --certify <line> [--no-prerelease]
Self-test:            stamped_version.py --self-test
"""
import re
import sys

PRODUCER = "binder"

# binder/X.Y.Z with an optional prerelease suffix. Anchored: trailing junk is a
# failure, not something to ignore.
_RELEASE = re.compile(r"binder/(\d+)\.(\d+)\.(\d+)(?P<pre>-[0-9A-Za-z.\-]+)?\Z")

# A Go PSEUDO-VERSION: `<base>-<14-digit UTC timestamp>-<12 hex commit>`, which
# debug.ReadBuildInfo() reports for an un-ldflagged `go build` in a VCS checkout.
#
# The separator before the timestamp is `-` or `.` depending on which of Go's
# three forms applies (`v0.0.0-<ts>-<rev>` with no base tag, `vX.Y.Z-0.<ts>-<rev>`
# on a release base, `vX.Y.Z-pre.0.<ts>-<rev>` on a prerelease base). Matching
# only `-` misses the form CI actually produces — that exact off-by-one-character
# bug shipped once already and is locked by the self-test below.
_PSEUDO = re.compile(r"[-.]\d{14}-[0-9a-f]{12}")


def certify(line, allow_prerelease=True):
    """Return None if `line` (the full `binder --version` output) certifies as a
    build fit to gate against, or a human-readable reason why it does not.

    Question 1 (stamped at all?) is applied unconditionally. Question 2
    (prerelease permitted?) is applied only when the caller says so.
    """
    line = (line or "").strip()
    if not line:
        return "produced no --version output at all"

    # --- Question 1: is this build stamped? Absolute, every caller. ---
    if _PSEUDO.search(line):
        return (f"{line!r} is a Go VCS pseudo-version, which means the build was "
                f"NOT ldflag-stamped — gating against it would pin every literal "
                f"to a commit hash and still report success")
    if "+" in line:
        return (f"{line!r} carries `+build` metadata (e.g. `+dirty`), which means "
                f"the build was NOT ldflag-stamped")

    m = _RELEASE.fullmatch(line)
    if not m:
        return (f"{line!r} is not a stamped release version "
                f"({PRODUCER}/X.Y.Z[-prerelease])")

    # --- Question 2: which release shapes does this caller accept? ---
    if m.group("pre") and not allow_prerelease:
        return (f"{line!r} is a prerelease and this gate does not accept "
                f"prereleases")
    return None


# The truth table. Written down independently of the implementation, and asserted
# in BOTH policy modes, because the whole risk of unifying two predicates is that
# one of them quietly relaxes. The pseudo-version and metadata rows must reject
# under *both* modes — that is the "reds before and after" property in table form.
_TRUTH_TABLE = [
    # (version line,                              strict?, lenient?)   ok = True
    ("binder/0.5.3",                                  True,  True),
    ("binder/10.20.30",                               True,  True),
    # prerelease: the ONLY row where the two modes are allowed to differ.
    ("binder/1.2.3-rc1",                              False, True),
    ("binder/0.6.0-beta.2",                           False, True),
    # pseudo-versions: rejected by both, in all three of Go's forms.
    ("binder/0.5.4-0.20260914164646-656b05e16068",    False, False),
    ("binder/0.0.0-20260914164646-656b05e16068",      False, False),
    ("binder/1.2.3-pre.0.20260914164646-656b05e16068", False, False),
    # build metadata: rejected by both, with and without a pseudo-version.
    ("binder/0.5.3+dirty",                            False, False),
    ("binder/0.5.4-0.20260914164646-656b05e16068+dirty", False, False),
    # not a release at all.
    ("binder/dev",                                    False, False),
    ("binder/(devel)",                                False, False),
    ("binder/0.5",                                    False, False),
    ("binder/0.5.3 extra",                            False, False),
    ("binder/v0.5.3",                                 False, False),
    ("",                                              False, False),
]


def _self_test():
    failures = []
    for line, want_strict, want_lenient in _TRUTH_TABLE:
        for allow, want in ((False, want_strict), (True, want_lenient)):
            reason = certify(line, allow_prerelease=allow)
            got = reason is None
            mode = "prerelease-allowed" if allow else "prerelease-rejected"
            if got != want:
                failures.append(
                    f"  {line!r} [{mode}]: expected "
                    f"{'ACCEPT' if want else 'REJECT'}, got "
                    f"{'ACCEPT' if got else 'REJECT'}"
                    + ("" if got else f" ({reason})")
                )
    rows = len(_TRUTH_TABLE)
    # The table itself must not go vacuous, and must actually exercise both the
    # agreeing and the disagreeing cases — otherwise "all rows pass" could be
    # true of a table that no longer tests the decomposition.
    if rows < 15:
        failures.append(f"  truth table shrank to {rows} rows; expected >= 15")
    if not any(s != l for _, s, l in _TRUTH_TABLE):
        failures.append("  no row distinguishes the two policy modes")
    if not any(s == l is False for _, s, l in _TRUTH_TABLE):
        failures.append("  no row is rejected by both modes")
    if failures:
        print(f"stamped_version self-test FAILED ({len(failures)} row(s)):")
        print("\n".join(failures))
        return 1
    print(f"stamped_version self-test OK: {rows} rows x 2 modes")
    return 0


def main():
    args = sys.argv[1:]
    if args and args[0] == "--self-test":
        return _self_test()
    allow = True
    if "--no-prerelease" in args:
        allow = False
        args = [a for a in args if a != "--no-prerelease"]
    if len(args) != 2 or args[0] != "--certify":
        sys.stderr.write(
            "usage: stamped_version.py --certify <version-line> "
            "[--no-prerelease]\n       stamped_version.py --self-test\n")
        return 2
    reason = certify(args[1], allow_prerelease=allow)
    if reason:
        sys.stderr.write(f"{reason}\n")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
