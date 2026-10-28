# Renderer releases

[Picsum 0.1.0](https://github.com/dayflower/arprt-renderers/releases/tag/picsum/v0.1.0) is published and anonymously pullable for Linux
amd64 and arm64. Its release notes record the immutable image reference,
source revision and validation scope. The following procedure applies to
subsequent releases; choose a version that has not been published before.

## Initial setup

1. Enable Actions for `dayflower/arprt-renderers` and allow the workflow's
   `GITHUB_TOKEN` to write packages and releases.
2. Require CI on main and restrict creation/update/deletion of `picsum/v*` tags to
   maintainers. Do not force-move release tags.
3. Ensure an existing `arprt-renderer-picsum` GHCR package, if any, is connected to
   this repository and grants its Actions read/write access. The OCI source label
   connects new packages. A 404 can mean hidden resources when package permissions
   are incorrect, so check package ownership/access before the first release.
4. Confirm both native CI runners are available (`ubuntu-24.04` and
   `ubuntu-24.04-arm`). No private registry or photo-service secret is required.

## Publishing Picsum

Set [VERSION](../renderers/picsum/VERSION) and update the examples/documentation to
the intended version in a reviewed commit on main. Keep versions independent of
ARPRT and other renderers. Before pushing any Codex-contributed commits, inspect
their messages and preserve the required co-author attribution.

Run the checks in [DEVELOP](DEVELOP.md), then create an annotated tag
on that main commit (this example assumes VERSION was updated to 0.1.1):

```sh
picsum_version=0.1.1
git tag -a "picsum/v${picsum_version}" -m "Picsum ${picsum_version}"
git push origin "refs/tags/picsum/v${picsum_version}"
```

The [Picsum workflow](../.github/workflows/release-picsum.yaml) accepts only canonical
stable `picsum/vMAJOR.MINOR.PATCH` tags matching VERSION. It requires an annotated
tag whose commit is on origin/main and tests that exact revision. It then:

1. Calls the reusable [Picsum checks](../.github/workflows/check-picsum.yaml)
   directly to run Picsum-scoped lint/format and offline tests, build/check native
   Linux amd64/arm64 containers, and export the exact tested images. It does not
   run another renderer's setup or tests. Picsum's release helpers and tests live
   under `renderers/picsum/`.
2. Serializes Picsum releases and checks all GHCR package-version pages for the
   version and both architecture tags. It also rejects an existing GitHub release.
   Explicit 404 is accepted only for initial missing resources; auth, network,
   rate-limit and server failures stop publication.
3. Publishes `ghcr.io/dayflower/arprt-renderer-picsum:<version>-amd64` and
   `:<version>-arm64`, then assembles `:<version>` as a multi-platform index.
   There is no mutable `latest` tag. OCI image/index metadata records version,
   source repository and exact source revision; images include the MIT license.
4. Creates a renderer-specific GitHub release, attaches index digest/platform
   metadata and records the immutable image reference. This is workflow evidence,
   not a claim of cluster, live-service or physical-device acceptance.

Workflow guards prevent its own reruns from replacing existing version/platform
tags. GHCR tags are not intrinsically immutable; limit package write permissions
and use digest references for deployments. Uncoordinated external pushes are
outside the workflow's serialization. Never delete existing tags to bypass a
guard. If publication fails after any platform tag was pushed, choose a new
version and tag after investigating; a rerun intentionally stops. If only release
creation failed after the complete image was published, inspect the recorded
digest and manually finish the release without rebuilding or repushing images.

## Public availability

Confirm that the GHCR package is Public; a new package may require changing
its visibility in package settings. Verify pulls without credentials using
a fresh empty Docker config and the same `picsum_version` selected above:

```sh
mkdir -p dist/anonymous-docker
DOCKER_CONFIG="$PWD/dist/anonymous-docker" docker pull --platform linux/amd64 "ghcr.io/dayflower/arprt-renderer-picsum:${picsum_version}"
DOCKER_CONFIG="$PWD/dist/anonymous-docker" docker pull --platform linux/arm64 "ghcr.io/dayflower/arprt-renderer-picsum:${picsum_version}"
```

Also pull the release's `ghcr.io/dayflower/arprt-renderer-picsum@sha256:...`
reference and run
`make -C renderers/picsum container-check IMAGE=<digest-reference>` on each native
architecture where a native host is available. Record the actual verification
scope in the release, distinguishing it from the native prepublication CI
checks. Update the READMEs and pin the verified digest in the Screen examples
after public availability is confirmed. If release immutability is enabled,
assets cannot be added after publication; record later verification in the
editable release notes. No Helm publication is needed for the Job renderer.
