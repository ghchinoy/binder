// ts-conformance.mjs — binds the astro-okf TypeScript codec to the Go source of
// truth for issue #171.
//
// It runs the SHIPPED TypeScript (packages/astro-okf/dist, what actually gets
// published) over the shared input corpus and diffs the result against the
// golden that scripts/conformance/gogolden derived live from the Go code:
//
//   - splitFrontmatter's thrown message vs the Go codec's error text, for each
//     frontmatter case;
//   - isHumanActor / isValidActor vs okf.IsHumanActor / okf.IsValidActor, for
//     each actor case (the trust.ts re-implementation #171 also covers).
//
// Nothing here restates a Go literal: the expected values come only from the
// golden. A mismatch is reported loudly, naming the file that diverged and the
// exact Go-vs-TS values, and exits non-zero.
//
// Usage: node ts-conformance.mjs <corpus.json> <go-golden.json>

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const [corpusPath, goldenPath] = process.argv.slice(2);
if (!corpusPath || !goldenPath) {
  console.error("usage: node ts-conformance.mjs <corpus.json> <go-golden.json>");
  process.exit(2);
}

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..", "..");
const distEntry = path.join(repoRoot, "packages", "astro-okf", "dist", "index.js");

let mod;
try {
  mod = await import(distEntry);
} catch (e) {
  console.error(
    `FATAL: could not import ${distEntry}: ${e.message}\n` +
      "Build the package first: npm run build --workspace astro-okf",
  );
  process.exit(2);
}
const { splitFrontmatter, isHumanActor, isValidActor } = mod;

const corpus = JSON.parse(readFileSync(corpusPath, "utf8"));
const golden = JSON.parse(readFileSync(goldenPath, "utf8"));

const goError = new Map(golden.frontmatter.map((c) => [c.name, c.error]));
const goActor = new Map(golden.actors.map((a) => [a.actor, a]));

let fails = 0;

// Error strings: packages/astro-okf/src/parse.ts (compiled to dist).
for (const c of corpus.frontmatter) {
  let tsErr = "";
  try {
    splitFrontmatter(c.input);
  } catch (e) {
    tsErr = e.message;
  }
  const want = goError.get(c.name) ?? "";
  if (tsErr === want) {
    console.log(`  ok   parse.ts frontmatter[${c.name}] matches Go`);
  } else {
    console.log(
      `  FAIL TypeScript parse.ts DIVERGED from Go on frontmatter[${c.name}]:\n` +
        `         Go: ${JSON.stringify(want)}\n` +
        `         TS: ${JSON.stringify(tsErr)}`,
    );
    fails++;
  }
}

// Actor derivations: packages/astro-okf/src/trust.ts (compiled to dist).
for (const actor of corpus.actors) {
  const want = goActor.get(actor);
  if (!want) {
    console.log(`  FAIL corpus actor ${JSON.stringify(actor)} missing from Go golden`);
    fails++;
    continue;
  }
  const gotHuman = isHumanActor(actor);
  const gotValid = isValidActor(actor);
  if (gotHuman === want.isHuman && gotValid === want.isValid) {
    console.log(`  ok   trust.ts actor ${JSON.stringify(actor)} matches Go`);
  } else {
    console.log(
      `  FAIL TypeScript trust.ts DIVERGED from Go on actor ${JSON.stringify(actor)}:\n` +
        `         Go: isHuman=${want.isHuman} isValid=${want.isValid}\n` +
        `         TS: isHuman=${gotHuman} isValid=${gotValid}`,
    );
    fails++;
  }
}

if (fails > 0) {
  console.log(`  -> TypeScript: ${fails} divergence(s) from the Go source of truth.`);
  process.exit(1);
}
process.exit(0);
