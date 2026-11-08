# ARPRT Prometheus renderer

An independent Go module for a Prometheus Job renderer provided by ARPRT, an
unofficial Kubernetes controller for TRMNL BYOS. ARPRT has no affiliation with
or endorsement by TRMNL.

Configuration, bounded acquisition, snapshot loading, normalized panel models,
e-paper HTML/SVG rendering and a Job CLI are implemented. The CLI captures through
webcappi, validates a static PNG, rotates once and atomically replaces output.
A static scratch image, offline container checks and independent native
amd64/arm64 verification/release workflows are available.
[Prometheus 0.2.0](https://github.com/dayflower/arprt-renderers/releases/tag/prometheus/v0.2.0)
is published as `ghcr.io/dayflower/arprt-renderer-prometheus:0.2.0`. The verified public multi-platform reference is:

```text
ghcr.io/dayflower/arprt-renderer-prometheus@sha256:66dc7b6f899561ca175ed3bd577d90ebc78e92950318d628d43ecb3ddf97d2b7
```

Native amd64/arm64 release checks and anonymous pulls passed. The public amd64
image also passed postpublication container checks. See [VALIDATION.md](VALIDATION.md)
for the exact source revision and separate acceptance limits.

The module pins Go 1.27.3 and uses only the standard library, including embedded
IANA tzdata. It has no shared renderer SDK or runtime dependency installation.

## Safe area

This renderer supports the optional `renderTarget.safeAreaInsets` extension from
[ARPRT v0.8.0](https://github.com/dayflower/arprt/blob/v0.8.0/notes/PROTOCOL.md#safe-area)
while retaining Render Context v1. Omitted objects/edges mean zero. Insets must
be integers from 0 through 4095 and leave positive logical width and height;
invalid values fail before acquisition/capture. Existing unknown Context fields
remain ignored.

Insets follow the upright logical viewing orientation, including 90/270-degree
targets; they are not rotated a second time. Capture still uses the full logical
viewport and the PNG remains physical 800 × 480. Existing design padding stays
inside the safe area, and omitted/zero insets preserve the previous layout.
Safe area does not reserve space for controller overlays.

The dashboard grid uses the inset rectangle. Plot-width estimates and query
sampling use its reduced width too. Fixed-pixel grids that no longer fit fail
the existing browser geometry checks; automatic font/image scaling is not added.

The opt-in `TestBrowserSafeArea` test captures synthetic content with asymmetric
insets (`top: 4`, `bottom: 8`, `left: 12`, `right: 16`) at all four rotations and
both palettes, checks white pixels outside the content rectangle, and verifies
physical output dimensions/rotation. Run it from this renderer's directory:

```sh
SAFE_AREA_WEBCAPPI=http://127.0.0.1:8080 \
SAFE_AREA_LAYOUT_DIR=/tmp/safe-area-layout \
go test -run '^TestBrowserSafeArea$' -v .
```

Ordinary source/container checks remain offline; the browser test requires a
separately operated webcappi and is skipped by default. Source/browser checks
are not Kubernetes or physical-display acceptance.

## Configuration

See [the synthetic example](fixtures/config.json) and
[config.schema.json](config.schema.json), JSON Schema draft 2020-12. `ParseConfig`
validates that embedded schema, applies its defaults once, then validates
cross-field constraints: globally unique IDs, rectangles, exact area membership,
timezone, bounded durations, variable scope, templates and ordered axis bounds.
Schema validation by an editor is necessary but does not perform these semantic
checks. Duplicate keys, nulls, trailing documents, invalid UTF-8 and JSON nesting
over 64 levels are rejected. No file or network access occurs during parsing.

Root keys are `version` (required, exactly 1), `timezone` (default `Asia/Tokyo`),
`prometheus`, `layout`, `panels` (all required), and `allMissing` (`fail` by default,
or `display`). `Local` and unknown timezones fail. Unknown fields and fields
belonging to another panel type fail, even when supplied as false or zero.

`prometheus` accepts `url`, optional `auth`, and `scrapeInterval` (default `60s`).
The final URL is required: an explicitly supplied `--prometheus-url` overrides
`prometheus.url`, including when the configured URL is invalid. Explicitly empty
overrides fail. Only absolute HTTP(S) URLs without userinfo, query or fragment
are allowed; dot segments, repeated/escaped separators and control characters
in paths are rejected. `APIURL` preserves the base path
when appending `/api/v1/query` or `/api/v1/query_range`. All redirects are
rejected. URLs and queries must never appear in routine logs or errors.

`auth` requires `type` (`bearer` or `basic`) and an absolute `credentialsFile`
path, up to 512 characters. The credential file contract matches iCalendar:
Bearer is a token using `[A-Za-z0-9._~+/-]+={0,}` with optional surrounding
whitespace; Basic is a strict JSON object with nonempty UTF-8 `username` and
`password`, preserving whitespace and rejecting control characters and colons
in the username. Files are limited to 8 KiB and read once by `NewClient`, before
HTTP. That client is dedicated to Prometheus; capture integration must use its
own client and credentials. Credentials are not read by the planning library and
are never included in a snapshot.

All durations use positive whole `s`, `m` or `h` quantities, without combinations
or leading zeroes; maximum 366 days (8784h). Unicode string limits count code
points, not bytes. Titles/sign labels/reference labels are at most 64 characters,
prefixes/suffixes 32, text 512, legends 128 and input queries 8192. Numeric axis
bounds must be finite and between -1e100 and 1e100; when both are set, min < max.
`decimals` is 0–6, default 0. Prefix/suffix strings never trigger unit conversion.

## Panels and placement

Every panel has an `id` matching `^[a-z][a-z0-9-]{0,62}$` and a `type`.
IDs are globally unique, including group children. A child alone may specify
`grow` (integer 1–100, default 1). Optional titles belong to non-text panels.

| Type | Fields beyond ID/title |
| --- | --- |
| `value` | Required `query`; prefix/suffix/decimals; optional `signLabels` and `staleAfter` |
| `line` | Required `range`, `series` (1–8 queries); `minStep` (default `1s`); prefix/suffix/decimals, `yAxis`, `referenceLines`, `stack` |
| `bar` | Required `buckets`, `series` (1–4 queries); prefix/suffix/decimals, `yAxis`, `stack` |
| `text` | Required `text`; `format`, `size`, `weight`, `align`, `verticalAlign` |
| `group` | Required `panels` (1–12 non-group children); `direction`, `wrap`, `gap` |

`series` entries require `query` and accept `legend` and `style`. Styles are
`primary` (default), `secondary`, `tertiary`, `positive`, `negative`, `muted`.
Legend templates permit only `{{label_name}}` references, without whitespace or
expressions. The normalization contract uses complete label sets as identities
and sorts them lexicographically by label name/value, with configured query index
as a tie breaker. Missing legend labels become `?`; duplicate display names gain
stable numeric suffixes. Without a template, differing label pairs form the name;
otherwise the query index supplies a fallback. Returned series across all queries
and buckets of a panel are limited to 8; configured query count is a separate limit.

`referenceLines` permits at most 8 entries, each with **exactly one** of `value`
(a finite constant) or `query` (an instant query), plus optional `label` and
`style`. Reference queries use the value-variable scope and evaluate at `now`.
`stack` is boolean, default false, on line/bar only. Positive and negative stacks
have separate zero baselines; missing contributions remain unknown rather than
observed zero. A missing reference is omitted and does not count as panel data.

`signLabels` requires `positive` and `negative`, with optional `abs` (false by
default). Zero uses the positive label. Negative zero is normalized before
formatting. `buckets` requires `unit` (`day` or `hour`) and `count` (1–31).
`includeCurrent` defaults to true and selects the current partial bucket plus
the previous count−1; false selects count completed buckets.

`layout.landscape` is required; `layout.portrait` is optional. A Grid has required
`areas` (1–32 equal-length rows, 1–16 columns). Cells are top-level IDs or `.`.
Each top-level panel must appear, and each named region must form one rectangle.
`columns`/`rows` are whitespace-separated tracks of exactly the right count:
positive decimal `fr`, positive integer `px`, or `auto`; numeric amounts are at
most 800. No CSS functions or other CSS syntax are accepted. Omitted tracks are
all `1fr`. `gap` is an integer 0–32 CSS px, default 8.

Omitted portrait places panels in first-appearance order in landscape, scanning
left to right and then top to bottom, in one column; rows are `auto` except the
final `1fr`. It inherits landscape gap. A group defaults to `direction: row`,
`wrap: false`; `column` is also supported. Its omitted gap inherits the active
orientation's Grid gap. Wrapping uses a 120 px basis. Rendering prefixes CSS
area identifiers with `p-` so IDs such as `span` remain safe.

Text is always plain, HTML-escaped text. Only exact expressions of the form
`{{now "2006-01-02 15:04 MST"}}` are supported; the quoted Go time layout is 1–128
characters without quotes or newlines. No general template execution occurs.
Unknown, unbalanced or extra template delimiters fail. `format` is only `plain`;
`size` is `small`, `medium` (default), `large`, `xlarge`; `weight` defaults to
`normal` (or `bold`); horizontal alignment defaults to `start`, vertical to
`center` (both also accept `start`, `center`, `end`).

## Clock and query plan

`ParseOptions` accepts required `--config`, optional `--context` (default
`/arprt/context.json`), `--output` (default `/arprt/output.png`), `--input <dir>`,
`--prometheus-url`, and `--now <RFC3339>`, required `--webcappi-url`, and optional
`--webcappi-token-file` (a separate bounded Bearer credential file). An explicit now must use whole seconds.
Without it, the clock is sampled exactly once and truncated to seconds.
`BuildPlan` takes that instant explicitly and never reads a clock.

Times are nonnegative Unix seconds through year 9999. One localized `now` is
shared by every panel, timestamp and text expression. Day boundaries use local
calendar dates, including 23/25-hour DST days. Zones/dates with a nonexistent
midnight or skipped calendar date fail explicitly; repeated midnight uses its
earliest occurrence. Hour buckets are consecutive
elapsed hours anchored at the current local day's midnight; spring's missing
hour is skipped, fall's repeated hours remain separate and labels include the
numeric UTC offset. At half-hour DST transitions, hour labels may end in `:30`.
Completed buckets use their calendar end; current buckets use `min(end, now)`.
The plan preserves both that evaluation end and the uncut boundary and partial flag.

| Variable | Scope and replacement |
| --- | --- |
| `$__now` | All queries; fixed execution Unix second |
| `$__today_start` | All queries; local midnight Unix second |
| `$__today_range` | All queries; elapsed seconds from midnight, with `s` suffix |
| `$__range` | Line series only; configured period, as seconds with `s` suffix |
| `$__step` | Line series only; planned step, as seconds with `s` suffix |
| `$__rate_interval` | Line series only; max(4 × scrapeInterval, step + scrapeInterval) |
| `$__bucket_start` / `$__bucket_end` | Bar series only; actual start/clipped evaluation Unix seconds |
| `$__bucket_range` | Bar series only; actual elapsed evaluation duration with `s` suffix |

Tokens use ASCII letters, digits and underscores following `$`. Full tokens are
matched: `$__now_extra` cannot partially expand as `$__now`. Unknown, malformed
and out-of-scope variables fail. Other PromQL syntax is the operator's
responsibility and will be checked by Prometheus. Using a zero-length duration
variable at exact midnight (or an exact hour boundary) fails planning; the
renderer never changes a calendar boundary to make a selector legal. Use completed
buckets or queries based on timestamp subtraction when this case must succeed.

Value and freshness requests are instant evaluations at `now`; line uses
start=now−range, end=now; each queried reference is instant at `now`; bar uses one
instant request per configured query per bucket. Order is panel order, then
query order (bucket-major for bar). Request IDs are
`<panel>/<role>/<query-index>/<bucket-index>`; bucket index is −1 outside bar.
Roles are `value`, `freshness`, `range`, `reference`, `bucket`. They identify query
slots, not yet-normalized returned label sets. Constant references and text
produce no requests. Every auxiliary request counts against the same budget.

For fixed/fr columns, `PlotWidths` distributes the logical viewport minus gaps,
then applies group grow ratios, rounds down and reserves 96 px for axes/direct
labels. Browser CSS uses border-box, zero external padding, `min-width:0` on
Grid/flex children, and a plot with `min-width:1px`; additional decoration must fit
inside the reserved area. Auto columns and wrapping groups use a conservative
1 px sampling width because their content-dependent dimensions cannot be
predicted before data retrieval. This gives only the two endpoint evaluations;
use fixed/fr columns and non-wrapping groups for detailed histories. No browser
measurement or extra capture request is needed before API execution.

The number of intervals is max(1, min(499, plotWidth−1)). Step is the larger of
`minStep` and ceil(range/intervals), in whole seconds. Thus both endpoints fit
within 500 evaluations; Prometheus's final returned point can precede `end` if
step does not divide range. The normalization contract breaks a line whenever
adjacent finite samples are not exactly one planned step apart, or a nonfinite
sample occurs. It never bridges missing steps or interpolates gaps.

## Freshness and missing data contract

`staleAfter` defaults to disabled. When enabled it is accepted only for a bare
named instant vector selector, optionally followed by a nonempty brace-enclosed
list of double-quoted label matchers. Aggregation, arithmetic, functions, range
selectors, `offset` and `@` are rejected for this option. The extra query is
`timestamp(<selector>)` at the same now. This restriction follows the
[Prometheus 2.42.0 engine's special treatment of timestamp on a selector](https://github.com/prometheus/prometheus/blob/v2.42.0/promql/engine.go#L1216):
aggregations/arithmetic receive evaluation timestamps and cannot establish the
age of the original data. Recording-rule sample age describes that rule's output,
not the age of its upstream inputs. No live server acceptance is claimed.

Normalization accepts 0 or 1 value sample and fails multiple samples before
nonfinite filtering. Empty results and NaN/±Inf are missing. Freshness response
errors or malformed/multiple/future timestamps fail; absent/nonfinite freshness
marks an otherwise present value stale, never fresh. Age strictly greater than
staleAfter marks it stale. Stale finite values still count as available data.
If every data panel is missing, `allMissing: fail` preserves the previous output;
`display` permits empty panels. Text-only screens are valid. Text and reference
lines never make an otherwise missing data screen available.

## Snapshots and limits

`ExpectedManifest(plan)` describes synthetic files using
[snapshot.schema.json](snapshot.schema.json). It never records live responses.
A snapshot directory contains `manifest.json` and files `response-000.json`,
`response-001.json`, etc. The manifest has `version: 1`, `planHash` and `entries`.
Each entry has the exact serialized `request` plus `file`. All request fields,
including unused numeric fields as zero, are required. PlanHash is lowercase
SHA-256 of Go's compact `encoding/json` serialization of the Plan. The plan
includes hashes of the defaulted configuration (including URL override) and
Context, fixed now, geometry, bucket metadata and ordered requests. Keep manifest
and plan together rather than attempting to reconstruct their hashes by hand.

`LoadSnapshot` requires exact plan hash, request order/parameters and file count;
files cannot be reused or escape the directory, including through symlinks.
Response files must be regular files. Extra unreferenced files are ignored.
It returns bounded raw response bytes. `ParseResponses` and `LoadModel` validate
them with the same `ParseResponse` parser used by the HTTP client.
Use `--now` to reproduce the same plan. There is no API fallback on missing
or mismatched snapshots. `--input` replaces only Prometheus access: rendering still requires webcappi.

| Resource | Limit |
| --- | --- |
| Configuration / Context | 256 KiB / 1 MiB |
| Panels | 24 top-level, 48 total including group containers |
| Grid | 32 rows × 16 columns (including generated portrait) |
| Group children / grow / gap | 12 / 1–100 / 0–32 px |
| Line query slots / bar query slots | 8 / 4 |
| Returned series per panel / line evaluations / bar buckets | 8 / 500 / 31 |
| Reference lines | 8 per line panel |
| Duration | 366 days, in positive integer s/m/h |
| Expanded query | 16 KiB |
| Snapshot manifest | 1 MiB |
| Response / cumulative response bytes | 4 MiB / 32 MiB, after decompression |
| Prometheus attempts | 256 including retries and auxiliary requests |
| Planned tasks | 128, reserving a retry for every task |
| Concurrency | 4, including auxiliary queries |
| Credentials file / generated HTML | 8 KiB / 1 MiB |
| Total / shared fetch budget / output reserve | 60 s / 20 s / 5 s |

Snapshot, configuration, response series counts, decompression, transport
concurrency/deadlines and credential limits are enforced. Generated HTML is limited to 1 MiB. The transport uses POST form
requests with a server `timeout` bounded by remaining shared fetch time. Timeout
is transport-dependent and is not part of snapshot identity. Only connection
failure and HTTP 503 retry once, with at most one second backoff within the same
budget. Retry bytes and attempts count cumulatively; cancellation never extends
a deadline. Capture receives at most the remaining total time minus the output
reserve. It cannot renew the total deadline.

## Acquisition and panel models

`LoadConfig` reads and validates bounded configuration. `NewClient` freezes the
Prometheus URL and authorization, then `Client.Fetch` executes a validated Plan.
The client sends only form bodies, never queries in the URL, and includes an
integer-millisecond server timeout rounded down from the shared deadline.
Only a dial failure or a complete HTTP 503 response retries, once after a
cancellable one-second backoff. TLS, response-header/body failures, malformed
gzip/JSON, API errors and other HTTP statuses fail without retry. Identity and
gzip responses are supported; unknown content encodings fail. Every body read,
including partial reads and discarded retry bodies, charges the decompressed
budget. Concurrent reads reserve the remaining allowance; a single total-budget
probe byte detects overflow. A fatal failure cancels pending/active tasks and
returns no partial result map. No response is recorded automatically.

The parser follows the [Prometheus HTTP API float result formats](https://prometheus.io/docs/prometheus/latest/querying/api/):
instant queries accept scalar or vector; range queries require matrix. String
results, native histograms (including mixed float/histogram results), duplicate
label identities, invalid label/value types, duplicate/unsorted samples and
out-of-range/off-grid timestamps fail. Evaluation timestamps must be exact
nonnegative whole Unix seconds through year 9999 and match the planned instant
or range grid. Timestamp numeric tokens are limited to 32 bytes and exponents
to signed 8-bit integers before exact parsing. Float values must be decimal
strings, with optional exponent, or `NaN`, `Inf`, `+Inf`, `-Inf`; numeric overflow
fails. Optional warnings/infos must be string arrays; only counts survive
parsing. Unknown envelope/data metadata is ignored for API extensibility.
Individual result entries must contain exactly metric and float value(s).

`LoadModel(ctx, config, plan, input)` selects live acquisition or the exact
snapshot, then calls `Normalize`. Snapshot mode never reads credentials and
never falls back to HTTP. A canceled context, bad snapshot, acquisition error or
normalization error returns no model. The runtime checks this boundary before capture or output writes; acquisition
and normalization alone never write output.
The caller owns the total execution deadline and later output reserve; the
client separately enforces the shared 20-second fetch ceiling. `FetchStats`
reports live attempts, decompressed bytes, annotation counts and elapsed time.
`Diagnostic` exposes only a validated panel ID and a fixed error classification;
underlying network/filesystem errors and server text are discarded. There is
no implicit logging.

Models contain leaf panels in configuration order; the configuration retains
group/layout information. A series identity is its full label set **and** query
index, so equal labels from different configured queries remain distinct. Sort
order compares sorted label name/value pairs, then query index. Default legends
include differing label pairs; an otherwise empty name becomes `Query N`.
Template misses become `?`; duplicate names receive ` (1)`, ` (2)`, etc., skipping
names already reserved by another series. No series is silently truncated,
including empty/nonfinite series or labels appearing only in another bucket.

Line models include the whole planned evaluation grid, with `Present: false`
at absent/nonfinite points; segments never bridge these gaps. Bar points retain
one slot per bucket and preserve the plan's partial/boundary metadata. Raw
missing points use a zero storage value with `Present: false`, never an observed
zero. Stack bases/tops use independent positive and negative accumulators in
stable series order. A missing earlier contribution marks every later present
contribution at that position `StackKnown: false`, conservatively for both signs;
such points are excluded from renderable segments. Their finite values still
count as available data. Nonfinite stack sums fail. References do not contribute
to stacks or availability. Value models preserve the signed numeric value,
formatted literal prefix/suffix, sign label and stale state. Displayed negative
zero is removed even when a small negative number rounds to zero.

## Compatibility and verification

The reviewed protocol baseline is
[ARPRT v0.2.2, revision d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87](https://github.com/dayflower/arprt/blob/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87/notes/PROTOCOL.md).
`ParseContext` accepts Render Context v1 with device.name and physical 800×480
gray4/spectra6, and clockwise rotation 0/90/180/270 (omitted means 0).
Logical dimensions are 480×800 at 90/270 and 800×480 otherwise.
The capture contract follows
[webcappi revision d86aa33583120ddd90a3ab05b81d5fb5ff27dbde](https://github.com/dayflower/webcappi/blob/d86aa33583120ddd90a3ab05b81d5fb5ff27dbde/notes/API.md):
raw UTF-8 HTML POST to `/capture`, `html_mode=file`, exact logical viewport,
`ready_selector=#prometheus-ready`, no profile override, resizing or full-page
mode. The renderer waits for fonts and two frames, checks geometry before readiness,
validates the returned PNG, rotates once and atomically replaces output. ARPRT owns final
palette conversion, dithering and overlays. All runtime writes remain in the output parent; no writable `/tmp` is required.

```sh
make -C renderers/prometheus setup
make -C renderers/prometheus check
make check
```

Optional independent schema and browser geometry checks:

```sh
cd renderers/prometheus
PROMETHEUS_SCHEMA_PYTHON=python3 go test -run 'Test(ConfigDefaultsAndPortrait|RejectConfig|InputBoundaries)' -v
PROMETHEUS_BROWSER_IMAGE=your-local-chromium-image go test -run '^TestBrowserPlotWidths$' -v -timeout 240s
```

The Python check requires an independently installed `jsonschema` with draft
2020-12 support; it is not a runtime or ordinary-test dependency. The browser
image must provide `/opt/chrome/chrome`; the experiment is network-disabled and
uses disposable browser-only `/tmp` and `/work` tmpfs, without exposing credentials.
It compares geometry of a minimal Grid/flex prototype, not finished renderer HTML
or image readability. See [VALIDATION.md](VALIDATION.md) for actual evidence.
Production `image` and `container-check` commands are described below. Real capture
and completed-HTML geometry have separate evidence in VALIDATION.md. Native
arm64 release execution passed; live Prometheus, Kubernetes and physical-display
acceptance remain unverified.

## CLI and display

```sh
make -C renderers/prometheus build
renderers/prometheus/dist/renderer --config renderers/prometheus/examples/history.json \
  --context renderers/prometheus/fixtures/render-context-v1.json \
  --webcappi-url http://webcappi.example:8080 --output /absolute/output/dashboard.png
```

The output parent must already exist and be writable. Context, configuration,
snapshots and credentials need only read access. Capture and Prometheus use
independent HTTP transports and credentials; neither follows redirects. Capture
performs one attempt, with a deadline equal to the original execution deadline
minus the five-second output reserve. It cannot extend the 60-second total budget.
Failures report fixed classifications without lower-level URLs, PromQL or bodies.

Text sizes are 16/20/28/36 CSS px; headings are 18 px, value numbers 40–64 px,
axis labels 13 px and other chart labels 14 px. Display numbers group thousands
without unit conversion. Text lines, titles, numbers and bounded chart labels
use explicit ellipsis. Values never shrink below 40 px. Layouts that cannot fit
their panels, text lines or readable charts do not become ready and fail capture.
Each chart needs at least 160 px width and 180 px total height. Configuration
validity alone does not imply that an arbitrary Grid fits the viewport.

Charts use renderer-generated SVG with a nested normalized data plot and fixed
pixel labels, keeping the planned 96 px horizontal reserve. The browser only
places/truncates labels after measuring fonts; it never builds queries or alters
step. Y ticks use a 1/2/5 progression with 3–5 ticks for ordinary finite ranges;
constant and empty domains expand safely. Explicit axis bounds clip data. Bar
charts include zero unless explicitly bounded. Positive and negative stacks keep
separate bases, and unknown contributions stay absent. Missing buckets show a
dash; missing line steps split paths; isolated points get markers.

Direct series labels occupy the right margin, use stable numeric prefixes and
are shifted vertically to avoid collisions. Long names/axis units may be
ellipsized; use short legends and units for a small display. X ticks use local
hour/day boundaries and show the timezone/offset; colliding tick labels are
omitted without dropping data. Hour-bucket labels retain UTC offsets. A partial
bucket has an explicit `*` label and a `* partial` key, without a surrounding
outline. Its final physical-display treatment is still an operator acceptance item.

| Logical style | gray4 | spectra6 |
| --- | --- | --- |
| primary | Black, 3 px solid | Blue, 3 px |
| secondary | Black, 3 px dashed | Red, 3 px |
| tertiary | Dark gray, 2 px with markers | Green, 3 px |
| positive / negative | Black fill / white fill with black outline | Green / red |
| muted | Light gray | Yellow |

Returned series additionally receive eight deterministic dash/marker combinations,
including when they share a logical style. Gray4 bars also vary opaque fills,
outlines and interior markers. Fills are opaque, with no gradients,
external fonts, assets or chart libraries. ARPRT owns final dithering/overlays.
Stale values retain their number in light gray with an explicit `! Stale` mark.

## Synthetic energy examples and browser checks

The tracked examples split [summary](examples/summary.json),
[history](examples/history.json) and [daily](examples/daily.json) for readability.
They use synthetic metric names and public example hosts, without credentials.
Export deterministic Context/configuration/snapshot/HTML sets for both palettes
and orientations (fixed now `2026-11-07T12:00:00+09:00`):

```sh
cd renderers/prometheus
PROMETHEUS_EXAMPLE_OUTPUT=/absolute/disposable/examples go test -run '^TestExportSyntheticExamples$' -v
```

The exporter only creates synthetic responses; it cannot record live responses.
Histories contain a 30-minute gap, remaining energy and reference limits. Daily
buckets include missing data, negative export values and the partial current day.
To reproduce an exported set, pass its `config.json`, `context.json` and directory
as `--config`, `--context` and `--input`, plus the fixed `--now` above and a
webcappi URL. Snapshot mode still uses capture, and never falls back to the API.

Completed-HTML browser validation is opt-in and excluded from ordinary tests:

```sh
PROMETHEUS_WEBCAPPI=http://127.0.0.1:8080 \
PROMETHEUS_WEBCAPPI_TOKEN_FILE=/absolute/synthetic-token \
PROMETHEUS_LAYOUT_DIR=/absolute/disposable/screenshots \
go test -run '^TestBrowserDashboard$' -v -timeout 300s
```

Use a separately provisioned local webcappi with Chromium and CJK fonts; omit the
token-file variable when capture requires no authentication. This runs 17 cases
in both palettes and orientations, enforcing production readiness and comparing
planned/actual plot widths, and optionally saves PNG/HTML for visual inspection.
Browser geometry, screenshot inspection, stub contracts and production-container
verification are distinct evidence; see VALIDATION.md.

## Container and distribution

```sh
make check
make -C renderers/prometheus image
make -C renderers/prometheus container-check
```

The default image `arprt-renderer-prometheus:dev` is local only. `IMAGE`, `VERSION`
and `REVISION` override its name and OCI labels. The Dockerfile uses a pinned Go
builder and copies only the static CLI, CA certificates and licenses into scratch.
Timezone data, the two schemas, CSS and readiness JavaScript are embedded in the
binary. The Dockerfile-specific context allowlist excludes fixtures, examples,
snapshots, credential files and local outputs. Test sources enter only build/test
stages; neither tests nor a compiler enter the runtime. See
[third-party notices](THIRD_PARTY_NOTICES.md).

The checker builds the separate `test-export` stage, mounts that executable and
synthetic fixtures read-only, and invokes `/app/renderer` from the exact supplied
production image. Tests use only container loopback with network none, UID/GID
1000, read-only root, all capabilities dropped, no-new-privileges and `/arprt` as
the sole writable output parent (also TMPDIR). There is no writable `/tmp` or
runtime installation. It covers both palettes, four rotations, default/custom
paths, snapshot acquisition, independent credentials and failure preservation.
Only the host architecture executed locally is established by this command.

[Prometheus checks](../../.github/workflows/check-prometheus.yaml) run scoped
source checks and native Linux amd64/arm64 builds and execution. Normal CI calls
all renderer workflows and shared checks. The independent
[release workflow](../../.github/workflows/release-prometheus.yaml) calls only
Prometheus verification, exports the exact tested native image archives and
loads them for publication without rebuilding. It owns the `release-prometheus`
concurrency group, `prometheus/vMAJOR.MINOR.PATCH` annotated tags and
`ghcr.io/dayflower/arprt-renderer-prometheus` package, with no latest tag.
Guards reject VERSION/tag/checkout/main-ancestry mismatches and used release or
platform versions. VERSION 0.2.0 is published with safe-area support and partial-bucket outline
removal as described in the changelog. See
[release procedure](../../notes/RELEASE.md) and [changelog](CHANGELOG.md).

## Kubernetes configuration and updates

[screen.yaml](screen.yaml) contains synthetic ConfigMap, separate Prometheus and
capture Secrets, and an ArprtScreen. It pins the verified public 0.2.0 index
digest above. Both platform images were pulled anonymously before updating the
example; publication does not apply these resources or modify device assignments.

Create inputs in the controller namespace. The example mounts whole directories
read-only and uses mode 0444 so the Job's UID/GID 1000 can read projected files;
root-owned mode 0400 is unreadable to this user. Use namespace/RBAC isolation for
Secrets. Basic Prometheus authentication can instead mount a strict JSON file
with `username` and `password` and set `auth.type` to `basic`. Capture takes its
own bearer file via `--webcappi-token-file`; never reuse the Prometheus file.
Context and output paths are supplied by ARPRT. No writable configuration,
credential mount or extra temporary volume is needed.

Generate the configuration name from its content, for example:

```sh
kubectl create configmap prometheus-config --from-file=config.json --append-hash --dry-run=client -o yaml
```

Review/apply the new ConfigMap, then update the Screen's config volume reference
to that exact generated name. The tracked example uses a SHA-256 content suffix
instead; either method must change the reference when the content changes. Keep
the old ConfigMap until Jobs referencing it finish. Version Secrets similarly
and change their references for deterministic credential rollout. Existing Jobs
read credentials once at startup. Validate a configuration and queries before
rollout; a Secret/config update alone does not prove a successful render.

Where egress is restricted, allow the Job to reach cluster DNS (UDP/TCP 53),
Prometheus's actual HTTP(S) service port and webcappi's actual HTTP(S) service
port, using the site's namespace/pod selectors or destination CIDRs. The synthetic
HTTPS hosts use TCP 443. The renderer requires no other runtime download access;
Chromium runs in webcappi, whose own network policy is separate. NetworkPolicy
cannot generally select arbitrary DNS names. No cluster policy is installed by
these examples.

## Operator acceptance handoff

Record each scope independently in [VALIDATION.md](VALIDATION.md):

- Compare instant values, freshness and range/bucket results with real Prometheus
  at the same fixed evaluation time and timezone. Check units, counter resets,
  aggregation and source timestamps; synthetic tests do not validate operator PromQL.
- Run the opt-in real-browser checks and inspect summary/history/daily PNGs for
  both palettes and orientations with the intended fonts, legends and units.
- In an isolated cluster, verify UID-readable ConfigMap/Secret mounts, Job
  security settings, DNS/egress, PVC output collection, Screen updates and hashed
  config/credential rollouts. Force API/capture failures and confirm the prior
  successful image survives without temporary-file leaks.
- On gray4 and spectra6 hardware, check text/axes, eight-series distinctions,
  stale/missing data, signed stacks and rotation/overlays. Decide whether the
  provisional `* partial` label is acceptable.
- Before claiming arm64 support was executed, obtain passing native arm64 CI;
  a cross-build, pull or workflow definition alone does not establish execution.

Live Prometheus, cluster and physical checks are operator acceptance work. No
local-environment deployment, image publication or tag push is performed by
implementation or offline validation.
