# ARPRT Countdown renderer

A standalone Go Job renderer that displays calendar days until one configured
date. ARPRT is an independent, unofficial Kubernetes controller for TRMNL BYOS,
with no affiliation with or endorsement by TRMNL.

Version [0.1.0](VERSION) is implemented but unreleased. No public image is
available. The renderer sends self-contained HTML to a separately operated
webcappi, validates capture, rotates once and atomically writes an opaque RGB PNG.
There is no target-data API, persistent state or runtime dependency installation.

## Quick start

```sh
make -C renderers/countdown setup
make -C renderers/countdown build
renderers/countdown/dist/renderer --config /absolute/config.json \
  --context /absolute/context.json --output /absolute/output.png \
  --webcappi-url http://localhost:8080
```

Create the required configuration file:

```json
{
  "version": 1,
  "targetDate": "2027-01-01",
  "timezone": "Asia/Tokyo"
}
```

`targetDate` is a valid `YYYY-MM-DD` date in years 0001–9999. `timezone` defaults
to `Asia/Tokyo`; IANA identifiers and `UTC` are supported, while `Local` is
rejected. Add optional `"title": "New Year"` to display an event name. Omitted
or space-only titles reserve no title space. Titles retain Unicode,
are trimmed and HTML-escaped, reject control characters, and accept up to 128
code points after trimming. Long titles use one line with ellipsis.

Unknown keys, duplicate keys, nulls, invalid UTF-8, trailing JSON documents,
invalid dates/timezones and configurations over 64 KiB fail before capture.
See [the schema](config.schema.json), [synthetic configuration](fixtures/config.json)
and [complete requirements and specification](SPEC.md). Runtime date and timezone
validation is authoritative; the schema alone does not perform those checks.

## Display and date semantics

The screen shows `COUNTDOWN`, an optional title, a large count, a status label,
and `TARGET`/`TODAY` dates in `YYYY-MM-DD` format. On `2026-11-10`, target
`2027-01-01` displays `52 DAYS TO GO`. Tomorrow displays `1 DAY TO GO`.
The target day displays `0 TODAY IS THE DAY`; later dates display
`0 TARGET PASSED`. Targets never recur automatically or produce negative counts.

One execution-start clock sample determines today's date in the configured
timezone. Counts are differences between Gregorian calendar dates, rather than
24-hour durations, so leap days, DST and midnight boundaries have explicit
semantics. Capture crossing midnight keeps the sampled date. ARPRT controls
refresh; the renderer has no scheduler. The [Screen example](screen.yaml) uses
a 30-minute interval to limit stale dates and recover missed runs. It does not
guarantee a refresh exactly at local midnight.

Both `gray4` and `spectra6` use black and white artwork with possible font
antialiasing. Portrait stacks the date groups; landscape places them at opposite
footer edges. Numbers fit within 220 px (landscape) or 200 px (portrait), down to
64 px, without abbreviation or truncation. Required content that cannot fit
prevents readiness rather than producing clipped output. webcappi needs installed
fonts for user titles; Noto Sans CJK JP or Noto Sans JP is recommended for Japanese.
No fonts, images or styles are downloaded by the page.

## Job and capture contract

`--config` and `--webcappi-url` are required. `--context` defaults to
`/arprt/context.json`, and `--output` to `/arprt/output.png`.
`--webcappi-token-file` optionally reads a bounded Bearer token file once.
Projected-volume symlinks are supported. Capture URLs must be absolute HTTP(S)
without userinfo, query or fragment. Redirects are rejected. Titles are sent to
the trusted capture service; input paths, titles and credentials are not logged.

Render Context v1 requires a nonempty `device.name` and physical 800 × 480
`gray4`/`spectra6` target. Clockwise rotation 0/90/180/270 defaults to zero;
90/270 capture a 480 × 800 logical viewport. Output remains physical 800 × 480,
8-bit opaque RGB. ARPRT owns final palette conversion, dithering and overlays;
start with `dithering: none` for text.

Optional `renderTarget.safeAreaInsets` uses upright logical coordinates, with
omitted edges zero and integer edges 0–4095 that leave positive dimensions.
Design padding remains inside the safe area; outside pixels remain white.
Insets do not reserve overlay space or rotate a second time. Unknown Context
fields are ignored; malformed known fields fail. Valid insets can still leave
too little room for the minimum layout, which then fails readiness.

Capture POSTs UTF-8 HTML to `/capture` with `html_mode=file`, exact logical
dimensions and `ready_selector=#countdown-ready`, using the default browser
profile. Readiness waits for installed fonts, measured fitting, bounds checks
and two animation frames. Capture shares a 40-second budget and retries 503/504
once within a total 60-second execution deadline. PNG size, CRCs, static-image
structure, decoding and dimensions are validated before rotation/encoding.

The scratch runtime contains a static binary, CA certificates and licenses,
with embedded timezone data. Only the output parent needs write access.
A unique `.countdown-*.tmp` sibling is closed and atomically renamed after success;
errors and cancellation preserve the previous output and clean temporary files.
No browser, shell, compiler, runtime installation or writable `/tmp` is needed.

## Build, validation and distribution

```sh
make check
make -C renderers/countdown image
make -C renderers/countdown container-check
```

The container checker mounts a separate exported static test binary into the
exact production image under network-none, UID/GID 1000, read-only root, dropped
capabilities and no privilege escalation. It exercises the shipped CLI, default
and custom paths, palettes, rotations, input/capture failures and SIGTERM.

Optional browser/layout checks require a separately operated local webcappi:

```sh
cd renderers/countdown
COUNTDOWN_WEBCAPPI=http://127.0.0.1:8080 COUNTDOWN_LAYOUT_DIR=/tmp/countdown-layouts \
  go test -run '^TestBrowserLayouts$' -v -timeout 180s
```

The matrix covers all status labels, absent/ordinary/long Unicode titles,
one/seven-digit numbers, both palettes, all rotations and asymmetric insets.
It checks nonempty output, white safe-area margins and physical dimensions,
and rejects an insufficient safe area. Set only `COUNTDOWN_LAYOUT_DIR` to export
HTML without capture; that is not browser acceptance. Ordinary tests remain
offline and synthetic. See [VALIDATION.md](VALIDATION.md) for actual evidence
and the separate browser, native architecture, cluster and physical-test limits.

The independent module pins Go 1.27.3 and uses only the standard library.
Normal CI calls `check-countdown.yaml`; `countdown/vMAJOR.MINOR.PATCH` releases
call only Countdown checks and publish the exact tested native amd64/arm64 images
without rebuilding. The renderer is registered with the common release
coordinator. The Screen example uses a local development image and synthetic
capture endpoint; build/load the image and replace the endpoint before use.
