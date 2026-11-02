# ARPRT iCalendar renderer

An independent Go Job renderer for ARPRT, an unofficial Kubernetes controller
for TRMNL BYOS. Neither project is affiliated with or endorsed by TRMNL.
The renderer fetches configured iCalendar sources, resolves calendar semantics,
and sends escaped HTML with English interface labels and Unicode event text to a separately operated webcappi for capture.
The image is local and unpublished. The independent [VERSION](VERSION) is 0.1.0;
CI and guarded release workflows are prepared, but no tag/image has been published.

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

`agenda` shows Today and Tomorrow without enclosing boxes and suppresses
sections that have no appointments. It does not display `Now`, `No events`,
source numbers/names, a source legend or an explanatory footer. Month also omits
both legends. Large typography and monochrome calendar distinctions remain.

In landscape, Today/Tomorrow stack in the left column and Upcoming occupies
the right column. Portrait keeps the vertical flow. Empty groups do not reserve
a column.

The remaining five days form one chronological `Upcoming` list. Each row
carries its own date, title and time; empty dates create no row or reserved slot.
Holiday names are attached to the time metadata of appointments on that date,
rather than generating separate holiday-only rows. Day-spanning appointments
retain their daily continuation arrows. Only today's finished timed events
are removed by normalization, as before.

Browser measurement removes complete overflowing rows and displays exact
`+N` counts in month headers, `+N more` in Today/Tomorrow headers, and a combined
`+N more` count in the Upcoming heading. Month candidates remain bounded to
eight per day. Agenda retains sixteen candidates per day and at most thirty-two
combined Upcoming candidates; all additional appointments count toward omission.
Dense earlier dates may use the remaining list space, with later appointments
included in the overall omitted count. This replaces the initial fixed slots
for all seven days. The complete normalization model remains unchanged.

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
Bearer and independent capture-token mounts. Replace placeholders and load the
local image on test nodes before applying it in the controller namespace.
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
separately, then pin the verified index digest in the Screen example. There is
no published digest to pin yet. Renderer releases do not deploy Screens or alter
Device assignments.
