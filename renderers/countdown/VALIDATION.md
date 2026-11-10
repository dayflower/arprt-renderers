# Countdown validation

## Initial implementation: 2026-11-10

Version 0.1.0 is unreleased. No registry publication, cluster deployment or
physical-display acceptance is claimed.

Local validation uses Linux amd64, independently pinned Go 1.27.3, synthetic
configuration/Context and loopback capture services. No target-data service,
real device identity, private registry or credentials are required.

- `make check` passes shared Ruff checks, release-coordinator tests and all
  renderer source checks. Countdown's tests cover strict bounded configuration,
  title escaping, timezone boundaries, Gregorian leap years, DST, a skipped
  local date, the full years 0001–9999 range and one execution-start clock sample.
- `make -C renderers/countdown image` builds the scratch production image.
  `make -C renderers/countdown container-check` passes offline tests and shipped
  CLI checks in that image under network-none, UID/GID 1000, read-only root,
  dropped capabilities, no privilege escalation and only the output parent
  writable. Default/custom paths, projected configuration symlinks, palettes,
  rotations, capture/input failures and SIGTERM cancellation are exercised.
- The locally built static CLI also completes configuration-to-PNG execution
  against the real Chromium adapter with Noto CJK fonts. This supplements the
  offline production-image checks; it does not test a webcappi deployment.
- PNG tests check signature/CRCs, truncation, trailing bytes, animation rejection,
  dimensions, all rotated corner positions, transparent-to-white compositing,
  opaque RGB encoding, atomic replacement and temporary-file cleanup.
- Capture tests cover the request contract, Bearer authorization, 503/504 retries,
  nonretryable statuses, redirects without credential forwarding, invalid capture
  bodies and cancellation. Oversized configuration, Context and token files fail
  before capture.
- Real Chromium layout checks pass through a temporary local Playwright HTTP
  adapter implementing the used capture request and readiness contract. This is
  browser evidence, not a test of the separately operated webcappi application.
  The adapter blocks page resource requests, waits for `#countdown-ready`, fails
  on the page's layout-error marker and captures the exact viewport.
- The browser matrix passes 48 captures: six scenarios (reference, title,
  tomorrow, target day, past target and long Unicode title with seven digits),
  two palettes and four rotations. It verifies nonempty output, white inset
  margins and physical output dimensions; a deliberately insufficient safe area
  is rejected. Captures use zero insets for the reference and asymmetric
  4/8/12/16 top/bottom/left/right insets for other scenarios.
- The matrix passes with the host's DejaVu fallback and with Noto Sans CJK JP.
  Noto CJK reference and long-title captures were visually inspected for readable
  dates, complete numbers and title ellipsis. Portrait artwork is captured upright
  at 480 × 800, then rotated once into the physical 800 × 480 PNG.
- Changed workflow and Screen YAML files parse successfully. Countdown's release
  guards and release-note evidence checks are covered offline; no release workflow
  or remote GitHub CI run has been performed as part of local implementation.

Browser libraries and Noto CJK were extracted into temporary verification
directories only; they are not runtime dependencies or bundled assets.
Generated HTML/PNG captures remain local validation artifacts, outside the
tracked source. To reproduce against a real webcappi, use the opt-in command in
[README.md](README.md).

Native arm64 execution remains unverified locally. The verification workflow
defines a separate native arm64 runner; its configuration is not evidence of a
passing run. Real webcappi deployment/authentication, Kubernetes Job scheduling,
ConfigMap/PVC integration, controller delivery/overlays, final palette conversion
and readability on gray4/Spectra 6 physical displays remain unverified.

## 2026-11-11 Countdown 0.1.0 publication

[Countdown 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/countdown/v0.1.0)
was published from source revision `70d30c97c685d38b8a9ffc627a322b25e1730bcf`.
The exact main-push [CI run](https://github.com/dayflower/arprt-renderers/actions/runs/42910448556)
and independent [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/42923914079)
passed. Release verification built and executed both Linux amd64 and arm64
natively under the Job restrictions before publishing those exact tested images
without rebuilding.

The public image tag is `ghcr.io/dayflower/arprt-renderer-countdown:0.1.0`.
The immutable multi-platform reference is:

```text
ghcr.io/dayflower/arprt-renderer-countdown@sha256:a6a227dabbc3693da6c37e07c5a1f4a0a303aef274662bb3bf8ee350f54e48c2
```

Anonymous version-tag and index-digest pulls passed for both platforms using a
fresh empty Docker configuration. Anonymous registry requests also verified the
index/manifest/config SHA-256 digests, exactly one image for each platform and
OCI version/source/revision labels. These pulls establish public availability;
visibility was verified by anonymous access rather than a package-settings API
query.

Published config digests match those computed directly from the exact image
archives tested on the release runners:

| Platform | Manifest digest | Config digest |
| --- | --- | --- |
| linux/amd64 | `sha256:06f8400b16d1a5815039deb99328bd0ad0b675a935ba47cc5f399fc834f88d42` | `sha256:af875efbab5fd352db55b413138bb262e1e3d07383b4ec27f1174ab796b2db5b` |
| linux/arm64 | `sha256:357a36b510d2dfea96642436de90d6dc02fd89c885004e76db4f52615ad32bd0` | `sha256:35acd1d091daf9b4fa29b4d6c1d4f90a125afc31291f1c63b52432343809ee90` |

The public amd64 index digest passed `make -C renderers/countdown container-check
IMAGE=<immutable-reference>` on the local Linux amd64 host after publication,
including shipped-CLI execution, default/custom paths, palettes, rotations,
SIGTERM and previous-output preservation. Postpublication native arm64 execution
was not performed; anonymous arm64 pulls do not substitute for it.

The Screen example now pins the verified public index. No new webcappi browser,
Kubernetes/PVC/controller or physical-display acceptance was performed during
publication. The earlier local Chromium adapter evidence remains separate from
real webcappi integration, and no deployment has been made.
