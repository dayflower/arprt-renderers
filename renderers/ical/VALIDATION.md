# iCalendar validation record

## 2026-11-02: local Linux amd64

The independent module uses Go 1.27.3 and the checksummed dependency recorded
in go.mod/go.sum. The contract revisions and dependency selection are recorded
in [SEMANTICS.md](SEMANTICS.md). All input is synthetic; no live calendar or
credential was used.

Passed:

- `make check`: shared Ruff checks, iCalendar Go formatting/vet/tests and its
  nested dependency comparison, plus the existing Picsum/RSS checks.
- `make -C renderers/ical image`: static probe and Go test executable built using
  the digest-pinned Go 1.27.3 Alpine builder, copied with licenses into scratch.
- `make -C renderers/ical container-check`: 31 expected-value calendar fixtures,
  body/line/component/result bounds, forced worker cancellation and agenda time
  boundaries. The checker also launches the real `/app/ical-probe` for moved
  ranges, a custom DST timezone and a safe unknown-timezone error. It compares
  instants after UTC conversion, allowing the output to retain source offsets.

The local verification image is `arprt-renderer-ical-semantics:dev`, inspected as
`linux/amd64`, image identifier
`sha256:8213bdf139b8f69eba87b795d2faee225c17c1a3847b4f8c3346b3f4a2bea2ef`.
This is a local verification artifact, not a published renderer image/digest.

The checker uses network none, read-only root, UID/GID 1000, all capabilities
dropped and no privilege escalation. No writable output or `/tmp` mount is
needed for this read/compute-only experiment. The forced-stall test cancels a
child executing indefinitely and waits for it to exit, within the asserted
two-second tolerance. The normal semantic deadline is five seconds, subordinate
to the caller's deadline. No calendar computation is left running in a goroutine.

Not verified: native arm64 execution; configuration/credential file readers;
HTTP/TLS, redirect/authentication and multi-source orchestration; the complete
semantic validation/budget layer; day/holiday normalization; HTML/browser fonts
and layout; capture, PNG rotation and atomic output; Kubernetes mounts, Jobs/PVC
collection and Secret updates; live subscriptions; physical displays. These
results establish the semantics experiment and its process boundary only.

## 2026-11-02: file, fetch and day-model integration on Linux amd64

The offline suite now also covers strict configuration JSON, projected-volume
symlinks, URL and Basic/Bearer file snapshots, preserved Basic UTF-8/whitespace,
invalid credentials, source limits, and safe diagnostics. Loopback HTTP/TLS stubs
exercise mixed authentication and unauthenticated holiday sources, no shared
cookies, three concurrent fetches, same-origin redirects, cross-origin credential
protection, HTTPS downgrade rejection, redirect limits, decompressed body bounds,
status errors and cancellation. Invalid input prevents every HTTP request; a
failed source or semantic stage returns no partial model.

Day-model tests assert four-, five- and six-week month grids, Monday/Sunday week
starts, seven civil days across both DST transitions, exclusive all-day ends,
clipped multi-day intervals, ongoing/finished/instant boundaries, source ordering,
holiday-name deduplication and empty days. The existing 31 semantic fixtures
still pass. Additional worker tests reject invalid out-of-window events, malformed
or unreferenced zones, unsupported semantics, duplicate rule entries, incorrect
value types, invalid UTF-8 and property/parameter/text bounds before publishing
a model. Forced stalled workers are cancelled and reaped; all sources share the
same normalization deadline.

Verification commands: `make check`, `make -C renderers/ical image`, then
`make -C renderers/ical container-check`. The verification-only image contains
static binaries in scratch. Container tests use network none, loopback stubs,
read-only root, UID/GID 1000, dropped capabilities and no privilege escalation.
Only a disposable `/output` parent is writable for fixtures, with `TMPDIR` there;
no writable `/tmp` is needed. The real standalone probe is checked as before.
The image is local and unpublished; it is not a production renderer image.

