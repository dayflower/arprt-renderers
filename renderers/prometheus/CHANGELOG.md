# Changelog

## 0.3.0 (2026-11-08)

- Add fixed `small`/`medium`/`large`/`xlarge` sizes (40/48/64/80 CSS px) to
  value panels, preserving automatic 64-to-40 px shrinking when size is omitted.

## 0.2.0 (2026-11-08)

- Respect ARPRT v0.8.0 logical safe-area insets for dashboard placement and
  plot-width-based query sampling while retaining full-size PNG output.
- Preserve zero-inset layout and snapshot Context identity, and reject invalid
  insets or unusable browser geometry.
- Remove the full-height outline around partial bar buckets; retain the `*`
  bucket label and `* partial` key.

## 0.1.0 (2026-11-07)

- Add version-1 dashboards, bounded Prometheus acquisition and deterministic snapshots.
- Render text, values, line and calendar-bucket charts for gray4 and spectra6.
- Capture through webcappi, validate and rotate PNGs, and atomically replace output.
- Add a static scratch runtime, offline Job-restricted container checks and
  independent native amd64/arm64 verification and release workflows.

Published for Linux amd64 and arm64 after native offline container/CLI checks.
Anonymous tag/digest pulls passed for both platforms; postpublication container
checks passed on amd64. See [VALIDATION.md](VALIDATION.md) for acceptance limits.
