# ARPRT renderers

Renderers provided and maintained by the [ARPRT project](https://github.com/dayflower/arprt),
an independent, unofficial Kubernetes controller for TRMNL BYOS. Neither ARPRT
nor these renderers are affiliated with or endorsed by TRMNL. "Project-provided"
means maintained by ARPRT, not approved by TRMNL.

Sources share this repository; each renderer owns its implementation, dependencies,
examples, image, version and release. Renderer versions are independent of ARPRT
and of one another. No shared SDK is required.

| Renderer | Execution | Image | Release tags | Status |
| --- | --- | --- | --- | --- |
| [Picsum](renderers/picsum/README.md) | Kubernetes Job CLI | `ghcr.io/dayflower/arprt-renderer-picsum` | `picsum/vMAJOR.MINOR.PATCH` | Released [0.2.0](https://github.com/dayflower/arprt-renderers/releases/tag/picsum/v0.2.0) |

Picsum 0.2.0 is available for Linux amd64 and arm64 as
`ghcr.io/dayflower/arprt-renderer-picsum:0.2.0`. The Screen examples pin its
multi-platform image digest; release notes record the source revision and
validation scope.

Picsum 0.2.0 uses a static Go binary in a minimal runtime image. See the renderer
README and [Picsum changelog](renderers/picsum/CHANGELOG.md) for migration details
and local builds.

```sh
make setup
make check
make -C renderers/picsum image
make -C renderers/picsum container-check
```

See [Picsum usage](renderers/picsum/README.md) for local/offline rendering and Screen usage;
[DEVELOP](notes/DEVELOP.md) for development and CI;
and [RELEASE](notes/RELEASE.md) for independent image publication and its initial setup.
The source and fixtures are self-contained: tests never need an ARPRT checkout
or a changing upstream branch. MIT licensing and original copyright are preserved
in [LICENSE](LICENSE).

Picsum needs no Helm chart: ARPRT creates its Jobs from an `ArprtScreen`. If an
HTTP renderer is added later, a small Deployment/Service/configuration chart can
be installed separately from the controller Helm release and individually from
other renderers. Chart and image versions may differ. Do not bundle all renderers
into the controller chart. Consider splitting a renderer into its own repository
when maintainers, permissions, licensing or usage become independent.
