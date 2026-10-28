# Development

Use uv, Make and Docker, plus the language toolchain needed by each renderer
(Python 3.13 for Picsum). The root Makefile runs shared lint/format checks and
delegates renderer checks to their own Makefiles. Ruff and CI's uv are pinned in the root Makefile and workflow respectively.

Run these commands from the repository root:

```sh
make setup
make check
make -C renderers/picsum image
make -C renderers/picsum container-check
```

`make setup` calls each renderer's `setup` target. Picsum creates its own
`renderers/picsum/.venv/` and installs exactly its pinned requirements. Renderer
environments are separate, so dependencies and language versions can differ.

`make check` runs shared Ruff lint/import-order and formatting checks, then each
renderer's `check` target. `make format` applies shared Python formatting. To work
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
network. Only its disposable output parent is writable, including unit-test
temporary files; no `/tmp` tmpfs is provided. Dependencies are installed at image
build time, never inside a running renderer.

The [CI workflow](../.github/workflows/ci.yaml) runs on PRs, main pushes and manual
dispatch. It runs repository-wide lint/format checks and calls each renderer's
verification workflow. Each renderer workflow prepares its own toolchain and
dependencies; the shared CI job does not run renderer setup or tests.

The reusable [Picsum checks](../.github/workflows/check-picsum.yaml) run lint/format
on `renderers/picsum/` using the global Ruff configuration, set up Picsum's Python
environment, and run its tests. Native amd64 and arm64 jobs build and check the
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

During the initial migration, local Linux amd64 container builds, offline unit
tests and restricted CLI checks are the validation baseline. Native arm64 runs
are configured in GitHub Actions but have not yet run for this new repository.
Public GHCR pulls, release execution, Kubernetes scheduling/PVC collection and
physical displays remain unverified until separately exercised. A multi-platform
build or emulated execution alone is not evidence of native arm64 acceptance.

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
