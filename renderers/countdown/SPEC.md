# ARPRT Countdown renderer

Requirements and specification for the Countdown Job renderer.
ARPRT is an independent, unofficial Kubernetes controller for TRMNL BYOS,
with no affiliation with or endorsement by TRMNL.

Status: implemented for the unreleased 0.1.0 version. Confirmed requirements
reflect the design discussion; the detailed behavior below defines the initial
implementation. This document does not extend the ARPRT protocol.

## Confirmed requirements

- Display the number of days remaining until one configured date.
- Make the remaining day count the primary visual element.
- Support an optional event title. Omit the title and its reserved space when
  none is supplied; do not assume that the event is New Year.
- Format both the target date and today's date as `YYYY-MM-DD`.
- Follow the reviewed monochrome concept: a calendar icon and `COUNTDOWN`
  header, a large centered number, a status label and a date footer.
- Use a landscape 800 × 480 screen as the primary design reference.

The reference scenario is target `2027-01-01`, today `2026-11-10`, no title,
and `52 DAYS TO GO`. The generated concept is a visual reference, not a
pixel-accurate fixture or evidence of browser/device acceptance.

## Initial scope

One execution reads configuration and Render Context, samples the current
instant once, computes a date model, renders HTML, captures it through a
separately operated webcappi, and atomically replaces the output PNG.
There is no source API, calendar feed, persistent state or renderer scheduler.
ARPRT owns Job scheduling, final palette conversion, dithering, overlays and
delivery. Refresh at least daily after midnight in the selected timezone;
more frequent ARPRT refreshes can reduce delays and recover missed runs.
Displayed `TODAY` means the date sampled for that render, not a live clock.

Initial output supports both existing palettes, all four rotations and safe
area insets. Multiple targets, recurring anniversaries, hours/minutes/seconds,
business-day counting, holiday calendars, progress bars, configurable themes,
custom labels and interface localization are outside the initial scope.

## Configuration

Use a UTF-8 JSON object supplied through required `--config`:

```json
{
  "version": 1,
  "targetDate": "2027-01-01",
  "timezone": "Asia/Tokyo",
  "title": "New Year"
}
```

Omit `title` to reproduce the reference scenario.

| Field | Required | Meaning |
| --- | --- | --- |
| `version` | Yes | Integer, exactly `1` |
| `targetDate` | Yes | A valid Gregorian date, exactly `YYYY-MM-DD`, years `0001`–`9999` |
| `timezone` | No | IANA timezone identifier; default `Asia/Tokyo`; `UTC` is accepted |
| `title` | No | Optional display text, at most 128 Unicode code points after trimming |

Reject unknown configuration fields, duplicate keys, null values, invalid UTF-8,
trailing JSON documents and files over 64 KiB. Dates with whitespace, missing
zero padding, timestamps, offsets or impossible month/day combinations fail.
Reject unknown timezones, the host-dependent `Local` timezone and numeric UTC
offsets. Embed timezone data so results do not depend on host files.

Trim surrounding title whitespace. An empty result behaves as an omitted title.
Reject control characters, including embedded newlines and tabs. Preserve other
Unicode text and HTML-escape it; never interpret it as markup. Long titles use
a single line with ellipsis, while the complete title remains in the HTML
`title` attribute. No network font loading is permitted.

## Date calculation and states

Capture one execution-start instant from the system clock. Convert it into
the configured timezone to obtain today's year, month and day. The target is
a date in that same timezone, with no time of day.

Compute `delta = ordinal(targetDate) - ordinal(today)` using proleptic Gregorian
calendar dates. Do not divide elapsed seconds by 86,400 or subtract local
midnight timestamps. Leap days and 23/25-hour DST days must not change calendar
day counting. A date remains a calendar label even if a historical timezone
transition skipped that local date. Reject a sampled local year outside
`0001`–`9999` before capture.

Today's date is excluded: tomorrow is one day away. Display a decimal integer
without a sign, separators or leading zeros. The displayed count is
`max(delta, 0)`; the full accepted date range requires up to seven digits.

| Condition | Number | Status label |
| --- | --- | --- |
| `delta > 1` | `delta` | `DAYS TO GO` |
| `delta == 1` | `1` | `DAY TO GO` |
| `delta == 0` | `0` | `TODAY IS THE DAY` |
| `delta < 0` | `0` | `TARGET PASSED` |

Past targets are a successful render, not an input error. Do not roll the target
into another year or display negative days. The configured target date remains
visible in every state. A render crossing midnight keeps its original sampled
date throughout; the next Job updates it.

## Visual layout

- White background, black type and rules for both `gray4` and `spectra6`.
  Font antialiasing may introduce intermediate grays; no semantic distinction
  relies on gray or color.
- Header: a small embedded outline calendar icon, `COUNTDOWN`, then a thin rule.
  Use original inline SVG geometry or a vendored licensed asset; no remote icons.
- Main region: optional centered title, then the dominant count and status label.
  Without a title, recenter the count and label within the available main region.
- Footer: a thin rule; `TARGET` and target date on the left, `TODAY` and sampled
  local date on the right. Dates always use `YYYY-MM-DD`, including zero padding.
- Interface labels are English. User titles retain their original language.
  webcappi must have fonts for the supplied title; recommend installed Noto Sans
  CJK JP for Japanese text, with a local sans-serif fallback.

