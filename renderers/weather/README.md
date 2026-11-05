# ARPRT Weather renderer

A Go Job renderer that fetches one Open-Meteo forecast, generates English HTML
with embedded weather and wind SVGs, captures it through a separately operated
webcappi service, and atomically writes an opaque RGB PNG. ARPRT is independent
and unofficial, with no affiliation with or endorsement by TRMNL.

[Weather 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/weather/v0.1.0)
is publicly available for Linux amd64 and arm64 as
`ghcr.io/dayflower/arprt-renderer-weather:0.1.0`. The immutable multi-platform
reference is
`ghcr.io/dayflower/arprt-renderer-weather@sha256:2eef95da45eb4382ead5baf8f02d234bb5d52e4ca7af73af9fd11554c91d0ca3`.
VERSION remains independently set to 0.1.0. See the [changelog](CHANGELOG.md) and
[publication evidence](VALIDATION.md#2026-11-06-weather-010-publication).
The local development image remains `arprt-renderer-weather:dev`.

## Build and use

```sh
make -C renderers/weather setup
make -C renderers/weather build
make -C renderers/weather image
make -C renderers/weather container-check
renderers/weather/dist/renderer --latitude 35.0 --longitude 135.0 \
  --webcappi-url https://webcappi.example.com \
  --context /absolute/context.json --output /absolute/output.png
```

| Option | Default / behavior |
| --- | --- |
| `--latitude` | Required finite WGS84 latitude, -90 through 90 |
| `--longitude` | Required finite WGS84 longitude, -180 through 180 |
| `--location-name` | Optional display-only place name below Now time (development builds) |
| `--model` | Omitted: Open-Meteo automatic selection; otherwise one model ID |
| `--temperature-unit` | `celsius`; accepts `celsius` or `fahrenheit` |
| `--context` | `/arprt/context.json`, read-only Render Context v1 |
| `--output` | `/arprt/output.png`; parent must already exist |
| `--webcappi-url` | Required HTTP(S) base URL without credentials/query/fragment |
| `--webcappi-token-file` | Optional read-only Bearer token file, at most 8 KiB |
| `--input` | Optional read-only JSON snapshot instead of a live API request |

Zero coordinates are valid; omission, NaN and infinity are rejected. Model IDs
use lowercase ASCII letters, digits and underscores and are passed to `models`
without a local list that would become stale. The provider validates model
availability. Explicit models never fall back to a different model. A model may
cover fewer than seven days or omit variables; these values remain unavailable.
API request errors remain fatal. The API endpoint is fixed to the public
`https://api.open-meteo.com/v1/forecast`; commercial customer endpoints and API
keys are not supported. See [third-party notices](THIRD_PARTY_NOTICES.md).

`--input` supports offline reproduction with the same response schema, including
`timezone`, `utc_offset_seconds` and UNIX timestamps. Coordinates remain required
but are not checked against the snapshot's grid coordinates. Snapshot values must
already use the requested temperature unit and m/s; supplied unit metadata is
validated. Snapshots are not converted. The synthetic
[fixture](fixtures/forecast.json) is dated November 5, 2026; old data are not
relabeled as current forecasts. The renderer samples the execution clock once.

## Display and time rules

- Today: date, English weather name/icon and API daily high/low and maximum precipitation probability.
- Now: model-based current temperature and its valid local time, not a station
  observation. An optional user-supplied place name appears below its local time. Missing values or samples over two hours from execution time, or
  outside today's local date, display a dash.
- Hourly Forecast: today's 00:00, 03:00, 06:00, 09:00, 12:00, 15:00, 18:00 and
  21:00, including past hours. Each slot shows a weather icon, temperature, wind
  direction icon, wind speed and precipitation probability.
- Tomorrow: date, English weather name/icon and API daily high/low and maximum precipitation probability.
- 7-Day Forecast: today through six days later, including Today and Tomorrow; each day includes
  maximum precipitation probability.
- Footer: retrieval time, optional requested model and Open-Meteo
  attribution link. In snapshot mode the timestamp is processing time.

Dates are fixed month/day plus English weekday, such as `11/5 (Thu)`, without
zero padding. Time uses `03:00`. Date/weekday formatting is independent of future
translation. Labels and WMO code names are isolated in the English dictionary in `labels.go`;
English is the only shipped language, with no locale switch. Temperature unit is
independent of language: one decimal place, with negative zero normalized. The
current value and section legends show °C or °F; other temperatures are bare
numbers without a standalone degree symbol.
Wind uses one decimal place in m/s. Humidity is neither requested nor displayed.

In development builds, `--location-name "Example City"` adds a plain-text
place name beneath the Now time in both orientations. Omission or whitespace
leaves no location row. Surrounding whitespace is trimmed; the name accepts
up to 128 Unicode characters without control characters. Long names fit on one
line with an ellipsis; the full text remains in the HTML title. The name is
display-only and does not change coordinates, model or timezone, and is never
sent to Open-Meteo. The timezone identifier is no longer displayed, but all
times still use the forecast location timezone. This option and display change
are not in the published 0.1.0 image.

The API resolves the location timezone with `timezone=auto`. Hourly and current
UNIX instants are converted using embedded IANA timezone data, including DST.
Daily date labels use the API's `utc_offset_seconds` convention; they are not
reaggregated from the eight displayed hours. Week days are built by calendar
addition. When a selected local hour is nonexistent at a DST transition it is
unavailable; when repeated the first occurrence wins.

Wind arrows point toward the direction the wind travels: a northerly wind points
down. The API's incoming bearing is rotated 180 degrees. A speed that rounds to
0.0 m/s displays a calm circle. Missing direction displays a dash while preserving
available speed. WMO codes map to embedded icons; unknown codes display
an unknown icon/name. Individual missing values display `—`. Seven daily and eight
hourly slots are always retained. A response with no displayable data fails.
Malformed arrays, times, values and inverted daily high/low pairs fail.

Development builds use the Render Context palette automatically: `gray4`
keeps monochrome artwork and text; `spectra6` adds restrained accents.
Daily highs are red, lows and precipitation probabilities blue. Sun artwork
is red, rain blue, thunderstorms yellow, and snow/hail light blue. Partly
cloudy, cloud, fog and unknown icons, current/hourly temperatures, wind, section headings,
borders and background retain their black/white appearance. Icons use one
stroke color each; pinned Lucide source files are unchanged. ARPRT still owns
final device palette conversion and dithering. This behavior is not in the
published 0.1.0 image.

Landscape places Today/Now/Tomorrow above the hourly and weekly strips. Portrait
places Now above Today/Tomorrow, hourly slots in two rows and weekly days in aligned
date, weather, probability and high/low columns. Hourly slots emphasize larger
weather icons and temperatures, followed by probability and wind; humidity is omitted. No external fonts, images, styles or browser data requests are made.
Only HTML-escaped text crosses into the page; SVG markup comes from pinned, embedded Lucide assets; wind arrows remain
renderer-authored. The capture service needs an installed sans-serif font.

## Contracts and runtime

The imported baseline is ARPRT v0.2.2 Render Context v1. Context requires device.name
and an 800 × 480 gray4/spectra6 physical target; other sizes/palettes fail. Rotation
0/90/180/270 is clockwise and applied exactly once after capture. At 90/270 the
logical viewport is 480 × 800. ARPRT owns palette conversion, dithering, overlays
and device delivery size limits. The renderer's PNG ceiling is 32 MiB.

The webcappi contract follows the existing RSS renderer baseline: raw UTF-8 HTML
POST to `/capture`, `html_mode=file`, exact logical width/height and
`ready_selector=#weather-ready`, with no browser profile, resizing or full-page
capture. Readiness waits for installed fonts and two animation frames. URLs with
embedded credentials are rejected. Capture authentication is never sent to the
forecast API. Neither request follows redirects. Capture retries HTTP 503/504
once, with at most two seconds of backoff, within one shared 40-second budget.

The total execution deadline is 60 seconds, with a 15-second weather request
budget. Forecast JSON and Context are bounded to 2 MiB and 1 MiB respectively,
after decompression; HTML is bounded to 1 MiB. PNG signature, CRCs, static-image
structure, complete decoding and exact logical dimensions are validated before
rotation and opaque RGB encoding. Errors/cancellation preserve the previous output.
Temporary `.weather-*.tmp` files are unique siblings of the output, removed on
failure and renamed only after successful write/close and cancellation checks.

The scratch runtime contains a static Go binary, CA certificates, embedded
timezone data and licenses. It supports UID/GID 1000, read-only root, dropped
capabilities, no privilege escalation and no ServiceAccount token. Only the output
parent is writable; no writable `/tmp` or runtime installation is needed. Token
contents, coordinates, response bodies, Context device data and endpoint URLs are
not logged. Configure Job egress for Open-Meteo, DNS and the trusted capture service.

[screen.yaml](screen.yaml) pins the verified public multi-platform digest of
Weather 0.1.0 and uses synthetic coordinates and a synthetic capture endpoint.
Replace the capture URL before use. For local development, build/load
`arprt-renderer-weather:dev` and replace the image reference. Refresh is managed by ARPRT, initially
every 30 minutes; the renderer does not run a scheduler or persist a cache. For
custom `contextPath`/`imagePath`, pass matching `--context`/`--output` arguments.

## Location settings from a Kubernetes Secret

[screen-secret.yaml](screen-secret.yaml) contains a synthetic Secret and
ArprtScreen that pass latitude, longitude and the display name through
`renderer.env[].valueFrom.secretKeyRef`. ARPRT copies these standard environment
references and arguments into the renderer Job. Kubernetes expands
`$(WEATHER_LATITUDE)`, `$(WEATHER_LONGITUDE)` and `$(WEATHER_LOCATION_NAME)` in
the argument list before starting the process. No shell, Kubernetes client or
ServiceAccount token is needed in the renderer; it continues to accept ordinary
CLI arguments. See Kubernetes documentation on
[Secret environment variables](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/#define-container-environment-variables-using-secret-data)
and [arguments using environment variables](https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#use-environment-variables-to-define-arguments).

This example uses `arprt-renderer-weather:dev` because `--location-name` is
unreleased and unsupported by the public 0.1.0 image. Build/load the development
image onto eligible cluster nodes, or replace the image reference with your
own accessible image built from this source. Replace the synthetic capture URL.
Create the Secret and Screen in the same controller namespace; Secret references
cannot cross namespaces. Quote coordinate values under `stringData` as strings.
Keep actual personal values in your private deployment configuration.

All three Secret keys are required in this example. To omit the display name,
remove both its `WEATHER_LOCATION_NAME` environment entry and the
`--location-name` argument/value pair. To use the public 0.1.0 image with Secret
coordinates, make those same removals and use the verified digest in
[screen.yaml](screen.yaml).

Newly created Jobs read the current Secret values. Changing a Secret does not
change an already started process's environment or trigger an immediate ARPRT
render. For an explicit configuration rollout, create a new versioned Secret
(for example `weather-location-v2`) and update all three Screen references.
Literal location values stay out of the Screen, but are expanded into the
running process's arguments; coordinates still go to Open-Meteo and the
display name is included in the HTML sent to webcappi. This example has been
checked offline, not applied to a cluster.

## Verification and releases

```sh
make check
make -C renderers/weather image
make -C renderers/weather container-check
```

Ordinary tests are offline and synthetic. Container checks export a separate
static test binary and run it with the exact production image, network none,
UID/GID 1000, read-only root and only the output parent writable. Loopback capture
stubs exercise the shipped CLI, default/custom paths, both palettes, all rotations,
PNG validation and previous-output preservation. No tests/fixtures ship in runtime.

Optional real-browser checks:

```sh
cd renderers/weather
WEATHER_WEBCAPPI=http://127.0.0.1:8080 WEATHER_LAYOUT_DIR=/tmp/weather-layouts \
  go test -run '^TestBrowserLayouts$' -v -timeout 180s
```

This captures synthetic normal, missing-data, Fahrenheit and precipitation cases with both palettes in both
orientations and checks element bounds before readiness. Screenshots still need
visual inspection. See [VALIDATION.md](VALIDATION.md) for actual scope and limits.

Normal CI calls `check-weather.yaml` for scoped source checks and native Linux
amd64/arm64 image execution. Independent annotated `weather/vMAJOR.MINOR.PATCH`
releases matching VERSION on origin/main call only Weather verification. Release
publication transfers the exact tested images without rebuilding, refuses used
version/platform tags or existing releases, and publishes no `latest`. Publication
and deployment are separate actions; see the root release guide.

Multiple locations, model comparison, precipitation amounts, warnings, historical views,
additional languages and wind speed unit selection are outside the initial scope.

Precipitation probability is displayed as integer percent. Today/Tomorrow use
the label `Precip`; hourly and weekly sections label the probability once in
their headings and display only percentages in individual slots. Current
conditions do not request or display it. Hourly values use
`precipitation_probability` for the selected hour, not a three-hour aggregation.
Daily values use the API's `precipitation_probability_max` for the full day, not
a maximum calculated from the displayed slots. Missing or unsupported values show
`—`, including when an explicitly selected model does not provide probabilities.
Values must be within 0–100%; supplied units must be `%`.

A live compatibility check on November 5, 2026 returned probabilities for automatic
model selection, while explicit `jma_seamless` returned null probabilities. Omit
`--model` to use automatic selection when probability availability matters. No
secondary model is fetched to fill an explicitly selected model's missing fields.

Today and Tomorrow have wider summary columns and larger headings, weather icons
and high/low temperatures. Now occupies a narrower landscape column and a compact
portrait header. Portrait stacks probability below each summary temperature range.

Today/Tomorrow weather artwork uses a 96 px square in landscape and 88 px in
portrait, with condition text and numbers beside it. Before capture readiness,
the renderer centers each weather drawing inside its SVG using its actual
geometry bounds. This keeps sun, cloud and precipitation artwork visually aligned
in the weekly strip despite different internal whitespace. Wind arrows are not
repositioned by this step.

All 29 weather codes in Open-Meteo's published WMO interpretation table are
mapped, including 97 (heavy thunderstorm). Weather icons use a pinned subset of
[Lucide SVGs](icons/lucide/ORIGIN.md); original assets and licenses are vendored
under `icons/lucide/`. No external Go module or runtime icon download is required.

| Open-Meteo codes | Lucide artwork |
| --- | --- |
| 0 | `sun` |
| 1, 2 | `cloud-sun` |
| 3 | `cloud` |
| 45, 48 | `cloud-fog` |
| 51, 53, 55, 56, 57 | `cloud-drizzle` |
| 61, 66, 80 | `cloud-hail` |
| 63, 81 | `cloud-rain` |
| 65, 67, 82 | `cloud-rain-wind` |
| 71, 73, 75, 77, 85, 86 | `snowflake` |
| 95, 97 | `cloud-lightning` |
| 96, 99 | `regex` |
| Unknown or missing | `circle-question-mark` |

The artwork deliberately repurposes `cloud-hail` for light rain and `regex` for
hail, according to the dashboard's chosen visual mapping. Heavy rain uses
`cloud-rain-wind` without inferring wind speed from the weather code. English
condition names and accessible descriptions retain their weather-code meaning,
independent of the upstream icon names. Related conditions still share artwork,
including freezing precipitation and rain/snow showers.

Tomorrow mirrors Today: its weather artwork sits at the right edge, with the
heading, condition, temperatures and precipitation aligned toward the right in
both orientations.
