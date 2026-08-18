# Security policy

## Reporting a vulnerability

Report privately through
[GitHub's advisory form](https://github.com/rmpato/poke/security/advisories/new)
rather than a public issue.

Include what you did, what happened, and what you expected. A proof of concept
helps. You will get an acknowledgement within a few days.

## What is, and is not, a vulnerability

poke stores your HTTP traffic locally **on purpose**, and by default it stores
request headers as sent — including bearer tokens and cookies. That behaviour is
documented in detail in [docs/security.md](docs/security.md), along with the
three ways to change it. A report that history contains a token is not a
vulnerability; it is the documented trade-off. It is still worth telling us if
the documentation failed to reach you.

In scope:

- a secret that survives `redact.mode: "store"` and reaches disk anyway
- files or directories created wider than `0700`/`0600`
- a crafted history file, blob reference or release archive that causes reads or
  writes outside the poke data directory
- poke altering the request curl sends, or captured data leaving the machine
- the updater installing bytes whose checksum does not match the published
  `checksums.txt`

## Network activity

poke makes no network requests of its own except a once-daily check for a newer
release, and an update you explicitly ask for. No telemetry; your requests,
history and headers never leave the machine. The details, and the single
environment variable that switches the check off, are in
[docs/security.md](docs/security.md#the-one-thing-poke-does-over-the-network-by-itself).

Handling a report: [docs/runbooks/security-response.md](docs/runbooks/security-response.md).
