> **Current fork documentation:** For File Browser **3.3.1**, use [README](README.md) and [installation/upgrade](docs/INSTALL_3.3.1.md), plus [integrations](docs/INTEGRATIONS_3.3.1.md). Material below is retained as historical/underlying reference and may describe earlier versions.

# Filebrowser custom build 3.1.0

Updated 2026-11-01 from the supplied `filebrowser-master(1).zip`. This is a custom source bundle, not an upstream File Browser release. The Erugo sharing, bandwidth limits, ClamAV, ConvertX, Collabora/WOPI, and archive features remain in the source.

## Build and run

Use Node.js **26.13.0**, pnpm **12.16.1**, Go **1.27.3**, and Task **3.54.0**. Node's supported LTS line is used for the build. The module now requires Go 1.27; macOS binaries require macOS 13 or newer.

```sh
npm install --global pnpm@12.16.1
task build
./filebrowser --root /path/to/files
```

The frontend build runs the Vue checker, the TypeScript 7 checker for TypeScript modules, and the checker for build/test configuration before bundling. `VERSION` supplies the custom build version even when the source was downloaded without Git.

For a local source container build, use:

```sh
docker build -f Dockerfile.source -t filebrowser-custom:3.1.0 .
# Or use the bundled example, which also starts its Redis cache:
docker compose up --build -d
```

`Dockerfile` and `Dockerfile.s6` continue to package prebuilt binaries for GoReleaser. The new `Dockerfile.source` builds this custom application. Container bases, the Redis example, and the documentation container are versioned and pinned to registry digests.

**Existing Compose installations:** the old example mounted the `filebrowser` volume at `/flux/vault`; the updated example mounts it at the default served path `/srv` and adds persistent `/database` and `/config` volumes. Before switching an existing installation, preserve its actual mounts, database path, root path, and configuration. Do not initialize an empty database over the existing one. The updated example is a starting configuration, not an automatic data migration.

## Compatibility changes

| Component | Change and compatibility fix |
|---|---|
| Video.js | Replaced the v8 constructor and its v8-only mobile/hotkey plugins with `@videojs/html` 10.2.0. `MediaPlayer.vue` uses the v10 player and skin elements, native video sources and subtitle tracks, local locale registration, the previous MKV container hint, and explicit playback cleanup on close. Vite recognizes the custom elements. The component was renamed to prevent Vue's self-component inference from colliding with the `video-player` element. |
| Video controls | The v10 skin supplies play/pause, touch gestures, keyboard shortcuts, fullscreen, captions, and playback speed. Browser tests cover playback, keyboard pause/seek, caption toggling, and the fullscreen/speed controls. A narrow, versioned pnpm patch adds this app's previous 2.5× and 3× rates to v10's otherwise fixed list; the browser test verifies 3× playback. Keyboard behavior is scoped to the player. The new skin uses its standard seek controls; shortcut seek remains 10 seconds. |
| Pinia 4 / Router 5 / VueUse 15 | Added the required `@vue/devtools-api` v8 peer. The router store plugin and file-selection refresh behavior have regression coverage. The production browser login/listing test exercises the upgraded store and router together. |
| TypeScript 7 | TS7 is the native compiler and no longer provides the JavaScript compiler API required by Vue's checker and ESLint. Following Microsoft's migration path, `@typescript/native` aliases TS 7.0.2, while `typescript` aliases `@typescript/typescript6` 6.0.2 for compiler-API consumers. These are intentional cooperating packages. Vue SFCs are checked by `vue-tsc`; the native checker uses a Vue-module declaration at that boundary and checks the TypeScript modules separately. Existing exclusions for legacy untyped Options API components remain. |
| EPUB | Updated Vue Reader's location callback to a string CFI and retained support for the initial numeric location. Overrode epub.js's old xmldom dependency with `@xmldom/xmldom` 0.9.12. Tests open a real zipped EPUB through the XML-parser fallback and render it through Vue Reader in Chromium. epub.js itself is already at its newest published 0.3.93 release. |
| KaTeX / Markdown | Upgraded KaTeX to 0.19.1. `marked-katex-extension` 5.1.13 still declares a peer range below 0.19, so pnpm has a narrow allowance for this exact tested pair. This is a local compatibility assertion, not an upstream support declaration. Inline/block MathML, invalid formulas, and unsafe HTML/links have regression coverage. Markdown parsing is isolated from global parser state, uses untrusted KaTeX input, and fails closed when the sanitizer is unsupported. |
| Go YAML | Migrated direct imports from the unmaintained `gopkg.in/yaml.v3` path to `go.yaml.in/yaml/v3` 3.0.6. |
| Backend dependencies | Updated direct and indirect Go requirements and regenerated checksums. The newer linter's archive cleanup, unused callback parameters, redundant field selectors, and Erugo cleanup-variable findings were fixed. |
| pnpm 12 | Regenerated the dependency graph, kept strict peer checking, and recorded lifecycle permissions explicitly: esbuild's installer is allowed; funding-only and package-manager-enforcement hooks are denied. Security-sensitive transitive packages are pinned in `pnpm-workspace.yaml`. |
| Build/release/docs | Updated CI action versions, GoReleaser, golangci-lint, Task and release tooling. Migrated the Homebrew hook to GoReleaser's declarative `install_steps`. Frontend tests and the documentation build now gate releases. Documentation dependencies and transitive packages are pinned with hashes, and missing site pages/links were repaired. |

