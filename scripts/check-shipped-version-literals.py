#!/usr/bin/env python3
"""Version-literal drift gate for binder's SHIPPED OUTPUT (issue #60).

#60 was filed to stop a hand-maintained `binder/0.3.0` exemplar going stale. It
did not: the trap fired through v0.4.0 and again through v0.5.3, reaching users
on the invalid-actor ERROR path and not only in `--help`. Two releases escaped
because nothing mechanical ever compared what the binary PRINTS against what the
binary IS. That comparison is this gate.

# What it checks

Run a STAMPED binder over binder's user-facing text surfaces and require that
every `binder/<X.Y.Z>` literal it emits equals the stamped binary's own version.
Separately, `binder/v<digit>` anywhere in that output is always a finding: PR #52
established that the git tag carries a leading `v` and the producer string must
not, so a v-prefixed producer version is wrong at every release, not just stale.

# Why the output and not the source

The source half of this guard lives in Go, in internal/version's
TestNoHardCodedVersionLiteralInShippedGo: it parses shipped .go files and rejects
any hard-coded binder version in a string literal. That half is cheaper, runs in
`make check`, and catches the mistake at authoring time — but it structurally
CANNOT check values, because a `go test` build is unstamped and reports
binder/dev (the same "KNOWN LIMIT" that forced #169's gate out of process).

So the two halves are complementary, not redundant:

  - the Go test answers "is any version hard-coded?" (no stamped build needed,
    no idea what the right version is);
  - this script answers "does what ships actually say the shipping version?"
    (needs a stamped build, checks values, and is indifferent to how the string
    was constructed — a literal, a concatenation, or a template all reach it the
    same way).

A literal assembled at run time, or one arriving from a non-Go source, passes the
first and is caught by the second.

# Minimum-coverage assertion (why this gate cannot pass vacuously)

"0 findings" is only trustworthy if the gate actually reached the text it is
meant to pin. A version gate that inspects nothing and exits 0 is the exact
silent-permissive failure #169 exists to remove, so this gate carries an EXPLICIT
inventory of the must-reach surfaces and asserts each one both RAN and yielded at
least one version literal. A renamed flag, a reworded error, or a command that
stops emitting the exemplar fails LOUD instead of passing green.

The inventory is a per-surface list, not an aggregate floor: a floor like
">= 4 literals total" can be satisfied coincidentally while one specific surface
goes dark (another surface gaining a literal as this one loses it).

Each entry additionally carries a MINIMUM COUNT rather than merely requiring
presence, matching the inventory shape #185 landed for the transcript gate. Bare
presence is too weak wherever one surface carries several literals: both help
surfaces emit two exemplars (the flag's own example, and the shared actor-forms
hint appended to it), so one could stop rendering while the other kept the entry
satisfied. Counts are MINIMA — adding an exemplar needs no inventory edit;
losing one fails loud and says by how much.

KNOWN LIMIT, recorded rather than claimed closed. Cardinality is not identity.
A count cannot distinguish four literals from a DIFFERENT four, so an offsetting
edit — one exemplar added as another is hidden, on the same surface — nets the
same total and passes. Exact counts do not fix this; they fail the same way and
additionally make every legitimate addition an inventory edit, which is why the
floor is a minimum. What actually closes it is identity rather than quantity
(pinning *which* literal each surface must carry), and that is deliberately not
attempted here. The drift comparison limits the blast radius in practice — every
literal that IS seen must equal the stamped version — so the residual hole is
narrow: a surface that loses a real exemplar while gaining an unrelated but
correctly-versioned one. A shared inventory helper across this gate and #185's
is filed as follow-up rather than built late in a sequence across two in-flight
PRs.

# Documented transcripts of the error text (the third rule)

#60's fix makes the invalid-actor error version-derived, which immediately
staleds every doc that transcribes that error — `docs/tutorial.md` and
`docs/user_guide.md` both quote it verbatim. Those literals were *correct* before
this change (they are the actor-grammar example baked into the error, not a
drifted provenance stamp, which is why #169's JSON-fence scoping rightly never
touched them); they go stale in the same commit that changes the error string.

Bumping them to the current version would simply re-create #60 one release later
in a file nothing pins. So instead of writing down what the error says, this gate
DERIVES it: for every documented `binder: invalid actor "<actor>"` line found, it
re-runs the binary with that same actor and requires the documented line to equal
what the binary actually prints. There is no expected text anywhere in this
script, so there is nothing to keep up to date and drift is structurally
impossible rather than merely watched for.

This lives here rather than in #185's transcript gate on purpose: that gate scans
static markdown and is JSON-fence-scoped, and these are prose. Deriving the
expected value requires RUNNING the binary, which is this gate's defining
capability.

Exit non-zero if any literal drifts OR if any must-reach surface was not covered
OR if any documented transcript disagrees with the binary; exit 0 only when every
must-reach surface was covered and everything matches.

Usage: check-shipped-version-literals.py <stamped-binder-binary> [repo-root]
"""
import os
import re
import subprocess
import sys
import tempfile

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "lib"))
from stamped_version import certify  # noqa: E402

