# Weather validation record

## 2026-11-05: initial working-tree implementation

These checks describe the initial implementation in this working tree, not a
published release or an immutable source revision. Weather VERSION is 0.1.0.

- Repository `make check`: shared Ruff lint/format, all renderer source tests and
  release guard tests passed.
- Native Linux amd64 `make -C renderers/weather image` and `container-check`:
  passed. The exported test executable ran in the exact production scratch image
  with network none, UID/GID 1000, read-only root, dropped capabilities, no
  privilege escalation and only the output parent writable.
- Shipped CLI: both gray4/spectra6 targets, clockwise 0/90/180/270 rotations,
  asymmetric PNG corner placement, opaque RGB output, default/custom paths,
  read-only Context mounts, capture/input failures and previous-output
  preservation passed with offline synthetic inputs and loopback capture stubs.
- Forecast normalization: local date boundaries, non-hour timezone offsets,
  spring/fall DST transitions and weeks crossing them, fixed API daily offsets,
  shortened horizons, missing arrays/values, unknown WMO codes, units, rounding,
  wind direction/calm handling and malformed data passed offline tests.
- HTTP/PNG boundaries: decompressed weather size, truncation, timeout, redirect
  rejection, capture retry limits, content type, dimensions, chunk CRCs, animation,
  trailing bytes and cancellation before output replacement passed offline tests.
- Real Chromium captures through the locally available `webcappi:font-cdp-fix`
  image passed for normal, missing-data and three-digit Fahrenheit cases, each
  at 800 × 480 and 480 × 800. Browser checks rejected out-of-canvas elements and
  horizontal hourly/daily slot overlap. Screenshots were visually inspected.
  A discovered Fahrenheit weekly-column overlap was corrected by stacking daily
  high/low values in landscape. No external fonts or page resources were used.
- The temporary local browser service used `sandbox: false` because the host
  denied Chromium user-namespace sandbox startup. It was bound to loopback and
  used only generated synthetic HTML and renderer-generated live forecast HTML.
  This verifies layout, not sandbox-enabled capture deployment compatibility.
- Live API compatibility: synthetic WGS84 coordinates (35, 135), automatic model
  selection/Celsius and explicit `jma_seamless`/Fahrenheit completed the local CLI
  pipeline through real webcappi, including landscape and portrait rotation.
  API access was read-only; the requests established point-in-time compatibility,
  not forecast accuracy or availability of every model/region.

Native arm64 execution, remote GitHub Actions execution, Kubernetes Job/PVC
collection, physical-display readability, publication and anonymous registry
pulls have not been performed. The workflows configure native amd64/arm64 checks;
configuration alone is not evidence of an arm64 pass. The Screen example uses a
local development image and synthetic capture endpoint, not a deployed service.

## 2026-11-05: precipitation probability

Added hourly `precipitation_probability` and daily
`precipitation_probability_max`. Today, Tomorrow, eight hourly slots and seven
daily slots display integer percentages under `Precip`; Now remains temperature
only. Daily probabilities use the provider's full-day maximum directly. Hourly
probabilities are selected at the displayed hour without a three-hour aggregation.

Offline tests cover API field selection, all 17 display positions, zero percent,
daily versus hourly values, missing/null arrays and elements, invalid percentages
and incorrect units. Synthetic browser captures include 100%, missing values and
three-digit Fahrenheit temperatures in both orientations. Summary rows wrap when
necessary to prevent temperature/probability collisions.

Read-only live requests at synthetic coordinates (35, 135) returned numeric
probabilities with automatic model selection and null hourly/daily probabilities
with explicit `jma_seamless`. Missing probabilities display a dash; no unrequested
model is fetched. This is a point-in-time compatibility observation, not a promise
about every model. Native arm64, cluster and physical-display limits above remain.


## 2026-11-05: layout without humidity

Removed humidity from the API request, normalized view, dictionary and synthetic
fixture. Older snapshots containing extra humidity fields remain readable because
unknown fields are ignored. The hourly query test explicitly rejects humidity.

