# Development

Use uv, Make and Docker, plus the language toolchain needed by each renderer
(Go 1.27.3 for Picsum, RSS and iCalendar, selected by their independent `go.mod` files).
Python 3 is also required for renderer container/release tools. The root Makefile runs shared lint/format checks and
delegates renderer checks to their own Makefiles. Ruff and CI's uv are pinned in the root Makefile and workflow respectively.

Run these commands from the repository root:

```sh
make setup
make check
make -C renderers/picsum image
make -C renderers/picsum container-check
```

`make setup` calls each renderer's `setup` target. Picsum downloads and verifies
its fixed Go modules. A Go installation with automatic toolchain selection can
download the pinned toolchain. Renderer modules remain separate, so dependencies
and language versions can differ. `make -C renderers/picsum build` produces the
local static CLI at `renderers/picsum/dist/renderer`.

`make check` runs shared Ruff lint/import-order and formatting checks, then each
renderer's `check` target. It also runs `tools-check`, the offline Python
standard-library tests for the common release coordinator. Picsum checks Go formatting, runs `go vet` and Go
tests, and runs the Python standard-library release guard tests.
`make format` applies shared Python formatting; use `gofmt -w` for Go files. To work
only on Picsum:

```sh
make -C renderers/picsum setup
make -C renderers/picsum check
```

Picsum's `check` target runs its offline conversion/download-boundary and release
guard tests. Its release tools and tests live under `renderers/picsum/`.
Its `image` and `container-check` targets accept `IMAGE` (default
`arprt-renderer-picsum:dev`); `image` also accepts `VERSION` and `REVISION` for
OCI labels. The container checker generates synthetic inputs and disables the
network. It exports a static Go test binary using the Dockerfile's `test-export`
target and mounts it read-only into the exact production image. The runtime
contains no Python, compiler or test suite. Only its disposable output parent is
writable, including unit-test temporary files; no `/tmp` tmpfs is provided.
Dependencies are downloaded at build time, never inside a running renderer.

The [CI workflow](../.github/workflows/ci.yaml) runs on PRs, main pushes and manual
dispatch. It runs repository-wide lint/format checks and calls each renderer's
verification workflow. Each renderer workflow prepares its own toolchain and
dependencies; the shared CI job runs release-coordinator tests via `make tools-check` but
does not run renderer setup or tests.

The reusable [Picsum checks](../.github/workflows/check-picsum.yaml) run lint/format
on `renderers/picsum/` using the global Ruff configuration, set up Picsum's Go
toolchain, and run its tests. Native amd64 and arm64 jobs build and check the
shipped image, including all three CLI modes, four clockwise rotations, accepted
target palettes, default paths and separate custom paths. Automated tests make
no live Picsum requests.

The Picsum release calls this same renderer workflow directly. It checks only
Picsum, so another renderer's dependencies or test failures do not block that
release. The workflow exports the exact tested native images for publication,
without rebuilding in the publish job. For local checks with the same scope:

```sh
make lint format-check RUFF_PATHS=renderers/picsum
make -C renderers/picsum setup
make -C renderers/picsum check
```

Ruff's version, Python target version and lint rules remain repository-wide.
`RUFF_PATHS` only selects which paths to check or format; its default is `.`.

## Validation scope

