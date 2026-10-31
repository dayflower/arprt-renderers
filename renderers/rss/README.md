# RSS renderer

A Go CLI that reads one RSS feed, generates its own HTML, requests a PNG from
an external webcappi service, and atomically writes an ARPRT Job output file.
It displays headlines, optional summaries, categories, sources, bookmark counts,
optional thumbnails and feed-supplied article dates.
ARPRT is independent and unofficial, with no affiliation with or endorsement by
TRMNL. This renderer has no published image or release yet.

## Supported input and limits

RSS 2.0 and RSS 1.0/RDF are supported. Atom and XML DTDs are rejected. XML uses
namespace URIs, not fixed prefixes. UTF-8, Shift_JIS and EUC-JP are covered by
synthetic tests; other declared encodings depend on the WHATWG charset labels
supported by the pinned `golang.org/x/net/html/charset` package. XML external
entities and external DTDs are never fetched.

RSS 2.0 keeps item order. RSS 1.0 follows `channel/items/rdf:Seq`, skips undefined
references, and appends unreferenced items in document order. Taxonomy sequences
have no effect. Duplicate identifiers retain the first valid titled item;
identifier-free items are not deduplicated by title. Identifiers use guid,
rdf:about, then a valid HTTP(S) link. Missing titles are skipped. A valid empty
feed displays an empty state; a nonempty feed with no valid titled items within
the parsing limit fails.

Summaries can be absent or empty. HTML is parsed into text paragraphs and list
items; script, style, embedded content and feed attributes are discarded.
`content:encoded` is used only when description is absent in generic and
Livedoor processing. Hatena uses description only; Google News hides summaries. Entities are decoded
once by the HTML parser after XML decoding. Plain descriptions retain line
boundaries. Titles, summaries and metadata are escaped through `html/template`.
Dates use pubDate or, when absent, dc:date. Invalid dates are hidden. Feed title
falls back to `RSS feed`; article source falls back to the feed title and is
hidden when it duplicates the display title. Article URLs are retained without
tracking-parameter rewriting; article pages are never fetched.

The default `auto` profile uses exact known feed hosts with RSS structure:
`news.google.com` with RSS 2.0 source elements, `news.livedoor.com` with RSS 2.0
descriptions, and `b.hatena.ne.jp` with RSS 1.0/RDF. Hatena item extensions in the
`http://www.hatena.ne.jp/info/xmlns#` namespace also identify local or mirrored
feeds. Unknown feeds use `generic`; titles and partial host matches never select
a profile. NHK and Yahoo! use generic processing. Local inputs and mirrors can
select `hatena`, `livedoor` or `google-news` explicitly. `generic` disables all
feed-specific corrections.

- Hatena uses the first nonempty dc:subject, dc:date and a nonnegative bookmark
  count (including `0 users`). Missing or invalid counts stay hidden. Descriptions
  never fall back to decorated content:encoded. A valid HTTP(S) hatena:imageurl is
  used as an optional thumbnail candidate.
- Livedoor removes a final standalone anchor whose text is exactly `記事を読む`
  after whitespace normalization. The same text within prose or before further
  content stays intact. Lists retain their item boundaries; `ざっくり言うと`
  becomes a small label when it accompanies a list, without repeating it in the
  summary. Other summary text is retained.
- Google News hides description and content:encoded related stories, displays
  source separately, and strips only an exact trailing ` - {source}` from the
  title. Other hyphens and unmatched source suffixes remain. Missing source
  retains the original title and uses the normal feed-title fallback.

