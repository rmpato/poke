<div align="center">

<img src="docs/img/wordmark.svg" alt="poke + pogo" width="620">

**Poke your APIs. Pogo through your requests.**

`poke` runs curl and remembers what it ran.
`pogo` is a terminal UI to find, inspect, replay, edit and diff those requests.

[**rmpato.github.io/poke**](https://rmpato.github.io/poke)

[![CI](https://github.com/rmpato/poke/actions/workflows/ci.yml/badge.svg)](https://github.com/rmpato/poke/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rmpato/poke.svg)](https://pkg.go.dev/github.com/rmpato/poke)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go 1.24+](https://img.shields.io/badge/go-1.24+-00ADD8.svg)

</div>

---

Your shell remembers the *command*. It does not remember the response, the
status, how long it took, or which of the six nearly identical `curl` lines in
your history was the one that worked.

`poke` is curl with a memory. Use it exactly like curl:

```bash
poke https://api.example.com/users
```

Same output, same exit code, same flags — it hands your arguments to the real
curl binary. It just also writes down what happened.

Later:

```bash
pogo
```

<img src="docs/img/pogo-list.svg" alt="pogo showing recent requests" width="100%">

That is the whole idea. You never saved anything. It was just there.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/rmpato/poke/main/install.sh | sh
```

Or with Go:

```bash
go install github.com/rmpato/poke/cmd/poke@latest
go install github.com/rmpato/poke/cmd/pogo@latest
```

Or from source:

```bash
git clone https://github.com/rmpato/poke && cd poke && make install
```

Then:

```bash
poke --help
pogo --help
poke --update   # later on
```

You need `curl` on your PATH. That is the only runtime dependency, and it is
already on your machine.

## The workflow

**1. Make a request.** Type `poke` where you would have typed `curl`.

```bash
poke -X POST https://api.example.com/users \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Pato"}'
```

**2. Keep working.** Nothing to save. Nothing to name. Nothing to organize.

**3. Come back when it breaks.**

```bash
pogo
```

**4. Find it.** Press `/`, type `users`. Or `status:5xx`. Or `method:POST host:api`.

**5. Look at it.** Press `⏎` — request, response, headers, body, timings.

**6. Run it again.** Press `r`.

**7. Change it and run that.** Press `e`, edit the curl command, `ctrl+r`.

**8. See what changed.** Press `d` on two responses.

The original request is never modified. Replays and edits become new entries,
so "what did it return an hour ago?" stays answerable.

## poke

> curl, with a memory.

```console
$ poke https://api.example.com/users/42
{
  "id": 42,
  "name": "Pato",
  "active": true
}
```

poke is a wrapper, not a reimplementation. Your argv goes to the real `curl`
untouched, so every flag works, piping works, `-o` works, `-v` works, exit codes
match, and your shell scripts do not notice the difference.

```bash
poke -sSL https://example.com | jq .
poke -X DELETE https://api.example.com/users/41
poke -F upload=@photo.jpg https://api.example.com/files
poke --poke-no-capture https://api.example.com/secret   # run without recording
```

Capture rides along on side channels that do not disturb curl's behavior —
`-D` for response headers, a tee for the body, `--write-out` for timings. Two
things curl does differently when its output is a pipe (the progress meter and
the refusal to dump binary data into your terminal) are deliberately restored,
so wrapping is invisible. See [docs/architecture.md](docs/architecture.md).

## pogo

> the last hundred requests you made, made useful.

```console
$ pogo
```

<img src="docs/img/pogo-inspect.svg" alt="pogo inspecting a request and response" width="100%">

- **History** — every request, newest first, with status, duration, size and age.
- **Search** — `/` for free text, or `method:POST`, `status:4xx`, `host:`, `is:starred`, `is:failed`.
- **Inspect** — request, response, headers, query, bodies, and a JSON tree you can fold.
- **Replay** — `r`. One key. New entry.
- **Edit and replay** — `e`, change anything, `ctrl+r`.
- **Diff** — `d` on two requests, JSON-aware.
- **Copy as curl** — `y` then `c`, or `C` for a version with the secrets masked.
- **Star** — `s` keeps the ones that matter.
- **Group by host** — `t`, when history gets noisy.

<img src="docs/img/pogo-search.svg" alt="pogo filtering by status:4xx" width="100%">

Everything is a keystroke, the footer always shows what applies, and `?` lists
the rest.

### Timing

Where curl reports it, pogo shows where the time actually went:

<img src="docs/img/pogo-timing.svg" alt="pogo showing a timing breakdown" width="100%">

Nothing here is estimated. If your curl is too old to report timings, pogo says
so instead of inventing numbers.

### Diff

Two responses from the same endpoint, ten minutes apart:

<img src="docs/img/pogo-diff.svg" alt="pogo diffing two responses" width="100%">

The comparison is JSON-aware: reordered keys and reformatted whitespace are not
differences, changed values are.

## Why not just shell history?

```bash
history | grep curl
```

finds the command you typed. It does not tell you:

| | shell history | pogo |
|---|---|---|
| the command | ✅ | ✅ |
| what came back | ❌ | ✅ |
| status code | ❌ | ✅ |
| how long it took | ❌ | ✅ |
| where the time went | ❌ | ✅ |
| searching by status or host | ❌ | ✅ |
| replay | retype it | `r` |
| edit and replay | retype it | `e` |
| compare two responses | ❌ | `d` |

pogo is HTTP-aware history for your terminal. That is all it is trying to be.

## Storage and security

History lives in `~/.local/share/poke` (or `$XDG_DATA_HOME/poke`, or
`$POKE_HOME`), as append-only JSONL plus payload files. Directory `0700`, files
`0600`, nothing encrypted, nothing sent anywhere.

**Request headers routinely carry credentials, and by default poke stores them
as sent.** Your history file will contain bearer tokens, cookies and API keys in
plain text. That is a trade-off, not an accident: stripping them would make
replay silently fail to authenticate. pogo masks secrets on screen, but the file
on disk holds the real thing.

If you want the other trade-off:

```bash
POKE_REDACT=store poke -H "Authorization: Bearer $TOKEN" https://api.example.com/me
```

Secrets are then removed before anything is written, the entry is marked
redacted, and pogo tells you that replaying it will not authenticate.

Read [docs/security.md](docs/security.md) before pointing this at production
credentials. It is short and it does not hedge.

## Architecture

```
   poke <curl args> ──▶ curlargs ──▶ runner ──▶ the real curl
                        (metadata)   (execute + capture)
                                        │
                                     capture ──▶ store  (JSONL + blobs, local)
                                        ▲          │
                                        │          ▼
                          pogo replays ─┘         tui
```

Both binaries share one execution path: a replay is not a reconstruction of
your request, it is the same code running the same argv. Storage knows nothing
about the UI, the UI never touches the filesystem, and the curl layer is
testable without a network.

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), Bubbles. No database, no
cgo, no daemon, no account. Details in
[docs/architecture.md](docs/architecture.md).

## Development

```bash
make          # build both binaries into ./bin
make check    # gofmt, vet, test, race — the same gates CI runs
make help     # everything else
```

Tests run the real curl against local `httptest` servers; nothing touches the
public internet. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Roadmap

Ideas, not promises. Roughly in the order they would be useful:

- a structured editor for URL, headers and body, alongside the curl buffer
- collections and environments, if they can stay out of the way
- variables (`{{token}}`) resolved at replay time
- import from a `.har` file or a Postman collection, export back out
- richer diffing: headers and status, not only bodies
- per-host redaction rules
- a `--json` output mode for scripting against your own history

If you want something on this list, an issue that describes the moment you
wanted it is worth more than a feature request.

## License

MIT — see [LICENSE](LICENSE).