# A binder version literal in emitted text: binder/<major>.<minor>.<patch>, with
# an optional prerelease suffix (goreleaser can ship v1.2.3-rc1).
VERSION_LITERAL = re.compile(r"binder/(\d+\.\d+\.\d+(?:-[0-9A-Za-z.\-]+)?)")

# The v-prefixed producer form banned by PR #52. Checked independently of the
# drift comparison because it is wrong even when the digits are current.
V_PREFIXED = re.compile(r"binder/v\d")

# A documented transcript of the invalid-actor error. The actor is captured so
# the expected text can be re-derived by running the binary with that same actor
# rather than written down here.
DOC_ACTOR_LINE = re.compile(r'^binder: invalid actor "(?P<actor>.*?)"; valid forms:.*$')

# Where documented transcripts are looked for, and the floor below which "no
# mismatches" stops meaning anything. Two are known today (docs/tutorial.md and
# docs/user_guide.md); if the count ever drops the gate reds rather than quietly
# verifying nothing. README.md is #185's surface and is deliberately not scanned.
DOC_ROOTS = ["docs"]
MIN_DOC_TRANSCRIPTS = 2

# Cobra's subcommand block, used to sweep the whole command tree for help text.
AVAILABLE_COMMANDS = re.compile(r"^Available Commands:$")
COMMAND_LINE = re.compile(r"^  (\S+)\s")

# Subcommands with no binder-authored help worth sweeping: `completion` is
# generated by Cobra and `help` just reprints another command's text.
SKIP_COMMANDS = {"completion", "help"}


def run(binder, args, env=None):
    """Run binder and return stdout+stderr together. The exit code is ignored on
    purpose: several must-reach surfaces ARE error paths, and their text is the
    thing under test."""
    full_env = dict(os.environ)
    if env:
        full_env.update(env)
    p = subprocess.run([binder] + args, capture_output=True, text=True, env=full_env)
    return p.stdout + p.stderr


def discover_commands(binder):
    """Return every subcommand path (as an argv list) reachable from the root,
    by walking Cobra's `Available Commands:` blocks."""
    found = []

    def walk(path):
        out = run(binder, path + ["--help"])
        in_block = False
        for line in out.splitlines():
            if AVAILABLE_COMMANDS.match(line):
                in_block = True
                continue
            if in_block:
                if not line.strip():
                    break
                m = COMMAND_LINE.match(line)
                if not m:
                    continue
                name = m.group(1)
                if name in SKIP_COMMANDS:
                    continue
                child = path + [name]
                found.append(child)
                walk(child)

    walk([])
    return found


