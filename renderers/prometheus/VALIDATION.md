# Validation record

## 2026-11-07: Prometheus 0.1.0 publication

[Prometheus 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/prometheus/v0.1.0)
was published from annotated tag `prometheus/v0.1.0` at source revision
`37ef17c0988c33b936a6c03cdd6943d8ea1446c2`.
The exact source passed main CI, local `make check`, and native amd64 image and
container checks before tagging. The
[release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/42372087408)
passed scoped source checks and native Linux amd64/arm64 builds, offline tests
and shipped-CLI execution under Job security restrictions. Publication loaded
those exact tested image archives without rebuilding.

The public image is `ghcr.io/dayflower/arprt-renderer-prometheus:0.1.0`.
Its immutable multi-platform reference is:

```text
ghcr.io/dayflower/arprt-renderer-prometheus@sha256:3f51122caa1ca4bbe3953c063932e2792676db49f564f830d054d53b9e356d07
```

The release asset `prometheus-index.json` records these platform manifests:

| Platform | Manifest digest |
| --- | --- |
| linux/amd64 | `sha256:83ff1e30dc5a111c148634486d978db946e5a7385a894f65b0b2553371bb5a9d` |
| linux/arm64 | `sha256:4d92d13513894b9483b68add93952d24da6338e327569a25bdfe498f02438bcd` |

With a fresh empty Docker authentication configuration, anonymous version-tag
and index-digest pulls succeeded for both platforms. OCI platform/version/source/
revision labels matched the expected architecture, `0.1.0`, this repository and
the exact source revision above. The public amd64 index reference then passed
`make -C renderers/prometheus container-check IMAGE=<index-reference>`, including
production CLI paths, palettes, all four rotations and failure preservation under
Job restrictions. The Screen example pins this verified public index.

Native arm64 execution passed before publication on the release runner;
postpublication native arm64 execution was not performed. Anonymous arm64 pulls
establish availability. Earlier synthetic browser evidence below is separate
and was not repeated during publication. No live Prometheus numeric comparison,
Kubernetes Job/mount/PVC/configuration-update acceptance or physical-display
check was performed. The partial-current-bucket display treatment remains an
operator acceptance item. Publication does not deploy resources or modify devices.

## Container distribution and handoff, 2026-11-07

Work ran on native Linux amd64 (`x86_64`) with Go 1.27.3, on uncommitted changes
based on source revision `5b7c17c778fe164de5ca26f7db960e943d6e38a6`.
The local image is `arprt-renderer-prometheus:dev`, built with VERSION `0.1.0`
and REVISION equal to that base revision. The REVISION label identifies the base,
not an exact committed representation of these working changes. Local image ID
reported by `docker image inspect` (also the local manifest digest):
`sha256:2f52aa64b3a78b2e4d81b3408b9ad15f2a2d35255097ca668557688b8d4ade5c`.
Build-reported image config digest:
`sha256:d9862d265a6531e363b19e130ab331af936f943e223ee2278b77d8c4fc49aa5a`.
These are local build identifiers, not public GHCR references.

### Source and container evidence

`make check`, `make -C renderers/prometheus image` (with the VERSION and
REVISION arguments above) and `make -C renderers/prometheus container-check`
passed. Shared checks cover every
renderer and release-coordinator tests. Prometheus release tests cover canonical
renderer tags, VERSION matching, annotated tags, exact checkout/main ancestry,
used version/platform tags, deleted-image versions with existing Releases,
fail-closed API errors, paginated lookups and release index evidence. Coordinator
tests cover Prometheus registration, required source/native CI jobs and renderer
specific index assets. All GitHub responses in these tests are synthetic.

The scratch image contains the static CLI, CA certificates and repository/Go
licenses and notices. Schemas, readiness assets and timezone data are embedded.
The container checker inspects exported runtime file membership, builds a separate
static test executable via `test-export`, mounts it and fixtures read-only, and
runs against the exact production image. No fixtures, snapshots, credentials,
test binary, compiler or source tree ship in the runtime.

Native amd64 offline tests and the production CLI passed with network none,
loopback stubs, UID/GID 1000, read-only root, dropped capabilities,
no-new-privileges and only the output parent `/arprt` writable. TMPDIR is `/arprt`;
there is no writable `/tmp` or runtime installation. The CLI tests cover gray4
and spectra6, all four rotations with asymmetric pixels, protocol default paths,
independent custom paths and credentials, snapshots, cancellation, failure
preservation and temporary-file cleanup. The default output is owned by UID 1000.
Optional browser and independent-schema checks are excluded from this offline run.

### Distribution and acceptance limits

