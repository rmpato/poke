# Architecture

<sub>[Docs](README.md) · [Keys](keybindings.md) · [APIs](apis.md) · [Security](security.md) · **Architecture**</sub>

One binary with two faces, and one shared execution path between them.

```
                    ┌──────────────────────────────┐
   your terminal    │  pogo curl <curl arguments>  │
                    │  internal/cli (cobra + fang) │
                    └───────────────┬──────────────┘
                                    │
                    ┌───────────────▼──────────────┐
                    │  internal/curlargs           │  metadata only
                    │  (what did this command say?)│  never affects execution
                    └───────────────┬──────────────┘
                                    │
                    ┌───────────────▼──────────────┐
                    │  internal/runner             │
                    │  exec curl, tee, side-channel│──▶ the real curl binary
                    └───────────────┬──────────────┘
                                    │
                    ┌───────────────▼──────────────┐
                    │  internal/capture            │  ← pogo replays through
                    │  run + build entry + persist │     this exact path
                    └───────────────┬──────────────┘
                                    │
                    ┌───────────────▼──────────────┐
                    │  internal/store              │
                    │  append-only JSONL + blobs   │
                    └───────────────┬──────────────┘
                                    │
                    ┌───────────────▼──────────────┐
                    │  internal/apis               │  which API? which env?
                    │  eTLD+1 + hostname reading   │  overridable, always
                    └───────────────┬──────────────┘
                                    │
                    ┌───────────────▼──────────────┐
                    │  internal/tui → internal/ui  │  the whis kit, vendored
                    │  screens          + /home    │  as source we own
                    └──────────────────────────────┘
```

Layering rules that are actually enforced by the package graph:

- `store` imports no UI code and knows nothing about terminals.
- `tui` never touches the filesystem; it holds a `*store.Store` and issues
  commands.
- `runner` takes an injectable binary path, so it is testable without a network.
- `capture` is the only place execution and storage meet, and **both faces go
  through it**. A replay is not a reconstruction of the original request; it is
  the same code running the same argv.
- `apis` is pure: a URL and a registry in, a conclusion out. It reads no files,
  which is what makes every grouping rule a table-driven test rather than a
  screenshot someone has to squint at.
