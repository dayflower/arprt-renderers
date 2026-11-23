# ARPRT Swatch renderer

A standalone Go Job renderer that displays solid color tiles labeled with their
`#rrggbb` values, for evaluating how colors look on physical displays after
ARPRT palette conversion and dithering. ARPRT is an independent, unofficial
Kubernetes controller for TRMNL BYOS, with no affiliation with or endorsement
by TRMNL.

[Swatch 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/swatch/v0.1.0)
is published for Linux amd64 and arm64 as `ghcr.io/dayflower/arprt-renderer-swatch:0.1.0`.
The verified public multi-platform reference is:

```text
ghcr.io/dayflower/arprt-renderer-swatch@sha256:7ffdd35f7f81584058388a564c5df5b920ec4503aa0f0c06b1f5e35d1467fe20
```

The renderer sends self-contained HTML to a separately operated webcappi, validates capture, rotates once and atomically
writes an opaque RGB PNG. There is no source API, persistent state or runtime
dependency installation.

## Quick start

```sh
make -C renderers/swatch setup
make -C renderers/swatch build
renderers/swatch/dist/renderer --config /absolute/config.json \
  --context /absolute/context.json --output /absolute/output.png \
  --webcappi-url http://localhost:8080
```

The minimal configuration shows the palette reference row and the built-in
24-color candidate set:

```json
{"version": 1}
```

All fields:

```json
{
  "version": 1,
  "note": "dithering: none",
  "reference": true,
  "colors": [
    {"color": "#aaaa00", "label": "muted (current)"},
    {"color": "#808080"}
  ]
}
```

| Field | Meaning |
| --- | --- |
| `version` | Required integer `1` |
| `note` | Optional header text, up to 128 code points; use it to record the Screen's dithering mode |
| `reference` | Optional boolean, default `true`; show the palette's exact colors first |
| `colors` | Optional array of 1–24 `{color, label}` entries replacing the built-in set |

`color` is exactly `#rrggbb` (case-insensitive, displayed lowercase) and must be
unique. `label` is optional, up to 32 code points. Notes and labels are trimmed,
HTML-escaped, reject control characters and use ellipsis when they do not fit.
Unknown keys, duplicate keys, nulls, invalid UTF-8, trailing JSON documents and
configurations over 64 KiB fail before capture. See [the schema](config.schema.json),
[synthetic configuration](fixtures/config.json) and
[complete requirements and specification](SPEC.md).

## Display

The page uses a fixed 30-slot grid, so tile sizes do not depend on the number
of colors: 6 × 5 in landscape and 3 × 10 in portrait. A one-line header shows
`SWATCH`, the palette, the rotation and the note. Device names are not shown.

The reference row contains the exact palette entries ARPRT quantizes to:

| Palette | Reference colors |
| --- | --- |
| `spectra6` | `#000000` `#ffffff` `#bf0000` `#438a1c` `#6440ff` `#fff338` |
| `gray4` | `#000000` `#555555` `#aaaaaa` `#ffffff` |

ARPRT's Spectra 6 values are EPF reference colors rather than pure RGB
primaries. Candidates start on the row after the reference group. The built-in
candidates cover yellows, ochres/browns, oranges, greens, purples,
magentas/pinks, teals and grays:

```text
#ffff00 #cccc00 #aaaa00 #cc9900 #808000 #996633 #ff8000 #ffaa00
#00ff00 #00c000 #008000 #66aa00 #80c080 #800080 #8000ff #aa66cc
#cc99ff #ff00ff #ff8080 #008080 #00c0c0 #555555 #808080 #aaaaaa
```

Tiles are opaque solid fills. Overlay text is black or white, whichever has the
higher WCAG contrast ratio against the tile (ties choose black), so text itself
needs no color conversion. Tiles whose contrast against white is below 1.25,
such as white and yellow, get a 1 px black outline. If tiles would be smaller
than 64 × 36 px or a color value would not fit, readiness fails and the previous
output is preserved.