Not verified: native arm64 execution, real ConfigMap/Secret mount updates,
Kubernetes Jobs/PVC collection, live subscriptions, browser layout/fonts,
webcappi capture, PNG rotation/atomic replacement, and physical displays.
These remain separate integration/acceptance work.

## 2026-11-02: phase 3 Job renderer and browser layout on Linux amd64

The production image now runs `/app/renderer`; the semantics probe remains a
separate local development executable. The scratch runtime contains a static
binary, CA certificates and licenses, with embedded timezone data. Tests are
exported separately and mounted read-only into that production image.

Verification commands: `make check`, `make -C renderers/ical image`, followed by
`make -C renderers/ical container-check`. Offline checks cover Context v1,
both palettes and all four clockwise rotations (including asymmetric corner
positions and transparency compositing), PNG signature/CRC/animation/trailing
bytes/dimensions, capture readiness parameters, independent Bearer authentication,
503/504 retry bounds, redirect rejection, failures and cancellation. The real
production CLI is tested in both views, with default and custom Context/output
paths, invalid PNG output preservation and SIGINT during capture. Sibling
temporary files are cleaned after cancelled writes and rename failures.

Container checks use network none, UID/GID 1000, read-only root, dropped
capabilities, no privilege escalation, PID/memory/CPU limits and no writable
`/tmp`. Only the disposable output parent is writable. Synthetic loopback
HTTP/TLS stubs and worker IPC require no external service. Neither the shipped
runtime nor the renderer installs dependencies or writes browser scratch.

Real browser captures use local `webcappi:font-cdp-fix`, labeled source revision
`40136ce4a0e6bd9473f931786474449414855838`, and Chromium
156.0.8078.47 with installed Noto CJK fonts. The host disallows the browser's
user-namespace sandbox; this synthetic-only local webcappi ran with
`sandbox: false`. This is layout evidence, not validation of a production
browser sandbox configuration. Source/render credentials were synthetic and
no live subscriptions were accessed.

`TestLayouts` captures 20 combinations: five synthetic models (empty, dense,
long Japanese text, four-week February and six-week August), month/agenda and
800×480/480×800. The dense models use ten sources, 80 events per day and four
long holiday names. Review included the PNGs, neighboring-month cells,
independent day slots, holiday strips, ellipses, legends and omission counts.
The minimum six-week landscape cells retain one event row; dense portrait
agenda slots retain separate holiday and omitted-count rows. A separate local
DevTools inspection of these same 20 documents checks viewport containment,
nonoverlapping date/holiday/event/omission regions, complete event rows and
visible-plus-omitted event totals. This browser inspection is separate from
the ordinary offline Go suite and from the capture API's PNG validation.

Not verified: native arm64; Kubernetes ConfigMap/Secret effective mounts and
updates, Jobs/PVC collection or Screen status; live calendar providers; physical
displays and palette readability. CI/release integration and cluster/device
acceptance remain phase 4. No image or tag was published.

## 2026-11-02: e-paper readability and English interface revision

The interface now uses English dates, weekdays, status labels and defaults
(`Calendar`, `Untitled`). User-provided Japanese titles, source names, holidays
and event summaries remain Unicode text and are HTML-escaped without translation.
This intentionally supersedes the initial Japanese-interface plan.

Event text increased from 11px to 18px in month view, and from 16–17px to 22px
in primary agenda slots. Future-day events use 18px and all supplementary text
uses at least 16px. Month prioritizes the summary before a timed start suffix;
source/time detail is easier to read in agenda. The legend now shows the first
two sources with an explicit additional-source count. Hidden-event counts share
date headers, preserving room for larger event text. Dense holiday-filled month
cells can show counts without a complete event row rather than reduce type size.

All styling uses black and white: inverted today digits, double-underlined
holidays, inverted weekend column headings, underlined weekend dates and dotted
frames around neighboring-month digits. No distinction depends on text hue.

