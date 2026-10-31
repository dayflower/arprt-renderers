# RSS renderer

A Go CLI that reads one RSS feed, generates its own HTML, requests a PNG from
an external webcappi service, and atomically writes an ARPRT Job output file.
It displays headlines, optional summaries, categories, sources, bookmark counts
and feed-supplied article dates.
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
  retained as a candidate only.
- Livedoor removes a final standalone anchor whose text is exactly `記事を読む`
  after whitespace normalization. The same text within prose or before further
  content stays intact. Lists retain their item boundaries; `ざっくり言うと`
  becomes a small label when it accompanies a list, without repeating it in the
  summary. Other summary text is retained.
- Google News hides description and content:encoded related stories, displays
  source separately, and strips only an exact trailing ` - {source}` from the
  title. Other hyphens and unmatched source suffixes remain. Missing source
  retains the original title and uses the normal feed-title fallback.

Images are never downloaded or embedded. No thumbnails, custom templates,
browser installation or runtime dependency installation are provided.

Input bounds are 4 MiB after HTTP decompression, 64 XML levels and 50,000 XML
elements. At most the first 200 ordered items are normalized. Each HTML field is
parsed from at most 65,536 Unicode code points. Titles are limited
to 1,024 Unicode code points, feed names and sources to 256, categories to 128,
and summaries to 6,000 across at most 32 blocks. HTML is bounded to 1 MiB.
The default display limit is 8 articles, with a configurable maximum of 20.
Headlines use up to two lines, summaries up to three lines in total, with at
most three bullet items; missing summaries and metadata reserve no space. A fixed
script waits for fonts, clamps summary blocks to the remaining line budget, and
removes articles that do not fit, preserving order. It signals completion using
`#rss-ready`. It drops the first article's summary and metadata if
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
image contains no tests or fixtures. `IMAGE` can select another locally built
image for both `image` and `container-check`.

The tests verify software contracts using stub PNGs. They do not render Japanese
text with Chromium or verify actual font selection, line layout, clipping or
readability. Native Linux amd64 is the local container validation target;
arm64 execution, live feeds, a real webcappi browser, Kubernetes Jobs/PVC
collection and physical displays remain unverified. No public image availability
or end-to-end controller acceptance is claimed. See
[third-party notices](THIRD_PARTY_NOTICES.md) for dependency licenses.