def check_doc_transcripts(binder, repo_root, corpus, env):
    """Require every documented `binder: invalid actor ...` line to equal what the
    binary actually prints for that same actor.

    Returns (findings, transcripts_found). The expected text is DERIVED by
    running the binary, never written down, so this cannot itself go stale."""
    findings = []
    found = 0
    for root_name in DOC_ROOTS:
        root = os.path.join(repo_root, root_name)
        if not os.path.isdir(root):
            findings.append((f"{root_name}/", "MISSING-DOC-ROOT",
                             "declared documentation root does not exist; the "
                             "scan silently shrank instead of failing"))
            continue
        for dirpath, _, filenames in os.walk(root):
            for name in sorted(filenames):
                if not name.endswith(".md"):
                    continue
                path = os.path.join(dirpath, name)
                rel = os.path.relpath(path, repo_root)
                with open(path, encoding="utf-8") as fh:
                    for lineno, line in enumerate(fh, 1):
                        m = DOC_ACTOR_LINE.match(line.rstrip("\n"))
                        if not m:
                            continue
                        found += 1
                        actual = run(
                            binder,
                            ["enrich", corpus, "--verified-by", m.group("actor")],
                            env,
                        ).strip()
                        documented = line.rstrip("\n")
                        if documented not in actual.splitlines():
                            findings.append((
                                f"{rel}:{lineno}", "DOC-DRIFT",
                                f"documents {documented!r} but the binary prints "
                                f"{actual!r}",
                            ))
    return findings, found


