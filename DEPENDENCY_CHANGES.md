# Dependency changes — custom build 3.1.0

Verified 2026-11-01. Exact resolved JavaScript versions and integrity hashes are in `frontend/pnpm-lock.yaml`; Go checksums are in `go.sum`. The TypeScript compatibility package is explained in `UPGRADE_NOTES.md`.

## Original frontend packages

| Package | Previous lock | Updated requirement or replacement |
|---|---|---|
| `@chenfengyuan/vue-number-input` | `2.0.1` | `2.0.1` |
| `@vueuse/core` | `14.3.0` | `15.0.0` |
| `@vueuse/integrations` | `14.3.0` | `15.0.0` |
| `ace-builds` | `1.44.0` | `1.45.2` |
| `csv-parse` | `6.2.1` | `7.0.3` |
| `dayjs` | `1.11.20` | `1.11.23` |
| `dompurify` | `3.4.12` | `3.4.17` |
| `epubjs` | `0.3.93` | `0.3.93` |
| `filesize` | `11.0.17` | `11.0.29` |
| `js-base64` | `3.7.8` | `3.9.6` |
| `jwt-decode` | `4.0.0` | `4.0.0` |
| `lodash-es` | `4.18.1` | `4.18.1` |
| `marked` | `18.0.3` | `18.1.2` |
| `marked-katex-extension` | `5.1.8` | `5.1.13` |
| `material-icons` | `1.13.14` | `1.13.14` |
| `normalize.css` | `8.0.1` | `8.0.1` |
| `pinia` | `3.0.4` | `4.1.0` |
| `pretty-bytes` | `7.1.0` | `7.2.0` |
| `qrcode.vue` | `3.9.1` | `3.11.0` |
| `tus-js-client` | `4.3.1` | `4.3.1` |
| `utif` | `3.1.0` | `3.1.0` |
| `video.js` | `8.23.7` | `Replaced by @videojs/html 10.2.0 (Video.js 10)` |
| `videojs-hotkeys` | `0.2.30` | `Removed; built-in Video.js 10 shortcuts` |
| `videojs-mobile-ui` | `1.2.5` | `Removed; built-in Video.js 10 touch gestures` |
| `vue` | `3.5.34` | `3.5.44` |
| `vue-i18n` | `11.4.2` | `11.4.17` |
| `vue-lazyload` | `3.0.0` | `3.0.0` |
| `vue-reader` | `1.3.4` | `1.3.9` |
| `vue-router` | `5.0.7` | `5.4.0` |
| `vue-toastification` | `2.0.0-rc.5` | `2.0.0-rc.5` |
| `@intlify/unplugin-vue-i18n` | `11.2.3` | `11.2.8` |
| `@tsconfig/node24` | `24.0.4` | `Replaced by @tsconfig/node26 26.0.1` |
| `@types/lodash-es` | `4.17.12` | `4.17.12` |
| `@types/node` | `24.12.4` | `26.6.6` |
| `@typescript-eslint/eslint-plugin` | `8.59.3` | `8.72.1` |
| `@vitejs/plugin-legacy` | `8.0.2` | `8.2.3` |
| `@vitejs/plugin-vue` | `6.0.7` | `6.0.10` |
| `@vue/eslint-config-prettier` | `10.2.0` | `10.2.0` |
| `@vue/eslint-config-typescript` | `14.7.0` | `14.9.0` |
| `@vue/tsconfig` | `0.9.1` | `0.9.1` |
| `autoprefixer` | `10.5.0` | `10.6.2` |
| `eslint` | `10.4.0` | `10.14.0` |
| `eslint-config-prettier` | `10.1.8` | `10.1.8` |
| `eslint-plugin-prettier` | `5.5.5` | `5.5.7` |
| `eslint-plugin-vue` | `10.9.1` | `10.12.0` |
| `postcss` | `8.5.18` | `8.5.32` |
| `prettier` | `3.8.3` | `3.9.10` |
| `terser` | `5.47.1` | `5.51.2` |
| `typescript` | `5.9.3` | `npm:@typescript/typescript6@6.0.2` |
| `vite` | `8.0.13` | `8.3.5` |
| `vite-plugin-compression2` | `2.5.3` | `2.5.4` |
| `vitest` | `4.1.6` | `5.0.5` |
| `vue-tsc` | `3.2.9` | `3.3.14` |

## Added frontend and validation packages

| Package | Requirement |
|---|---|
| `@videojs/html` | `10.2.0` |
| `@vue/devtools-api` | `8.2.1` |
| `katex` | `0.19.1` |
| `@tsconfig/node26` | `26.0.1` |
| `@typescript/native` | `npm:typescript@7.0.2` |
| `@playwright/test` | `1.64.1` |
| `rollup` | `4.67.4` |
| `esbuild` | `0.28.2` |
| `jsdom` | `30.1.3` |

