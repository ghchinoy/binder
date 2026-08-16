# Changelog

## [0.3.1](https://github.com/ghchinoy/binder/compare/v0.3.0...v0.3.1) (2026-08-16)


### Bug Fixes

* key docs-impact gate on option text and declare bound ([#116](https://github.com/ghchinoy/binder/issues/116)) ([2548cf7](https://github.com/ghchinoy/binder/commit/2548cf7c6b29d11704334cf86a0f5087d3c8802b))
* preserve unchanged pre-existing keys byte-for-byte ([427503e](https://github.com/ghchinoy/binder/commit/427503e40b224d270a5d5a27eb2afb9cb99161c8))
* quote SKILL.md description so frontmatter is valid YAML ([#90](https://github.com/ghchinoy/binder/issues/90)) ([568b3c6](https://github.com/ghchinoy/binder/commit/568b3c608b8a6247d85816daabec03fbdf7bd093)), closes [#88](https://github.com/ghchinoy/binder/issues/88)
* stop code-region stray bracket from swallowing next link ([#117](https://github.com/ghchinoy/binder/issues/117)) ([8d47111](https://github.com/ghchinoy/binder/commit/8d4711168ecdecc88648c3f745c8292a996d2657))


### Continuous Integration

* enforce PR template docs-impact field ([#108](https://github.com/ghchinoy/binder/issues/108)) ([6bf1f0d](https://github.com/ghchinoy/binder/commit/6bf1f0db9178073b2724cf81f96e0e4d88e7f3b9))


### Documentation

* add in-memory labeled property graph primer ([#110](https://github.com/ghchinoy/binder/issues/110)) ([31769de](https://github.com/ghchinoy/binder/commit/31769deab3972ebc8f2cfc8e36a3bdc40784d99f))
* add okf cross-check step to tutorial, de-jargon pass ([#105](https://github.com/ghchinoy/binder/issues/105)) ([57579cb](https://github.com/ghchinoy/binder/commit/57579cb991300258900fb7b355a693bf82c45117))
* correct anchor slug rules and name which slug is which ([#91](https://github.com/ghchinoy/binder/issues/91)) ([62ea325](https://github.com/ghchinoy/binder/commit/62ea32537d9a554832fdb7e4f22c9f26d200d1da))
* editorial pass on the LPG primer and link it from the guide ([#114](https://github.com/ghchinoy/binder/issues/114)) ([80a8eb6](https://github.com/ghchinoy/binder/commit/80a8eb6516d8cac8562c609ec29d7248c070d2f6))
* fix false trust and contract claims in okf-convert skill ([#112](https://github.com/ghchinoy/binder/issues/112)) ([84116dc](https://github.com/ghchinoy/binder/commit/84116dcd18e9cc8736a6167d9cada555d57afc52))
* focus the README on using binder, not building it ([#98](https://github.com/ghchinoy/binder/issues/98)) ([7e2eb3c](https://github.com/ghchinoy/binder/commit/7e2eb3cb704b10821caf9cc2715a85e8c9ac6bfa))
* record validate's reserved-file scope in guide and tutorial ([#102](https://github.com/ghchinoy/binder/issues/102)) ([23a82a3](https://github.com/ghchinoy/binder/commit/23a82a300bc29226c773f6417ad14b231bebfd77)), closes [#97](https://github.com/ghchinoy/binder/issues/97)
* tidy download v-note and mark JSON output agent-ready ([#107](https://github.com/ghchinoy/binder/issues/107)) ([8ddc8a0](https://github.com/ghchinoy/binder/commit/8ddc8a06a9970c1cb611caf6b2569431cf017609))
* trim README to quickstart, rehome MCP parity to guide ([#111](https://github.com/ghchinoy/binder/issues/111)) ([b10b9ab](https://github.com/ghchinoy/binder/commit/b10b9ab2f2f3aa51808bae4daf0f312919a7ed4a))

## [0.3.0](https://github.com/ghchinoy/binder/compare/v0.2.1...v0.3.0) (2026-08-16)


### Features

* **convert:** --external-root suppresses advisories for known sibling workspaces ([#25](https://github.com/ghchinoy/binder/issues/25)) ([7c3174f](https://github.com/ghchinoy/binder/commit/7c3174f4709dd89dda2c7e82d2d025d7d2567b74))
* **enrich:** --overwrite-keys refreshes named keys in place, refusing trust keys ([#22](https://github.com/ghchinoy/binder/issues/22)) ([8fcf727](https://github.com/ghchinoy/binder/commit/8fcf727d44d16627b8342a4ecf05ebfa8939ef65))
* **graph:** read-only query_graph MCP tool with five traversal operations ([#33](https://github.com/ghchinoy/binder/issues/33)) ([bb383ae](https://github.com/ghchinoy/binder/commit/bb383ae62792ed9ad73bd3bc06199a1e9e89e822))
* **mcp:** expose external_root on convert and name all seven tools in help ([#62](https://github.com/ghchinoy/binder/issues/62)) ([8ea12b0](https://github.com/ghchinoy/binder/commit/8ea12b0e224d07c9cbfd287e416ff04c9f2ddad5))
* **review:** reclassify entrypoints vs orphans in review and lint ([#24](https://github.com/ghchinoy/binder/issues/24)) ([3546fda](https://github.com/ghchinoy/binder/commit/3546fdab22a69c59e96792b6aa7e6af943753d4c))
* **status:** validate --status-map vocabulary; opt-in canonicalization ([#23](https://github.com/ghchinoy/binder/issues/23)) ([7f4ca6b](https://github.com/ghchinoy/binder/commit/7f4ca6b4c8161b5abf90d61db1a6a20fd8e3f4ea))


### Bug Fixes

* **build:** canonicalize the version stamp to a single no-v form across all install paths ([e9df155](https://github.com/ghchinoy/binder/commit/e9df155f10243e89ff55d76ff4df4df0c887342c))
* **cli:** stop claiming root index.md is a recognized entrypoint ([b98f502](https://github.com/ghchinoy/binder/commit/b98f502fe554ade38bb7c83d718e96a56d346e7d)), closes [#73](https://github.com/ghchinoy/binder/issues/73)
* **cli:** usage errors exit 2 and infer emits stable empty arrays ([8b2083a](https://github.com/ghchinoy/binder/commit/8b2083afc8ca6c160408f4ec9c9384d41a83316c))
* **infer:** write the zero-mapping diagnostic to stderr so stdout stays machine-consumable ([#67](https://github.com/ghchinoy/binder/issues/67)) ([bcba1fd](https://github.com/ghchinoy/binder/commit/bcba1fd2f654e1e1c81a9d5e71d309b8bb50c386))
* keep underscores and hyphen runs in okf heading slugs ([#84](https://github.com/ghchinoy/binder/issues/84)) ([9b34a0f](https://github.com/ghchinoy/binder/commit/9b34a0ffe85f53e7c0e9dc00bd423db137a676c8))
* **lint:** stop electing a root index.md as an entrypoint ([#75](https://github.com/ghchinoy/binder/issues/75)) ([34c9483](https://github.com/ghchinoy/binder/commit/34c9483aa0ee5bc9ef17625c750e3633e8773adf)), closes [#71](https://github.com/ghchinoy/binder/issues/71) [#72](https://github.com/ghchinoy/binder/issues/72)
* make validate disclose unchecked reserved-file scope ([#83](https://github.com/ghchinoy/binder/issues/83)) ([6208383](https://github.com/ghchinoy/binder/commit/62083831b8cc18491561c800ac20d5fa912762c7))


### Documentation

* **clijson:** envelope keys are struct-order, not sorted ([948260e](https://github.com/ghchinoy/binder/commit/948260ef4d8cf215f713d92c572bf6bac5c308eb))
* correct entrypoints label, tutorial claims, MCP parity ([#78](https://github.com/ghchinoy/binder/issues/78)) ([69cd81f](https://github.com/ghchinoy/binder/commit/69cd81fc5664fc686f3164be9a0f66daade17039))
* correct README, tutorial and RELEASING against the shipped v0.3.0 binary ([#65](https://github.com/ghchinoy/binder/issues/65)) ([9df8eaf](https://github.com/ghchinoy/binder/commit/9df8eafe8ff615fd059b085cc1632c17372c57f4))
* **graph:** document the graph surface and ship a graph-format sample ([#36](https://github.com/ghchinoy/binder/issues/36)) ([1fb9284](https://github.com/ghchinoy/binder/commit/1fb928420fd59524b139d998e01a84d1fd946d55))
* **lint:** correct the divergence example in a doc comment ([#81](https://github.com/ghchinoy/binder/issues/81)) ([5bdc3e1](https://github.com/ghchinoy/binder/commit/5bdc3e117dd548dbd62225e1b6a2587f6088c15b))
* **readme:** correct lint/review agreement and root entrypoint claims ([#69](https://github.com/ghchinoy/binder/issues/69)) ([7bad872](https://github.com/ghchinoy/binder/commit/7bad87209ec2ea88e0b384a2d4841675d4834e7d))
* **user-guide:** correct and complete user guide for v0.3.0 ([#70](https://github.com/ghchinoy/binder/issues/70)) ([2293734](https://github.com/ghchinoy/binder/commit/2293734f2c353dfdbbf3d62a08e0931186281315))

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
