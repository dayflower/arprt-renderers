# ARPRT Swatch renderer

Requirements and specification for the Swatch Job renderer.
ARPRT is an independent, unofficial Kubernetes controller for TRMNL BYOS,
with no affiliation with or endorsement by TRMNL.

Status: implemented as unreleased version 0.1.0. Confirmed requirements
reflect the design discussion; the detailed behavior below defines the initial
implementation. This document does not extend the ARPRT protocol.

## Confirmed requirements

- Display a sheet of solid color tiles for evaluating how colors appear on
  physical displays after ARPRT palette conversion and dithering.
- The primary motivation is choosing colors for `spectra6`, where light colors
  such as yellow have poor visibility on a white background. `gray4` is also
  supported.
- Show only filled tiles. Line, dash, stroke-width and text-color samples are
  out of scope.
- Overlay each tile with its color as `#rrggbb`, so a photograph of the display
  identifies every tile without a separate key.
- Provide a built-in candidate set covering yellows, ochres/browns, oranges,
  greens, purples, magentas/pinks, teals and grays, and allow replacing it
  through configuration.
- Optionally show the target palette's exact colors as a reference row.
- Use the name `swatch`: `renderers/swatch/`, image
  `ghcr.io/dayflower/arprt-renderer-swatch`, tags `swatch/vMAJOR.MINOR.PATCH`.

The renderer is an evaluation tool for operators and developers, not an
everyday dashboard. Its output is a deterministic function of configuration
and Render Context.

## Initial scope

One execution reads configuration and Render Context, builds a tile model,
renders self-contained HTML, captures it through a separately operated
webcappi, and atomically replaces the output PNG. Browser capture is used,
rather than drawing pixels directly, so that tiles and overlaid text pass
through the same rasterization path as other HTML renderers.

The renderer emits unquantized RGB. It never converts to the target palette,
dithers or simulates the display. ARPRT owns palette conversion, dithering,
overlays and delivery. Comparing conversion modes is done by displaying the
same configuration through Screens with different
`spec.imageProcessing.dithering` values; the renderer cannot observe that
setting, so operators record it in `note`.

Initial output supports both existing palettes, all four rotations and safe
area insets. Line/text samples, gradients, color measurement, automatic
judgement, palette simulation, palettes other than `gray4`/`spectra6`,
configurable tile sizes and interface localization are outside the initial
scope.

## Configuration

Use a UTF-8 JSON object supplied through required `--config`:

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

The minimal configuration `{"version": 1}` displays the reference row and the
built-in candidate set.

| Field | Required | Meaning |
| --- | --- | --- |
| `version` | Yes | Integer, exactly `1` |
| `note` | No | Header text, at most 128 Unicode code points after trimming |
| `reference` | No | Boolean, default `true`; show the palette reference row |
| `colors` | No | Array of 1–24 tile entries; default is the built-in set |

Each `colors` entry is an object:

| Field | Required | Meaning |
| --- | --- | --- |
| `color` | Yes | Exactly `#` followed by six hexadecimal digits; case-insensitive |
| `label` | No | Secondary tile text, at most 32 Unicode code points after trimming |

Normalize colors to lowercase for display and duplicate detection. Reject
duplicate colors within `colors`; a candidate may equal a reference color.
Reject shorthand (`#abc`), alpha (`#rrggbbaa`), named colors and functional
notation. An empty `colors` array is invalid; omit the field to use the
built-in set.

Reject unknown fields, duplicate keys, null values, invalid UTF-8, trailing
JSON documents and files over 64 KiB. Trim `note` and `label`; an empty result
behaves as omitted. Reject control characters, including newlines and tabs.
Preserve other Unicode text and HTML-escape it; never interpret it as markup.
Text that does not fit uses a single line with ellipsis.

### Built-in candidate set

The default set has exactly 24 colors, displayed in this order:

| Group | Colors |
| --- | --- |
| Yellow | `#ffff00` `#cccc00` `#aaaa00` |
| Ochre / brown | `#cc9900` `#808000` `#996633` |
| Orange | `#ff8000` `#ffaa00` |
| Green | `#00ff00` `#00c000` `#008000` `#66aa00` `#80c080` |
| Purple | `#800080` `#8000ff` `#aa66cc` `#cc99ff` |
| Magenta / pink | `#ff00ff` `#ff8080` |
| Teal | `#008080` `#00c0c0` |
| Gray | `#555555` `#808080` `#aaaaaa` |

