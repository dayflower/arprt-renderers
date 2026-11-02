# Calendar semantics decision

Reviewed on 2026-11-02. This document fixes the behavior required before any
HTML integration. The probe demonstrates the difficult interpretation paths;
it is deliberately not the complete file/network/semantic validation pipeline.

## Baselines

Source review used clean local checkouts at these exact revisions:

| Project | Revision and reviewed surface |
| --- | --- |
| ARPRT | [`bcdeda4fcc4e718704976c25bfe9044181303f05`](https://github.com/dayflower/arprt/tree/bcdeda4fcc4e718704976c25bfe9044181303f05): `notes/RENDERERS.md`, `notes/PROTOCOL.md`, `api/v1alpha1/types.go`, `internal/jobrender/job.go` |
| webcappi | [`d86aa33583120ddd90a3ab05b81d5fb5ff27dbde`](https://github.com/dayflower/webcappi/blob/d86aa33583120ddd90a3ab05b81d5fb5ff27dbde/notes/API.md): raw HTML POST `/capture`, `html_mode=file`, viewport and readiness |

ARPRT supplies read-only same-namespace ConfigMap/Secret volumes, Context v1 and
an output parent as the only writable mount. Jobs use UID/GID/fsGroup 1000,
read-only root, dropped capabilities, no privilege escalation and no service
account token. Input contents are not part of Context's immutable snapshot;
content changes do not immediately trigger a Job. Required missing objects can
prevent startup with FailedMount. Renderer code owns file parsing/authentication.

The eventual renderer targets 800×480 gray4/spectra6. For 90/270 degrees it
captures 480×800, then rotates once. ARPRT owns palette conversion/dithering and
overlays. The browser must have Japanese fonts and must not fetch external
resources. Capture/PNG/atomic replacement remain outside this experiment.

The independent module pins Go 1.27.3 (`go 1.27.0`, `toolchain go1.27.3`), matching
the already available toolchain used by the other renderers. No shared SDK or
cross-renderer dependency is introduced. Static CGO-disabled compilation is
verified by the scratch test image.

## Dependency decision and evidence

Use [`aanelson/iCalendar`](https://github.com/aanelson/iCalendar/tree/2737d214c3bc15de3b8c6b173aa071af8158ac4b)
at `v0.0.0-20261012201849-2737d214c3bc` for content parsing, typed values,
VTIMEZONE compilation and its own recurrence engine. It has no module
dependencies, uses BSD-3-Clause and needs Go 1.25 or later. The inspected module
revision is dated 2026-10-12; it is recent and untagged, with an explicitly
unstable API. Pin its revision/checksum, keep it behind an adapter and rerun this
suite on any update. Recent activity does not establish long-term maintenance.

The alternatives, fixed in the comparison-only nested module, are:

| Candidate | Version / module date | License | Observation and decision |
| --- | --- | --- | --- |
| emersion/go-ical | `v0.0.0-20250609112844-439c63cef608` / 2025-06-09 | MIT | Recurrence uses rrule-go. The synthetic gap becomes 06:30Z instead of 07:30Z; a custom VTIMEZONE cannot resolve. Replacing timezone semantics would require a larger adapter. |
| arran4/golang-ical | `v0.3.8` / 2026-10-18 | Apache-2.0 | Typed property parsing, without a full occurrence/override resolver. Same observed gap error and custom-zone failure with default parsing. Not selected. |
| teambition/rrule-go | `v1.8.2` / 2023-01-13 | MIT | Ordinary BYDAY/BYMONTHDAY/BYSETPOS/COUNT probes pass. It is a recurrence generator, not a calendar/VTIMEZONE/revision resolver; iterator calls have no cancellation. Not selected. |

Version/date evidence comes from `go list -m -json` and downloaded fixed source;
licenses and API implementations were inspected. Run
`make -C renderers/ical/experiments/compare check` to reproduce observations.
Known deficiencies are characterization expectations, not accepted RFC behavior.
The comparison also proves that the selected library alone chooses the wrong
DTSTAMP revision and applies THISANDFUTURE only to its own occurrence. The parent
module's fixtures prove the renderer adapter corrects those two cases.

## Required interpretation and ownership

Use [RFC 5545](https://www.rfc-editor.org/rfc/rfc5545.html), including its verified
[erratum 4271](https://www.rfc-editor.org/errata/eid4271). That erratum resolves
the contradictory treatment of nonexistent recurrence times: keep them using
the pre-gap offset. Invalid dates such as February 30 are skipped. Explicit and
generated ambiguous fall-back times select the first reading. A nominal day
from a spring-gap wall time ends at the same wall time on the next day; it need
not last 24 hours. These are fixed expected values, not platform defaults.

| Meaning | Decision / owner | Executable evidence |
| --- | --- | --- |
| UTC, IANA TZID, floating | Library; floating interpreted in display zone, unknown TZID fatal | `floating`, `dst-*`, `unknown-zone` |
| Custom VTIMEZONE | Library compiles its definition, including DST rules; no display-zone fallback | `custom-zone`, `custom-zone-dst` |
| All-day and multi-day | Preserve civil dates; DTEND exclusive; absent end defaults to one civil day | `overlap`, `exclusive-end` |
| Timed end / DURATION / no end | Library; exact DTEND duration versus nominal DURATION; absent end is instantaneous | `set`, `floating`, `dst-duration`, `dst-gap`, `overlap` |
| Window extraction | Half-open interval overlap, including starts before window | `overlap`, `exclusive-end` |
| RRULE | Library BYDAY/BYMONTHDAY/BYSETPOS/COUNT/UNTIL; no finite UNTIL required | `bysetpos`, `bymonthday`, `set`, `old-unbounded`, `leap-day` |
| RDATE and EXDATE | Library recurrence set including PERIOD-specific end | `set` |
| Detached instances | Renderer adapter suppresses original by RECURRENCE-ID and tests replacement's actual interval independently | `moved`, `range-moved-in` |
| Revisions | Renderer picks highest SEQUENCE, then latest UTC DTSTAMP, then later input position, separately per source/UID/original RECURRENCE-ID | `revisions`, `timestamp` |
| Cancellation | Winning cancelled master suppresses entire UID; cancelled override suppresses that original; cancelled range suppresses following originals until a later range/exact override | `cancelled`, `range-cancelled` |
| THISANDFUTURE | Renderer applies wall-clock start difference and replacement duration from its original RID forward; later range replaces it, exact instance takes precedence | `range`, `range-date`, `range-dst`, `range-moved-in`, `range-replaced` |
| Missing summary and TEXT | Library unescaping; renderer default `名称未設定`; template must HTML-escape text | `floating`, remaining fixtures' missing summaries |
| Old/dense/impossible rules | Candidate bound plus hard process deadline; no silent truncation | `old-unbounded`, `old-count`, `dense`, `impossible`, supervisor tests |

The renderer groups by source before UID; never merge ordinary events from
different sources. Compare timed RIDs by resolved instant, DATE RIDs by civil
date, retaining original identity after movement. Missing SEQUENCE is zero;
missing DTSTAMP sorts before present stamps (a consumer tolerance), malformed
present values fail. Revisions are selected before expansion. Validate every
event before discarding older revisions so malformed entries never disappear
silently. For conflicts not specified by RFC, an explicit winning detached
component is authoritative over the master's generated set, including EXDATE;
EXDATE still excludes unmodified generated instances.
The `excluded-detached` fixture fixes this consumer conflict rule explicitly.

Range resolution must inspect all detached components, including ones whose
original and actual dates are outside the unshifted window. Expand masters over
a window widened for all range shifts/durations, apply range changes in original
RID order, then intersect the actual interval with the display window. Apply
nominal shifts on the master's wall clock across DST, retain civil DATE values,
and use exact DTEND duration versus nominal DURATION. Bound the lookaround and
all candidates, not just the final visible set. An orphan ordinary detached event
is a standalone occurrence; an orphan THISANDFUTURE is invalid without a master.
Do not feed RANGE directly to the upstream Expander and assume it propagates.

The full validation layer must enforce RFC value types and allowed property
cardinality, DTEND/DURATION exclusivity, valid positive spans (except timed
instantaneous events), RID/master type consistency, unique zone definitions,
legal RRULE forms and unknown important recurrence/date semantics. Validate
unreferenced zone definitions and out-of-window events too. Obsolete EXRULE,
RANGE values other than THISANDFUTURE, non-Gregorian RSCALE and unknown semantic
extensions are whole-source `unsupported_semantics` errors, never ignored.
Ordinary non-display fields such as LOCATION/DESCRIPTION/ATTACH/URL and VTODO
are not interpreted or fetched. This fixes rejection policy without restricting
the required RRULE/RDATE/EXDATE/VTIMEZONE/THISANDFUTURE feature set.

The probe implements revision/range supplementation and basic validation; it
does not implement that entire validation layer or the full parameter/string
budgets. Production work must preserve these decisions and add those validators
before accepting network input or integrating HTML. There is no undecided
feature subset delegated to layout work.

## Normalization and display boundary

Read the clock once at execution start. Compute display periods in the display
timezone using civil-date operations, never repeated 24-hour additions. Month
uses the full visible 4–6 week grid including adjacent-month cells; agenda uses
today through six days later. Both views consume the same normalized source set.

The private parser/worker model may contain source-local UID and original RID,
typed civil/instant start/end, summary and cancellation. Credentials, subscription
URLs and the raw calendar stay outside any display model. Only the parent knows
source ID, display label, ordering and holiday role. Attach those to normalized
instances after successful worker completion for all sources.

Day normalization splits interval intersections with each civil day, recording
effective start/end, previous/next-day continuation and ongoing state. All-day
dates are never shifted by instant conversion. A timed instant belongs only to
its start day, is never ongoing, and is finished only when `now > start`; a
nonzero timed event is ongoing for `start <= now < end`, finished at `now >= end`.
Agenda hides today's finished timed instances but keeps all-day events. Other
days are not filtered by the current clock.

Holiday sources must contain only all-day events. Each covered civil date gains
the holiday name; deduplicate same date/name, preserve source order for different
names, and never count holidays as ordinary appointments. A timed holiday is
fatal. This is a normalization rule, not template logic.

The display model contains only civil day/date labels, source labels, escaped-at-
rendering text, per-day intervals, continuation/ongoing flags and omitted counts.
Sort all-day first, then effective start, configured source order, summary and
stable source/UID/RID tie-breaker. Layout controls finite per-day slots, holiday
name overflow and legend omission for up to ten sources, without changing the
semantic event set. No parsing, timezone lookup, recurrence expansion, secret
reading or network access belongs in templates.

## Verification scope

Synthetic expected-value tests and the dependency comparison establish software
semantics for the enumerated cases. The worker-stall test deliberately starts
nonterminating computation, cancels it and waits for its death; bounded-output
and dense/old/impossible-calendar tests cover distinct termination paths. Static
scratch execution establishes local Linux amd64 build/runtime compatibility.
It does not establish native arm64 execution, a full RFC certification, browser
layout, live subscription compatibility, Kubernetes mounts/collection or physical
display readability. Those require separate evidence; this directory provides
no production renderer image or release acceptance claim.