## Go module requirements

Includes direct and indirect requirements. Modules omitted after `go mod tidy` are no longer required in the root manifest; the checksum file also covers test dependencies. New pseudo-versions retain their upstream revision and timestamp.

| Module | Previous | Updated |
|---|---|---|
| `github.com/STARRY-S/zip` | `v0.2.3` | `v0.2.3` |
| `github.com/andybalholm/brotli` | `v1.2.2` | `v1.2.7` |
| `github.com/asdine/storm/v3` | `v3.2.1` | `v3.2.1` |
| `github.com/asticode/go-astikit` | `v0.59.0` | `v0.59.0` |
| `github.com/asticode/go-astisub` | `v0.42.0` | `v0.45.0` |
| `github.com/asticode/go-astits` | `v1.15.0` | `v1.17.0` |
| `github.com/bodgit/plumbing` | `v1.3.0` | `v1.3.0` |
| `github.com/bodgit/sevenzip` | `v1.6.5` | `v1.6.5` |
| `github.com/bodgit/windows` | `v1.0.1` | `v1.0.1` |
| `github.com/cespare/xxhash/v2` | `v2.3.0` | `v2.3.0` |
| `github.com/cpuguy83/go-md2man/v2` | `v2.0.7` | `v2.0.7` |
| `github.com/davecgh/go-spew` | `v1.1.2-0.20180830191138-d8f796af33cc` | `Removed by tidy` |
| `github.com/disintegration/imaging` | `v1.6.2` | `v1.6.2` |
| `github.com/dsnet/compress` | `v0.0.2-0.20230904184137-39efe44ab707` | `v0.0.2-0.20230904184137-39efe44ab707` |
| `github.com/dsoprea/go-exif/v3` | `v3.0.1` | `v3.0.1` |
| `github.com/dsoprea/go-logging` | `v0.0.0-20200710184922-b02d349568dd` | `v0.0.0-20200710184922-b02d349568dd` |
| `github.com/dsoprea/go-utility/v2` | `v2.0.0-20221003172846-a3e1774ef349` | `v2.0.0-20221003172846-a3e1774ef349` |
| `github.com/ebitengine/purego` | `v0.10.2` | `v0.11.2` |
| `github.com/flynn/go-shlex` | `v0.0.0-20150515145356-3f9db97f8568` | `v0.0.0-20150515145356-3f9db97f8568` |
| `github.com/fsnotify/fsnotify` | `v1.10.1` | `v1.10.1` |
| `github.com/go-errors/errors` | `v1.5.1` | `v1.5.1` |
| `github.com/go-ole/go-ole` | `v1.3.0` | `v1.3.0` |
| `github.com/go-viper/mapstructure/v2` | `v2.5.0` | `v2.5.0` |
| `github.com/golang-jwt/jwt/v5` | `v5.3.1` | `v5.3.1` |
| `github.com/golang/geo` | `v0.0.0-20260713102120-857a528af641` | `v0.0.0-20261029132150-f51d36688936` |
| `github.com/golang/snappy` | `v1.0.0` | `v1.0.0` |
| `github.com/gorilla/mux` | `v1.8.1` | `v1.8.1` |
| `github.com/gorilla/websocket` | `v1.5.3` | `v1.5.3` |
| `github.com/hashicorp/golang-lru/v2` | `v2.0.7` | `v2.0.7` |
| `github.com/inconshreveable/mousetrap` | `v1.1.0` | `v1.1.0` |
| `github.com/jellydator/ttlcache/v3` | `v3.4.1` | `v3.4.1` |
| `github.com/klauspost/compress` | `v1.19.1` | `v1.20.3` |
| `github.com/klauspost/pgzip` | `v1.2.6` | `v1.3.0` |
| `github.com/maruel/natural` | `v1.3.0` | `v1.3.0` |
| `github.com/marusama/semaphore/v2` | `v2.5.0` | `v2.5.0` |
| `github.com/mholt/archives` | `v0.1.5` | `v0.1.5` |
| `github.com/mikelolasagasti/xz` | `v1.0.1` | `v1.0.1` |
| `github.com/minio/minlz` | `v1.2.0` | `v1.2.3` |
| `github.com/mitchellh/go-homedir` | `v1.1.0` | `v1.1.0` |
| `github.com/nwaples/rardecode/v2` | `v2.2.5` | `v2.4.5` |
| `github.com/pelletier/go-toml/v2` | `v2.4.3` | `v2.4.3` |
| `github.com/pierrec/lz4/v4` | `v4.1.27` | `v4.1.33` |
| `github.com/pmezard/go-difflib` | `v1.0.1-0.20181226105442-5d4384ee4fb2` | `Removed by tidy` |
| `github.com/power-devops/perfstat` | `v0.0.0-20240221224432-82ca36839d55` | `v0.0.0-20260916203055-22a1a467d9f0` |
| `github.com/redis/go-redis/v9` | `v9.21.0` | `v9.23.0` |
| `github.com/russross/blackfriday/v2` | `v2.1.0` | `v2.1.0` |
| `github.com/sagikazarmark/locafero` | `v0.12.0` | `v0.12.0` |
| `github.com/samber/lo` | `v1.53.0` | `v1.53.0` |
| `github.com/shirou/gopsutil/v4` | `v4.26.6` | `v4.26.10` |
| `github.com/sorairolake/lzip-go` | `v0.3.8` | `v0.3.8` |
| `github.com/spf13/afero` | `v1.15.0` | `v1.15.0` |
| `github.com/spf13/cast` | `v1.10.0` | `v1.10.0` |
| `github.com/spf13/cobra` | `v1.10.2` | `v1.10.2` |
| `github.com/spf13/pflag` | `v1.0.10` | `v1.0.10` |
| `github.com/spf13/viper` | `v1.21.0` | `v1.21.0` |
| `github.com/stangelandcl/ppmd` | `v0.1.1` | `v0.1.1` |
| `github.com/stretchr/testify` | `v1.11.1` | `v1.12.1` |
| `github.com/subosito/gotenv` | `v1.6.0` | `v1.6.0` |
| `github.com/tomasen/realip` | `v0.0.0-20180522021738-f0c99a92ddce` | `v0.0.0-20180522021738-f0c99a92ddce` |
| `github.com/ulikunitz/xz` | `v0.5.16` | `v0.5.17` |
| `github.com/yusufpapurcu/wmi` | `v1.2.4` | `v1.2.4` |
| `go.etcd.io/bbolt` | `v1.5.0` | `v1.5.0` |
| `go.uber.org/atomic` | `v1.11.0` | `v1.12.0` |
| `go.yaml.in/yaml/v3` | `v3.0.4` | `v3.0.6` |
| `go4.org` | `v0.0.0-20260112195520-a5071408f32f` | `v0.0.0-20261028031214-ed6929a230fe` |
| `golang.org/x/crypto` | `v0.54.0` | `v0.58.0` |
| `golang.org/x/image` | `v0.44.0` | `v0.47.0` |
| `golang.org/x/net` | `v0.57.0` | `v0.62.0` |
| `golang.org/x/sync` | `v0.22.0` | `v0.24.0` |
| `golang.org/x/sys` | `v0.47.0` | `v0.49.0` |
| `golang.org/x/text` | `v0.40.0` | `v0.43.0` |
| `gopkg.in/check.v1` | `v1.0.0-20201130134442-10cb98267c6c` | `v1.0.0-20201130134442-10cb98267c6c` |
| `gopkg.in/natefinch/lumberjack.v2` | `v2.2.1` | `v2.2.1` |
| `gopkg.in/yaml.v2` | `v2.4.0` | `v2.4.0` |
| `gopkg.in/yaml.v3` | `v3.0.1` | `Removed by tidy` |

