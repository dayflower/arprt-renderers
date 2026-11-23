# Swatch validation

## Initial implementation: 2026-11-23

This section records prepublication evidence. See the publication record
below for the released image. No cluster deployment or physical-display
acceptance is claimed.

Local validation uses Linux amd64, independently pinned Go 1.27.3, synthetic
configuration/Context and loopback capture services. No source service, real
device identity, private registry or credentials are required.

- `make check` passes shared Ruff checks, release-coordinator tests and all
  renderer source checks. Swatch's tests cover strict bounded configuration,
  color normalization and duplicate rejection, the 1–24 color limit, note/label
  trimming, limits and escaping, the built-in set order, reference row placement
  per palette/orientation, WCAG overlay text selection including ties, the
  light-tile outline threshold and exact tile fills in generated HTML.
- `make -C renderers/swatch image` builds the scratch production image.
  `make -C renderers/swatch container-check` passes offline tests and shipped
  CLI checks in that image under network-none, UID/GID 1000, read-only root,
  dropped capabilities, no privilege escalation and only the output parent
  writable. Default/custom paths, projected configuration symlinks, palettes,
  rotations, capture/input failures and SIGTERM cancellation are exercised.
- PNG, capture and atomic-write tests are shared in behavior with Countdown:
  signature/CRCs, truncation, trailing bytes, animation, dimensions, rotated
  corner positions, opaque RGB encoding, 503/504 retries, redirects without
  credential forwarding, cancellation and temporary-file cleanup.
- The browser matrix passed 24 captures against a disposable local webcappi
  container (`ghcr.io/dayflower/webcappi@sha256:5cdbb03348521199737c28445841d9f559dae2aa5a49fc87f095c9096a4b0140`)
  on loopback with its bundled fonts. Chromium sandboxing was disabled only for
  this disposable test. Scenarios are the default set, 24 labeled candidates
  with a long Unicode note, and a single candidate without the reference row,
  each with both palettes and all four rotations. Captures verify white
  safe-area margins, that every tile keeps its exact configured RGB value, and
  physical output dimensions; a deliberately insufficient safe area is rejected.
- The locally built static CLI completed configuration-to-PNG execution against
  the same webcappi. Default spectra6 landscape and labeled gray4 portrait
  captures were visually inspected for complete color values, label ellipsis,
  outlines on light tiles and correct clockwise rotation.
- Changed workflow and Screen YAML files parse successfully. Swatch's release
  guards and release-note evidence checks are covered offline; no release
  workflow or remote GitHub CI run has been performed as part of local
  implementation.

The spectra6 reference colors were taken from ARPRT's image-processing palette
at revision `a4b512f`. Generated HTML/PNG captures remain local validation
artifacts under ignored `dist/`, outside the tracked source.

Native arm64 execution remains unverified locally. The verification workflow
defines a separate native arm64 runner; its configuration is not evidence of a
passing run. Kubernetes Job scheduling, ConfigMap integration, controller
palette conversion/dithering/overlays and the appearance of any color on a
physical Spectra 6 or gray4 display are not tested. Collecting those physical
observations is the purpose of this renderer; record them here when performed.

## 2026-11-24 Swatch 0.1.0 publication

[Swatch 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/swatch/v0.1.0)
was published from source revision `eb8c4e762c2bfc1d6a0070dd368a8449b05edeee`.
The exact main-push [CI run](https://github.com/dayflower/arprt-renderers/actions/runs/45292166346)
and independent [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/45292396360)
passed. Release verification built and executed both Linux amd64 and arm64
natively under the Job restrictions before publishing those exact tested images
without rebuilding.

The public image tag is `ghcr.io/dayflower/arprt-renderer-swatch:0.1.0`.
The immutable multi-platform reference is:

```text
ghcr.io/dayflower/arprt-renderer-swatch@sha256:7ffdd35f7f81584058388a564c5df5b920ec4503aa0f0c06b1f5e35d1467fe20
```

The index is a Docker manifest list
(`application/vnd.docker.distribution.manifest.list.v2+json`) with exactly one
image for each platform. Anonymous version-tag and index-digest pulls passed for
both platforms using a fresh empty Docker configuration, and both platform
images carry OCI version `0.1.0`, the source revision above and the repository
source label. Visibility was verified by anonymous access rather than a
package-settings API query.

Published config digests match those of the exact image archives tested on the
release runners:

| Platform | Manifest digest | Config digest |
| --- | --- | --- |
| linux/amd64 | `sha256:c548fe1c27ba0890b1a5a7b0b94008ff6e394804cf30685856d8416bbed03354` | `sha256:ceb048ad1823d1a2fad5d7656409ab9ceb774a3ec02c06bb004b0f3cacb9b278` |
| linux/arm64 | `sha256:e8678b687ddcd5b8a8a201ae3358b68dd89acd53f0e5ae05190aeeaa3e40bd53` | `sha256:8fc5877c43984dfe1e3aef74a338fb1d3faab64fdc213b5bc3eea477276bdbda` |

The public amd64 index digest passed `make -C renderers/swatch container-check
IMAGE=<immutable-reference>` on the local Linux amd64 host after publication,
including shipped-CLI execution, default/custom paths, palettes, rotations,
SIGTERM and previous-output preservation. Postpublication native arm64 execution
was not performed; anonymous arm64 pulls do not substitute for it.

The Screen examples now pin the verified public index. No new webcappi browser,
Kubernetes/ConfigMap/controller or physical-display acceptance was performed
during publication, and no deployment has been made.
