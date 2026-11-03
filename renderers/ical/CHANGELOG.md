# iCalendar changelog

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
