# RSS validation record

## Software and browser acceptance: 2026-10-31

The renderer source was `bef4a34dbc71ecb88e61218d89249ee7dd84c412`.
`make check`, `make -C renderers/rss image` and
`make -C renderers/rss container-check` passed on native Linux amd64.
The checked local production image manifest was
`sha256:ad9a70aa194139f854727abbfe53ddee9c08228de2896fdc1f2b5d444ed5e1be`.
This identifies a local build, not an available registry image or release digest.
No executable sources changed during this acceptance run.

The real CLI fetched each live feed and requested captures from a separately
operated webcappi browser with Japanese fonts. All ten runs succeeded. Each
produced an opaque 800 × 480 physical PNG: rotation 0 used an 800 × 480 logical
viewport, and rotation 90 used a 480 × 800 logical viewport before rotation.
Both orientations were visually inspected with images enabled and the default
`auto` profile, item limit, summaries and timezone.

| Feed | Observed result in both orientations |
| --- | --- |
| Google News | Headline list without related-story summaries; sources displayed separately without redundant exact suffixes. Long headlines stopped after two lines. |
| NHK | Paragraph summaries retained Japanese text and existing ellipses within the three-line budget. Articles ended before the viewport boundary. |
| Hatena Bookmark | Thumbnails, bookmark counts and first categories appeared. Long titles and summaries were clamped and complete articles retained feed order. |
| Yahoo! News | Compact headline/date rows with no reserved summary or image area. The portrait viewport displayed all eight requested items. |
| Livedoor | CTA-only summaries disappeared. Bullet summaries and the small summary label remained, with the total three-line budget taking precedence over showing all three bullets. Thumbnails kept their aspect ratios. |

No layout correction was needed for these captures. Live content is a
point-in-time sample, not exhaustive coverage of every feed variation, image
server or font. Captures and actual news content are kept outside the repository
and image build context. Missing metadata, image failures, mixed image rows,
extreme titles, malformed inputs and generic feeds are additionally covered by
synthetic offline tests. Earlier real-browser checks covered synthetic all-image,
mixed-image and no-image lists at both logical viewport sizes.

## Kubernetes integration: 2026-10-31

A disposable, single-node Kubernetes v1.36.4 kind cluster on native Linux amd64
ran the same RSS production image. A separate ARPRT controller used source
revision `047d9d4be121fadbdf3eb46cc8604a9c14694260` and image manifest
`sha256:cdf21bc4cd9aaf89144845fc69bdce450d1c2dd86837eb23a4ccb0ffd17f7537`,
with controller chart 0.4.0 and CRD chart 0.2.0. This is integration evidence for
that controller build; the renderer protocol baseline remains ARPRT v0.2.2.

Only synthetic device identities and a synthetic RSS feed were used in the
cluster. Feed input came from an in-cluster HTTP service, while captures used a
real, separately operated webcappi browser. The controller created the renderer
Jobs and collected their output from its PVC. A read-only observer inspected
durable state; a synthetic BYOS client retrieved the processed images through
the authenticated display/image API.

| Check | Result |
| --- | --- |
| First render | Display initially returned pending; the RSS Job completed, its Context selected 800 × 480 gray4, and an authenticated client retrieved a valid PNG. |
| Job restrictions | The generated Job used UID/GID 1000, read-only root, all capabilities dropped, no privilege escalation, no ServiceAccount token and exactly one writable output mount. |
| Screen update | Changing the feed display title and using separate custom Context/output paths produced a newer successful artifact and a different delivered PNG digest. |
| Failed render | A malformed RSS response caused the real renderer Job to fail. The latest-success attempt remained unchanged, and the delivered PNG retained the preceding successful digest. |

The test namespace, PVC, controller, synthetic credentials and test-owned CRDs
were removed after the run, and the disposable cluster was deleted. Existing
cluster CRDs, workloads and physical-device configurations were not changed.
This run did not verify production-cluster networking/storage, scheduled refresh
over an extended period, native arm64 scheduling, Spectra 6 delivery, or physical
displays. Offline tests separately cover both palettes and all four rotations.

## Physical-display observation: 2026-10-31