Passed `make check`, production image build and
`make -C renderers/ical container-check` on Linux amd64. Real webcappi captures
now cover 24 combinations, adding a normal-density sample with Japanese
appointments and English interface labels. Local DevTools inspection checks
16px minimum text, black/white text colors, viewport containment, nonoverlapping
date/holiday/event regions, complete event rows and exact visible-plus-hidden
counts. The browser environment and synthetic-only sandbox limitation are the
same as in the phase 3 record above. Physical e-paper legibility and native arm64
remain unverified; browser screenshots are not physical-display acceptance.

## 2026-11-02: simplified calendar and event-based agenda

User review removed the month footer, source legend and numeric source labels.
Agenda also removes enclosing boxes, the `Now` marker, `No events`, and empty
Today/Tomorrow sections. Ongoing/filtering semantics remain in the normalized
model; rendering no longer adds a status marker. Typography and monochrome
calendar distinctions from the readability revision remain.

The last five agenda days now form one chronological Upcoming list instead of
five day cards. Every appointment carries its own date and time; empty dates
create neither headings nor reserved space. Future holiday names accompany
appointments rather than creating holiday-only rows. Thirty-two combined
Upcoming candidates and sixteen per-day candidates bound HTML generation;
all extra appointments contribute to the single Upcoming omitted count.
Today/Tomorrow regions shrink to their contents, subject to their reserved
maximum height when Upcoming is present, eliminating large intervening gaps
for sparse calendars. This replaces the original fixed per-day-slot contract.

Verification uses `make check`, image build and container-check on Linux amd64,
plus the existing 24 synthetic real-webcappi captures. Added expected-value
coverage checks that empty dates/priority sections and source labels are absent,
Japanese appointment text is preserved, and the first/last Upcoming dates remain
ordered. DevTools inspection verifies complete rows, exact visible-plus-omitted
counts for both priority days and the combined list, dates on each Upcoming row,
nonoverlapping sections, minimum 16px text and removal of agenda borders/legends.
Browser sandbox and unverified native arm64, cluster and physical-display scope
are unchanged from the preceding records.

## 2026-11-02: landscape agenda columns

Landscape agenda places Today/Tomorrow vertically in the left column (40%) and
Upcoming in the right column, separated by whitespace. Upcoming rows use a date
column with a large title and time beneath it. Portrait keeps its existing
vertical flow. When either group is empty, the remaining group uses the available
width. Text sizes, omission accounting and borderless styling are preserved.

Verified with `make check`, image build, then container-check on Linux amd64,
plus 24 synthetic real-webcappi captures. DevTools inspection checks the groups
are aligned side by side without overlap in landscape and flow vertically in
portrait, alongside row containment, exact omission totals and 16px minimum text.
The previously recorded browser sandbox and native arm64/cluster/physical-display
limitations remain.

## 2026-11-02: phase 4 CI, distribution and acceptance

The following results apply to the phase 4 working tree, not a published release.
The local production image was built with VERSION=0.1.0 and the deliberately
non-release REVISION label `phase4-local`; its architecture was inspected as
`linux/amd64`. Release CI supplies the exact tagged source revision instead.

### CI and release preparation

- Passed `make check`, including nine new offline release guard/notes tests.
  They cover canonical renderer-specific tags, VERSION/annotated-tag/main
  ancestry/exact-checkout guards, paginated package lookups, partial publication,
  existing releases, fail-closed API errors and index digest/platform validation.
- Built the production image, verified version/revision labels, and passed
  `make -C renderers/ical container-check` under the existing network-none Job
  restrictions. Both palettes, rotations, paths and failure/cancellation tests
  remain in this offline suite.
- Passed actionlint for normal CI and both new iCalendar workflows.
  Normal CI now calls iCalendar checks with native amd64/arm64 runners.
  The release calls only those checks and publishes their exported native images
  without rebuilding. VERSION, package, tag prefix and concurrency are independent.
- These are local amd64 results and workflow configuration. No GitHub Actions
  run or native arm64 execution of this working tree was performed. Both native
  jobs must pass for the reviewed revision before publication; cross-compilation
  or successful amd64 execution would not satisfy the arm64 requirement.

### Disposable native amd64 Kubernetes acceptance

