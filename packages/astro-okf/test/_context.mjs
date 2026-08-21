// A minimal stand-in for Astro's LoaderContext, so the loader can be driven in
// a plain `node --test` run without booting Astro.
//
// NOT a test file — the node test runner only collects *.test.mjs.
//
// This is a stand-in, not a mock of convenience: every member it provides is
// one the real LoaderContext provides (confirmed against the installed
// astro@7.2.4 types in astro/dist/content/loaders/types.d.ts), with the same
// signature, and `parseData` runs the REAL schema. What it cannot prove is that
// Astro drives a loader the way this harness does — that is what the example
// host's actual `astro build` in test/example/ is for. The two together are the
// Phase 1 evidence; neither alone would be.

import { createHash } from "node:crypto";
import { pathToFileURL } from "node:url";

/** A DataStore backed by a Map, matching astro's DataStore interface. */
export function createStore() {
  const map = new Map();
  return {
    get: (key) => map.get(key),
    entries: () => [...map.entries()],
    set: (entry) => {
      map.set(entry.id, entry);
      return true;
    },
    values: () => [...map.values()],
    keys: () => [...map.keys()],
    delete: (key) => void map.delete(key),
    clear: () => map.clear(),
    has: (key) => map.has(key),
    addModuleImport: () => {},
  };
}

/**
 * Builds a LoaderContext whose `parseData` validates against `schema`, so a
 * loader that emits a shape the schema rejects fails here rather than silently
 * passing a unit test and failing in a real build.
 */
export function createContext({ schema, root, collection = "kb" } = {}) {
  const store = createStore();
  const logs = { info: [], warn: [], debug: [], error: [] };
  return {
    store,
    logs,
    context: {
      collection,
      store,
      meta: new Map(),
      logger: {
        info: (m) => logs.info.push(m),
        warn: (m) => logs.warn.push(m),
        debug: (m) => logs.debug.push(m),
        error: (m) => logs.error.push(m),
        options: {},
        label: "astro-okf",
        fork: () => ({}),
      },
      config: { root: root ? pathToFileURL(root + "/") : undefined },
      parseData: async ({ data }) => (schema ? await schema.parseAsync(data) : data),
      renderMarkdown: async (content) => ({ html: content }),
      generateDigest: (data) =>
        createHash("sha256")
          .update(typeof data === "string" ? data : JSON.stringify(data))
          .digest("hex"),
    },
  };
}
