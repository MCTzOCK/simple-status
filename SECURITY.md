# Security Policy

## Supported versions

Only the latest tagged release receives security fixes.

## Reporting a vulnerability

Please do **not** open a public issue for security problems. Instead, use
GitHub's [private vulnerability reporting](../../security/advisories/new)
for this repository. Include a description of the issue, reproduction steps
and affected versions if you can.

You should receive a response within a few days. We will coordinate a fix
and disclosure; credit is given unless you prefer to remain anonymous.

## Scope notes

simple-status is a monitoring tool that is typically deployed on private
networks. Keep in mind:

- The status page and API are unauthenticated by design — restrict access
  at the network level if your monitored service list is sensitive.
- Config files may contain secrets (e.g. `headers.Authorization`); protect
  the file accordingly (`chmod 600`).
- `insecure_tls: true` intentionally disables certificate verification for
  a probe; use it only for internal endpoints.