A dedicated kind cluster and kubeconfig used the cached node image
`kindest/node@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed`.
The controller was built from ARPRT
`bcdeda4fcc4e718704976c25bfe9044181303f05` and installed with its local production
controller/CRD charts. A synthetic Device made authenticated BYOS requests; no
physical device identities or credentials were used in this cluster.

The calendar service used Python's standard-library HTTPS server with an
acceptance-only self-signed certificate and fixed synthetic Basic/Bearer checks.
Its CA was mounted read-only over the renderer trust directory for this test;
production keeps the image's public CA bundle. No TLS verification was disabled
in the renderer. An unauthenticated all-day holiday endpoint checked that no
Authorization header arrived. Capture used real `webcappi:font-cdp-fix`, revision
`40136ce4a0e6bd9473f931786474449414855838`, with an independent synthetic capture
Bearer token and installed Noto CJK fonts. It used `sandbox: false` for the host
limitation already described above; this does not validate a production browser
sandbox. Browser capture and synthetic source services run separately from the
restricted renderer Job.

Passed, in order:

1. Mounted configuration ConfigMap, URL/Basic/Bearer/capture Secrets and test CA
   as six disjoint read-only volumes. Requested mode 0400 remained readable with
   effective UID/GID/fsGroup 1000. The controller-created Job had read-only root,
   all capabilities dropped, no privilege escalation/service-account token and
   exactly one writable output mount.
2. Fetched two authenticated normal calendars and one holiday calendar; captured
   real agenda HTML; completed the Job; collected its PNG from the PVC; delivered
   an authenticated 800x480 gray4 PNG.
3. Updated URL, Basic password and Bearer token Secret contents in place.
   The latest attempt and Job count remained unchanged during an eight-second
   observation. This is a bounded observation, not a guarantee of scheduling
   latency or an atomic multi-file update.
4. Changed the Screen refreshInterval to create the next Job. It successfully
   read the new URL/password/token. The resulting PNG visibly contained
   `Work v2` and `Family v2` instead of their v1 labels, and its SHA-256 changed
   from `fd16d5a45422e4a8460105d54c4506294bf2fa2fcd5bc10c33982310ef28e4e2`
   to `3f826945aeff684be54d1b039d89c9176095782754f7623a92b8ad8ba33b1e97`.
   Secret content changes alone are not documented as immediate render triggers;
   versioned objects/Screen reference updates remain the consistency recommendation.
5. Replaced the Bearer token with an invalid synthetic value and changed the
   Screen spec again. The renderer Job failed with JobFailed. The latest
   successful attempt and delivered PNG hash remained the v2 values.
6. Referenced a required missing URL Secret. Kubernetes reported FailedMount
   before renderer startup; the last successful delivered PNG remained unchanged.
   This is distinct from a running renderer rejecting authentication.
7. Removed the test namespace, Helm releases and test-owned CRDs, then deleted
   the dedicated kind cluster. The operator's selected cluster was not used by
   this isolated suite.

These results verify file input, mount permissions, next-Job updates, capture,
collection and failure preservation on native amd64. They do not test a real
calendar provider, production-scale scheduling or native arm64 Kubernetes.

### Physical display acceptance

The user identified available TRMNL OG and E1002 devices in the local cluster.
Temporary synthetic Screens used the same renderer binary, existing capture
service, `dithering: none`, 800x480 and rotation 0. The OG received month/gray4;
the E1002 received agenda/spectra6. Japanese and English all-day appointments
covered today and several following days. Both Screens became ImageReady.
Only temporary Screen assignments were changed; existing playlist references
were saved and restored after review. Temporary Screens, ConfigMaps, source Pod
and Service were then removed. Device names, hardware IDs, private endpoints
and real authentication data are intentionally omitted from this record.

The user reported no visibility problems on either display. This is user-observed
physical acceptance of those two synthetic layouts, not inferred from PNGs or
controller status. The user suggested making more use of color on the E1002;
that remains a possible future improvement. The accepted layout retains the
previous black/white distinctions, including on color displays.

