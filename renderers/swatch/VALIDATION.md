# Swatch validation

## Initial implementation: 2026-11-23

Version 0.1.0 is unreleased. No registry publication, cluster deployment or
physical-display acceptance is claimed.

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
