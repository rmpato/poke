# Screenshot tooling

The images in `docs/img` are captured from a running `pogo`, not drawn.

- `pty_driver.py` — runs a program under a pty, answers the terminal capability
  queries Bubble Tea sends on startup, presses keys, and returns the raw byte
  stream.
- `termsvg.py` — replays that stream through a terminal emulator (`pyte`) and
  renders the resulting cells to a self-contained SVG.
- `demo_server.py` — a small local API that produces realistic responses.
- `capture.py` — the list of screenshots and the keys that reach them.

Full procedure: [docs/runbooks/screenshots.md](../../docs/runbooks/screenshots.md).
