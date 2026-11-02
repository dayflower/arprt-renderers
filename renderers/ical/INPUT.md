# Input, errors and resource contract

## Configuration

Configuration is UTF-8 JSON following [config.schema.json](config.schema.json).
Unknown keys, duplicate keys at every depth, nulls, trailing JSON, unknown
versions and invalid values fail before any HTTP request. Defaults apply only
to absent keys. The title defaults to `カレンダー`, view to `month`, timezone to
`Asia/Tokyo`, week start to `sunday`; absent source arrays mean empty arrays.

There must be 1–10 sources across both arrays, with globally unique IDs. Source
order is regular calendars followed by holiday calendars, each in configuration
order. A holiday-only configuration is valid. Names and title cannot consist
only of whitespace. Identifiers use lowercase ASCII as in the schema so they
are safe diagnostic labels. Strings must be valid UTF-8, without NUL/control
characters. Lengths in the schema count Unicode code points; byte limits below
also apply. `timezone` must load from embedded IANA data; `Local` and empty names
are forbidden. No host-local timezone fallback is allowed.

File paths are clean absolute POSIX paths, at most 512 bytes, without NUL, `.`
or `..` components. Follow symlinks, including Kubernetes projected-volume
symlinks, but require regular files; FIFOs and devices are rejected without
waiting for a producer. Read each file once into bounded memory before starting
HTTP; do not reopen during retries. This is an execution snapshot, not a transaction across
files. Use versioned ConfigMaps/Secrets and change Screen references when a
consistent multi-file update is required.

Exactly one of `url` and `urlFile` is required per source. A URL file contains
one UTF-8 URL and optionally one terminal LF or CRLF, with no surrounding spaces.
Normalize `webcal` to HTTPS. Accept only absolute HTTP/HTTPS URLs with a nonempty
host and valid port; reject userinfo, fragments (including an empty `#`), control
characters and whitespace. Preserve path/query subscription values. Authentication
requires HTTPS. At each of at most five redirects validate again; HTTPS downgrade
is forbidden, and authenticated requests cannot change origin (scheme, normalized
host, effective port). No cookie jar is shared across sources. Only HTTP 200 and
a parseable calendar succeed. Do not follow event links, attachments or images.
Explicitly configured internal services are allowed; network policy is operational.

## Credentials

Omitted `auth` means no source authentication. `auth.type` and
`auth.credentialsFile` must both exist when `auth` exists. Source authentication
is never sent to another source or webcappi. Capture authentication is independent.

Basic files are strict UTF-8 JSON with exactly two required keys, `username` and
`password`, both nonempty strings. Duplicate/unknown keys, null, trailing JSON,
ASCII controls U+0000–001F and U+007F are rejected. A colon in the username is
rejected; a colon in the password is allowed. Spaces and non-ASCII characters
are preserved, without normalization or trimming. Encode the UTF-8 bytes of
`username:password` with standard Base64. Do not guess a legacy character encoding.

Bearer files contain the RFC 6750 token alphabet `[A-Za-z0-9._~+/-]+={0,}` and
optionally one terminal LF or CRLF. Empty tokens, spaces, embedded newlines,
extra terminal newlines and other characters are rejected. URL files and bearer
files use the same single-terminator rule. The input reader enforces these rules
before any request. The standalone semantics probe does not read credentials or perform HTTP.

## Deadlines and bounds

All limits are fixed, not user-configurable. Exceeding a semantic bound is a
whole-render failure, never a truncated success. Display omission is separate.

| Resource | Limit |
| --- | --- |
| Config JSON | 64 KiB |
| URL bytes / URL file | 8,192 / 8,194 bytes including optional CRLF |
| Each credential file | 8 KiB |
| Combined sources / concurrent fetches | 10 / 3 |
| Decompressed body per source | 4 MiB |
| One fetch / whole fetch stage | 15 seconds / 20 seconds |
| Whole execution | 60 seconds, starting before file reads |
| Combined parse/normalize stage | 5 seconds, bounded by whole deadline |
| Capture including retry/delay | 40 seconds, bounded by whole deadline |
| Unfolded content line / nesting | 16 KiB / 16 components |
| Components per source (including root/alarms/zones) | 10,000 |
| Properties per source / parameters per property | 100,000 / 64 |
| UID / SUMMARY / other retained TEXT | 1,024 / 1,024 / 4,096 code points |
| RRULE COUNT / INTERVAL | 999,999,999 |
| Recurrence candidates visited / normalized instances | 20,000 per source / 20,000 combined |
| RRULE BY-list entries | 366 per part, reject duplicates |
| RDATE/EXDATE values | 20,000 combined per source |
| Visible window | At most 42 civil days (up to 43 elapsed days at DST) |
| Range shift + duration lookaround | At most 366 elapsed days |
| Private worker IPC result | 8 MiB |
| Generated UTF-8 HTML | 1 MiB |
| Capture PNG | 32 MiB, exact logical dimensions before allocation |

These are application bounds, not a claim of hard OS memory isolation. Input
loading enforces file/string/source limits. Fetching bounds decompressed bodies,
parallelism and deadlines. The semantic worker checks component/property/parameter,
retained text, date-list and rule budgets before revision selection. Every rule,
including unreferenced VTIMEZONE rules, is preflighted before any timezone
compilation. Typed accessors validate dates, spans, recurrence sets and zones,
including discarded revisions and out-of-window events. Unknown event semantics
are fatal; ordinary standard non-display fields and VTODO are not fetched or
rendered. Unknown VEVENT properties are conservatively rejected as
`unsupported_semantics`, including vendor extensions whose meaning is unknown.

The supervisor owns the deadline across **all sources**; no source receives a
fresh five seconds. Old COUNT rules and impossible/sparse rules may compute
before yielding; result count alone is insufficient. Synchronous parsing, zone
compilation and expansion run only inside a killable worker. The supervisor
kills and reaps it on timeout or cancellation, consumes bounded IPC completely,
and discards all results on failure. Day conversion checks the same stage
deadline before returning a model. HTML/capture/output limits apply when those
components are connected.

Fetch has no automatic retries. Capture may retry 503/504 once, within its shared
deadline, with integer Retry-After capped at two seconds (otherwise one second).
No capture redirects, 4xx retry or transport-error retry. No runtime dependency
installation, persistent cache, renderer state or writable scratch outside the
output parent. Future PNG replacement uses a sibling temporary file and happens
only after complete capture/PNG validation, a successful close and cancellation
check; failure retains the existing output and cleans temporary files.

## Errors

Public diagnostics contain only a validated source ID when applicable, a closed
stage/class and optionally HTTP status. Never wrap or print dependency errors,
URLs, file paths, credentials, bodies, UID or event text. Do not let panic stacks
or child stderr escape the supervisor. Source indices can identify invalid IDs
before validation. Do not return partial models on any error.

| Stage | Classes |
| --- | --- |
| input | `invalid_config`, `input_read`, `invalid_url`, `invalid_credentials` |
| fetch | `transport`, `http_status`, `redirect_policy`, `resource_limit`, `deadline` |
| semantics | `invalid_calendar`, `unsupported_semantics`, `resource_limit`, `deadline`, `semantic_worker` |
| capture | `transport`, `http_status`, `invalid_png`, `resource_limit`, `deadline` |
| output | `output_write`, `cancelled` |

Input/fetch orchestration and the semantic worker expose only these closed
classes, validated source IDs and HTTP status. Unknown recurrence/date semantics
use `unsupported_semantics`; validation failures use `invalid_calendar` and
explicit budget failures use `resource_limit`. All are fatal; an empty valid
calendar succeeds. Parser recovery is never accepted as complete success.