## Build, CI, containers and documentation

| Component | Updated version |
|---|---|
| Node.js | 26.13.0 |
| pnpm | 12.16.1 |
| Go build toolchain | 1.27.3; module language version 1.27.0 |
| Task | 3.54.0 |
| commit-and-tag-version | 13.3.0 |
| GoReleaser | 2.19.0 |
| golangci-lint | 2.15.0 |
| BusyBox | 1.38.0-musl |
| Alpine | 3.24.3 |
| LinuxServer s6 base | Alpine 3.24, pinned digest |
| Redis example | 8.10.3-alpine |
| Python documentation build | 3.15.0 |
| `actions/checkout` | v7.0.1 |
| `pnpm/action-setup` | v6.1.0 |
| `actions/setup-node` | v7.1.0 |
| `actions/setup-go` | v7.0.0 |
| `golangci/golangci-lint-action` | v9.3.0 |
| `go-task/setup-task` | v2.2.0 |
| `docker/setup-qemu-action` | v4.4.0 |
| `docker/setup-buildx-action` | v4.4.1 |
| `docker/login-action` | v4.7.0 |
| `goreleaser/goreleaser-action` | v7.3.0 |
| `actions/setup-python` | v7.1.0 |
| `amannn/action-semantic-pull-request` | v6.1.1 |
| `marocchino/sticky-pull-request-comment` | v3.0.5 |
| `mkdocs-material` | 9.8.0 |
| `mkdocs-minify-plugin` | 0.8.0 |
| `mkdocs-redirects` | 1.2.3 |
| `markdown-callouts` | 0.4.0 |

All documentation transitive dependencies are pinned with hashes in `www/requirements.txt` (36 packages). Docker base manifests are pinned to verified registry digests. Packages with no newer release remain on their current version; Vue Toastification retains its newest Vue 3 release, 2.0.0-rc.5, rather than the unrelated Vue 2 `latest` tag.
