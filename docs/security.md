# What pogo stores, and what that means

<sub>[Docs](README.md) · [Keys](keybindings.md) · [APIs](apis.md) · **Security** · [Architecture](architecture.md)</sub>

pogo writes a copy of your HTTP traffic to disk. That is the entire feature, and
it is also the entire risk. This page says exactly what lands there so you can
decide whether you are comfortable with it.

## Where

```
$POGO_HOME               # if set
$XDG_DATA_HOME/pogo      # if XDG_DATA_HOME is set
~/.local/share/pogo      # otherwise (macOS and Linux alike)
```

```
~/.local/share/pogo/
├── history.jsonl    # one JSON record per line: the request log
├── blobs/           # request and response payloads too large to inline
└── .lock           # advisory lock, so concurrent writers do not interleave
```

The directory is created with mode `0700` and every file with `0600`. Nothing is
encrypted. Anything running as your user can read it, and so can anything that
can read your home directory: a backup agent, a sync client, a container mount,
a colleague at your unlocked laptop.

`pogo where` prints the directory. Deleting it deletes your history.

## What is recorded

For every request:

- the full command line as you typed it, including every flag and value
- method, URL, request headers, and the request body when pogo can reconstruct it
- response status, response headers for every hop of a redirect chain, and the
  response body up to a size cap
- exit status, curl's error message, duration, and curl's timing breakdown
- the working directory the command ran in
- the environment name that resolved any `{{variables}}` (the values are not
  stored), and the API the request went to

## The part that matters

**Request headers routinely carry credentials.** By default pogo stores them
exactly as sent. That means your history file contains, in plain text:

- `Authorization: Bearer …` tokens
- `Cookie:` and `Set-Cookie:` values, including session cookies
- `X-Api-Key` and similar
- credentials in URLs (`https://user:pass@host`) and in query strings
  (`?access_token=…`)
- whatever is in your request bodies, including passwords sent to a login endpoint

This is a deliberate trade-off, not an oversight. Replay is the reason pogo
exists, and a history that stripped credentials would produce requests that fail
to authenticate — a feature that looks like it works and does not. pogo keeps
the data and tells you plainly, rather than hiding the exposure behind a
reassuring default.

pogo masks these values on screen (`S` reveals them), but masking is a display
choice. The file on disk holds the real thing.

### If that is not the trade-off you want

Switch redaction to `store` mode, and secrets are removed *before* anything is
written:

```yaml
redact:
  mode: store
```

in `~/.config/pogo/config.yaml`, or per-invocation:

```bash
POGO_REDACT=store pogo curl -H "Authorization: Bearer $TOKEN" https://api.example.com/me
```

Entries captured this way are marked `redacted`, and pogo says so when you open
one, because replaying them will not authenticate. That is the cost of the safer
default, and it is why it is not the default.

You can also turn capture off entirely for one command:

```bash
pogo curl --pogo-no-capture -H "Authorization: Bearer $TOKEN" https://api.example.com/me
```

or for a whole shell session with `POGO_NO_CAPTURE=1`.

### Keeping secrets out of history entirely

The strongest option is not to capture the secret in the first place. Write the
request with a variable:

```bash
pogo curl -H "Authorization: Bearer {{token}}" '{{base}}/users'
```

curl gets the real value; history stores `{{token}}`. Replay still works,
because it resolves the variable again at run time. See
[apis.md](apis.md).

### Different rules for different hosts

Production credentials and a local dev server rarely deserve the same treatment:

```yaml
redact:
  mode: display
  hosts:
    api.stripe.com: { mode: store }
    .internal.example.com: { mode: store }
    localhost:8080: { off: true }
```

A host key matches exactly, or as a suffix when it starts with a dot. The
matching policy is applied both when capturing and when displaying.

### Extending what counts as a secret

```yaml
redact:
  mode: display
  headers: [X-Internal-Signature, X-Customer-Token]
  query_params: [sso_ticket]
```

These names are added to the built-in list (`Authorization`,
`Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `Api-Key`,
`X-Auth-Token`, `X-Amz-Security-Token`, `X-Csrf-Token`, `X-Xsrf-Token`) and are
matched case-insensitively.

## Sharing a request

`y` then `C` copies the command with credentials masked — safe to paste into an
issue, and obviously not runnable. `y` then `c` copies the real command, which
is what you want for your own terminal and not for a bug report.

Masking covers header values, credential-bearing curl options (`-u`,
`--oauth2-bearer`, `-b`, …), URL userinfo, and known token-ish query parameters.
It cannot know that your custom `X-Session` header is sensitive unless you tell
it, and it cannot find a token buried in a JSON request body.

## Size limits

Bodies are capped at 2 MiB (response) and 1 MiB (request) by default; anything
larger is truncated in storage and marked as such. The cap never affects what
curl transfers or what you see in the terminal. Binary payloads are detected and
stored but never rendered.

## What pogo does not do

- **No telemetry.** Your requests, history, headers and bodies never leave the
  machine.
- **No modification of the request.** curl receives your arguments verbatim.
- **No account, no cloud, no daemon.**

## The one thing pogo does over the network by itself

pogo checks GitHub for a newer release at most once a day, and only when it is
attached to an interactive terminal, so scripts and pipelines never trigger it.
The check is an ordinary HTTPS GET for the repository's latest release; it sends
nothing about you, your requests or your history. The answer is cached in
`update-check.json` beside your history.

The check runs in a **detached background process**, so it never delays the
request you actually ran. It never installs anything: it prints one line saying
a release exists. Installing always requires `pogo update`, or pressing `u`
and confirming in the UI.

Turn it off entirely:

```bash
export POGO_NO_UPDATE_CHECK=1
```

or in `config.yaml`:

```yaml
update:
  disabled: true
```

The updater itself verifies every download against the release's published
SHA-256 checksums and refuses to install anything unverified.

## Deleting things

- `x` in the UI removes an entry and its payloads. The log keeps a tombstone, so
  the entry is gone from history but the record of the deletion remains until
  the log is compacted.
- `pogo compact` rewrites the log, dropping deleted entries and orphaned
  payloads for good.
- `rm -rf "$(pogo where)"` removes everything.

Compaction also applies the `max_entries` cap (5000 by default), oldest first.
Starred entries are never dropped by the cap.

## Reporting a vulnerability

See [SECURITY.md](../SECURITY.md).
