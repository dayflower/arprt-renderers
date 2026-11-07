# Validation record

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
