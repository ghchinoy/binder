# Changelog

## [0.2.1](https://github.com/ghchinoy/binder/compare/v0.2.0...v0.2.1) (2026-08-16)


### Features

* **config:** binder config get, set, unset — persistent settings in ./.binder.yaml, or in the user config with --global ([#47](https://github.com/ghchinoy/binder/issues/47)) ([a5a4eb9](https://github.com/ghchinoy/binder/commit/a5a4eb9cec613ebb8af111715919ce2016f3d1ac))

## [0.2.0](https://github.com/ghchinoy/binder/compare/v0.1.0...v0.2.0) (2026-08-16)


### Features

* **infer:** binder infer — propose type-map from corpus signals ([#38](https://github.com/ghchinoy/binder/issues/38)) ([#43](https://github.com/ghchinoy/binder/issues/43)) ([092a7dc](https://github.com/ghchinoy/binder/commit/092a7dc9bbc747c01b34ed0e335364d890834f62))

## 0.1.0 (2026-08-16)


### Features

* **#13:** --json machine-readable output + stable exit-code contract ([#16](https://github.com/ghchinoy/binder/issues/16)) ([5efa87b](https://github.com/ghchinoy/binder/commit/5efa87b3201f43911ab404015a37aab919ce6184))
* **#14:** okf-convert Agent Skill + Plugin bundle ([#26](https://github.com/ghchinoy/binder/issues/26)) ([341010b](https://github.com/ghchinoy/binder/commit/341010bf7f5140a7bcaec61b37d46ae74606eadd))
* **#15:** binder mcp — stdio MCP server (convert/validate/review/lint/graph) ([#27](https://github.com/ghchinoy/binder/issues/27)) ([b0d0d13](https://github.com/ghchinoy/binder/commit/b0d0d138fa40b66e0b57294c826f4d5fee6e7712))
* **#5:** binder enrich — in-place, frontmatter-only enrichment ([#21](https://github.com/ghchinoy/binder/issues/21)) ([e82566a](https://github.com/ghchinoy/binder/commit/e82566a23d6ef50e1497746d31e766c54bba24ee))
* **#6:** resolve workspace-relative file:// URIs as internal edges ([#17](https://github.com/ghchinoy/binder/issues/17)) ([2d6db9f](https://github.com/ghchinoy/binder/commit/2d6db9f3129f9fb5abdc41c4a4aec28ac753f1d8))
* **#7,#10:** declarative trust/lifecycle flags, binder config, and --strict mode ([#19](https://github.com/ghchinoy/binder/issues/19)) ([b64621c](https://github.com/ghchinoy/binder/commit/b64621ce5dd3bd0c5e35d78214d6db794bd52262))
* **#8:** binder lint — standalone source-corpus linter ([#20](https://github.com/ghchinoy/binder/issues/20)) ([7f00a86](https://github.com/ghchinoy/binder/commit/7f00a86e621405108457d9dbfb73702c0a033770))
* **#9:** type-grouped catalog + backlink/graph synthesis in root index.md ([#18](https://github.com/ghchinoy/binder/issues/18)) ([7993ce0](https://github.com/ghchinoy/binder/commit/7993ce0770bb18b2249e2765cddf9b8f41524b0b))
* **mcp:** add read-only list_graphs graph-introspection tool ([#15](https://github.com/ghchinoy/binder/issues/15) follow-on) ([#32](https://github.com/ghchinoy/binder/issues/32)) ([cd93ec6](https://github.com/ghchinoy/binder/commit/cd93ec6f75e6a89108cb41770fae51e90070e08f))
* native OKF v0.2 codec (yaml.v3 + goldmark); drop factile ([8269ee0](https://github.com/ghchinoy/binder/commit/8269ee09a629f7bacfcb466d5add4c5c09f09171))
* **okf:** shape-validate trust signals as advisories (actor/date/status/sources) ([87c043f](https://github.com/ghchinoy/binder/commit/87c043f604ee14b1ea91b1b2c81246f8ce9f91cc))
* Phase 1 vertical slice — okf model+interfaces, factileadapter, converter, CLI ([d223047](https://github.com/ghchinoy/binder/commit/d223047853dcf2f7c43551f1543c71e0afcdca54))
* Phase 2 relationship extraction, per-dir index, review & graph ([b363dd5](https://github.com/ghchinoy/binder/commit/b363dd57a0d9186690726768127ddc561965ad4e))


### Bug Fixes

* materialize fm-ref edges to body; coherent review of residual wikilinks & unterminated-fence recovery ([98cb0d4](https://github.com/ghchinoy/binder/commit/98cb0d41a330bd99b9f1e161f9044eeae43c4796))
* **native:** preserve nested/list order when stamping frontmatter (R1) ([cadab97](https://github.com/ghchinoy/binder/commit/cadab97bce5545cdd56ce55a4ee893dd50030541))
* never-reject unparseable frontmatter in convert; coherent review broken-link/recovery reporting ([fde7525](https://github.com/ghchinoy/binder/commit/fde752547a8b11baf11932cd8c6e1a0da88e82e5))
* **release:** remove package-name, reset manifest to 0.0.0, pin initial-version 0.1.0 ([#37](https://github.com/ghchinoy/binder/issues/37)) ([a0836d5](https://github.com/ghchinoy/binder/commit/a0836d5a2eb5922325266b153fea598263411028))
* report recovery from an explicit persisted marker, not a body-shape guess ([f200d89](https://github.com/ghchinoy/binder/commit/f200d89fcbbabf4dde1f6d5407edc7797eeeca6b))


### Continuous Integration

* add release pipeline (release-please + goreleaser) ([#30](https://github.com/ghchinoy/binder/issues/30)) ([11a6ac4](https://github.com/ghchinoy/binder/commit/11a6ac44d6f378bc1a31bd1b3131f915c86d91d5))

## Changelog
