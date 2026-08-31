/**
 * Frontmatter splitting and OKF value normalization.
 *
 * Two jobs, both deliberately dumb:
 *  1. split a markdown file into its YAML frontmatter block and its body,
 *  2. normalize the handful of shapes the OKF spec says a consumer MUST accept
 *     (notably §5.2's "a bare `{ by, at }` mapping is a one-element list").
 *
 * Normalization happens BEFORE the Zod schema sees the data, so the schema can
 * describe one canonical shape instead of a union of every spelling. Nothing
 * here invents a value: a field that is absent stays absent.
 */

import YAML from "yaml";
import type { Actorstamp } from "./trust.js";

/** The result of splitting a markdown file. */
export interface SplitFile {
  /** Parsed YAML frontmatter, or `{}` when the file has no frontmatter block. */
  data: Record<string, unknown>;
  /** The markdown body with the frontmatter block removed. */
  body: string;
  /** True when a `---` frontmatter block was present at all. */
  hasFrontmatter: boolean;
}

const FRONTMATTER_RE = /^﻿?---\r?\n([\s\S]*?)\r?\n---[ \t]*(?:\r?\n|$)/;

// A document that opens a frontmatter fence: its first line is exactly `---`
// (optionally preceded by a BOM). Used only to tell "no frontmatter at all"
// apart from "a fence that was opened but never closed".
const OPENS_FENCE_RE = /^﻿?---(?:\r?\n|$)/;

/**
 * Splits a markdown file into frontmatter data and body.
 *
 * A file with no leading `---` block is returned verbatim as the body with an
 * empty `data` — reserved files such as a per-directory `index.md` carry no
 * frontmatter at all (spec §8), so "no frontmatter" is normal, not an error.
 *
 * Malformed frontmatter, however, is an error, not "empty frontmatter". This
 * throws — matching binder's Go codec (`internal/okf/native`, the source of
 * truth for this wording) — for an unterminated fence, a top level that is a
 * sequence, and a top level that is a scalar (issue #164). Presenting any of
 * these as `data: {}` would let a document nobody parsed be rendered with
 * derived facts (`_okf.stale`, `_okf.tier`) as if its frontmatter had been read
 * and found empty.
 *
 * The error strings below match that Go codec's wording. They are now ENFORCED
 * against it by the cross-language conformance check (issue #171):
 * scripts/conformance/cross-language-conformance.sh derives the expected text
 * live from the Go codec and diffs this copy against it, so editing a Go string
 * without updating this one turns that suite red instead of drifting silently.
 */
export function splitFrontmatter(text: string): SplitFile {
  const match = FRONTMATTER_RE.exec(text);
  if (!match) {
    // No closing fence was found. If the document nonetheless opened a fence,
    // the block is unterminated — an error, not "no frontmatter" (the Go codec
    // rejects it as "invalid frontmatter: unterminated '---' block").
    if (OPENS_FENCE_RE.test(text)) {
      throw new Error("invalid frontmatter: unterminated '---' block");
    }
    return { data: {}, body: stripBOM(text), hasFrontmatter: false };
  }
  const raw = match[1] ?? "";
  const parsed = raw.trim() === "" ? {} : YAML.parse(raw);
  // A comment-only or empty block is legitimately empty frontmatter: the Go
  // codec yields an empty map here, not an error.
  if (parsed === null || parsed === undefined) {
    return { data: {}, body: text.slice(match[0].length), hasFrontmatter: true };
  }
  // A top level that is not a mapping — a sequence or a scalar — is invalid
  // frontmatter. The Go codec errors ("invalid frontmatter: expected a mapping
  // at the top level") rather than silently collapsing it to `{}`.
  if (typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("invalid frontmatter: expected a mapping at the top level");
  }
  return {
    data: parsed as Record<string, unknown>,
    body: text.slice(match[0].length),
    hasFrontmatter: true,
  };
}

function stripBOM(s: string): string {
  return s.charCodeAt(0) === 0xfeff ? s.slice(1) : s;
}

/**
 * Renders a frontmatter scalar as a string the same way binder's trust
 * projection reads it (`okf.asString`), so the two agree on odd YAML scalars.
 */
export function asString(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v;
  return String(v);
}

/**
 * Normalizes the `generated` / `verified` families into a list of actorstamps.
 *
 * Port of binder's `projectActorstamps`: the spec (§5.2) says a bare
 * `{ by, at }` mapping MUST be treated as a one-element list, and both
 * spellings appear in real binder output. Anything that is neither a mapping
 * nor a list yields `undefined` — absent, not an empty list, because "the
 * author wrote nothing" and "the author wrote an empty list" are different
 * facts and only the first one is true here.
 */
export function normalizeActorstamps(v: unknown): Actorstamp[] | undefined {
  if (v === null || v === undefined) return undefined;
  if (Array.isArray(v)) {
    const out: Actorstamp[] = [];
    for (const item of v) {
      if (isPlainObject(item)) out.push(toActorstamp(item));
    }
    return out;
  }
  if (isPlainObject(v)) return [toActorstamp(v)];
  return undefined;
}

/**
 * Normalizes a single `generated` mapping. `generated` is a lone actorstamp in
 * the spec (§5.2), not a list, so it is normalized separately from `verified`.
 */
export function normalizeActorstamp(v: unknown): Actorstamp | undefined {
  if (!isPlainObject(v)) return undefined;
  return toActorstamp(v);
}

function toActorstamp(m: Record<string, unknown>): Actorstamp {
  const stamp: Actorstamp = { by: asString(m.by) };
  const at = asString(m.at);
  if (at !== "") stamp.at = at;
  return stamp;
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}