The hourly strip now emphasizes larger weather icons, temperature and wind speed,
with precipitation probability between temperature and wind. Portrait keeps eight
slots in two rows and aligns weekly dates, icons, probabilities and high/low
columns. Removed humidity rows and adjusted spacing preserve all daily slots.
Source/container checks and the six-case synthetic real-browser matrix cover the
updated layout; native arm64, cluster and physical-display limits above remain.


## 2026-11-05: larger daily summary and simpler numeric labels

Removed standalone degree symbols from forecast temperatures; Now and section
legends retain explicit °C/°F. Hourly and weekly probability cells now contain
only percentages, with `Precip %` shown once in each section heading. Today and
Tomorrow retain their individual precipitation labels.

Landscape allocates wider columns and a taller summary to Today/Tomorrow, with
larger weather icons, headings and high/low values; Now uses a narrower column.
Portrait uses a compact Now header and larger daily cards with probability below
the temperature range. Weekly high/low values fit on one line without degree
symbols. The six-case real-browser matrix passed bounds, section-overflow and
slot-overlap checks; screenshots were visually inspected. Source/container
checks cover the final layout; arm64, cluster and physical-display limits remain.


## 2026-11-05: larger summary artwork and aligned weather drawings

Today/Tomorrow now place the weather drawing beside condition text, temperatures
and precipitation, using 96 × 96 px SVGs in landscape and 88 × 88 px in portrait.
Before readiness, each weather SVG's artwork is centered from its actual geometry
bounds rather than relying on the different whitespace of individual paths.
Wind arrows remain unchanged.

The six-case real-browser matrix passed geometry-center assertions for every
rendered weather SVG, including clear, partly cloudy, overcast, rain, snow,
thunderstorm, fog and unknown icons, as well as canvas/section/slot bounds checks.
Landscape and portrait normal/Fahrenheit screenshots were visually inspected.
Source and native amd64 container checks cover the final implementation; arm64,
cluster and physical-display limits above remain.


## 2026-11-05: published weather-code coverage audit

Compared the weather-code mapping with the current Open-Meteo documentation.
Code 97 (heavy thunderstorm) was missing and is now mapped to the thunderstorm
SVG with its own English label. The offline coverage test now includes all 29
published codes. Related intensities intentionally share artwork; this is code
coverage rather than one unique SVG per code.


## 2026-11-05: vendored Lucide weather artwork

Replaced renderer-authored weather drawings with twelve Lucide SVGs pinned at
commit `ee8e10d746a10907205366cfd984922f0c58939a`. Original SVGs, upstream
license and source/checksums live under `icons/lucide/`. Go embeds the artwork;
there is no runtime network dependency or icon package installation. The shipped
image includes the complete upstream license. Wind arrows remain unchanged.

The selected visual mapping uses `cloud-hail` for light rain, `cloud-rain` for
moderate rain, `cloud-rain-wind` for heavy rain, `regex` for hail and `snowflake`
for snow. English condition names and accessible descriptions retain the original
weather-code meaning. All 29 published weather codes and unknown/missing cases
have nonempty artwork. Asset tests reject executable or externally referenced
SVG content.

The real-browser matrix expanded to eight captures: normal, missing-data,
Fahrenheit/heavy-thunderstorm and mixed precipitation, each in landscape and
portrait. Mixed precipitation covers light/moderate/heavy rain, hail, snow, fog
and drizzle. Geometry-center and bounds checks passed, and normal/mixed-
precipitation screenshots were visually inspected. These checks exercise all
twelve Lucide drawings. `make check` and native amd64 container checks cover this
implementation; arm64, cluster and physical-display limits above remain.


## 2026-11-05: right-aligned Tomorrow summary

Mirrored the Tomorrow card: the weather SVG occupies the rightmost grid column,
while its heading and condition/temperature/probability text align toward the
right. Landscape and portrait retain the existing SVG sizes and artwork centering.
The eight-case real-browser matrix passed bounds/geometry checks; normal landscape
and portrait captures were visually inspected. Source and native amd64 container
checks cover this layout; arm64, cluster and physical-display limits remain.
