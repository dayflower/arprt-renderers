# iCalendar changelog

## 0.3.0 (unreleased)

- Respect ARPRT v0.8.0 logical safe-area insets in month and agenda layouts,
  preserving design padding, full-size PNG output and zero-inset layouts.
- Fit event rows and omission counts to the available content rectangle; reject
  browser layouts where required headers or date rows do not fit.

## 0.2.0

- Always show Today and Tomorrow in agenda view, including empty days, with
  holiday names and monochrome date markings.
- Show holidays without appointments as chronological Upcoming rows. Retain
  holiday names in time metadata on dates with appointments and show additional
  holidays with `+N`.
- Prioritize appointments over holiday-only rows during candidate selection and
  browser fitting, counting omitted holiday rows in `+N more`. Preserve the
  per-day and Upcoming candidate limits and reserve space for holiday headings.

## 0.1.2

- Accept valid date-only DTSTAMP creation metadata, with `VALUE=DATE` or without
  a VALUE parameter, so iCloud Japanese holiday calendars can be rendered.
- Treat date-only stamps like missing stamps during revision selection, retaining
  SEQUENCE, UTC timestamp and input-order precedence. Preserve strict validation
  of event/recurrence dates, invalid or duplicate stamps and TZID-bearing stamps.
- Add synthetic coverage for Japanese holiday names, yearly COUNT rules,
  multi-value EXDATE, gzip fetching and holiday-only/mixed-source rendering.

## 0.1.1

- Ignore vendor `X-` properties on VEVENT, STANDARD and DAYLIGHT so feeds with
  Google Calendar conference/Hangouts links and Apple structured locations can
  be rendered. Extension values are not interpreted, displayed or fetched.
- Preserve input budgets and conservative rejection of unsupported recurrence/date
  semantics and unknown non-`X-` properties. Add synthetic fixtures and regression
  coverage for vendor properties, rendering, rejected semantics and input limits.

## 0.1.0

- Initial independent ARPRT iCalendar Job renderer with month/agenda views,
  recurrence and timezone resolution, file-based configuration and credentials,
  and capture through external webcappi.