Native arm64 CI is configured but was not executed during this local work. No
cross-build or image pull is represented as native execution. No Actions run,
image publication, tag push, public availability or immutable public digest was
created or verified. VERSION 0.1.0 remains unreleased. The release workflow is
prepared to transfer tested native archives without rebuilding; its definition
alone is not successful release evidence.

Real-browser evidence from the earlier display work below is separate and was
not rerun for this packaging change. No live Prometheus numeric comparison,
Kubernetes Job/mount/PVC/configuration-update acceptance or physical-display
check was performed. The synthetic Screen and separate Secret mounts were
reviewed against the local ARPRT API types; no cluster resources were applied.
The README hands off explicit checks for those scopes, including forced failure
preservation, egress, UID-readable files and hashed ConfigMap updates. The final
partial-current-bucket treatment remains an operator decision on real displays.

## E-paper display and Job CLI, 2026-11-07

Source checks and execution ran on Linux amd64 (`x86_64`), Go 1.27.3, on working
changes based on revision `b07beab8ce6fb3115b428f93b276e7eb7079a7c2`.
The standard-library module now includes the static `cmd/renderer` CLI,
renderer-generated HTML/SVG, embedded readiness CSS/JavaScript, an independent
capture client, PNG validation, clockwise rotation and atomic output replacement.
No production Prometheus image, release version or publication was created.

### Offline and executable evidence

`make check`, Prometheus race tests and `make -C renderers/prometheus build`
passed. Tests build and invoke the real CLI against synthetic Prometheus/capture
loopback servers for gray4/spectra6 and all four rotations. They inspect POST
forms, HTML capture MIME/readiness/viewport/base-prefix parameters and distinct
Prometheus/capture credentials. HTTP/capture failures, redirect rejection, bad
MIME, invalid PNG, wrong dimensions, acquisition failure, cancellation before and
during capture, and output replacement failure preserve prior bytes. Atomic-write
tests cover cancellation and rename/unwritable-parent failures with temporary-file
cleanup. Asymmetric RGBA PNGs verify each rotated corner and white compositing,
not just the final 800×480 dimensions. Static-PNG tests cover signatures, chunk
CRCs, truncation, trailing content, APNG rejection and full decoding.

Semantic SVG tests cover isolated points, split missing intervals, finite empty/
constant/mixed/negative/extreme domains, 1/2/5 ticks, escaped labels, prefixes/
suffixes/decimals, numeric grouping, omitted unknown stack contributions,
positive/negative baselines, partial outlines, palette styles and HTML limits.
Synthetic exports reparse the configuration and reload exact plan-bound snapshots;
the final CLI also rendered an exported history snapshot through real webcappi.
No live response recording or API fallback was added.

A disposable restricted CLI harness additionally ran `TestRuntimeCLI` with
`PROMETHEUS_CLI` and `PROMETHEUS_DEFAULT_PATHS=1`, verifying `/arprt/context.json`,
`/arprt/output.png` and independent custom paths. It mounted the static CLI and a
separate static test executable read-only into an existing scratch Weather image
(`sha256:3cc7564307ec684f6478955e8a344013888ee08495d4739464881020354952e8`).
Execution used network none, UID/GID 1000, read-only root, all capabilities dropped,
no-new-privileges and only `/arprt` writable, with read-only Context mounts and
`TMPDIR=/arprt`. No writable `/tmp`, compiler or runtime installation was used.
This verifies the CLI under those restrictions, **not** a Prometheus production
image or its packaging. The required Prometheus `container-check` was attempted
and explicitly failed because no production image exists; that remains
separate distribution work.

### Real browser and visual evidence

A disposable local webcappi service used Chromium 156.0.8078.47 and image
`sha256:69254f3a042acbbe17835d8eaa886193405743ee894b595bda56403ed2d05654`
on amd64, with a synthetic independent Bearer token. No external source was
queried. Completed HTML capture tests passed both palettes and logical 800×480 /
480×800 viewports. The original 16-case, 64-capture matrix covered normal and
generated/explicit portrait placement, auto/px tracks, grow ratios, wrapping,
eight line series, line/bar stacks, all missing, Japanese, long text, hour buckets
and the three energy examples. Strengthened wrapping/Japanese/text cases passed
again with actual multi-child wrapping, Japanese legends and multiple text lines.
An additional eight-bar-series case and the affected stacked/daily bar cases
passed both palettes/orientations after fill/outline/marker improvements.

Readiness waits for fonts and two animation frames, positions fixed-pixel SVG
labels, resolves direct-label collisions, intentionally ellipsizes labels and
omits colliding X tick labels, then checks panel/content/label rectangles before
exposing `#prometheus-ready`. The optional tests additionally compare the Go
sampling estimate against the **completed** plot width before allowing capture.
The 96 px reserve is preserved. Geometry success does not establish physical
panel readability or acceptance of every possible configuration.