def main() -> int:
    if not 2 <= len(sys.argv) <= 3:
        sys.stderr.write(__doc__.strip().splitlines()[-1] + "\n")
        return 2
    binder = sys.argv[1]
    # The repo whose docs are checked. Defaults to this script's own repo, and is
    # overridable so the fixture harness can point the gate at a patched copy and
    # PROVE the doc rule reds — a rule never shown to fail is not known to work.
    repo_root = os.path.abspath(
        sys.argv[2] if len(sys.argv) == 3
        else os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    )

    # check=True: a binary that runs but exits non-zero must raise rather than
    # let a partial/empty --version through. Same reasoning as #169's gate.
    expected = subprocess.run(
        [binder, "--version"], capture_output=True, text=True, check=True
    ).stdout.strip()

    # Refuse to run against a build that does not certify. The predicate is NOT
    # kept here: it lives in scripts/lib/stamped_version.py, shared with the
    # build helper and with #169's gate, because a precondition each caller
    # re-decides for itself is not shared at all. This gate permits prereleases
    # (goreleaser can ship v1.2.3-rc1); the absolute checks — no pseudo-version,
    # no build metadata — are applied to every caller and are not negotiable.
    #
    # The gate must still certify even though build_stamped_binder already did:
    # the checker takes a binary PATH and can be handed one that the helper never
    # built, which is exactly what the fixture harness does.
    reason = certify(expected, allow_prerelease=True)
    if reason:
        sys.stderr.write(
            f"FATAL: `{binder} --version` did not certify — {reason}.\n"
            f"This gate is not a stamped release build. Build one with\n"
            f'  go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version='
            f'$(git describe --tags --abbrev=0)" -o <bin> .\n'
            f"or just use scripts/lib/stamped-binder.sh, which builds AND "
            f"certifies in one step.\n"
        )
        return 2

    workdir = tempfile.mkdtemp()
    src = os.path.join(workdir, "corpus")
    os.makedirs(src, exist_ok=True)
    with open(os.path.join(src, "a.md"), "w") as fh:
        fh.write("# A\n\nbody\n")
    out = os.path.join(workdir, "out")

    # An isolated HOME so a developer's real ~/.binder.yaml cannot change what
    # the error-path surfaces below emit.
    isolated = {"HOME": workdir, "XDG_CONFIG_HOME": workdir}

    # MUST-REACH INVENTORY. Each entry is (key, label, argv, env). These are the
    # surfaces #60 enumerated, expressed as things the binary actually does:
    #
    #   - the two --verified-by flag-help copies (cmd/convert.go, cmd/enrich.go);
    #   - the invalid-actor error raised from the FLAG path, which is where a
    #     stale exemplar reached users;
    #   - the invalid-actor error raised from the CONFIG-LOAD path, the other
    #     consumer of config.ActorFormsHint. Both are listed because they are
    #     separate call sites: one going stale while the other stays correct is
    #     precisely the divergence the shared hint exists to prevent.
    #
    # The MCP convert tool's invalid-actor error is the fourth #60 site and is
    # NOT reachable here — driving it needs a JSON-RPC session over stdio. It is
    # covered instead by internal/mcp's TestInvalidActorExemplarTracksLiveVersion
    # and by the Go source gate. Recorded rather than silently omitted.
    # The trailing int is the MINIMUM number of version literals the surface must
    # emit. The two help surfaces are 2 because the --verified-by usage string
    # carries its own example AND appends config.ActorFormsHint(), which carries
    # a second; requiring only 1 would let either go dark unnoticed.
    must_reach = [
        ("help:convert", "convert --verified-by flag help",
         ["convert", "--help"], None, 2),
        ("help:enrich", "enrich --verified-by flag help",
         ["enrich", "--help"], None, 2),
        ("error:flag-actor", "invalid-actor usage error (flag path)",
         ["convert", src, "-o", out, "--verified-by", "agent:bot"], isolated, 1),
        ("error:config-actor", "invalid-actor usage error (config-load path)",
         ["config", "list"], dict(isolated, BINDER_VERIFIED_BY="agent:bot"), 1),
    ]

    findings = []
    covered = {}          # key -> literals found on that surface
    surfaces_scanned = 0
    literals_checked = 0

    def scan(key, label, text):
        nonlocal surfaces_scanned, literals_checked
        surfaces_scanned += 1
        hits = VERSION_LITERAL.findall(text)
        covered.setdefault(key, []).extend(hits)
        for got in hits:
            literals_checked += 1
            if ("binder/" + got) != expected:
                findings.append(
                    (label, "DRIFT",
                     f"emits binder/{got} but the binary is {expected}")
                )
        for m in V_PREFIXED.finditer(text):
            findings.append(
                (label, "V-PREFIXED",
                 f"emits a v-prefixed producer version at offset {m.start()}; "
                 f"the canonical form has no leading v (PR #52)")
            )

    # 1. The must-reach inventory.
    for key, label, argv, env, _ in must_reach:
        scan(key, label, run(binder, argv, env))

    # 2. A breadth sweep over every command's help text, so a NEW surface that
    #    introduces a stale literal is caught without anyone remembering to
    #    extend the inventory. The inventory guarantees the floor; this
    #    guarantees the reach.
    for path in discover_commands(binder):
        label = "binder " + " ".join(path) + " --help"
        scan("sweep:" + " ".join(path), label, run(binder, path + ["--help"]))

    # 3. Documented transcripts of the error text must equal what the binary
    #    prints. Expected values are derived from the binary, not written down.
    doc_findings, doc_found = check_doc_transcripts(binder, repo_root, src, isolated)
    findings.extend(doc_findings)

    # 4. Vacuous-pass guard: every must-reach surface has to have produced AT
    #    LEAST its declared number of version literals. Running a command that
    #    prints nothing relevant is not coverage, and neither is a surface that
    #    quietly drops one of the two exemplars it is supposed to carry.
    coverage_fail = [
        (key, label, len(covered.get(key, [])), minimum)
        for key, label, _, _, minimum in must_reach
        if len(covered.get(key, [])) < minimum
    ]

    # The documented-transcript rule has its own coverage floor: "no mismatches"
    # is worthless if the scan found no transcripts to compare.
    if doc_found < MIN_DOC_TRANSCRIPTS:
        coverage_fail.append(
            ("docs:invalid-actor-transcripts",
             "documented invalid-actor transcripts under " + "/, ".join(DOC_ROOTS),
             doc_found, MIN_DOC_TRANSCRIPTS)
        )

    print(f"# shipped-output version gate: {surfaces_scanned} surface(s) scanned, "
          f"{literals_checked} version literal(s) checked, "
          f"{doc_found} documented transcript(s) verified against the binary, "
          f"stamped binary = {expected}")
    for label, kind, detail in findings:
        print(f"{label}: [{kind}] {detail}")
    if coverage_fail:
        print("# COVERAGE FAILURE: must-reach surface(s) emitted fewer version "
              "literals than declared, so a green result would be VACUOUS. "
              "Likely cause: a renamed flag or command, a reworded error, or a "
              "surface that stopped showing the actor exemplar.")
        for key, label, got, minimum in coverage_fail:
            print(f"# MISSING-COVERAGE: {label} [{key}] "
                  f"emitted {got} version literal(s), expected at least {minimum}")
    print(f"# {len(findings)} drift finding(s), "
          f"{len(coverage_fail)} coverage failure(s)")
    return 1 if (findings or coverage_fail) else 0


if __name__ == "__main__":
    sys.exit(main())
