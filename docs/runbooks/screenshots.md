# Regenerating the screenshots

<sub>[Docs](../README.md) · [Runbooks](README.md)</sub>

Every image in `docs/img` is a real frame from a real `pogo` process. Nothing is
mocked up, which means the docs cannot quietly drift from the UI — and also that
regenerating them is a procedure rather than a drawing session.

## What you need

```bash
python3 -m venv .venv
.venv/bin/pip install pyte
make            # builds ./bin/pogo
```

## 1. Produce a history worth photographing

The demo server answers on `127.0.0.1:8080` with realistic payloads:

```bash
python3 scripts/screenshots/demo_server.py &
```

Then seed a history. `seed.sh` makes every request for real, against real
hostnames pointed at the local server with curl's `--resolve`, so the pictures
show `api.staging.acme.com` while every byte stays on your machine:

```bash
scripts/screenshots/seed.sh ./bin/pogo /tmp/pogo-shots
```

It produces one API in three environments, a second API, and something running
on localhost — which is the shape the list is built to make sense of, and the
thing worth photographing.

**It takes about nine minutes, and that is the point.** The list shows relative
ages, and twelve requests made in the same second all read `0s`, which looks
fabricated because it is. `GAP=45` (the default) spaces them out; `GAP=0` is for
iterating on the tooling, not for the committed images.

Two details the script relies on:

- The response body is **not** discarded with `-o /dev/null`. pogo records what
  curl writes to stdout, so a request run that way is stored with no body and
  every body pane in the screenshots comes out empty.
- `/orders/9021` alternates between `pending` and `completed`, so requesting it
  twice gives the diff screenshot something true to show.

Then add the environments the pictures refer to:

```bash
export POGO_HOME=/tmp/pogo-shots POGO_ENV_FILE=/tmp/pogo-shots/environments.yaml
./bin/pogo env set staging --api acme.com base=https://api.staging.acme.com token=sk_test_9f2b1c
./bin/pogo env set prod    --api acme.com base=https://api.acme.com token=sk_live_4f9c2a
./bin/pogo env set dev     --api acme.com base=http://dev-api.acme.com
./bin/pogo env set staging ua=pogo/1.0
./bin/pogo env use staging
```

Star a couple of entries through the UI (`s`) rather than editing the history
file — the point is that the state in the picture is state the app produced.

## 2. Capture

```bash
env -u POGO_HOME -u POGO_CONFIG .venv/bin/python scripts/screenshots/capture.py \
  --pogo ./bin/pogo --home /tmp/pogo-shots --sandbox-home /tmp/pogo-sandbox \
  --env-file /tmp/pogo-shots/environments.yaml
```

`--sandbox-home` runs pogo with a HOME whose default data directory *is* the
demo history, so the header reads `~/.local/share/pogo` — its real default —
rather than a scratch path. Clearing `POGO_HOME` matters: the pty inherits your
shell's environment, and an exported one would win over the sandbox.

The first-run walkthrough is captured from a configuration that has never seen
it; every other shot is taken with it dismissed, which is the state a user is in
for all but the first thirty seconds.

To iterate on one image:

```bash
.venv/bin/python scripts/screenshots/capture.py --only pogo-diff.svg \
  --home /tmp/pogo-shots --sandbox-home /tmp/pogo-sandbox
```

## 3. Check before committing

```bash
git diff --stat docs/img/
```

Open a couple in a browser. Look for:

- **Wrapped rows.** A row wider than the pane wraps and doubles in height. There
  are tests for this (`TestRowsNeverExceedWidth`), but check anyway.
- **Trailing space artefacts.** lipgloss pads every line of a rendered block, so
  a newline inside `Render()` injects spaces that shift the next line. Banned by
  `TestNoNewlinesInsideRender`.
- **Empty panes.** A body pane showing `—` usually means the request that
  produced it discarded its output.
- **Real secrets.** The demo token is fake. Make sure yours was too.

## How it works

`pty_driver.py` runs pogo under a pty and answers the terminal capability
queries Bubble Tea sends at startup (background colour, cursor position) —
without replies the program blocks forever waiting for a terminal that never
speaks. It then presses keys, waits for the frame to settle, and **kills** the
process rather than quitting it: leaving the alternate screen restores the
previous buffer, which would erase the frame you came to capture.

`termsvg.py` replays the bytes through `pyte` and walks the resulting cell grid,
grouping runs of identical styling into `<text>` spans.