Not verified physically: OG agenda, E1002 month, portrait/other rotations, timed
appointment details, dense overflow, long holiday names or other calendars.
Those layout cases have separate earlier browser/offline evidence. Live
subscriptions, provider interoperability and native arm64 remain unverified.
No release tag, GHCR image or GitHub release was created; the local-cluster test
image is not a published distribution artifact.

## 2026-11-02: iCalendar 0.1.0 publication

[iCalendar 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/ical/v0.1.0)
was published from annotated tag `ical/v0.1.0`, source revision
`fa4967af5ae50cf517f47b73b6749c6fa252c978`.
The [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/41452941481)
passed source checks and native Linux amd64/arm64 builds, offline unit tests
and production CLI checks under Job security restrictions. It published those
exact tested image archives without rebuilding. An earlier main arm64 build
failed while downloading a module from the Go proxy; the tagged release's
native arm64 build and execution both succeeded.

The public image is `ghcr.io/dayflower/arprt-renderer-ical:0.1.0`; its immutable
multi-platform reference is
`ghcr.io/dayflower/arprt-renderer-ical@sha256:077ee2b4423d0420dda649436637f1f6d601038009d22ba4c372561a337c7a4c`.
Anonymous version-tag and index-digest pulls succeeded for both Linux amd64 and
arm64 using a fresh empty Docker config. The public amd64 image's OCI version
and revision labels matched 0.1.0 and the source revision above.

Local `make check`, the version/revision-labeled image build and its container
checker passed before tagging. After publication,
`make -C renderers/ical container-check IMAGE=<immutable-reference>` passed on
native Linux amd64, including offline tests and the production CLI's default
and custom paths, both palettes, all rotations and failure/cancellation behavior.
No postpublication native arm64 execution was performed; its anonymous pulls
verify availability, while native execution evidence comes from release CI.

The earlier browser, isolated Kubernetes and user-observed physical-display
acceptance records remain separate evidence for the same renderer implementation.
This publication did not repeat those acceptance checks, deploy a Screen or
change Device assignments. Live calendar subscriptions/provider compatibility
and the broader physical cases listed above remain unverified.

## 2026-11-03: iCalendar 0.1.1 publication

