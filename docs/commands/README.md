# binder command reference

Generated from binder's Cobra command tree by `make docs` (`cmd/gendocs`). Do not edit by hand; a drift test (`internal/gendocs`) fails CI if these files fall out of sync with the command tree.

- [binder config get](binder_config_get.md) — Get the resolved value of a configuration key
- [binder config list](binder_config_list.md) — List all resolved configuration values and their sources
- [binder config set](binder_config_set.md) — Set a persistent configuration value in .binder.yaml or user config
- [binder config unset](binder_config_unset.md) — Remove a persistent configuration value
- [binder config](binder_config.md) — Manage configuration (show, get, set, unset)
- [binder convert](binder_convert.md) — Convert a markdown corpus into an OKF v0.2 bundle
- [binder enrich](binder_enrich.md) — Inject missing OKF frontmatter into a source markdown tree, in place
- [binder graph](binder_graph.md) — Export the bundle's concept graph (dot|json|graphml|html)
- [binder index](binder_index.md) — (Re)generate the per-directory index.md nav tree (spec §8)
- [binder infer](binder_infer.md) — Inspect a source markdown corpus and propose a --type-map
- [binder lint](binder_lint.md) — Check a source markdown corpus for broken links, missing titles, orphans, stale, schema issues
- [binder mcp](binder_mcp.md) — Run binder as a stdio MCP server (additive verbs as MCP tools)
- [binder project](binder_project.md) — Project a bundle into offline property-graph DDL (Spanner SQL/PGQ)
- [binder review](binder_review.md) — Summarize a bundle: concepts, unresolved links, orphans, trust tiers, stale
- [binder validate](binder_validate.md) — Check a bundle for OKF v0.2 conformance (spec §11)
- [binder](binder.md) — Convert a plain-markdown corpus into a conformant OKF v0.2 bundle
