# Development

Use uv, Make and Docker, plus the language toolchain needed by each renderer
(Go 1.27.3 for Picsum, selected by its `go.mod`). Python 3 is also required for
Picsum container/release tools. The root Makefile runs shared lint/format checks and
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
renderer's `check` target. Picsum checks Go formatting, runs `go vet` and Go
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
dependencies; the shared CI job does not run renderer setup or tests.

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

The Go migration is unreleased. Local Linux amd64 builds, offline tests and CLI
checks under Job restrictions have passed. Native arm64 execution, live Picsum
requests, Kubernetes integration and physical displays remain unverified for
the Go implementation. CI is configured to check both native architectures.
The 0.1.0 evidence below applies to the released Python/Pillow implementation.

The [Picsum 0.1.0 release](https://github.com/dayflower/arprt-renderers/releases/tag/picsum/v0.1.0) passed native Linux amd64 and arm64
container builds, offline unit tests and CLI checks under Job restrictions.
Anonymous public tag/digest pulls and OCI revision/version labels were
verified for both platforms. Published image config digests match the exact
image archives tested on the native runners. The public amd64 image also
passed the offline container checker after publication.

Live Picsum requests, Kubernetes scheduling/PVC collection and physical
displays remain unverified for this release. The native prepublication
checks and the postpublication amd64 execution have distinct scopes;
a multi-platform build or emulation alone is not native arm64 acceptance.

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
