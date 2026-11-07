# Validation record

## Configuration and query planning, 2026-11-07

This is source-level implementation work on Linux amd64 with Go 1.27.3.
There is no renderer executable, production image or published version yet.

Offline tests cover schema/defaults, type-specific fields, Unicode limits,
bounded JSON, unsafe track syntax, rectangle membership, group/ID constraints,
portrait order, variable scope/token boundaries, escaped text and restricted
expressions, URL overrides/prefixes, Context/rotation and one-shot clock selection.
Fixed-time tests cover Asia/Tokyo, New York's 23/25-hour dates, skipped/repeated
hours, year rollover, midnight zero-duration rejection, partial bucket endpoints,
step/rate interval limits, stable plans, auxiliary/retry request reservations,
snapshot mismatches, path confinement and per-response/cumulative byte ceilings.

An independent Python jsonschema 4.10.3 draft-2020-12 validator was used to compare
the embedded structural validator against the accepted/rejected configuration
corpus. Cross-field semantics remain explicitly outside JSON Schema.

A real, local Chromium 156.0.8078.47 geometry experiment ran with network none
using browser image ID
`sha256:312db162e2a080f26f72e52d6ac7f5b46135883ccd32b1ee540beeca9f33660a`.
Seven cases passed: landscape, generated portrait, explicit portrait, auto
columns, wrapping group, grow ratios and px columns. Fixed/fr sampling widths
matched measured browser plot widths after rounding down (e.g. 432 px landscape,
384 px generated portrait, 140 px explicit portrait). Auto/wrap conservative
1 px sampling widths did not exceed measured widths. This deliberately sacrifices
history resolution in those content-dependent layouts. The experiment's minimal
Grid/flex HTML is not the final renderer, and is not a screenshot or readability
acceptance check. Browser temporary files are unrelated to the renderer's future
output-only runtime contract.

Prometheus timestamp semantics were reviewed against the upstream v2.42.0 engine
source, not a live API. Freshness is restricted to bare selectors because
aggregations/arithmetic do not preserve the original source sample timestamp.

`make check` passed, including all existing renderers and the new offline tests.
The optional independent schema comparison and seven browser geometry cases
passed as described above. Production `make -C renderers/prometheus container-check`
was attempted and failed with an explicit unavailable message because there is
no executable/image yet; this is not a passing container verification.
No native arm64 execution,
webcappi capture, actual API response normalization, live Prometheus, Kubernetes
Job/PVC collection, atomic output replacement, or physical gray4/spectra6 display
acceptance has been performed for this module. No cluster or deployment changes
were made.
