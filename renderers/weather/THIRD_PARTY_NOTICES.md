# Third-party notices

The renderer uses the Go standard library and embedded Go timezone data; it has
no external Go modules. The Go license is copied from the pinned build image into
the runtime. ARPRT renderer code is licensed under the repository's MIT license.

Weather data are provided by [Open-Meteo](https://open-meteo.com/) under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). The generated dashboard
includes attribution and a link to Open-Meteo. API access is subject to the
provider's [terms](https://open-meteo.com/en/terms); the public endpoint is intended
for non-commercial use. Commercial customer endpoint/key configuration is not
implemented in this initial version.

Weather icons are selected SVGs from [Lucide](https://lucide.dev/), vendored at
commit `ee8e10d746a10907205366cfd984922f0c58939a`. Original SVG files are unchanged;
the renderer embeds their drawing elements into its accessible, monochrome
wrapper. See [source and checksums](icons/lucide/ORIGIN.md) and the complete
[upstream license](icons/lucide/LICENSE), including ISC and Feather notices. The
license is also copied into the runtime image as `Lucide-LICENSE`.

Wind arrows and calm symbols remain renderer-authored SVGs under the repository's
MIT license. No icon package, web font or image service is used at runtime.
webcappi is separately operated and is not bundled.
