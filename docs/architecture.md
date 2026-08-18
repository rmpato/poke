# Architecture

Two binaries, one module, one shared execution path.

```
                    ┌──────────────────────────────┐
   your terminal    │  poke  <curl arguments>      │
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
                    │  internal/tui  →  pogo       │
                    └──────────────────────────────┘
```

Layering rules that are actually enforced by the package graph:

- `store` imports no UI code and knows nothing about terminals.
- `tui` never touches the filesystem; it holds a `*store.Store` and issues
  commands.
- `runner` takes an injectable binary path, so it is testable without a network.
- `capture` is the only place execution and storage meet, and **both binaries go
  through it**. A replay is not a reconstruction of the original request; it is
  the same code running the same argv.

## poke is a wrapper, not a reimplementation

Your arguments are handed to the real `curl` verbatim. poke appends its own
capture options and never rewrites yours. Three side channels collect the data:

| What | Mechanism | Why it is safe |
|---|---|---|
| status + response headers (all redirect hops) | `-D <tmpfile>` | Ancient, universal, touches neither stdout nor stderr |
| response body | tee stdout through a bounded buffer | Bytes reach you unchanged and unbounded; only the stored copy is capped |
| timings | `--write-out '%output{file}%{json}'` | Written to a file, so stdout stays clean |
| duration, exit status | measured around the process | — |

If you passed your own `-D` or `-w`, poke uses yours and forgoes its own; your
command is not something to overwrite.

### The two behaviors poke has to restore

Teeing means curl writes into a pipe instead of your terminal, and curl changes
behavior when its output is not a terminal. Both differences are restored
deliberately:

1. **Progress meter.** With a pipe, curl turns it on. poke passes
   `--no-progress-meter` when stdout is a real terminal *and* the body is
   heading there — the same condition curl uses. On curl older than 7.67, which
   lacks that flag, poke skips body capture rather than pass an option curl
   would reject.
2. **Binary output guard.** curl refuses to spray binary data at a terminal
   unless you asked with `-o -`. Through a pipe it cannot tell, so poke performs
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

## Storage

An append-only JSONL log of operations, plus a blob directory for payloads.

```json
{"op":"put","at":"…","entry":{…}}
{"op":"patch","at":"…","id":"…","patch":{"favorite":true}}
{"op":"del","at":"…","id":"…"}
```

Loading folds the operations in order. Starring or deleting appends a record
rather than rewriting one, which is what makes history immutable by construction
rather than by discipline — and what lets several `poke` processes append while
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
`pogo --compact` rewrites the log, applies the entry cap and sweeps orphaned
blobs.

## Package map

| Package | Responsibility |
|---|---|
| `internal/curlargs` | Parse a curl command line into displayable metadata; shell quoting and splitting |
| `internal/runner` | Execute curl, tee output, collect status, headers, timings |
| `internal/capture` | Turn an execution into a history entry and persist it |
| `internal/store` | Append-only log, blob store, folding, compaction, locking |
| `internal/history` | On-disk types, redaction policy, id generation |
| `internal/config` | Paths, defaults, config file, environment overrides |
| `internal/tui` | Bubble Tea model, views, search, JSON rendering, diff |
| `internal/curledit` | Apply structured field edits to an existing command line |
| `internal/environment` | Resolve `{{variables}}` on the way to curl, never on the way to disk |
| `internal/harimport` | Turn a browser HAR export into history entries |
| `internal/selfupdate` | Release checks and checksum-verified updates |
| `internal/clipboard` | System clipboard with an OSC 52 fallback for SSH and tmux |

## Testing

- `curlargs` is table-driven, including the property that an unknown option
  cannot swallow a URL.
- `runner` and `capture` run the real curl against `httptest` servers. No test
  touches the public internet; they skip if curl is absent.
- `store` covers folding, compaction, path-traversal rejection and concurrent
  writers under `-race`.
- `tui` drives `Update` with real messages and asserts on rendered frames —
  including that every frame fits the terminal exactly and no row overflows.