`DEPENDENCY_CHANGES.md` records every original frontend package and every root Go module requirement, including unchanged packages, removed plugins, and replacements. Packages with no newer release were retained. Vue Toastification remains at its newest Vue 3 release, **2.0.0-rc.5**; the npm `latest` tag **1.7.14** is for Vue 2 and is not a valid upgrade for this application.

## Validation and limits

See `VALIDATION.md` and `validation/results/` for commands and results. The source includes the compiled frontend used for the production smoke test; it can be rebuilt with the pinned toolchain. Ready-built Linux amd64 and arm64 executables and a native test restore installer are now included; follow `NATIVE_INSTALL.md`. Dependency caches and an old nested source ZIP are omitted from the deliverable.

The upgrade does not change the configured external Erugo, ConvertX, ClamAV, Collabora, or Redis servers. Their running versions and real service credentials were not supplied. Existing integration code and its tests are preserved, but live interoperability with those services still needs a staging run. Docker images were not built in this environment because a Docker daemon is unavailable.

A second smoke run initializes the database with a binary built from the original supplied dependencies, then runs the upgraded application against it, including login, listing, upload and download. This checks the sample database upgrade path, not an unseen production database.

The transfer smoke test verifies **64 MiB + 123 bytes**, including a recovered TUS offset, byte-range retrieval, and complete download checksum. It is a compatibility test, not evidence of multi-gigabyte capacity or throughput. The previously assessed resumability, scanning, archive, and background-job architecture improvements remain separate work.

Upgrading dependencies does not repair upstream architectural issues in JWT revocation or the optional command runner. Existing security documentation remains applicable. GoReleaser 2.19 still supports the existing Docker release configuration, while warning that `dockers`/`docker_manifests` will move to its v2 configuration in a future major release. Publishing destinations remain the supplied upstream defaults and must be configured for your fork before publishing. Material for MkDocs 9.8 is current but reaches end of life on 2027-05-05.

## Primary migration references

- [Microsoft TypeScript 7 migration and compiler-API compatibility](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/)
- [Video.js v10 Vue setup](https://videojs.org/docs/guides/installation/vue)
- [Pinia 4 release notes](https://github.com/vuejs/pinia/releases/tag/v4.0.0)
- [Go 1.27 release notes](https://go.dev/doc/go1.27)
- [GoReleaser deprecations and Homebrew migration](https://goreleaser.com/resources/deprecations/)
- [Material for MkDocs release notes](https://squidfunk.github.io/mkdocs-material/changelog/)
