# Semantics validation record

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