- `ui` and `home` are copies of [whis](https://github.com/rmpato/whis), vendored
  per that project's own rule: you own the copy, there is no dependency to
  upgrade and nothing upstream that can change under you. They import nothing
  from pogo, so the dependency only ever points one way.

## pogo is a wrapper, not a reimplementation

Your arguments are handed to the real `curl` verbatim. pogo appends its own
capture options and never rewrites yours. Three side channels collect the data:

| What | Mechanism | Why it is safe |
|---|---|---|
| status + response headers (all redirect hops) | `-D <tmpfile>` | Ancient, universal, touches neither stdout nor stderr |
| response body | tee stdout through a bounded buffer | Bytes reach you unchanged and unbounded; only the stored copy is capped |
| timings | `--write-out '%output{file}%{json}'` | Written to a file, so stdout stays clean |
| duration, exit status | measured around the process | — |

If you passed your own `-D` or `-w`, pogo uses yours and forgoes its own; your
command is not something to overwrite.

### The two behaviors pogo has to restore

Teeing means curl writes into a pipe instead of your terminal, and curl changes
behavior when its output is not a terminal. Both differences are restored
deliberately:

1. **Progress meter.** With a pipe, curl turns it on. pogo passes
   `--no-progress-meter` when stdout is a real terminal *and* the body is
   heading there — the same condition curl uses. On curl older than 7.67, which
   lacks that flag, pogo skips body capture rather than pass an option curl
   would reject.
2. **Binary output guard.** curl refuses to spray binary data at a terminal
   unless you asked with `-o -`. Through a pipe it cannot tell, so pogo performs
   the same check (a NUL byte in the chunk, which is exactly curl's test),
   prints curl's own warning verbatim, and exits 23 as curl does.

Everything else — exit codes, stderr, `-v` output, interactive stdin, `-o`,
`-O`, `--next`, config files — is curl's, untouched.

Timings need curl 8.3 or newer (`%output{}` in `--write-out`). On older curl the
timing pane says so instead of inventing numbers.

## Parsing is metadata only

`internal/curlargs` exists so pogo can show `POST api.example.com/users` instead
of a row of shell tokens. It carries a table of curl's options and their arity,
generated from the local curl by `scripts/gen-curl-options.sh` and verified
against curl's own `requires parameter` error.

The safety property: **a parser gap degrades a history record, never a request.**
Anything unrecognised lands in `Unrecognized`, pogo says the parse is
incomplete, and a token becomes a URL only if it looks like one — so an unknown
option's value cannot be misfiled as the URL.

## Editing preserves what it does not understand

The obvious way to build a field editor — read a request into fields, then
generate a command from those fields — silently destroys anything the fields do
not model. A request carrying `--cacert`, `--resolve`, `-k` or `--unix-socket`
would come back without them and fail for reasons the user cannot see.

So `internal/curledit` works the other way round: the original argv stays
authoritative and each change is applied to it in place. Change a header value
and exactly one `-H` argument is rewritten. This is the same property that makes
capture safe, applied to editing.

## Variables are resolved for execution only

`internal/environment` expands `{{name}}` on the way to curl and nowhere else.
The command that reaches curl carries the token; the command that reaches
`history.jsonl` keeps the braces. A replay resolves again, against whatever the
environment holds then.

Two consequences worth stating: secrets written this way never enter history at
all, and `url_effective` is dropped from the captured metrics when expansion
happened, because it would put the resolved URL back on disk.

## Actions are data

Three parts of the UI need to know what pogo can do: the key handler dispatches
it, the command palette searches it, and the help reference lists it. Written
three times, they drift, and the result is a feature nobody can find.

So `internal/tui/commands.go` holds one registry — id, title, description, key,
group, availability, and the function to run — and all three read from it.
Adding an action to the registry makes it appear in the palette and the help
screen at once; there is no second place to update and forget.

That is also why the palette shows each command's key: finding something by
searching teaches the shortcut, so the palette makes itself unnecessary.

## Storage

An append-only JSONL log of operations, plus a blob directory for payloads.

```json
{"op":"put","at":"…","entry":{…}}
{"op":"patch","at":"…","id":"…","patch":{"favorite":true}}
{"op":"del","at":"…","id":"…"}
```

Loading folds the operations in order. Starring or deleting appends a record
rather than rewriting one, which is what makes history immutable by construction
rather than by discipline — and what lets several `pogo` processes append while
`pogo` has the file open.

Why not SQLite: the workload is "append one row, read a few thousand at
startup". SQLite would buy indexes nobody queries at the cost of cgo or a very
large pure-Go port. JSONL is greppable with `jq`, portable with `cp`, and
crash-safe under `O_APPEND` — which matters for a local-first tool people are
asked to trust with their traffic.

Payloads live outside the log so that opening pogo costs the number of requests,
not the number of bytes they moved. Payloads under 4 KiB are inlined so a whole
exchange stays visible in `tail -1 history.jsonl`.

Writes take an advisory `flock` on a dedicated lock file (a separate inode, so
compaction's rename cannot orphan it) and write each record in a single call.
`pogo compact` rewrites the log, applies the entry cap and sweeps orphaned
blobs.

## Package map

| Package | Responsibility |
|---|---|
| `internal/cli` | The command tree: Cobra + Fang, the curl wrapper, the bare-URL shorthand |
| `internal/curlargs` | Parse a curl command line into displayable metadata; shell quoting and splitting |
| `internal/runner` | Execute curl, tee output, collect status, headers, timings |
| `internal/capture` | Turn an execution into a history entry and persist it |
| `internal/store` | Append-only log, blob store, folding, compaction, locking |
| `internal/history` | On-disk types, redaction policy, id generation |
| `internal/config` | Paths, defaults, the YAML config store, environment overrides, API overrides |
| `internal/apis` | Which API a URL belongs to, and which environment of it |
| `internal/tui` | Bubble Tea model, views, command registry, search, JSON rendering, diff |
| `internal/ui` | The whis component kit: theme tokens, exact-rectangle layout, every component |
| `internal/home` | The whis home shell and the first-run walkthrough |
| `internal/curledit` | Apply structured field edits to an existing command line |
| `internal/environment` | Resolve `{{variables}}` on the way to curl, never on the way to disk; per-API values under global names |
| `internal/harimport` | Turn a browser HAR export into history entries |
| `internal/selfupdate` | Release checks and checksum-verified updates |
| `internal/clipboard` | System clipboard with an OSC 52 fallback for SSH and tmux |

## The design system

Every screen is composed from `internal/ui`, which is
[whis](https://github.com/rmpato/whis) copied in as source pogo owns. Three of
its rules do most of the work:

- **Exact rectangles.** Every renderer returns a block of exactly the size it
  was asked for, and `View()` is exactly the terminal's. Bubble Tea repaints the
  whole screen every frame, so a block one row short does not shift — it leaves
  the previous frame's row behind it. `render_guard_test.go` asserts this on
  every screen and dialog at nine terminal sizes.
- **One palette.** No screen constructs a colour. Every style in
  `internal/tui/styles.go` is derived from the theme tokens, including the ones
  that carry pogo's own meanings — method, status, environment, JSON — so
  switching theme reaches all of them.
- **Plain variants.** A selected row is drawn as one styled span, so its cells
  go in as plain text; any colour inside would end the highlight partway across
  the line. The list builds each row twice for exactly this reason, which is why
  `list.go` has both a plain and a coloured path.

## Testing

- `curlargs` is table-driven, including the property that an unknown option
  cannot swallow a URL.
- `apis` is table-driven over real hostname shapes: multi-part public suffixes,
  `github.io`, IP addresses, ports, and the case where a company is called
  Staging.
- `cli` asserts that a curl-shaped argument list survives `pogo curl` byte for
  byte, and that the bare-URL shorthand can never swallow a subcommand name.
- `runner` and `capture` run the real curl against `httptest` servers. No test
  touches the public internet; they skip if curl is absent.
- `store` covers folding, compaction, path-traversal rejection and concurrent
  writers under `-race`.
- `tui` drives `Update` with real messages and asserts on rendered frames —
  including that every frame fits the terminal exactly and no row overflows.
