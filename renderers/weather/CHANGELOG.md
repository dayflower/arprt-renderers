# Weather changelog

## 0.5.0 - 2026-11-12

- Start the eight three-hour forecast slots at the slot containing the current
  time in the forecast location timezone, continuing into tomorrow.
- Preserve unavailable slots when next-day forecast data are missing.

## 0.4.0 - 2026-11-09

- Show the Tomorrow date before its heading, for example `11/10 (Tue) Tomorrow`.
- Add Celsius/Fahrenheit units to Today/Tomorrow high and low values and each
  hourly temperature, preserving missing values without a unit.
- Adjust summary temperature sizing, spacing and wrapping to fit both
  orientations, including three-digit Fahrenheit values.

## 0.3.0 - 2026-11-08

- Respect ARPRT v0.8.0 logical safe-area insets for current conditions, forecasts
  and attribution, retaining design padding and full-size PNG output.
- Remove the summary's decorative minimum height when insets are present,
  preserving content and font sizes; reject browser overflow before readiness.
- Preserve the previous layout when safe-area insets are absent or zero.

## 0.2.0 - 2026-11-06

- Add optional `--location-name` below the Now time and remove the visible
  timezone identifier; forecast-local time calculations are unchanged.
- Add automatic palette-aware accents: Spectra 6 uses red daily highs and
  sun icons, blue daily lows, precipitation and rain icons, yellow thunderstorm
  icons and light-blue snow/hail icons. Partly cloudy icons remain black.
  Gray4 retains monochrome output.
- Document latitude, longitude and location-name settings from a Kubernetes
  Secret using standard environment references and argument expansion.

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
