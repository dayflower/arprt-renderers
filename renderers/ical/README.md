# ARPRT iCalendar semantics

This directory contains an independent Go module, synthetic calendar fixtures,
a supervised semantics probe and a reproducible dependency comparison. It is
not yet an executable Job renderer: configuration/credential loading, HTTP,
day models, HTML, capture and atomic PNG publication are not implemented.
No production image or renderer release is provided.

Read [the semantics decision](SEMANTICS.md) for the required behavior, dependency
evidence and division of responsibility, and [the input contract](INPUT.md) for
the fixed configuration, authentication, error and resource rules. The
[JSON Schema](config.schema.json) describes configuration structure; the input
contract adds checks JSON Schema cannot express. These are implementation
contracts, not claims that input loading already exists.

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

The verification-only Docker image contains static probe/test executables and
licenses in scratch. `container-check` runs the semantic tests, including worker
termination, without network, writable root or `/tmp`, as UID/GID 1000 with all
capabilities dropped and no privilege escalation. It does not certify the future
production image or PNG/Job integration. Source comparison uses a separate nested
module, so rejected candidates do not become renderer dependencies.

All fixtures are synthetic. Expected intervals and recurrence identities live
in `fixtures/cases.json`. Tests compare meaningful normalized values rather than
an implementation-generated snapshot. See [third-party notices](THIRD_PARTY_NOTICES.md).
The [validation record](VALIDATION.md) distinguishes local amd64 evidence from
the unverified browser, cluster, device and native arm64 paths.
