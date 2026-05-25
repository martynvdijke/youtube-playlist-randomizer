## [1.7.1](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.7.0...v1.7.1) (2026-05-25)


### Bug Fixes

* serve version dynamically from Go server instead of hardcoded stale value ([c98ca36](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/c98ca3647c329e69e974bb8af4780da15a1b1aa0))

# [1.7.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.6.3...v1.7.0) (2026-05-23)


### Features

* redesign logo with scattered play triangle and shuffle arrows ([7486e50](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/7486e503a1be43556fa594c50a3ed46c92046fbb))

## [1.6.3](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.6.2...v1.6.3) (2026-05-23)


### Bug Fixes

* **ui:** add autocomplete="off" to search input ([d688721](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/d688721082d99679fd0cda6844ccf63ec1136615))

## [1.6.2](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.6.1...v1.6.2) (2026-05-22)


### Bug Fixes

* use Pacific Time for quota date; show resume queue in stdout and web UI ([8884b1f](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/8884b1f4a82d5fe7729564248d849cc6a3263763))

## [1.6.1](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.6.0...v1.6.1) (2026-05-22)

# [1.6.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.5.5...v1.6.0) (2026-05-21)


### Bug Fixes

* return consistent empty state in handlePlaylistsHTML when ytClient is nil ([8731964](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/87319644b9cff5ee20781d6742c43020f64cc46b))


### Features

* update store, telemetry, and YouTube client ([9b78005](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/9b780057a18e56c9e26d7e12f6f1ddad3821cddb))

## [1.5.5](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.5.4...v1.5.5) (2026-05-21)


### Bug Fixes