The same checked RSS image was loaded into an existing native Linux amd64
cluster. Dedicated Screens fetched the live Hatena Bookmark feed through the
real webcappi service. Both 800 × 480 Devices retained rotation 0 and their
existing overlay settings and image-processing defaults. No rotation, overlay
or dithering comparison was performed.

Both targets reached `ImageReady`. After updating the physical displays, the
user confirmed that the RSS screen appeared on both devices and reported:

- gray4: text was easy to read.
- Spectra 6: text looked rougher and less clean.

This establishes basic physical RSS display on both targets for that feed and
configuration. It is user-reported visual evidence, not a controlled comparison.
The contribution of palette conversion, dithering, browser antialiasing or the
panel itself was not isolated. The Spectra 6 result is a display-quality
observation, not an attributed renderer defect or a release blocker.
Thumbnail quality, metadata completeness and overlay overlap were not separately
assessed. The other four live feeds and rotated layouts were not checked on
physical devices.

The original Device assignments and settings were restored, the test Screens
were removed, and the temporary GitOps exceptions were reverted and verified.
No real device identities, credentials or private endpoints are recorded here.

## Release readiness

At the time of the acceptance checks above, the initial independent version
was `0.1.0` and unreleased. Publication is recorded separately below. The
implementation and synthetic fixtures use the repository MIT license. The production image
includes that license, Go's license and notices for the fixed Go modules.
The Dockerfile allowlist excludes live captures, news snapshots, credentials and
Screen configurations from the build context.

The RSS release workflow and nine offline release guard tests were reviewed.
The workflow requires a canonical annotated `rss/vMAJOR.MINOR.PATCH` tag matching
VERSION, the exact tagged checkout on `origin/main`, and successful RSS-only
verification. It refuses existing version/platform tags and GitHub releases,
and publishes the exact tested native image archives without rebuilding.
Release metadata records the multi-platform digest and source revision.

Before publishing, require successful native amd64 and arm64 verification of
the intended source revision and confirm repository/package permissions. Check
the actual messages of every commit created or updated for publication and
preserve the required Codex co-author trailer. After publication, verify anonymous
pulls for both platforms and the immutable reference, then update the README
status and pin the verified multi-platform digest in `screen.yaml` as described
in [the release procedure](../../notes/RELEASE.md#publishing-rss).

Native arm64 execution and GitHub Actions execution were not performed in this
local acceptance run. No release tag was created, and no RSS image or release
was published.
Physical observation is limited to the Hatena Bookmark run above: gray4 text
was reported as readable, while Spectra 6 text was reported as rougher.
Photo quality, alternate dithering, overlay placement, other feeds and physical
rotation comparisons remain unverified. Software checks and browser captures
do not expand that physical-display evidence.

## Public RSS 0.1.0 verification: 2026-10-31

[RSS 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/rss/v0.1.0)
was published from source `8c3794fb38abb387fd6d3f4f9009cbd07238e556`.
The [release workflow](https://github.com/dayflower/arprt-renderers/actions/runs/41185957574)
passed source checks and native Linux amd64/arm64 builds, offline unit tests
and production CLI checks under Job security restrictions. Publication loaded
the exact tested image archives without rebuilding.

The verified immutable multi-platform reference is:

```
ghcr.io/dayflower/arprt-renderer-rss@sha256:5a21c46f62cb2c79a39b2e02327ca3954203470048ba0389b6252cfc9fc2c80d
```

An empty Docker configuration successfully pulled both the `0.1.0` tag and
that digest for Linux amd64 and arm64. Anonymous registry requests also verified
both platform tags, index/manifest digests and OCI version/revision labels.
Published image config digests matched the downloaded CI-tested archives:

| Platform | Published and tested config digest |
| --- | --- |
| linux/amd64 | `sha256:87c15eaf723350ba93d07d54039bbd5f23a74c84f46b9d765f2158a749eb79d6` |
| linux/arm64 | `sha256:e59977c253b896dff9a2b99ddabd162275db1dc83fb6f86f1aae61635956af45` |

The public amd64 digest passed `make -C renderers/rss container-check` after
publication. This is separate from native prepublication CI. Postpublication
native arm64 execution was not performed; anonymous arm64 pulls establish
availability, and native arm64 execution was checked before publication.
The earlier browser, cluster and physical-display observations retain their
stated scope and do not constitute new end-to-end tests of the published image.
