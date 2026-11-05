# Weather changelog

## 0.1.0 - 2026-11-06

Initial independent Weather renderer release for ARPRT.

- Render Open-Meteo current temperature, Today/Tomorrow summaries, eight
  three-hour forecast slots and seven daily forecasts with precipitation chance.
- Accept latitude/longitude, an optional model and Celsius/Fahrenheit.
- Provide English landscape/portrait dashboards with pinned Lucide artwork,
  wind-direction arrows and a mirrored Tomorrow summary.
- Preserve Render Context v1, clockwise rotations, strict PNG checks and atomic
  output replacement under Job security restrictions.
- Publish static scratch images for native Linux amd64 and arm64, verified
  offline before publication. See VALIDATION.md for acceptance scope.