## Comparing dithering modes

The renderer emits unquantized RGB and cannot observe the Screen's
`spec.imageProcessing.dithering`. Show the same configuration through Screens
that differ only in dithering and record the mode in `note` so photographs are
self-describing. [screen.yaml](screen.yaml) uses `none`;
[screen-dithered.yaml](screen-dithered.yaml) uses ARPRT's default `atkinson`.
`floyd-steinberg` and `bayer` are also supported by ARPRT. Both examples use
a 24-hour refresh interval because output changes only with configuration or
Context, and pin the verified public multi-platform digest; replace the
synthetic webcappi endpoint before use.

## Job and capture contract

`--config` and `--webcappi-url` are required. `--context` defaults to
`/arprt/context.json`, and `--output` to `/arprt/output.png`.
`--webcappi-token-file` optionally reads a bounded Bearer token file once.
Projected-volume symlinks are supported. Capture URLs must be absolute HTTP(S)
without userinfo, query or fragment. Redirects are rejected. Notes and labels
are sent to the trusted capture service; input paths, text and credentials are
not logged.

Render Context v1 requires a nonempty `device.name` and physical 800 × 480
`gray4`/`spectra6` target. Clockwise rotation 0/90/180/270 defaults to zero;
90/270 capture a 480 × 800 logical viewport. Output remains physical 800 × 480,
8-bit opaque RGB. Optional `renderTarget.safeAreaInsets` uses upright logical
coordinates; pixels outside the safe area remain white. ARPRT owns final
palette conversion, dithering and overlays.

Capture POSTs UTF-8 HTML to `/capture` with `html_mode=file`, exact logical
dimensions and `ready_selector=#swatch-ready`, using the default browser
profile. Capture shares a 40-second budget and retries 503/504 once within a
total 60-second execution deadline. PNG size, CRCs, static-image structure,
decoding and dimensions are validated before rotation/encoding. A unique
`.swatch-*.tmp` sibling is atomically renamed after success; errors and
cancellation preserve the previous output. The scratch runtime needs write
access only to the output parent; no browser, shell, compiler, runtime
installation or writable `/tmp` is needed.

## Build, validation and distribution

```sh
make check
make -C renderers/swatch image
make -C renderers/swatch container-check
```

The container checker mounts a separate exported static test binary into the
exact production image under network-none, UID/GID 1000, read-only root, dropped
capabilities and no privilege escalation. It exercises the shipped CLI, default
and custom paths, palettes, rotations, input/capture failures and SIGTERM.

Optional browser/layout checks require a separately operated local webcappi:

```sh
cd renderers/swatch
SWATCH_WEBCAPPI=http://127.0.0.1:8080 SWATCH_LAYOUT_DIR=/tmp/swatch-layouts \
  go test -run '^TestBrowserLayouts$' -v -timeout 300s
```

The matrix covers the default set, 24 labeled candidates with a long note, a
single candidate without the reference row, both palettes, all rotations and
asymmetric insets. It checks white safe-area margins, that every tile keeps its
exact configured RGB value in the capture, and physical dimensions, and rejects
an insufficient safe area. Set only `SWATCH_LAYOUT_DIR` to export HTML without
capture; that is not browser acceptance. See [VALIDATION.md](VALIDATION.md) for
actual evidence and the separate native architecture, cluster and
physical-display limits.

The independent module pins Go 1.27.3 and uses only the standard library.
Normal CI calls `check-swatch.yaml`; `swatch/vMAJOR.MINOR.PATCH` releases call
only Swatch checks and publish the exact tested native amd64/arm64 images to
`ghcr.io/dayflower/arprt-renderer-swatch` without rebuilding. The renderer is
registered with the common release coordinator. See the
[publication record](VALIDATION.md#2026-11-24-swatch-010-publication) for
verification evidence and limits.