The [Picsum 0.2.0 release](https://github.com/dayflower/arprt-renderers/releases/tag/picsum/v0.2.0)
passed native Linux amd64 and arm64 container builds, offline unit tests and CLI
checks under Job restrictions at source revision
`c14896bacee28e645089518bf1d8c0a09f602ab0`. Anonymous public tag/digest pulls and
OCI revision/version labels were verified for both platforms. Published image
config digests match the exact image archives tested on the native runners.
The public amd64 image also passed the offline container checker after publication.

Live Picsum requests, Kubernetes scheduling/PVC collection and physical displays
remain unverified for this release. Native prepublication checks and the
postpublication amd64 execution have distinct scopes; anonymous arm64 pulls
verify availability, not postpublication native arm64 execution.

ARPRT v0.2.2 is the imported contract/source baseline, not a claim of an end-to-end
test against a deployed controller. Record expanded validation in the relevant
release notes when it is performed.

## Adding a renderer

Create `renderers/<name>/` with its own README, implementation/tests, fixed
dependencies, Makefile, Dockerfile, fixtures, VERSION and any release tooling.
Keep implementation and distribution boundaries aligned with this directory so
it can be extracted later.
Each renderer Makefile provides `setup` and `check`; the root discovers Makefiles
under `renderers/*/` and calls these targets for whole-repository work. Keep
renderer-specific build and container checks there as well. Adding another
renderer does not require putting its dependencies into Picsum's environment or
changing the root Makefile.

Keep the root [README](../README.md) as a project introduction and renderer
index. Update it when adding or removing a renderer, or changing its type (Job or
HTTP), purpose or external service requirements. Keep version, image and
validation details in the renderer documents; routine releases do not require a
root README update.

ARPRT creates Jobs from an `ArprtScreen`, so Job renderers need no Helm chart.
If an HTTP renderer is added, install its Deployment/Service/configuration chart
separately from the controller Helm release and other renderers. Chart and image
versions may differ; do not bundle all renderers into the controller chart.
Consider splitting a renderer into its own repository when maintainers,
permissions, licensing or usage become independent.

The root LICENSE applies to all renderers and is copied into their images.
Use the repository root as the image build context with the renderer's Dockerfile
and a `Dockerfile.dockerignore` beside it to limit the context to its inputs and
LICENSE.

Add a renderer-specific verification workflow such as `check-<name>.yaml` with
its own toolchain setup, tests and native image checks. Call it from `ci.yaml`
for normal CI and directly from that renderer's release workflow. Do not call
all-repository setup/check targets from a renderer release. Give the renderer
its own `<name>/v*` tag filter, `ghcr.io/dayflower/arprt-renderer-<name>` image,
release guard and concurrency group. Changes to one renderer must not require
releasing another or changing the controller's version.
Run shared lint checks on every PR; do not add a common SDK or release framework
until multiple implementations need one. See [repository policy](../AGENTS.md) for
language, Git operations and commit attribution.

## RSS development and distribution

RSS is an independent Go module under `renderers/rss/`. Root `setup` and `check`
already discover it through its Makefile. Its pinned modules provide HTML/XML
charset handling and image decoding/resampling; dependencies and their licenses
are recorded in its README and third-party notices. Picsum uses no RSS modules.

```sh
make lint format-check RUFF_PATHS=renderers/rss
make -C renderers/rss setup
make -C renderers/rss check
make -C renderers/rss image
make -C renderers/rss container-check
```

RSS `check` includes Go formatting, vet, offline tests and Python release guards.
The production image is scratch with a static binary, CA certificates and
licenses; timezone data is embedded. Its Dockerfile-specific allowlist includes
only Go sources, module metadata and licenses. Fixtures, Screen examples,
release scripts and local capture outputs do not enter the build context.
The test-export stage builds a separate static test executable; tests and tools
are absent from the shipped runtime. Container checks run without external
network access, with UID/GID 1000, read-only root, dropped capabilities and only
the output parent writable, with no writable `/tmp` requirement.

[RSS checks](../.github/workflows/check-rss.yaml) run source checks and native
amd64/arm64 image builds and offline execution. Each native job checks image
architecture and version/revision labels. Normal CI calls all renderer
workflows plus shared lint. RSS releases call only RSS checks with image archive
export enabled; the publish job loads those exact tested images without rebuilding.
RSS has its own release guard, package, version and concurrency group.

The local RSS container checks cover Linux amd64. Native arm64 execution must be
confirmed by the arm64 runner; workflow configuration alone is not a passing run.
Offline stub captures verify the software contract, not Chromium layout. The
[RSS validation record](../renderers/rss/VALIDATION.md) separately records live-feed
captures in both orientations and an isolated native amd64 Kubernetes run using
the real RSS image and webcappi. That run verified Job execution, PVC collection,
Screen updates and preservation of the last successful image after a failed
render. A subsequent physical check displayed the live Hatena Bookmark feed on
both targets with existing rotation and overlay settings: the user reported
readable gray4 text and rougher Spectra 6 text. This narrow check does not establish
acceptance of other feeds, rotations, photos or overlays. Broader production-cluster
behavior remains unverified. RSS 0.1.0 passed native amd64 and arm64 release checks,
anonymous tag/digest pulls on both platforms, and postpublication amd64 container
checks. Postpublication native arm64 execution was not performed.

## iCalendar development and distribution

iCalendar owns its Go module, comparison experiment, VERSION, image, synthetic
fixtures and release helpers under `renderers/ical/`. Root setup/check discovers
its Makefile. Go 1.27.3 is pinned independently in both its main and comparison
modules; source CI caches both dependency sets.

```sh
make lint format-check RUFF_PATHS=renderers/ical
make -C renderers/ical setup
make -C renderers/ical check
make -C renderers/ical image VERSION=0.2.0 REVISION=your-source-revision
make -C renderers/ical container-check
```

`check` covers Go formatting, vet, offline semantic/input/capture/output tests,
dependency comparison and Python release guards. `image` accepts IMAGE, VERSION
and REVISION. The static scratch runtime includes CA certificates, embedded
timezone data and licenses; no browser or test tools ship in it. Container checks
export a separate static test executable and run it in the exact production image
with network none, UID/GID 1000, read-only root, dropped capabilities and only the
output parent writable. Dependencies are never installed at runtime.

[iCalendar checks](../.github/workflows/check-ical.yaml) run scoped lint/source
checks and native amd64/arm64 image execution. Normal CI calls them alongside
Picsum/RSS checks. The independent release calls only iCalendar checks, exporting
the tested native images for publication without rebuilding. Browser/layout,
cluster file mounts/Secret updates, physical-display readability and live-source
compatibility have separate scopes in the [validation record](../renderers/ical/VALIDATION.md).
No stub capture or cross-build substitutes for those acceptance checks.

iCalendar 0.2.0 passed native Linux amd64 and arm64 release builds and offline
container execution at source revision
`239d294e40c874e5a0b5914be9a3c6a69034ba33`. Anonymous version/digest pulls passed
on both platforms, and the public amd64 digest passed postpublication container
checks. Postpublication native arm64 execution was not performed. See the
[iCalendar validation record](../renderers/ical/VALIDATION.md#2026-11-05-icalendar-020-publication)
for the immutable reference and separate acceptance scope.

## Weather development and distribution

Weather is an independent Go module under `renderers/weather/`, with its own
VERSION, Dockerfile, synthetic fixtures, container checker and release guards.
It uses only the Go standard library. Root setup/check discovers its Makefile.

```sh
make -C renderers/weather setup
make check
make -C renderers/weather image
make -C renderers/weather container-check
```

`check-weather.yaml` runs scoped source checks and native amd64/arm64 image
execution; normal CI calls it alongside the other renderer workflows. Weather
releases call only Weather verification and transfer the tested images without
rebuilding. The runtime is scratch; tests use a separate exported binary and
loopback stubs under the Job security restrictions. Real-browser checks are
opt-in. Live API compatibility, browser layout, cluster execution and physical
display acceptance have separate scopes in the Weather validation record.

Weather 0.2.0 passed native Linux amd64 and arm64 release builds and offline
container execution at source revision
`971599d6be9838df78cfc8cc18bf0f2c8a88daed`. Anonymous version/digest pulls and
OCI revision/version labels passed for both platforms. The public amd64 image
also passed postpublication container checks. Postpublication native arm64
execution was not performed. See the [Weather publication record](../renderers/weather/VALIDATION.md#2026-11-06-weather-020-publication)
for the immutable reference and separate cluster/physical-display limits.

## Prometheus development

Prometheus owns an independent standard-library-only Go module under
`renderers/prometheus/`, with Go 1.27.3 pinned in its module metadata.
Root setup/check discovers its Makefile. Configuration validation, query
planning, bounded HTTP/snapshot acquisition and panel normalization are available;
image rendering, production containers and release workflows are not yet
implemented. No Prometheus release is registered with the coordinator.

```sh
make -C renderers/prometheus setup
make -C renderers/prometheus check
```

Its README defines the complete version-1 configuration, calendar/freshness
restrictions, deterministic request identity, snapshot format, accepted API
results, model semantics and resource budgets. Loopback tests cover authentication,
retry/cancellation, concurrency, decompression and cumulative limits; the same
response parser feeds HTTP and snapshot models. Optional independent schema and
Chromium geometry checks have separate dependencies and are excluded from
ordinary offline tests. The validation record
distinguishes the minimal browser experiment from actual capture acceptance.
`container-check` fails explicitly until a production runtime exists.

## Common release coordinator

`tools/release.py` is a repository-level operator tool for bump/check/publish/status,
not a renderer runtime or publication dependency. It uses Python 3.12+, Git and
GitHub CLI without extra Python dependencies. Renderer-specific guards, image
builds, release notes and distribution remain under each renderer.
See [RELEASE.md](RELEASE.md) for commands and manual postpublication work.

`make tools-check` runs offline tests with temporary Git repositories and fake
GitHub responses, including tag conflicts, exact-commit CI matching, pagination,
API errors, uncertain pushes, workflow delays, timeout/resume and release evidence.
It never pushes tags or creates a release. Normal CI runs these tests in the
common job; individual renderer release workflows do not depend on this tool.
