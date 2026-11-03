# ARPRT renderers

Renderers provided and maintained by the [ARPRT project](https://github.com/dayflower/arprt),
an independent, unofficial Kubernetes controller for TRMNL BYOS. Neither ARPRT
nor these renderers are affiliated with or endorsed by TRMNL.

## Renderers

| Renderer | Type | Purpose | External services |
| --- | --- | --- | --- |
| [RSS](renderers/rss/README.md) | Job | Display RSS feeds | webcappi for capture |
| [iCalendar](renderers/ical/README.md) | Job | Display month calendars and agendas | webcappi for capture |
| [Picsum](renderers/picsum/README.md) | Job | Display photographs | Lorem Picsum, or none with local images |

See each renderer's README for setup, configuration, images, releases and
validation scope.

## Development and releases

Each renderer owns its implementation, dependencies, image, version and release,
independently of ARPRT and the other renderers.

- [Development guide](notes/DEVELOP.md): local checks, CI and adding a renderer.
- [Release guide](notes/RELEASE.md): image publication and initial setup.
- [MIT license](LICENSE).
