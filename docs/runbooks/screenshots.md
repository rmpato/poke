# Regenerating the screenshots

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

Requests are made against a real hostname using curl's `--resolve`, so the
screenshots show `api.example.com` while every byte is local:

```bash
export POKE_HOME=/tmp/poke-shots
rm -rf "$POKE_HOME"

R=(--resolve api.example.com:8080:127.0.0.1)
AUTH=(-H 'Authorization: Bearer sk-live-4f9c2a8e1b7d')

./bin/poke -s "${R[@]}" http://api.example.com:8080/users
./bin/poke -s "${R[@]}" "${AUTH[@]}" http://api.example.com:8080/users/42
./bin/poke -s "${R[@]}" "${AUTH[@]}" http://api.example.com:8080/billing/invoices
./bin/poke -s "${R[@]}" -X DELETE http://api.example.com:8080/users/41
./bin/poke -s "${R[@]}" http://api.example.com:8080/orders/9021
```

**Space the requests out.** The list shows relative ages, and ten requests made
in the same second all read `0s`, which looks fabricated because it is. Put a
`sleep 45` between them, or run them from a script while you do something else.
The committed screenshots were produced over about ten real minutes.

The `/orders/9021` endpoint alternates between `pending` and `completed`, so
requesting it twice gives the diff screenshot something true to show.

Star a couple of entries through the UI (`s`) rather than editing the history
file — the point is that the state in the picture is state the app produced.

## 2. Capture

```bash
.venv/bin/python scripts/screenshots/capture.py --pogo ./bin/pogo --home "$POKE_HOME"
```

To iterate on one image:

```bash
.venv/bin/python scripts/screenshots/capture.py --only pogo-diff.svg --home "$POKE_HOME"
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