Hatena image URLs and the first valid HTTP(S) image/* enclosure in generic or
Livedoor feeds are optional thumbnail candidates. Google News does not select
images. Description images, article pages and media extensions are never crawled.
JPEG, PNG, GIF and WebP are decoded from their bytes, independently of the HTTP
Content-Type. GIF and animated WebP use the first frame. SVG, audio and video
are unsupported. Images are fitted within 96 × 72 without enlargement, composited
onto white and re-encoded as JPEG data URLs. Aspect ratios are preserved.
Missing, rejected or failed images leave article text intact and reserve no
thumbnail space. `--images=false` disables image requests entirely.
No custom templates, browser installation or runtime dependency installation are
provided.

Image requests use a separate client without an HTTP proxy or capture credentials.
By default, only public unicast destinations are allowed: loopback, private, link-local and
special-use ranges are rejected, including IPv4-mapped addresses. Every new
connection checks all DNS answers and dials a validated IP directly; redirects
also use this policy and are limited to five HTTP(S) hops without URL credentials.
The explicit feed and webcappi endpoints may still be internal services.
Set `--public-images-only=false` to disable destination IP filtering for image
requests, including redirects, when using trusted feeds with internal images.
HTTP(S)-only URLs, credential rejection, redirect limits, proxy/authentication
isolation and all size, format and deadline limits still apply. Apply outbound
network policy for additional operator restrictions.

At most the configured display limit of image candidates is fetched, with two
workers, five seconds per request, ten seconds for the entire image stage and the
shared overall deadline. Image bodies are limited to 2 MiB after decompression;
headers must declare positive dimensions totaling at most 4,000,000 pixels before
pixel allocation. WebP animation canvas and frame dimensions are both bounded.
Only generated data URLs cross into the template. All escaped text and template
overhead are reserved before adding images to the 1 MiB HTML budget, in article
order. Images that do not fit retry at half dimensions and lower JPEG quality,
then are omitted if necessary. CSP permits data images and blocks external
browser resources; webcappi does not fetch feed-supplied image URLs.

Input bounds are 4 MiB after HTTP decompression, 64 XML levels and 50,000 XML
elements. At most the first 200 ordered items are normalized. Each HTML field is
parsed from at most 65,536 Unicode code points. Titles are limited
to 1,024 Unicode code points, feed names and sources to 256, categories to 128,
and summaries to 6,000 across at most 32 blocks. HTML is bounded to 1 MiB.
The default display limit is 8 articles, with a configurable maximum of 20.
The feed title uses 16 px medium-weight text; article headlines use 23 px bold
text. Headlines use up to two lines, summaries up to three lines in total, with at
most three bullet items; missing summaries and metadata reserve no space. A fixed
script waits for fonts and embedded image decoding, clamps summary blocks to the
remaining line budget, and removes articles that do not fit, preserving order. It signals completion using
`#rss-ready`. It drops the first article's image, then summary and metadata if
necessary to prioritize its headline. Fonts must already exist in webcappi;
no web fonts or other browser network resources are requested.

## Build and use

Install Go (toolchain pinned in `go.mod`), Make, Docker and Python 3 for container
checks. Python is not part of the runtime. Run from the repository root:

```sh
make -C renderers/rss setup
make -C renderers/rss check
make -C renderers/rss build
renderers/rss/dist/renderer \
  --context renderers/rss/fixtures/render-context-v1.json \
  --input renderers/rss/fixtures/rss2.xml \
  --webcappi-url https://webcappi.example.com \
  --output /path/to/output.png
```

The output parent must already exist and be writable. Replace `--input` with
`--feed-url https://news.example.com/feed.xml` for HTTP(S) input. Exactly one is
required. The webcappi base URL is required; the renderer appends `/capture`
to its path. URLs with embedded credentials are rejected, and the webcappi base
URL cannot contain a query or fragment. Internal feed and webcappi services can
be configured explicitly; access policy for these endpoints belongs to the
operator. Restrict outbound access accordingly.

| Option | Default / behavior |
| --- | --- |
| `--context` | `/arprt/context.json`, read-only Render Context v1 |
| `--output` | `/arprt/output.png` |
| `--feed-url` | HTTP(S) feed, exclusive with `--input` |
| `--input` | Read-only local RSS file, exclusive with `--feed-url` |
| `--webcappi-url` | Required base URL |
| `--webcappi-token-file` | Optional read-only Bearer token file, at most 8 KiB |
| `--profile` | `auto`; also `generic`, `hatena`, `livedoor`, `google-news` |
| `--title` | Optional feed display title override |
| `--max-items` | `8`, accepted range 1–20 |
| `--summary` | `true`; use `--summary=false` to hide summaries |
| `--images` | `true`; use `--images=false` to disable image requests |
| `--public-images-only` | `true`; use `--public-images-only=false` to allow internal image destinations |
| `--timezone` | `Asia/Tokyo`, with timezone data embedded in the binary |

`--profile` refers to feed normalization; webcappi always uses its default
browser profile. Tokens, feed bodies, Context device data and endpoint URLs are
not logged. Tokens must not contain internal whitespace. Authentication is sent
only to webcappi, never the feed. Capture redirects are rejected; feed redirects
are bounded to five HTTP(S) redirects. Successful requests require HTTP 200.

## Contracts and runtime

The protocol baseline is [ARPRT v0.2.2 Render Context v1 and PNG contract](https://github.com/dayflower/arprt/blob/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87/notes/PROTOCOL.md),
with [Job lifecycle and collection](https://github.com/dayflower/arprt/blob/d53e6f6b71bf1e3bb1c7495ae842aff08c4d1a87/notes/DESIGN.md#job-lifecycle-and-collection).
Context requires version, target and device.name; unknown fields are ignored.
Physical output is limited to 800 × 480 and gray4/spectra6 targets. gray16 and
other sizes are rejected. Clockwise rotations 0/90/180/270 are supported;
omission means 0. At 90/270, webcappi receives a logical 480 × 800 viewport;
otherwise 800 × 480. Rotation happens once in this renderer.

The [webcappi API baseline](https://github.com/dayflower/webcappi/blob/d86aa33583120ddd90a3ab05b81d5fb5ff27dbde/notes/API.md)
is version 0.1.7. The renderer uses raw UTF-8 `text/html` POST, `html_mode=file`,
viewport width/height and `ready_selector=#rss-ready`. It omits output resizing,
full-page capture and browser profile selection. Font and readiness checks occur
in webcappi. API source review establishes request compatibility, not an actual
Chromium capture or deployment compatibility test.

The overall deadline is 60 seconds, propagated into feed requests (15 seconds)
and all capture attempts combined (40 seconds). Capture HTTP 503/504 can retry
once. Retry-After integer seconds are capped at two; otherwise the delay is one
second. Capture 4xx and transport errors are not retried. Signal cancellation
aborts HTTP work and prevents output replacement. No retry uses an independent
deadline that can extend the overall budget.

Capture responses must be `image/png`, at most 32 MiB, with valid chunk CRCs,
no animation or trailing data, and exact logical dimensions checked before
pixel allocation. The complete image must decode. Transparency is composited
onto white and the image rotates into an opaque static 8-bit RGB PNG. ARPRT owns
palette conversion, dithering, overlays and the device delivery byte budget.
The renderer does not impose a 90,000-byte device budget on RGB source PNGs;
the controller's separate 32 MiB renderer input ceiling applies.

The runtime is a static Go binary in scratch, with CA certificates, embedded
timezones and licenses. No shell, Chromium, fonts, compiler or package manager
is installed. It supports UID/GID 1000, read-only root, all capabilities dropped,
no privilege escalation and no ServiceAccount token. The only writable mount
is the output parent. Temporary files are unique sibling `.rss-*.tmp` files;
write/close must succeed before rename. Errors and cancellation retain existing
output and remove temporary files. The controller decides prior-output fallback.
No independent writable `/tmp` is required.

When overriding ARPRT Job `contextPath` or `imagePath`, also pass matching CLI
`--context`/`--output`; the controller does not rewrite arguments. Use a dedicated
output parent, since its writable mount hides image contents at that path.

## Offline validation

```sh
make check
make -C renderers/rss image
make -C renderers/rss container-check
```

The checker builds a separate static test binary with the pinned Dockerfile
and mounts it read-only into the production image. Test fixtures are synthetic.
Local HTTP stubs provide RSS and synthetic asymmetric PNGs; tests never contact
external feeds or webcappi. Synthetic cases cover all five feed structures,
namespace prefix changes, metadata omissions, exact suffix/CTA boundaries and
mixed summaries. The real CLI also verifies each profile through capture-request
HTML inspection. With the container network disabled, stubs use only
container loopback. The checker launches the real `/app/renderer`, tests default
and separate custom Context/output paths, both target palettes and all four
rotations, including corner positions, opacity, capture failures and output
preservation. All test temporary files go into the output mount. The shipped
image contains no tests or fixtures. Release guard tests run through the renderer
Makefile on the host. `IMAGE` can select another locally built image for both `image` and `container-check`.

Image tests cover candidates, public-address policy, DNS pinning, private redirects,
size and decompression bounds, unsupported/corrupt images, extreme aspect ratios,
transparency, animation first frames, two-worker concurrency, mixed failures and
HTML budget fallback. An injected client permits local image HTTP stubs without
relaxing the default production policy. The production CLI also confirms that
private images are omitted by default, embedded with destination filtering disabled
(including a redirect), and never requested with `--images=false`. Text and PNG
output succeed in each case.

Automated captures use stub PNGs and do not test Chromium layout. Separately,
manual captures through a real webcappi browser on 2026-10-31 checked synthetic
Japanese headlines, paragraphs, bullets, categories and thumbnails at 800 × 480
and 480 × 800. All-image, mixed-image and no-image lists were visually inspected:
headlines and summaries were clamped, images retained their space and articles
outside the viewport were removed. These six screenshots establish layout for
those synthetic cases, not comprehensive font coverage or acceptance of live
feed content. A subsequent acceptance run captured the five live Google News,
NHK, Hatena Bookmark, Yahoo! News and Livedoor feeds through the real CLI at both
logical viewport sizes. All ten captures succeeded and were visually inspected; no layout
correction was needed. These are point-in-time visual checks. See the
[validation record](VALIDATION.md) for observed feed behavior, software and cluster
verification, and remaining release and physical-display checks. No public image
availability is claimed. See [third-party notices](THIRD_PARTY_NOTICES.md) for
dependency licenses.

## Screen configuration

Build with `make -C renderers/rss image` and load `arprt-renderer-rss:dev` into
your cluster using its local image-loading mechanism. [screen.yaml](screen.yaml)
is a starting example with synthetic feed and webcappi endpoints: replace both
before use. The example uses the local build because no RSS image is published.
The planned registry is `ghcr.io/dayflower/arprt-renderer-rss`; after publication
and anonymous-pull verification, pin its verified multi-platform digest.
A matching 800 × 480 gray4 or spectra6 Device must select this Screen. Rotation
comes from the Device's Render Context, rather than a renderer CLI option.

The example asks ARPRT for `imageProcessing.dithering: none` to favor text.
Palette conversion remains in the controller. Evaluate photos, overlays and
physical readability separately. No overlay is enabled in the example.
Ensure Job egress can reach the configured feed, webcappi and optional image
hosts, including DNS and HTTPS. Operate webcappi separately with Japanese fonts;
this repository does not deploy it. For capture authentication, mount a read-only
token file using your deployment's supported Job configuration and pass
`--webcappi-token-file`; never put token contents in Screen arguments.

The default Context/output paths require no extra arguments. For custom paths,
set `source.job.contextPath` and `source.job.imagePath` and pass the same paths
with `--context` and `--output`. Merely changing the Screen paths does not change
CLI defaults. Input mounts must be read-only and the output parent writable.

## CI and independent releases

[RSS checks](../../.github/workflows/check-rss.yaml) run renderer-scoped shared
Ruff checks, fixed-module setup, Go checks and offline release guard tests.
Separate native Linux amd64 and arm64 runners build the production image,
check OCI version/revision labels and execute the offline container checker.
[Normal CI](../../.github/workflows/ci.yaml) also runs shared checks and Picsum
verification. Configuring both native jobs is not evidence that they have run;
local validation currently covers Linux amd64 only.

VERSION is independently set to `0.1.0`, an unreleased initial version.
[RSS releases](../../.github/workflows/release-rss.yaml) accept annotated,
canonical stable `rss/vMAJOR.MINOR.PATCH` tags matching VERSION on `origin/main`.
They call only RSS verification, transfer the exact tested native image archives
to publication, and refuse existing version/platform tags or GitHub releases.
No `latest` tag is published. The index metadata and release notes record the
immutable reference and source revision. See [release instructions](../../notes/RELEASE.md#publishing-rss)
for setup, partial-publication recovery and public-availability verification.
