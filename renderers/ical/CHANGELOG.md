# iCalendar changelog

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
