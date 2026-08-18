<div align="center">

<img src="docs/img/wordmark.svg" alt="poke + pogo — curl, but it remembers" width="620">

**Poke your APIs. Pogo through your requests.**

[Website](https://rmpato.github.io/poke) · [Docs](docs/) · [Keys](docs/keybindings.md) · [Security](docs/security.md)

[![CI](https://github.com/rmpato/poke/actions/workflows/ci.yml/badge.svg)](https://github.com/rmpato/poke/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rmpato/poke.svg)](https://pkg.go.dev/github.com/rmpato/poke/cmd/poke)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go 1.24+](https://img.shields.io/badge/go-1.24+-00ADD8.svg)

</div>

---

**`poke`** is curl with a memory. Type it where you would have typed curl — same
flags, same output, same exit code, because it hands your arguments to the real
curl binary. It also writes down what happened.

**`pogo`** is a terminal UI over everything poke has run: find a request,
inspect it, replay it, change it and run it again, diff two responses.

Your shell remembers the *command*. It does not remember the response, the
status, how long it took, or which of the six nearly identical `curl` lines in
your history was the one that worked.

## Quickstart

```bash
# 1. install (needs curl, which you already have)
curl -fsSL https://raw.githubusercontent.com/rmpato/poke/main/install.sh | sh

# 2. make a request, exactly as curl would
poke https://api.github.com/zen

# 3. open everything you have run
pogo
```

<img src="docs/img/pogo-list.svg" alt="pogo showing recent requests with method, host, path, status, duration, size and age" width="100%">

Then: `↑↓` to move, `⏎` to inspect, `r` to replay, `e` to edit and run, `/` to
search, `d` to diff two responses, `?` for everything else.

You never saved anything. You never named a collection. It was just there.

<img src="docs/img/loop.svg" alt="poke runs and records; pogo browses, replays and edits; the edited request runs again" width="100%">

<details>
<summary>Other ways to install</summary>

```bash
go install github.com/rmpato/poke/cmd/poke@latest
go install github.com/rmpato/poke/cmd/pogo@latest
```

```bash
git clone https://github.com/rmpato/poke && cd poke && make install
```

Update later with `poke --update`. Both binaries work on macOS and Linux, and
`curl` is the only runtime dependency.

</details>

## What you get

### Find it

`/` for free text, or `method:POST`, `status:4xx`, `host:api.example.com`,
`collection:auth`, `is:starred`, `is:failed`. `t` cycles grouping —
chronological, by host, by collection.

<img src="docs/img/pogo-search.svg" alt="pogo filtering history with status:4xx" width="100%">

### Read it

Request and response, headers, query parameters, bodies, with a JSON tree you
can fold and secrets masked until you ask (`S`).

<img src="docs/img/pogo-inspect.svg" alt="pogo inspecting a request and response, with a masked bearer token and a foldable JSON tree" width="100%">

### Change it and run it

`e` opens the request as fields. `ctrl+r` runs it as a **new** entry; the
original is never touched.

<img src="docs/img/pogo-edit.svg" alt="pogo editing a request as structured fields" width="100%">

Edits are applied to your original command rather than regenerating one, so a
request carrying `--cacert`, `--resolve` or `-k` keeps every one of those
options when you change a header.

### See what changed

`d` on two responses. JSON-aware — reordered keys and reformatted whitespace are
not differences, changed values are — and response headers are diffed too.

<img src="docs/img/pogo-diff.svg" alt="pogo comparing two responses from the same endpoint, showing one changed field" width="100%">

### See where the time went

Straight from curl's own instrumentation. Nothing is estimated; if your curl
cannot report it, pogo says so.

<img src="docs/img/pogo-timing.svg" alt="pogo showing DNS, connect, wait and download phases" width="100%">

### Keep secrets out of your history

Write the request with variables and it runs against real values while your
history stores the braces:

```bash
poke -H "Authorization: Bearer {{token}}" '{{base}}/users/42'
```

curl gets the token. `history.jsonl` gets `{{token}}`. A replay months later
resolves it again — against whatever the variable holds *then*. `E` switches
environment, so you can replay the same request against staging and production
and `d` the results.

→ [Environments, variables, collections and HAR import](docs/environments.md)

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
| search by status, host or collection | ❌ | ✅ |
| replay | retype it | `r` |
| edit and replay | retype it | `e` |
| compare two responses | ❌ | `d` |

pogo is HTTP-aware history for your terminal. That is all it is trying to be.

## Storage and security

History lives in `~/.local/share/poke` as append-only JSONL plus payload files.
Directory `0700`, files `0600`, nothing encrypted. Your requests never leave the
machine.

> [!IMPORTANT]
> **Request headers routinely carry credentials, and by default poke stores them
> as sent.** Your history file will contain bearer tokens, cookies and API keys
> in plain text. That is a trade-off, not an accident: stripping them would make
> replay silently fail to authenticate. pogo masks secrets on screen; the file on
> disk holds the real thing.

Three ways to change that, in increasing order of strength:

```bash
POKE_REDACT=store poke ...                                # strip secrets before writing
poke --poke-no-capture ...                                # do not record this one
poke -H "Authorization: Bearer {{token}}" '{{base}}/me'   # never capture it at all
```

→ [What is stored, what it exposes, and how to change it](docs/security.md)

## Documentation

| | |
|---|---|
| [docs/keybindings.md](docs/keybindings.md) | Every key, and the search syntax |
| [docs/environments.md](docs/environments.md) | Variables, environments, collections, HAR import |
| [docs/security.md](docs/security.md) | What lands on disk and how to control it |
| [docs/architecture.md](docs/architecture.md) | How poke wraps curl without changing it |
| [docs/runbooks/](docs/runbooks/) | Releasing, screenshots, triage, recovery |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Building, testing, and the two rules |

## How it works, briefly

```
   poke <curl args> ──▶ curlargs ──▶ runner ──▶ the real curl
                        (metadata)   (execute + capture)
                                        │
                                     capture ──▶ store  (JSONL + blobs, local)
                                        ▲          │
                                        │          ▼
                          pogo replays ─┘         tui
```

Both binaries share one execution path, so a replay is the same code running the
same argv — not a reconstruction of it. Storage knows nothing about the UI, the
UI never touches the filesystem, and the curl layer is testable without a
network.

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), Bubbles. No database, no
cgo, no daemon. Everything but the two commands lives under `internal/`
deliberately: poke is a tool, not a library.

→ [Architecture, including the two curl behaviors poke has to restore](docs/architecture.md)

## Development

```bash
make          # build both binaries into ./bin
make check    # gofmt, vet, test, race — the same gates CI runs
make help     # everything else
```

Tests run the real curl against local `httptest` servers; nothing touches the
public internet. Start with [CONTRIBUTING.md](CONTRIBUTING.md).

## Roadmap

Shipped in [v0.2.0](https://github.com/rmpato/poke/releases): structured request
editor · collections and grouping · environments and `{{variables}}` · HAR
import · response header diffing · per-host redaction rules.

Ideas, not promises:

- [ ] export a collection as a runnable script, or back out to HAR
- [ ] chained requests: use a value from one response in the next
- [ ] a `--diff` flag on `pogo` for scripting comparisons in CI
- [ ] response body search, which needs an index
- [ ] size and timing trends per endpoint
- [ ] `.pokerc` per project, so a repo can carry its own environment

If you want something on this list, an issue describing the moment you wanted it
is worth more than a feature request.

## License

MIT — see [LICENSE](LICENSE).
