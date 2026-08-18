# Contributing

<sub>[Docs](docs/) · [Architecture](docs/architecture.md) · [Runbooks](docs/runbooks/)</sub>

Bug reports, patches and "this felt wrong to use" notes are all welcome.

## Getting set up

```bash
git clone https://github.com/rmpato/poke && cd poke
make          # builds ./bin/poke and ./bin/pogo
make check    # gofmt, go vet, go test, go test -race
```

You need Go 1.24+ and curl. Tests skip themselves if curl is missing, and they
never touch the public internet — HTTP tests run against local `httptest`
servers.

Try your build without disturbing your real history:

```bash
POKE_HOME=/tmp/poke-dev ./bin/poke https://api.github.com/zen
POKE_HOME=/tmp/poke-dev ./bin/pogo
```

## The two rules

Almost everything else is negotiable. These are not:

**1. poke must not change what curl does.** The user's arguments are passed
through verbatim. stdout and stderr carry exactly what curl produced. Exit codes
match. If capturing something would require altering the request, the capture
loses. Where poke deliberately emulates curl behavior that a pipe would
otherwise break, it is documented in
[docs/architecture.md](docs/architecture.md) — add to that list rather than
adding an undocumented divergence.

**2. History is append-only.** Replaying or editing a request creates a new
entry. Nothing rewrites a capture. Deletes are tombstones. This is what makes
"what did that endpoint return this morning?" answerable.

## Things worth knowing

- **`internal/curlargs` is metadata only.** It exists so pogo can show
  `POST /users` instead of raw tokens. If it misparses something, a history
  record looks wrong — a request never runs wrong. Keep it that way: anything
  unrecognised goes into `Unrecognized` so the UI can admit it.
- **The curl option table is generated.** Run `scripts/gen-curl-options.sh`
  against a curl binary and regenerate rather than hand-editing
  `internal/curlargs/options.go`. `TestOptionTableMatchesLocalCurl` catches
  drift.
- **`internal/store` knows nothing about the UI, and `internal/tui` never
  touches the filesystem.** Keep that boundary.
- **Adding a stored field?** Update [docs/security.md](docs/security.md) if it
  could contain anything sensitive, and make sure redaction covers it. There is
  a test for exactly that class of miss
  (`TestHeaderOptionValuesAreRedacted`) — it exists because one slipped through.

## Tests

Meaningful tests, please, over coverage numbers. The suite already covers
parsing, storage round-trips and folding, redaction, replay, and TUI state
transitions including rendered frame geometry. New behavior should be testable
the same way.

For TUI work, `internal/tui` tests drive `Update` with real messages and assert
on the rendered output, so you can test interaction without a terminal.

## Runbooks

Procedures for the recurring jobs — releasing, regenerating the screenshots,
refreshing the curl option table, triaging a capture bug, recovering a damaged
history file, responding to a security report — live in
[docs/runbooks](docs/runbooks/). Follow them rather than reinventing the steps.

## Commits and pull requests

- Conventional-ish commit subjects (`feat:`, `fix:`, `docs:`) — the changelog is
  generated from them.
- One idea per pull request.
- `make check` before pushing; CI runs the same thing on Linux and macOS.
- The PR template has a short checklist for changes that touch curl execution or
  what gets stored. It is there because those are the two places a mistake is
  expensive.
