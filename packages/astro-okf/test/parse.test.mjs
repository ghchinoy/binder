// parse.test.mjs — frontmatter splitting and the spec 5.2 normalization that
// lets every downstream consumer see one canonical shape.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  deriveTier,
  normalizeActorstamp,
  normalizeActorstamps,
  splitFrontmatter,
} from "../dist/index.js";

test("splits a frontmatter block from the body", () => {
  const { data, body, hasFrontmatter } = splitFrontmatter(
    "---\ntype: Note\ntitle: Hello\n---\n\n# Hello\n\nBody text.\n",
  );
  assert.equal(hasFrontmatter, true);
  assert.deepEqual(data, { type: "Note", title: "Hello" });
  assert.equal(body, "\n# Hello\n\nBody text.\n");
});

test("a file with no frontmatter is all body, and that is not an error", () => {
  // Reserved per-directory index.md files carry no frontmatter at all (spec 8).
  const { data, body, hasFrontmatter } = splitFrontmatter("# Concepts\n\n* [a](a.md)\n");
  assert.equal(hasFrontmatter, false);
  assert.deepEqual(data, {});
  assert.equal(body, "# Concepts\n\n* [a](a.md)\n");
});

// Issue #164 — malformed frontmatter must fail closed, matching binder's Go
// codec (internal/okf/native) rather than being presented as empty frontmatter
// (`data: {}`). Before the fix these three cases returned quietly and let a
// document nobody parsed reach Zod as `{}`, so a maintainer saw "type Required"
// instead of the real defect and a consumer with a permissive schema got
// derived trust facts about an unread document. Each of these three throws only
// after the fix — they are the negative fixtures.

test("#164 case C: an unterminated frontmatter fence throws, not silently treated as no-frontmatter", () => {
  // The Go codec: "invalid frontmatter: unterminated '---' block".
  assert.throws(
    () => splitFrontmatter("---\ntype: Note\ntitle: Hello\n\n# Body with no closing fence\n"),
    /unterminated '---' block/,
  );
});

test("#164 case D: a top-level YAML sequence throws, not silently emptied", () => {
  // The Go codec: "invalid frontmatter: expected a mapping at the top level".
  assert.throws(
    () => splitFrontmatter("---\n- one\n- two\n---\n\n# Body\n"),
    /expected a mapping at the top level/,
  );
});

test("#164 case E: a top-level YAML scalar throws, not silently emptied", () => {
  assert.throws(
    () => splitFrontmatter("---\njust a bare string\n---\n\n# Body\n"),
    /expected a mapping at the top level/,
  );
});

test("#164 case A (positive control): valid human-reviewed frontmatter still parses and derives human-reviewed", () => {
  const { data, hasFrontmatter } = splitFrontmatter(
    "---\ntype: Note\nverified:\n  - by: human:alice\n---\n\n# Body\n",
  );
  assert.equal(hasFrontmatter, true);
  assert.equal(data.type, "Note");
  const tier = deriveTier(normalizeActorstamps(data.verified));
  assert.equal(tier, "human-reviewed");
});

test("a comment-only frontmatter block is legitimately empty, not an error", () => {
  // The Go codec yields an empty map here (doc has no content), not an error.
  const { data, hasFrontmatter } = splitFrontmatter("---\n# just a comment\n---\n\n# Body\n");
  assert.equal(hasFrontmatter, true);
  assert.deepEqual(data, {});
});

test("a --- horizontal rule inside the body is not mistaken for frontmatter", () => {
  const { hasFrontmatter, body } = splitFrontmatter("Intro\n\n---\n\nMore\n");
  assert.equal(hasFrontmatter, false);
  assert.equal(body, "Intro\n\n---\n\nMore\n");
});

test("CRLF line endings split the same way", () => {
  const { data, hasFrontmatter } = splitFrontmatter("---\r\ntype: Note\r\n---\r\n\r\nBody\r\n");
  assert.equal(hasFrontmatter, true);
  assert.deepEqual(data, { type: "Note" });
});

test("ISO dates stay strings, so no timezone is invented for them", () => {
  // stale_after is compared as a string against a YYYY-MM-DD day (spec 5.5).
  // A YAML parser that coerced these to Date objects would silently attach a
  // timezone the author never wrote.
  const { data } = splitFrontmatter(
    '---\nstale_after: 2027-01-01\ngenerated: { by: human:alice, at: 2026-01-02T09:00:00Z }\n---\n',
  );
  assert.equal(typeof data.stale_after, "string");
  assert.equal(data.stale_after, "2027-01-01");
  assert.equal(typeof data.generated.at, "string");
  assert.equal(data.generated.by, "human:alice");
});

test("spec 5.2: a bare {by, at} mapping normalizes to a one-element list", () => {
  assert.deepEqual(normalizeActorstamps({ by: "human:carol", at: "2026-03-01T12:00:00Z" }), [
    { by: "human:carol", at: "2026-03-01T12:00:00Z" },
  ]);
});

test("spec 5.2: a list of stamps is preserved in order", () => {
  assert.deepEqual(
    normalizeActorstamps([
      { by: "human:bob", at: "2026-02-01T10:00:00Z" },
      { by: "binder/0.1.0", at: "2026-02-02T10:00:00Z" },
    ]),
    [
      { by: "human:bob", at: "2026-02-01T10:00:00Z" },
      { by: "binder/0.1.0", at: "2026-02-02T10:00:00Z" },
    ],
  );
});

test("an absent family stays absent — it never becomes an empty list", () => {
  // "the author wrote nothing" and "the author wrote an empty list" are
  // different facts; only the first is true of a missing key.
  assert.equal(normalizeActorstamps(undefined), undefined);
  assert.equal(normalizeActorstamps(null), undefined);
  assert.equal(normalizeActorstamps("human:bob"), undefined);
  assert.equal(normalizeActorstamp(undefined), undefined);
  assert.equal(normalizeActorstamp("human:bob"), undefined);
});

test("`at` is omitted rather than emitted empty when the stamp has no time", () => {
  assert.deepEqual(normalizeActorstamps([{ by: "human:bob" }]), [{ by: "human:bob" }]);
  assert.deepEqual(normalizeActorstamp({ by: "binder/0.1.0" }), { by: "binder/0.1.0" });
});