Use approximately 24 px design padding and 2 px rules at 800 × 480. Target
32 px header text, 28 px title, 20 px dates and at least 16 px secondary labels.
Use a bold sans-serif count starting at 220 px in landscape and 200 px in
portrait, decreasing only as needed to fit the complete number, to a minimum
of 64 px. Never truncate, wrap or abbreviate the number or dates. Title ellipsis
is allowed; required labels and dates may not be removed or shrunk below their
minimum sizes. These sizes are initial layout parameters, subject to browser
validation while preserving the hierarchy and minimums.

For a 480 × 800 logical portrait viewport, retain the header and centered main
region, with target/today footer groups stacked vertically. All required content
must remain inside the safe area. If the minimum layout cannot fit, fail before
declaring capture readiness and preserve the previous output.

## Job and capture contract

The implementation is an independent Go module under
`renderers/countdown/`, following the existing static binary and scratch-image
approach. Pin its toolchain independently; do not add a
shared renderer SDK.

```sh
renderer --config /absolute/config.json \
  --context /arprt/context.json \
  --output /arprt/output.png \
  --webcappi-url http://capture.invalid:8080
```

`--context` and `--output` default to the paths above. `--webcappi-url` is required;
`--webcappi-token-file` optionally supplies a separate Bearer token, using the
existing renderer token grammar and 8 KiB limit. Require an absolute HTTP(S)
capture URL without embedded credentials, query or fragment. Reject redirects.
Read configuration, Context and optional credentials once, accepting projected
volume symlinks. Do not log titles, paths, credentials or device identities.

Accept Render Context `version: "v1"`, a nonempty `device.name`, and physical
800 × 480 targets with palette `gray4` or `spectra6`. Rotation defaults to zero
and accepts only clockwise 0/90/180/270. Ignore unknown Context fields for forward
compatibility. Limit Context to 1 MiB.

Logical dimensions are 800 × 480 for 0/180 and 480 × 800 for 90/270. Apply rotation
exactly once after validating capture. Output is always an opaque RGB PNG at
physical 800 × 480.

Support optional `renderTarget.safeAreaInsets` in logical upright coordinates:
omitted objects/edges mean zero, each edge is an integer from 0 through 4095,
and remaining width/height must be positive. Reject null/noninteger known edges.
Insets do not rotate again, shrink the capture viewport or reserve controller
overlay space. Keep design padding inside the inset rectangle and pixels outside
it white. Geometrically valid insets may still fail the minimum layout check.

POST self-contained UTF-8 HTML to webcappi `/capture` with `html_mode=file`,
exact logical dimensions and `ready_selector=#countdown-ready`, using the default
browser profile. The page makes no external resource requests. Wait for installed
fonts, fit the count, check required element bounds and complete two animation
frames before readiness. Limit generated HTML to 1 MiB and capture PNG to 32 MiB.
Validate PNG signature, CRCs, static-image structure, complete decoding and exact
logical dimensions before rotation and output encoding.

Use a total execution deadline of 60 seconds and a shared capture budget of
40 seconds. Retry HTTP 503/504 once, with at most two seconds of backoff within
both deadlines; other errors fail immediately. Cancellation and SIGTERM stop
work and clean temporary output. Report bounded error classes with nonzero exit
status; successful replacement exits zero.

Write a unique `.countdown-*.tmp` sibling of the destination, close it, check
cancellation and atomically rename only after full success. Failures preserve
existing output and remove temporary files. Support UID/GID 1000, a read-only
root filesystem, dropped capabilities, no privilege escalation and no
ServiceAccount token. Only the output parent is writable; no writable `/tmp`,
runtime installation or local browser is required. Network egress is needed only
for DNS and the trusted capture service. Titles are sent to that service.

## Acceptance criteria

| Area | Required evidence |
| --- | --- |
| Reference | `2026-11-10` to `2027-01-01` renders `52`, no title, exact date strings |
| Boundaries | Tomorrow, target day and past target produce the defined number and label |
| Calendar | Month/year rollover, leap/non-leap February, DST transitions and full date range use calendar differences |
| Clock | Different timezones near midnight produce their own local date; one clock sample is used throughout |
| Input | Invalid dates/timezones/configuration, unsafe Context and resource limits fail before capture |
| Title | Omitted/blank, ordinary, long and Japanese titles; markup is escaped and long text ellipsized |
| Layout | All states, one/seven-digit counts, titles and both orientations fit without clipped required content |
| Target | Both palettes, all rotations and asymmetric safe areas preserve dimensions and white margins |
| Capture | Readiness, timeout/retry, redirects, token isolation and malformed/oversized PNG handling |
| Output | Successful atomic replacement; previous output preserved and temporary files removed on failures/cancellation |
| Container | Exact production image executes offline stub scenarios under the Job restrictions on native amd64 and arm64 |

Keep ordinary automated tests synthetic and offline with an injected clock and
loopback capture stubs. Real-browser capture must separately verify layout,
fonts and readiness; stub tests do not establish those properties.

Before completing executable changes, run `make check` and
`make -C renderers/countdown container-check`. Record host architecture, native
arm64 evidence, actual browser checks and any skipped checks. Kubernetes/PVC
integration and physical-display readability require separate acceptance and
must not be inferred from unit, container or browser checks.

## Implementation boundaries

The independent module, configuration schema, synthetic fixtures, CLI, embedded
layout, unit/browser tests, Makefile, Dockerfile, container checker, Screen
example, VERSION, changelog and validation record live under this directory.
Renderer verification/release workflows, normal CI integration and release
coordinator registration follow the repository guides. Releases use independent
`countdown/vMAJOR.MINOR.PATCH` tags and
`ghcr.io/dayflower/arprt-renderer-countdown` images. Version 0.1.0 is unreleased;
no public image or deployment acceptance is claimed.
