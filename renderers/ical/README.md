# ARPRT iCalendar renderer

An independent Go Job renderer for ARPRT, an unofficial Kubernetes controller
for TRMNL BYOS. Neither project is affiliated with or endorsed by TRMNL.
The renderer fetches configured iCalendar sources, resolves calendar semantics,
and sends escaped HTML with English interface labels and Unicode event text
over HTTP to a separately operated webcappi for capture. ARPRT runs the renderer
as a Kubernetes Job, which writes the captured PNG to the output file.
[iCalendar 0.2.0](https://github.com/dayflower/arprt-renderers/releases/tag/ical/v0.2.0)
is available for Linux amd64 and arm64 as
`ghcr.io/dayflower/arprt-renderer-ical:0.2.0`. The immutable multi-platform reference is
`ghcr.io/dayflower/arprt-renderer-ical@sha256:5b5dc25f8184493ae9638b8c09789ade4d01b92b0c2dbf5365fd329e2a4b5388`.
The independent [VERSION](VERSION) is 0.2.0; the Screen example pins this digest.
See the [changelog](CHANGELOG.md) for changes.

```sh
make -C renderers/ical setup
make check
make -C renderers/ical build
make -C renderers/ical image
make -C renderers/ical container-check
renderers/ical/dist/renderer --config /absolute/config.json \
  --context /absolute/context.json --output /absolute/output.png \
  --webcappi-url http://localhost:8080
```

`--config` is required. Context/output default to `/arprt/context.json` and
`/arprt/output.png`. Context v1 must specify device.name and an 800×480 gray4 or
spectra6 target. Rotation defaults to zero; 90/270 use a 480×800 logical viewport
and rotate exactly once clockwise after capture. Output is an opaque RGB PNG;
ARPRT owns palette conversion, dithering and overlays. Use `dithering: none`
as the initial setting for text.

`--webcappi-url` is required and uses the service's default browser profile.
`--webcappi-token-file` optionally reads a separate Bearer token once before
source requests. It uses the same strict token grammar as source Bearer files.
Capture never follows redirects or forwards calendar credentials. POST uses
`html_mode=file`, the exact logical dimensions and `ready_selector=#ical-ready`.
Fonts must be installed on webcappi (Noto Sans CJK JP or Noto Sans JP recommended).
There are no external fonts, images or page requests. Readiness waits for local
fonts, measures available event slots and completes two animation frames.

Read [semantics](SEMANTICS.md), [input contract](INPUT.md) and
[configuration schema](config.schema.json). `LoadInput` follows projected volume
symlinks and snapshots configuration, URLs and Basic/Bearer files before HTTP.
Sources fetch with bounded concurrency; all must succeed. A supervised private
worker resolves semantics with a shared five-second deadline, then produces
complete day models without subscription URLs, credentials or UIDs. A single
execution-start clock sample and 60-second deadline cover all stages.

Vendor `X-` properties on VEVENT, STANDARD and DAYLIGHT are ignored without
interpretation, including Google Calendar's `X-GOOGLE-CONFERENCE` and
`X-GOOGLE-HANGOUT`, and Apple's `X-APPLE-STRUCTURED-LOCATION`. Property, parameter
and line-length budgets still apply. Unknown non-`X-` properties and unsupported
recurrence/date semantics remain fatal; see [semantics](SEMANTICS.md). This
compatibility change was introduced in 0.1.1.

Version 0.1.2 accepts valid date-only DTSTAMP creation metadata, including
`DTSTAMP;VALUE=DATE:19760401` from iCloud Japanese holiday calendars. Date-only
stamps rank like missing stamps when selecting revisions; SEQUENCE and valid
UTC timestamps keep their existing precedence. Other event/recurrence dates,
invalid dates, duplicate stamps and TZID-bearing stamps retain strict validation.

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

Both month and agenda views use the inset rectangle. Existing event fitting
removes rows that do not fit and updates omission counts without changing the
underlying calendar model. Browser readiness rejects layouts where required
headers or date rows no longer fit.

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

## Display behavior

`month` shows a complete 4–6 week month grid, including neighboring days and
Sunday/Monday week starts. The interface uses English dates, weekdays and labels;
source names, custom titles, holiday names and event text retain their language.
Defaults are `Calendar` for the page title and `Untitled` for a missing summary.
Japanese fonts are still required on webcappi for Japanese calendar content.

The layout favors e-paper readability over the number of visible appointments.
Month event text is 18px; agenda event text is 22px,
and supplementary text is at least 16px. Text does not shrink to fit. Month
prioritizes the event title, followed by the start time when space permits;
agenda shows a large title and separate time metadata. Overflow uses
ellipsis and explicit hidden-event counts. Very dense or holiday-filled month
cells may show only the date, holiday and hidden-event count; agenda provides
more space for appointment details.

All styling uses black and white. Today has white digits on black, holidays
have a double underline, and weekend weekday headings are reversed with
underlined weekend dates. Neighboring-month dates use a dotted frame and italic
digits. These distinctions survive monochrome output without relying on hue.

`agenda` always shows Today and Tomorrow without enclosing boxes, including
empty days. Holidays appear below each date heading with a double-underlined
date; multiple holidays use the first name followed by `+N`. It does not display
`Now`, `No events`,
source numbers/names, a source legend or an explanatory footer. Month also omits
both legends. Large typography and monochrome calendar distinctions remain.

In landscape, Today/Tomorrow stack in the left column and Upcoming occupies
the right column. Portrait keeps the vertical flow. Today/Tomorrow retain their
column even when both days are empty.

The remaining five days form one chronological `Upcoming` list. Each row
carries its own date, title and time. Dates without appointments or holidays
create no row. A holiday without appointments creates a compact row containing
only its double-underlined date and holiday name. Holidays on dates with
appointments remain attached to the time metadata, avoiding duplicate rows.
Multiple holidays use the first name followed by `+N`. Day-spanning appointments
retain their daily continuation arrows. Only today's finished timed events
are removed by normalization, as before.

Browser measurement removes complete overflowing rows and displays exact
`+N` counts in month headers, `+N more` in Today/Tomorrow headers, and a combined
`+N more` count in the Upcoming heading. Month candidates remain bounded to
eight per day. Agenda retains sixteen candidates per day and at most thirty-two
combined Upcoming candidates, including holiday-only rows. Candidate selection
and browser fitting omit holiday-only rows before appointments, starting with
the latest holiday; remaining rows stay chronological. Each omitted holiday-only
row counts as one item in `+N more`, regardless of the number of holiday names.
All additional appointments count toward omission. Dense earlier dates may use
the remaining list space, with later appointments included in the overall omitted
count. Today/Tomorrow holiday headings remain visible independently of event
fitting. This replaces the initial fixed slots for all seven days. The complete normalization model remains unchanged.

## Runtime and verification

The scratch runtime contains only a static renderer, CA certificates and
licenses, with embedded timezone data. No shell, browser, compiler, tests or
runtime installation is needed. Only the output parent is writable. A sibling
`.ical-*.tmp` file is closed, checked for cancellation and atomically renamed
only after complete PNG signature, CRC, static-image and dimension validation.
Capture/semantic/signal failures preserve prior output and clean temporary files.
Capture retries 503/504 once within a shared 40-second budget and the execution
deadline. Diagnostics use closed classes and never print input paths or HTTP
library errors. Calendar text is intentionally sent to the trusted capture
service; configure its access and logging accordingly.

`container-check` exports a separate static test executable, then mounts it
read-only into the exact production image with network none, UID/GID 1000,
read-only root, dropped capabilities, no privilege escalation and only the
output parent writable. Loopback stubs test real CLI execution, both palettes,
all rotations, default/custom paths, PNG failures and signal cancellation.
This validates software behavior; it does not establish Chromium layout.

For optional real-browser verification against a local webcappi:

```sh
cd renderers/ical
ICAL_WEBCAPPI=http://127.0.0.1:8080 ICAL_LAYOUT_DIR=/tmp/ical-layouts \
  go test -run '^TestLayouts$' -v -timeout 120s
```

The matrix captures empty, dense and long-text models with both views and
orientations. Review the resulting PNGs; successful stub captures alone do not
prove fonts or layout. Ordinary tests remain offline and synthetic.

[screen.yaml](screen.yaml) provides synthetic ConfigMap, URL Secret, Basic,
Bearer and independent capture-token mounts. Replace placeholders before
applying it in the controller namespace. The example uses the verified public
image digest.
Input directories must be disjoint from each other and the Context/output
mounts. Default permissions must allow UID/GID/fsGroup 1000 to read every file.
Use versioned objects and update Screen references for consistent multi-file
changes. Secret content changes alone do not trigger an immediate render; a
newly created Job reads current mounted contents. See the validation record for
actual cluster mount/update acceptance; content updates do not establish an atomic snapshot across multiple files.

The standalone `dist/ical-probe` still reads a private semantics request on
stdin and emits normalized instances. It performs no HTTP/capture/output work.
See [third-party notices](THIRD_PARTY_NOTICES.md) and
[validation scope](VALIDATION.md) for evidence and remaining limitations.

## CI and distribution

Normal CI calls [iCalendar checks](../../.github/workflows/check-ical.yaml), with
source checks and native Linux amd64/arm64 image builds and offline execution.
The source job also runs the isolated comparison module and release guard tests.
Native jobs assert architecture and OCI version/revision labels before testing.
A configured arm64 runner is not evidence of a successful arm64 execution;
consult [VALIDATION.md](VALIDATION.md) for completed runs and pending acceptance.

The [release workflow](../../.github/workflows/release-ical.yaml) accepts only
annotated stable `ical/vMAJOR.MINOR.PATCH` tags matching VERSION on origin/main.
It calls only iCalendar checks, then publishes the exact tested native image
archives without rebuilding. Publication uses independent
`ghcr.io/dayflower/arprt-renderer-ical` version/platform tags and no `latest`.
Existing version/platform tags or a GitHub release block publication; partial
publication requires a new version. The release record includes the immutable
index digest and exact source revision. Neither CI nor release automation tests
live subscriptions, a browser, a cluster or a physical display.

Follow [the release guide](../../notes/RELEASE.md#publishing-icalendar) only when
publication is requested. Verify anonymous pulls and postpublication execution
separately, then pin the verified index digest in the Screen example. For 0.2.0,
both platforms passed native prepublication checks and anonymous tag/digest pulls;
the public amd64 image also passed postpublication container checks.
Postpublication native arm64 execution was not performed. Renderer releases do
not deploy Screens or alter Device assignments.
