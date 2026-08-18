# Security policy

## Reporting a vulnerability

Please report security issues privately through
[GitHub's advisory form](https://github.com/rmpato/poke/security/advisories/new)
rather than a public issue.

Include what you did, what happened, and what you expected. A proof of concept
helps. You will get an acknowledgement within a few days.

## Scope

poke stores your HTTP traffic locally, on purpose. The following are **expected
behavior**, documented in [docs/security.md](docs/security.md), not
vulnerabilities:

- Bearer tokens, cookies and API keys appear in `history.jsonl` in plain text
  under the default `display` redaction mode.
- Anything running as your user can read that file.

Things that **are** in scope:

- A secret that survives `redact.mode: "store"` and reaches disk anyway.
- History files or directories created with permissions wider than `0700`/`0600`.
- A crafted history file, blob reference or release archive that causes reads or
  writes outside the poke data directory.
- poke altering the request curl sends, or leaking captured data anywhere off
  the machine.
- The self-updater installing bytes whose checksum does not match the published
  `checksums.txt`.

## Network activity

poke makes exactly two kinds of request it was not asked to make, both to
GitHub, both about releases and nothing else:

- **A release check**, at most once every 24 hours, only when attached to an
  interactive terminal. It sends no data beyond an ordinary HTTPS GET for
  `/repos/rmpato/poke/releases/latest`, and it never installs anything — it
  prints one line telling you a release exists. Disable it with
  `POKE_NO_UPDATE_CHECK=1` or `{"update": {"disabled": true}}`.
- **An update**, only after you run `--update` or confirm the prompt in pogo.

No telemetry. Your requests, history and headers never leave the machine. No
account, no cloud.