Built-in tiles have no label. Changing this set is a behavior change recorded
in the changelog.

### Reference row

| Palette | Reference colors, in order |
| --- | --- |
| `spectra6` | `#000000` `#ffffff` `#bf0000` `#438a1c` `#6440ff` `#fff338` |
| `gray4` | `#000000` `#555555` `#aaaaaa` `#ffffff` |

These are the exact palette entries ARPRT quantizes to (black, white, red,
green, blue, yellow for `spectra6`). ARPRT's Spectra 6 values are the EPF
reference/transmission colors, not pure RGB primaries; see
[ARPRT image processing](https://github.com/dayflower/arprt/blob/a4b512f/internal/imageprocessing/palette.go).
Update this table if ARPRT changes its palette definition. Reference tiles have
no label.

## Visual layout

The page has a white background and a fixed grid of 30 equal tile slots.
The tile size is the same regardless of how many colors are configured, so
photographs of different configurations remain comparable.

| Logical viewport | Grid | Approximate tile size |
| --- | --- | --- |
| 800 × 480 (rotation 0/180) | 6 columns × 5 rows | 124 × 80 px |
| 480 × 800 (rotation 90/270) | 3 columns × 10 rows | 148 × 70 px |

- Header: one line containing `SWATCH`, the palette name, `rotation N` and
  `note` when present, in black text on white, followed by a thin black rule.
  Device names are not displayed.
- Reference tiles occupy the first slots when `reference` is true. Candidate
  tiles start at the first slot of the next grid row after the reference
  group; remaining slots in a partially filled reference row stay empty.
- With `reference` true, at most 24 candidate slots remain in every
  palette/orientation combination. With `reference` false, candidates start at
  the first slot and the 24-entry limit still applies.
- Empty slots are plain white.
- Tiles are solid, opaque fills with no gradients, shadows, rounded corners or
  opacity. Separate tiles with white gaps of about 4 px.
- Draw a 1 px black outline only for tiles whose WCAG contrast ratio against
  white is below 1.25, so very light tiles remain distinguishable from the
  background. Other tiles have no outline.
- Overlay `#rrggbb` centered in each tile in bold, about 18 px, with tabular
  digits. When a label is present, show it below the color in about 12 px.
- Overlay text is either `#000000` or `#ffffff`, whichever has the higher WCAG
  contrast ratio against the tile color; ties choose black. Text is never
  drawn in the tile color.

Use about 16 px design padding. Sizes are initial layout parameters, subject to
browser validation while preserving equal tile sizes and the 30-slot grid.
The grid fills the area inside the safe area, padding and header. If that area
leaves tiles smaller than 64 × 36 px, or the color text does not fit a tile,
fail before declaring capture readiness and preserve the previous output.
Labels may be ellipsized; the color text may not.

Use installed local sans-serif fonts. No fonts, images or styles are loaded
from the network.

## Job and capture contract

The implementation is an independent standard-library-only Go module under
`renderers/swatch/`, following the existing static binary and scratch-image
approach. Pin its toolchain independently; do not add a shared renderer SDK.

```sh
renderer --config /absolute/config.json \
  --context /arprt/context.json \
  --output /arprt/output.png \
  --webcappi-url http://capture.invalid:8080
```

`--context` and `--output` default to the paths above. `--webcappi-url` is
required; `--webcappi-token-file` optionally supplies a separate Bearer token,
using the existing renderer token grammar and 8 KiB limit. Require an absolute
HTTP(S) capture URL without embedded credentials, query or fragment. Reject
redirects. Read configuration, Context and optional credentials once, accepting
projected volume symlinks. Do not log notes, labels, paths, credentials or
device identities.

Accept Render Context `version: "v1"`, a nonempty `device.name`, and physical
800 × 480 targets with palette `gray4` or `spectra6`. Rotation defaults to zero
and accepts only clockwise 0/90/180/270. Ignore unknown Context fields for
forward compatibility. Limit Context to 1 MiB.

Logical dimensions are 800 × 480 for 0/180 and 480 × 800 for 90/270. Apply
rotation exactly once after validating capture. Output is always an opaque
8-bit RGB PNG at physical 800 × 480.

Support optional `renderTarget.safeAreaInsets` in logical upright coordinates:
omitted objects/edges mean zero, each edge is an integer from 0 through 4095,
and remaining width/height must be positive. Reject null/noninteger known
edges. Insets do not rotate again, shrink the capture viewport or reserve
controller overlay space. Keep design padding inside the inset rectangle and
pixels outside it white.

POST self-contained UTF-8 HTML to webcappi `/capture` with `html_mode=file`,
exact logical dimensions and `ready_selector=#swatch-ready`, using the default
browser profile. The page makes no external resource requests. Wait for
installed fonts, check tile and color-text bounds and complete two animation
frames before readiness. Limit generated HTML to 1 MiB and capture PNG to
32 MiB. Validate PNG signature, CRCs, static-image structure, complete decoding
and exact logical dimensions before rotation and output encoding.

Use a total execution deadline of 60 seconds and a shared capture budget of
40 seconds. Retry HTTP 503/504 once, with at most two seconds of backoff within
both deadlines; other errors fail immediately. Cancellation and SIGTERM stop
work and clean temporary output. Report bounded error classes with nonzero exit
status; successful replacement exits zero.

Write a unique `.swatch-*.tmp` sibling of the destination, close it, check
cancellation and atomically rename only after full success. Failures preserve
existing output and remove temporary files. Support UID/GID 1000, a read-only
root filesystem, dropped capabilities, no privilege escalation and no
ServiceAccount token. Only the output parent is writable; no writable `/tmp`,
runtime installation or local browser is required. Network egress is needed
only for DNS and the trusted capture service. Notes and labels are sent to
that service.

## Screen examples

Provide two Screen examples using the same synthetic configuration:

- `screen.yaml` with `imageProcessing.dithering: none` and
  `note: "dithering: none"`.
- `screen-dithered.yaml` with ARPRT's default `atkinson` error-diffusion
  dithering and a matching `note`. Other supported modes are
  `floyd-steinberg` and `bayer`.

Use a long refresh interval (24h), since output only changes with
configuration or Context. Pin the image by digest once published and use a
synthetic capture endpoint.

## Acceptance criteria

| Area | Required evidence |
| --- | --- |
| Defaults | `{"version": 1}` renders the reference row and all 24 built-in colors in the defined order |
| Colors | Case normalization, duplicate rejection, invalid formats and the 1–24 limit |
| Reference | Correct colors per palette; `reference: false` omits the row; candidates start on a new row |
| Overlay | Black/white text selection by contrast, including ties; light-tile outline threshold |
| Text | Omitted/blank, ordinary, long and Japanese note/label; markup escaped and labels/notes ellipsized |
| Input | Invalid configuration, unsafe Context and resource limits fail before capture |
| Layout | 1, 24 and maximum tiles in both orientations keep equal tile sizes and unclipped color text |
| Target | Both palettes, all rotations and asymmetric safe areas preserve dimensions and white margins; insufficient safe areas fail readiness |
| Capture | Readiness, timeout/retry, redirects, token isolation and malformed/oversized PNG handling |
| Output | Opaque RGB output; atomic replacement; previous output preserved and temporary files removed on failures/cancellation |
| Container | Exact production image executes offline stub scenarios under the Job restrictions on native amd64 and arm64 |

Keep ordinary automated tests synthetic and offline with loopback capture
stubs. Unit tests must verify that tile fills in generated HTML are the exact
configured values. Real-browser capture must separately verify layout, fonts,
readiness and that captured tile interiors keep their exact RGB values; stub
tests do not establish those properties.

Before completing executable changes, run `make check` and
`make -C renderers/swatch container-check`. Record host architecture, native
arm64 evidence, actual browser checks and any skipped checks. Kubernetes/PVC
integration and physical-display appearance require separate acceptance and
must not be inferred from unit, container or browser checks. The purpose of
this renderer is to collect physical-display observations; those observations
belong in the validation record, not in automated tests.

## Implementation boundaries

The independent module, configuration schema, synthetic fixtures, CLI, embedded
layout, unit/browser tests, Makefile, Dockerfile, container checker, Screen
examples, VERSION, changelog and validation record live under this directory.
Renderer verification/release workflows, normal CI integration, release
coordinator registration and the root README renderer index follow the
repository guides. Releases use independent `swatch/vMAJOR.MINOR.PATCH` tags
and `ghcr.io/dayflower/arprt-renderer-swatch` images, starting at 0.1.0.
