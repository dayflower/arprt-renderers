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