Saved PNGs were visually inspected for the energy histories, daily buckets,
eight series, negative stacks, wrapping and Japanese/text cases in the tested
palettes/orientations. Inspection found and corrected unsupported SVG text
`calc()` coordinates, misplaced direct labels, excessive marker density and
insufficient Y tick counts and text vertical alignment. The final examples show readable ordinary headings,
40–64 px value numbers, 13 px axes and 14 px chart labels; deliberately long labels
are ellipsized, and series keep numeric identifiers. Screenshots are disposable
local artifacts, not golden tests or shipped assets. Reproduce them with the
README's opt-in capture/export commands.

### Acceptance limits

There was no native arm64 execution, Prometheus production image verification,
image publication, live Prometheus/numeric comparison, Kubernetes Job/mount/PVC
collection or physical gray4/spectra6 display check. No cluster changes were made.
Synthetic API tests do not validate operator PromQL, aggregation/counter resets or
upstream source age. The current partial-bucket `*` label plus outline is
provisional; its final physical-display treatment remains the explicit operator
acceptance decision. Configurations too small for readable charts/text fail
readiness rather than silently clipping. Longer units/legends may be deliberately
ellipsized, so operators should use the separate summary/history/daily examples
and short labels.

## Bounded acquisition and panel data, 2026-11-07

Source-level checks ran on Linux amd64 with Go 1.27.3, on working changes based
on revision `622c58fc50c108aab6df17012bf315f210629315`.
The independent standard-library module now includes a Prometheus-only client,
strict float response parsing and normalized panel models. There is still no
rendering CLI, capture integration, production image or published version.

Offline loopback tests inspect POST forms, instant/range parameters, shared
evaluation times, bounded server timeouts, preserved URL prefixes, bearer/basic
authorization frozen before requests, rejected redirects and redacted failures.
They cover a maximum of four workers, queued-task cancellation, fatal-response
cancellation, caller deadlines, cancellation during a one-second retry backoff,
dial-versus-read/TLS retry classification, HTTP 503 versus other statuses,
malformed/unsupported compression, decompressed per-response/cumulative limits,
retry-body accounting, exact-limit probes and the attempt ceiling.
Synthetic credential/configuration files exercise strict parsing and size limits.

Parser/model tests cover scalar/vector/matrix contracts, empty results,
pre-filter multiple-value rejection, NaN/infinities, timestamp bounds and exact
grid membership, duplicate/unordered samples, malformed labels, string/native
histogram rejection and annotation counts. They exercise freshness age boundaries,
missing/nonfinite/future freshness, signed values, rounded negative zero, stable
full-label/query identities and legends, label changes across buckets, panel-wide
series limits, missing line steps, partial buckets, reference omission,
independent positive/negative stacks, unknown contributions, stack overflow and
all-missing/text-only screens. The same synthetic values and gapped histories
produce equal models through HTTP and a validated snapshot. Snapshot mode also
succeeds with unavailable live credentials; malformed snapshots return no model.

`make check` and Prometheus `go test -race -mod=readonly ./...` passed.
The required `make -C renderers/prometheus container-check` was attempted and
failed with its explicit unavailable message: no production runtime/image exists.
This is not container verification. No native arm64 execution, live Prometheus,
capture/browser layout, Kubernetes Job/PVC collection, atomic image output or
physical gray4/spectra6 display acceptance is claimed. All acquisition tests use
synthetic loopback responses or local snapshots; they do not validate operator
PromQL, Prometheus aggregation/counter-reset behavior or upstream sample age.
No cluster or deployment changes were made. The future runtime must gate capture
and output replacement on successful `LoadModel`; the current library performs
no output writes.

## Configuration and query planning, 2026-11-07

This is source-level implementation work on Linux amd64 with Go 1.27.3.
There is no renderer executable, production image or published version yet.

Offline tests cover schema/defaults, type-specific fields, Unicode limits,
bounded JSON, unsafe track syntax, rectangle membership, group/ID constraints,
portrait order, variable scope/token boundaries, escaped text and restricted
expressions, URL overrides/prefixes, Context/rotation and one-shot clock selection.
Fixed-time tests cover Asia/Tokyo, New York's 23/25-hour dates, skipped/repeated
hours, year rollover, midnight zero-duration rejection, partial bucket endpoints,
step/rate interval limits, stable plans, auxiliary/retry request reservations,
snapshot mismatches, path confinement and per-response/cumulative byte ceilings.

An independent Python jsonschema 4.10.3 draft-2020-12 validator was used to compare
the embedded structural validator against the accepted/rejected configuration
corpus. Cross-field semantics remain explicitly outside JSON Schema.