* **deps:** update module google.golang.org/api to v0.280.0 ([#27](https://github.com/martynvdijke/youtube-playlist-randomizer/issues/27)) ([be56871](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/be5687134fdb70c7da10753be6fdd5defb2a2204))

## [1.5.4](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.5.3...v1.5.4) (2026-05-21)


### Bug Fixes

* set MaxResults(50) on YouTube API calls and pass playlist title to modal ([a38d34e](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/a38d34e48a96857c74560f726c9d3bc860119b69))

## [1.5.3](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.5.2...v1.5.3) (2026-05-21)

## [1.5.2](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.5.1...v1.5.2) (2026-05-20)


### Bug Fixes

* ensure Gotify notification always fires on release workflow ([18d400f](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/18d400f33117058024b80d5bfc32c1f53d49d052))

## [1.5.1](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.5.0...v1.5.1) (2026-05-20)

# [1.5.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.4.0...v1.5.0) (2026-05-19)


### Features

* add logo and version badge to header ([df74556](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/df745561f6e5dff17f68a649ed98919d3dec31fd))

# [1.4.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.3.0...v1.4.0) (2026-05-19)


### Features

* queue jobs on insufficient quota instead of rejecting, auto-resume on quota reset ([4665028](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/4665028ac194a6ef7f296af8be53f9a6310031a0))

# [1.3.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.2.2...v1.3.0) (2026-05-19)


### Features

* add OAUTH_CALLBACK_URL env var for Docker callback support ([dce2cbf](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/dce2cbf742b2dab9b9c4b0c6b3eb5e42a05fbb6c))

## [1.2.2](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.2.1...v1.2.2) (2026-05-19)


### Bug Fixes

* remove mock ([dfe3cc1](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/dfe3cc1dff46eb264bcecc0b2702852bb149acf5))
* remove old unit tests ([066ed68](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/066ed68b31c95d8cdce0a3aba31913fb773f5dda))

## [1.2.1](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.2.0...v1.2.1) (2026-05-19)


### Bug Fixes

* auto-detect Docker mount paths via DOCKER env var ([1befaa4](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/1befaa4b005d76453c13852236ee95a380326425))

# [1.2.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.1.2...v1.2.0) (2026-05-19)


### Bug Fixes

* align Dockerfile with traces/heat pattern and default to mock mode ([c6f64df](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/c6f64df3e52beecc794919cec761a7e429655b13))


### Features

* add semconv attributes, OTEL_SERVICE_NAME support, and job tracing spans ([2c5611c](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/2c5611c10d4207d8f4f834756cb60e78dc260fbe))

## [1.1.2](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.1.1...v1.1.2) (2026-05-19)


### Bug Fixes

* docker build ([356462d](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/356462d225fda7543a76f368c2756f9af4748ea1))

## [1.1.1](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.1.0...v1.1.1) (2026-05-18)


### Bug Fixes

* remove hardcoded version test ([9be9d17](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/9be9d178936cbc75ece11adc4487f1f245b80cb1))
* update version test to match 1.1.0 ([d2e37d8](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/d2e37d8cb38795d3af7ec233c10403813e27a697))

# [1.1.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v1.0.0...v1.1.0) (2026-05-18)


### Bug Fixes

* e2e tests - remove hidden from playlists container, fix quota bar test ([eb8463c](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/eb8463ccf2083a6e03fb4dc7d63698d589c74e3b))


### Features

* add OpenTelemetry instrumentation with traces and metrics ([55bec30](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/55bec30dd3ee6c34f0fc28ea889760b9609f893b))
* migrate CRUD scenes to htmx with server-rendered HTML fragments ([f611b9e](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/f611b9e2ed91cb2abf04f73d5140ca292d0fbba2))

# [1.0.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v0.5.2...v1.0.0) (2026-05-18)


* feat!: add OpenAPI 3.0 swagger specification documenting YouTube Data API v3 operations ([c3e4a8e](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/c3e4a8e8a9e448fda6d77408b870a9c37b863dd8))
* Merge pull request [#22](https://github.com/martynvdijke/youtube-playlist-randomizer/issues/22) from martynvdijke/go-rewrite ([c7f3f6c](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/c7f3f6c187a3a2db243b8641d14b3acc0d7dae2b))


### Bug Fixes

* playwright e2e tests and ignore test output dirs ([2aa49f9](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/2aa49f97bb17ee9f977e0f8460e30c594ca34b43))
* set config.RedirectURL to match local callback server ([7e3dca4](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/7e3dca44830c74acd9fada1b4a4a6f796ceaadaa))


### Features

* add SQLite-backed quota tracking, job persistence, and resume ([fc890ff](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/fc890ff2e533b0f0a77e29eb8d80f089e3b79adb))
* rewrite Python CLI to Go backend with tests ([81fe467](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/81fe4676d415d50c51ecf92c0ed8f0cec34ecb7f))


### BREAKING CHANGES

* static/swagger.json introduces a new JSON file that documents the API contract. The gitignore was updated to allow tracking the swagger file despite the *.json ignore rule.
* move over to go rewrite with web interface
* Major Breaking Release - rewrite into Go with web frontend

## [0.5.2](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v0.5.1...v0.5.2) (2025-08-14)


### Bug Fixes

* run on port 0 if browser does not work ([e061348](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/e06134842509bb90ffd53be774047be563d0ff3d))

## [0.5.1](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v0.5.0...v0.5.1) (2025-08-14)


### Bug Fixes

* remove dupl icate push, fix formatting issues ([be11f63](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/be11f632e9021917e6f7115f009db6d276c4d885))

# [0.5.0](https://github.com/martynvdijke/youtube-playlist-randomizer/compare/v0.4.0...v0.5.0) (2025-08-14)


### Bug Fixes

* use sed for pyprojet.toml version bump ([9f245be](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/9f245beee00ce636dac96d83af54d357ed315557))
* wrong version ([f74a6a4](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/f74a6a4eef4efb89f7ded4862c49707abfd5d190))


### Features

* make auth flow work on cli again ([4a5514f](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/4a5514f8593ba684342dc8898688d8464fdf40cd))
* refactor and added tests ([4fc4a66](https://github.com/martynvdijke/youtube-playlist-randomizer/commit/4fc4a66c61f90bfd868e18da807f265b54bef84e))