[iCalendar 0.1.1](https://github.com/dayflower/arprt-renderers/releases/tag/ical/v0.1.1)
was published from annotated tag `ical/v0.1.1`, source revision
`c9596e58a694340245a08f86c562234a296d1cc4`.
The [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/41601106107)
passed source checks and native Linux amd64/arm64 builds, offline unit tests
and production CLI checks under Job security restrictions. It published those
exact tested image archives without rebuilding.

This patch ignores vendor `X-` properties on VEVENT, STANDARD and DAYLIGHT,
including Google conference/Hangouts links and Apple structured locations.
Synthetic fixtures cover these properties, custom timezone observances and
full CLI rendering through a stub capture service. Regression tests preserve
input budgets and rejection of unsupported recurrence/date semantics and
unknown non-`X-` properties. No real calendar content or credentials were used.

The public image is `ghcr.io/dayflower/arprt-renderer-ical:0.1.1`; its immutable
multi-platform reference is
`ghcr.io/dayflower/arprt-renderer-ical@sha256:a0f9ea18051a804531fb0d6ff31197f06c1b5378281d17ee22bc2c306aa22365`.
Anonymous version-tag and index-digest pulls succeeded for both Linux amd64
and arm64 using a fresh empty Docker config. The public amd64 image's OCI
version/revision labels matched 0.1.1 and the source revision above.

Local `make check`, a 0.1.1 version/revision-labeled image build and its container
checker passed before tagging. After publication,
`make -C renderers/ical container-check IMAGE=<immutable-reference>` passed on
native Linux amd64, including the vendor-property tests, real production CLI,
both views/palettes, all rotations, default/custom paths and output preservation
after failure/cancellation. No postpublication native arm64 execution was
performed; native execution evidence comes from release CI, while its anonymous
pulls verify public availability.

Live Google Calendar feeds, real-browser rendering, Kubernetes and physical
displays were not tested for this patch. Earlier synthetic browser/cluster/display
acceptance records remain separate evidence. Publication did not deploy a Screen
or change Device assignments.

## 2026-11-03: iCalendar 0.1.2 publication

[iCalendar 0.1.2](https://github.com/dayflower/arprt-renderers/releases/tag/ical/v0.1.2)
was published from annotated tag `ical/v0.1.2`, source revision
`e2b41b42bde2484dec2a98182a61ddd0a4e17840`.
The [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/41606755101)
passed source checks and native Linux amd64/arm64 builds, offline unit tests
and production CLI checks under Job security restrictions. It published the
exact tested image archives without rebuilding.

This patch accepts valid DATE DTSTAMP metadata, including dates without VALUE,
for iCloud holiday compatibility. Date-only stamps rank like missing stamps in
revision selection; SEQUENCE and UTC timestamp ordering remain intact. Synthetic
fixtures cover Japanese summaries, yearly COUNT rules, multiple DATE EXDATEs,
gzip fetching, holiday-only and mixed-source rendering. Invalid dates, duplicate
stamps, floating or TZID-bearing stamps and existing rejected semantics remain
covered by regression tests.

Before publication, a snapshot of Apple's public Japanese holiday feed (44
VEVENTs) expanded successfully across 72 monthly windows from 2024 through 2029.
November 2026 included Culture Day and Labor Thanksgiving Day on their expected
civil dates. The same snapshot passed month/agenda CLI execution through HTML
submission and PNG output with a stub capture service. No additional validation
relaxation was needed. The real feed was kept outside the repository; automated
tests use synthetic fixtures only. These checks establish snapshot compatibility,
not continuing live-source availability or real-browser layout.

The public image is `ghcr.io/dayflower/arprt-renderer-ical:0.1.2`; its immutable
multi-platform reference is
`ghcr.io/dayflower/arprt-renderer-ical@sha256:7f1ef75678d70e762acaf7b090269668688a7780e7ad79b967891916caa2993c`.
Anonymous version-tag and index-digest pulls succeeded for both Linux amd64
and arm64 using a fresh empty Docker config. The public amd64 image's OCI
version/revision labels matched 0.1.2 and the source revision above.

Local `make check`, a 0.1.2 version/revision-labeled image build and its container
checker passed before tagging. After publication,
`make -C renderers/ical container-check IMAGE=<immutable-reference>` passed on
native Linux amd64, including the new stamp/holiday tests and production CLI
checks under Job restrictions. No postpublication native arm64 execution was
performed; its native execution evidence comes from release CI, while anonymous
pulls verify public availability.

Real-browser layout, Kubernetes and physical displays were not retested for
this patch. Earlier acceptance records remain separate evidence. Publication
did not deploy a Screen or change Device assignments.

## 2026-11-05: iCalendar 0.2.0 publication

[iCalendar 0.2.0](https://github.com/dayflower/arprt-renderers/releases/tag/ical/v0.2.0)
was published from annotated tag `ical/v0.2.0`, source revision
`239d294e40c874e5a0b5914be9a3c6a69034ba33`.
The [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/41927581172)
passed source checks and native Linux amd64/arm64 builds, offline unit tests
and production CLI checks under Job security restrictions. It published the
exact tested image archives without rebuilding. The
[main checks](https://github.com/dayflower/arprt-renderers/actions/runs/41927375400)
also passed for this source revision before tagging.

This minor release always displays Today and Tomorrow, including empty days,
and shows holiday names and monochrome holiday date markings. Upcoming adds
chronological rows for holidays without appointments. Candidate selection and
browser fitting prioritize appointments over holiday-only rows and include
omitted holiday rows in `+N more`. Multiple holidays use the first name and `+N`.
Regression coverage checks empty days, holiday-only and mixed dates, escaping,
per-day and combined candidate limits, and exact pre-layout omission totals.

Before the version bump, the agenda implementation was measured using synthetic
models in Chromium at 800×480 and 480×800. Empty, holiday-only, sample, dense,
long-text and mixed holiday/appointment cases passed row containment, date
ordering and omission-total checks. Mixed overflow retained appointments ahead
of holiday-only rows. Portrait priority holiday headings retained their minimum
height. These implementation checks are separate from release CI; they do not
establish live-subscription, cluster or physical-display acceptance.

The public image is `ghcr.io/dayflower/arprt-renderer-ical:0.2.0`; its immutable
multi-platform reference is
`ghcr.io/dayflower/arprt-renderer-ical@sha256:5b5dc25f8184493ae9638b8c09789ade4d01b92b0c2dbf5365fd329e2a4b5388`.
Anonymous version-tag and index-digest pulls succeeded for both Linux amd64
and arm64 using a fresh empty Docker config. Both platforms' OCI version and
revision labels matched 0.2.0 and the source revision above.

Local `make check`, a version-labeled image build and its container checker
passed before tagging. After publication,
`make -C renderers/ical container-check IMAGE=<immutable-reference>` passed on
native Linux amd64, including production CLI checks under Job restrictions.
Postpublication native arm64 execution was not performed; its native execution
evidence comes from release CI, while anonymous pulls verify public availability.
Kubernetes, physical displays and live subscriptions were not retested for this
release. Publication did not deploy a Screen or change Device assignments.

## 2026-11-08 safe-area source and browser checks

Repository-wide `make check` and this renderer's `container-check` passed.
The latter exercised the final Linux amd64 production image with network none,
UID/GID 1000, read-only root, dropped capabilities, and only output storage writable.

`TestBrowserSafeArea` passed 16 synthetic captures through a local
`webcappi:font-cdp-fix` image on Linux amd64. Cases use both gray4/spectra6
palettes, all four rotations, and logical insets top 4, bottom 8, left 12,
right 16 in both month and agenda views. The checks require successful renderer readiness, visible
content, white pixels outside the safe area, and unchanged 800 × 480 physical
PNG dimensions with the expected clockwise rotation.

The existing zero-inset `TestLayouts` matrix also passed.
A separate browser probe confirmed that a 1 × 1 content rectangle reports
layout overflow instead of successful readiness.

The local browser used `sandbox: false` because this host does not provide
Chromium's required user namespaces. This setting was confined to the temporary
capture service; renderer Job restrictions were unchanged. These synthetic
checks do not establish native arm64 execution, live-source compatibility,
Kubernetes integration with ARPRT v0.8.0, overlay avoidance, or physical-display
readability. No renderer release or cluster update was performed.

## 2026-11-08 iCalendar 0.3.0 publication

- Source revision: `d7c29272551b337e0aacd56daac9e5e9479810c6`.
- Release: [iCalendar 0.3.0](https://github.com/dayflower/arprt-renderers/releases/tag/ical/v0.3.0).
- Native release verification and publication: [successful workflow](https://github.com/dayflower/arprt-renderers/actions/runs/42460484909).
- Immutable multi-platform reference:
  `ghcr.io/dayflower/arprt-renderer-ical@sha256:1e907ae5e7d54fc599947860f25ac0612c48aaac006af814cd069e4482822b6f`.

The release verified native Linux amd64 and arm64 builds, offline tests and
production CLI execution under Job security restrictions, then published the
exact tested image archives without rebuilding. The release index asset contains
exactly linux/amd64 and linux/arm64 and matches the immutable digest above.

Anonymous version-tag and index-digest pulls passed on both platforms using an
empty Docker configuration. Explicit platform inspection verified OS/architecture
and OCI version `0.3.0` and revision `d7c29272551b337e0aacd56daac9e5e9479810c6` labels.
The public amd64 index reference passed `container-check` with network none,
UID/GID 1000, read-only root, dropped capabilities and only output storage writable.

Postpublication native arm64 execution was not performed. These publication
checks add no live-source, real-browser, Kubernetes or physical-display acceptance.
Earlier browser evidence has its own scope above. Updating the example digest
pins does not apply resources or change existing Device assignments.