A real, local Chromium 156.0.8078.47 geometry experiment ran with network none
using browser image ID
`sha256:312db162e2a080f26f72e52d6ac7f5b46135883ccd32b1ee540beeca9f33660a`.
Seven cases passed: landscape, generated portrait, explicit portrait, auto
columns, wrapping group, grow ratios and px columns. Fixed/fr sampling widths
matched measured browser plot widths after rounding down (e.g. 432 px landscape,
384 px generated portrait, 140 px explicit portrait). Auto/wrap conservative
1 px sampling widths did not exceed measured widths. This deliberately sacrifices
history resolution in those content-dependent layouts. The experiment's minimal
Grid/flex HTML is not the final renderer, and is not a screenshot or readability
acceptance check. Browser temporary files are unrelated to the renderer's future
output-only runtime contract.

Prometheus timestamp semantics were reviewed against the upstream v2.42.0 engine
source, not a live API. Freshness is restricted to bare selectors because
aggregations/arithmetic do not preserve the original source sample timestamp.

`make check` passed, including all existing renderers and the new offline tests.
The optional independent schema comparison and seven browser geometry cases
passed as described above. Production `make -C renderers/prometheus container-check`
was attempted and failed with an explicit unavailable message because there is
no executable/image yet; this is not a passing container verification.
No native arm64 execution,
webcappi capture, actual API response normalization, live Prometheus, Kubernetes
Job/PVC collection, atomic output replacement, or physical gray4/spectra6 display
acceptance has been performed for this module. No cluster or deployment changes
were made.

## 2026-11-08 safe-area source and browser checks

Repository-wide `make check` and this renderer's `container-check` passed.
The latter exercised the final Linux amd64 production image with network none,
UID/GID 1000, read-only root, dropped capabilities, and only output storage writable.

`TestBrowserSafeArea` passed 8 synthetic captures through a local
`webcappi:font-cdp-fix` image on Linux amd64. Cases use both gray4/spectra6
palettes, all four rotations, and logical insets top 4, bottom 8, left 12,
right 16. The checks require successful renderer readiness, visible
content, white pixels outside the safe area, and unchanged 800 × 480 physical
PNG dimensions with the expected clockwise rotation.

Reduced plot widths and resulting query steps are covered by offline tests.
Zero insets preserve the serialized Context identity used by snapshots.
A separate browser probe confirmed that a 1 × 1 content rectangle reports
layout overflow instead of successful readiness.

The local browser used `sandbox: false` because this host does not provide
Chromium's required user namespaces. This setting was confined to the temporary
capture service; renderer Job restrictions were unchanged. These synthetic
checks do not establish native arm64 execution, live-source compatibility,
Kubernetes integration with ARPRT v0.8.0, overlay avoidance, or physical-display
readability. No renderer release or cluster update was performed.

## 2026-11-08 partial-bucket outline removal

Removed the full-height outline around an incomplete bar bucket. The date/hour
label's `*` and the `* partial` key remain; bucket selection, query intervals and
bar geometry are unchanged. Earlier outline observations above describe the
previous appearance. Existing display tests now require the partial label and
key while rejecting the outline.

Repository-wide `make check` and Prometheus `container-check` passed against the
updated Linux amd64 production image under the existing Job restrictions.
No new arm64, Kubernetes or physical-display acceptance was performed.

## 2026-11-08 Prometheus 0.2.0 publication

- Source revision: `d7c29272551b337e0aacd56daac9e5e9479810c6`.
- Release: [Prometheus 0.2.0](https://github.com/dayflower/arprt-renderers/releases/tag/prometheus/v0.2.0).
- Native release verification and publication: [successful workflow](https://github.com/dayflower/arprt-renderers/actions/runs/42460462495).
- Immutable multi-platform reference:
  `ghcr.io/dayflower/arprt-renderer-prometheus@sha256:66dc7b6f899561ca175ed3bd577d90ebc78e92950318d628d43ecb3ddf97d2b7`.

The release verified native Linux amd64 and arm64 builds, offline tests and
production CLI execution under Job security restrictions, then published the
exact tested image archives without rebuilding. The release index asset contains
exactly linux/amd64 and linux/arm64 and matches the immutable digest above.

Anonymous version-tag and index-digest pulls passed on both platforms using an
empty Docker configuration. Explicit platform inspection verified OS/architecture
and OCI version `0.2.0` and revision `d7c29272551b337e0aacd56daac9e5e9479810c6` labels.
The public amd64 index reference passed `container-check` with network none,
UID/GID 1000, read-only root, dropped capabilities and only output storage writable.

Postpublication native arm64 execution was not performed. These publication
checks add no live-source, real-browser, Kubernetes or physical-display acceptance.
Earlier browser evidence has its own scope above. Updating the example digest
pins does not apply resources or change existing Device assignments.
