# Picsum renderer

A Go CLI for ARPRT Kubernetes Jobs. It downloads a random photograph
from [Lorem Picsum](https://picsum.photos/) or reads a local image, then writes a
PNG. It renders only the photograph; timestamp, battery and Wi-Fi overlays belong
to the controller. This is maintained by ARPRT, which is independent, unofficial
and not affiliated with or endorsed by TRMNL.

## Compatibility and limits

The protocol baseline is **Render Context v1 from ARPRT v0.2.2**, commit
[`d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87`](https://github.com/dayflower/arprt/tree/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87).
The imported renderer and synthetic fixtures are verified by standalone software
tests. Integration with a running ARPRT v0.2.2 controller, live photo downloads,
and physical displays has not been revalidated for this migration. No other
controller version is claimed as tested. See the upstream
[Context specification](https://github.com/dayflower/arprt/blob/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87/notes/PROTOCOL.md#render-context-v1)
and [PNG contract](https://github.com/dayflower/arprt/blob/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87/notes/PROTOCOL.md#png-contract);
this repository does not duplicate the protocol specification.

Only **physical 800 × 480** output is supported. Content rotates clockwise by
0/90/180/270 degrees; omitted rotation means 0. At 90/270 the logical canvas is
480 × 800 before rotation into an 800 × 480 PNG. Unknown Context fields are
ignored. Other display dimensions and `gray16` targets are unsupported.

| CLI `--palette` | Context target palette | Renderer output | Intended use |
| --- | --- | --- | --- |
| `gray4` (default) | `gray4` | 2-bit grayscale, four shades, < 90,000 bytes | OG photographs |
| `bw` | `gray4` | 2-bit grayscale, black/white samples only, < 90,000 bytes | OG monochrome photographs |
| `rgb` | `gray4` or `spectra6` | Opaque 8-bit RGB PNG | Controller-owned conversion, including E1002 Spectra 6 |

Gray modes preserve autocontrast and Floyd-Steinberg dithering, retry with direct
undithered quantization if the encoded PNG is at least 90,000 bytes, and fail if
it still cannot fit. `bw` does not guarantee a partial physical display refresh.
RGB preserves colors and delegates final palette conversion, dithering, overlays
and device output budget enforcement to ARPRT. Its source PNG may exceed the
device budget. `rgb` is a CLI choice, not a Device palette. Configure
`spec.imageProcessing.dithering` on the Screen for controller normalization;
compatible gray4 output retains its own dithering. See upstream
[display overrides](https://github.com/dayflower/arprt/blob/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87/notes/OPERATIONS.md#display-overrides)
for firmware requiring `spec.display.pngByteLimit: 90000`.

Picsum 0.2.0 replaces Python/Pillow with a pure Go implementation using
Lanczos resizing, centered cropping and JPEG EXIF orientation. It preserves the
output contract, not pixel-for-pixel or compressed-byte equality with Pillow:
resampling, quantization and PNG compression can produce different results.
See the [changelog](CHANGELOG.md) for the release changes.

Downloads are bounded to 8,000,000 bytes, use a 20-second timeout and at most
three attempts on I/O errors. Requests contain only dimensions and a random
cache-busting value, never device data. When Picsum supplies a photo ID, logs
include `https://picsum.photos/id/<id>/<width>/<height>` (480/800 at 90/270).
Local input and responses with no ID log no photo URL.

## Local rendering and offline checks

Run from the repository root. Install Go (the toolchain is pinned in `go.mod`),
Python 3 and Make for development. uv is needed only for the shared Ruff checks;
Python is used for container/release tooling, not the renderer runtime:

```sh
make -C renderers/picsum setup
make -C renderers/picsum build
renderers/picsum/dist/renderer \
  --context renderers/picsum/fixtures/render-context-v1.json \
  --input /path/to/photo.jpg --output /path/to/picsum.png
renderers/picsum/dist/renderer \
  --context renderers/picsum/fixtures/render-context-spectra6.json \
  --palette rgb --input /path/to/photo.jpg --output /path/to/picsum-rgb.png
make -C renderers/picsum check
```

Use `--palette bw` for black/white output. JPEG, PNG, GIF (first frame), TIFF,
BMP and WebP inputs work with `--input`; omit it to fetch from Picsum. Other
Pillow-specific formats are not supported by the Go implementation. `--context`
defaults to `/arprt/context.json`, `--output` to `/arprt/output.png`. Output is
written to a unique sibling `.picsum-*.tmp` file and atomically replaced,
preserving existing output if rendering fails. The output parent must exist and
be writable.

To build and exercise the actual image without any Picsum requests:

```sh
make -C renderers/picsum image
make -C renderers/picsum container-check
```

The container checker generates a synthetic color image and runs unit tests plus
the CLI with the network disabled, a read-only root, UID/GID 1000, all capabilities
dropped, no privilege escalation and no ServiceAccount token. Only the output
parent is mounted writable; there is no `/tmp` tmpfs. It tests default and custom
context/output paths and every supported palette/rotation combination. Unit-test
temporary files are redirected into that same output directory. A separate Go
test binary is compiled by the Dockerfile's `test-export` target and mounted
read-only into the production image. This keeps the test suite out of the shipped
image while exercising its runtime filesystem and security restrictions.
Neither normal runtime nor offline tests need package installation inside a
running container.

## Images and Kubernetes usage

[Picsum **0.2.0**](https://github.com/dayflower/arprt-renderers/releases/tag/picsum/v0.2.0) is available for Linux amd64 and arm64,
independently versioned from ARPRT. Pull the public image:

```sh
docker pull ghcr.io/dayflower/arprt-renderer-picsum:0.2.0
```

The Screen examples pin the immutable multi-platform reference:

`ghcr.io/dayflower/arprt-renderer-picsum@sha256:3efae210201cd37cb0b730892d5ab69edd73b20cbc6e1d35f2e4a9eb8ec14f34`

To build/push a custom image, use your own registry and update
`spec.source.job.renderer.image`:

```sh
make -C renderers/picsum image IMAGE=registry.example.com/arprt-renderer-picsum:0.2.0
make -C renderers/picsum container-check IMAGE=registry.example.com/arprt-renderer-picsum:0.2.0
docker push registry.example.com/arprt-renderer-picsum:0.2.0
```

Builds compile a static Go binary with fixed dependencies in `go.mod`/`go.sum`;
the Go build image is pinned by digest. The runtime uses `scratch` and contains
the binary, HTTPS CA certificates and license notices, with no shell, Python,
package manager or runtime dependency installation. Third-party licenses are
recorded in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and shipped in the image.
Release builds add OCI version, source repository, MIT license and source revision
labels. The release records the image index digest and source revision. CI
uses native amd64 and arm64 runners; anonymous pulls have been verified for
both. See [validation scope](../../notes/DEVELOP.md#validation-scope) for the
specific checks performed.

Allow DNS and outbound HTTPS to `picsum.photos` and its redirect CDN
`fastly.picsum.photos`, plus image-pull access from cluster nodes to your registry.
No runtime dependency downloads are needed. A Job failure follows the controller's
compatible-prior-output fallback behavior; it does not publish a partial file.

Apply the OG Screen and assign it to your Device:

```sh
kubectl -n arprt apply --validate=strict -f renderers/picsum/screen.yaml
kubectl -n arprt patch arprtdevice living-room --type=merge \
  -p '{"spec":{"playlistRef":null,"screenRef":{"name":"picsum"}}}'
```

For an 800 × 480 Spectra 6 target such as E1002, use the RGB Screen instead:

```sh
kubectl -n arprt apply --validate=strict -f renderers/picsum/screen-spectra6.yaml
kubectl -n arprt patch arprtdevice studio-display --type=merge \
  -p '{"spec":{"playlistRef":null,"screenRef":{"name":"picsum-spectra6"}}}'
```

Use the namespace containing your paired Device and Screen; names here are
synthetic. Both Screens refresh every 10 minutes and enable controller overlays
with rotating corners. Device polling and content rotation are configured on the
Device separately. Adjust overlay margins for your enclosure. Do not apply both
Screens as an automatic choice of device capabilities.

ARPRT creates the Job. It runs as UID/GID 1000, with a read-only root filesystem,
all capabilities dropped, no privilege escalation and no ServiceAccount token.
Only the specified output parent is writable; no `/tmp` or additional writable
directory is provided. Context is a read-only file mount. If overriding the Job's
`contextPath` or `imagePath`, also pass the matching CLI `--context`/`--output`:
the controller does not rewrite CLI arguments. Choose a dedicated output parent
because its mount hides files baked into the image at that path.

## Source provenance

Imported from `examples/renderers/job/picsum/` in ARPRT v0.2.2 at the baseline
commit linked above. The repository-wide [MIT license](../../LICENSE) applies
to this renderer and its fixtures. Both synthetic Context JSON files in `fixtures/` are unchanged
copies of the same filenames under `testdata/render-context/` in
`dayflower/arprt` at that same commit. Tests use these local copies without
fetching upstream data.
