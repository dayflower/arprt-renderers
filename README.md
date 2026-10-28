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
| [Picsum](renderers/picsum/README.md) | Kubernetes Job CLI | `ghcr.io/dayflower/arprt-renderer-picsum` | `picsum/vMAJOR.MINOR.PATCH` | Initial candidate 0.1.0; not yet published |

The checked-in Screen references describe the intended first image, which is
**not available yet**. Build/push your own image and replace the reference until
the public release and anonymous pull checks have completed.

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
