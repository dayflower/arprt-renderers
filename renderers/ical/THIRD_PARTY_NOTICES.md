# Third-party notices

The renderer module uses `github.com/aanelson/iCalendar` at
`v0.0.0-20261012201849-2737d214c3bc` (commit
`2737d214c3bc15de3b8c6b173aa071af8158ac4b`), copyright 2026 aanelson,
under the BSD 3-Clause license. The full license is in [licenses/iCalendar](licenses/iCalendar)
and is copied into the production image. It has no transitive module dependencies.

The Go standard library and embedded timezone data are built by the pinned Go
toolchain. Its Go license is copied into the production image. ARPRT source
retains the repository's MIT license.

The separate `experiments/compare` module uses these candidates only
on the development host; none is linked into the probe or production renderer image.
The comparison module runs in source CI only, including during release verification:

- github.com/arran4/golang-ical v0.3.8, Apache-2.0.
- github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608, MIT, copyright 2020 Simon Ser.
- github.com/teambition/rrule-go v1.8.2, MIT, copyright 2017–2023 Teambition.

Their module sources retain their licenses. Comparison dependencies are isolated
from the selected module; see their own go.mod/go.sum for development-only
dependencies. Calendar fixtures in this directory are synthetic ARPRT test data;
no upstream fixture corpus or real subscriptions are redistributed.
