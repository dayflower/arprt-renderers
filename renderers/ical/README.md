# ARPRT iCalendar semantics

This directory contains an independent Go module, synthetic calendar fixtures,
a supervised semantics probe, strict file input, bounded HTTP fetching, day and
holiday models, and a reproducible dependency comparison. It is not yet an
executable Job renderer: HTML, capture and atomic PNG publication remain absent.
No production image or renderer release is provided.

Read [the semantics decision](SEMANTICS.md) for the required behavior, dependency
evidence and division of responsibility, and [the input contract](INPUT.md) for
the fixed configuration, authentication, error and resource rules. The
[JSON Schema](config.schema.json) describes configuration structure; the input
contract adds checks JSON Schema cannot express. `LoadInput` follows projected
volume symlinks and snapshots all configuration, URLs and Basic/Bearer credentials
before HTTP begins. Only regular input files are accepted.

```sh
make -C renderers/ical setup
make -C renderers/ical check
make -C renderers/ical build
make -C renderers/ical image
make -C renderers/ical container-check
```

`dist/ical-probe` reads one JSON object from stdin with `Calendar` (the ICS text),
`Timezone` (an IANA name), and RFC 3339 `From`/`To` timestamps. It emits normalized
instances as JSON. Treat both streams as private calendar data. It has no
network functionality or output-image path. The probe always invokes its own
worker process with a five-second deadline, kills it on cancellation and waits
for termination. Runtime computation does not require temporary files.

`Prepare(ctx, configPath, now, workerCommand, transport)` is the integration
boundary for the future renderer. The executable supplies its own private worker
command; neither configuration nor calendar data can choose it. Pass a clock
sample taken once at execution start and retain the caller's execution deadline
through later capture/output work. A nil transport uses Go's default HTTP
transport and trust store. Up to three sources fetch concurrently without retries;
all sources must succeed. Source order determines errors, event ordering and
holiday-name ordering. Normalization uses one shared five-second deadline across
all worker processes and day conversion, subordinate to the execution deadline.

The returned model contains plain text and source labels, with no URL,
authentication or UID. Rendering must HTML-escape text. Month windows cover the
complete 4–6 week grid; agenda covers seven civil days and filters only today's
finished timed events. Events retain daily interval intersections, continuation
and ongoing flags. Holiday names are deduplicated separately. No appointments
are truncated by normalization; finite layout slots and omitted counts belong
to rendering. No partial model is returned after an error.

The verification-only Docker image contains static probe/test executables and
licenses in scratch. `container-check` runs the semantic tests, including worker
termination, plus file/HTTP/TLS/model tests without external networking or
writable root/`/tmp`, as UID/GID 1000 with all capabilities dropped and no privilege
escalation. Only a disposable `/output` parent is writable for test fixtures;
HTTP/TLS stubs bind loopback inside the isolated network namespace. It does not
certify the future production image or PNG/Job integration. Source comparison uses a separate nested
module, so rejected candidates do not become renderer dependencies.

All fixtures are synthetic. Expected intervals and recurrence identities live
in `fixtures/cases.json`. Tests compare meaningful normalized values rather than
an implementation-generated snapshot. See [third-party notices](THIRD_PARTY_NOTICES.md).
The [validation record](VALIDATION.md) distinguishes local amd64 evidence from
the unverified browser, cluster, device and native arm64 paths.
