#!/usr/bin/env python3
"""Regenerate the screenshots in docs/img from a real pogo process.

Every image in the documentation is genuine output: pogo runs under a pty, the
byte stream is replayed through a terminal emulator, and the resulting cells are
drawn to SVG. Nothing is drawn by hand, so the docs cannot drift from the UI.

Usage:
    python3 -m venv .venv && .venv/bin/pip install pyte
    .venv/bin/python scripts/screenshots/capture.py --pogo ./bin/pogo

See docs/runbooks/screenshots.md for the full procedure, including how the
demo history is produced.
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from pty_driver import drive          # noqa: E402
from termsvg import render as to_svg  # noqa: E402

# Each shot names the keys to press before the frame is captured. Keeping them
# declarative means adding a screenshot is one line, not a new script.
SHOTS = [
    ("pogo-list.svg",    [],                                  100, 16, "pogo"),
    ("pogo-inspect.svg", ["\r"],                              100, 27, "pogo — inspect"),
    ("pogo-timing.svg",  ["\r", "4"],                         100, 19, "pogo — timing"),
    ("pogo-diff.svg",    ["j", "d"] + ["j"] * 8 + ["d"],      100, 22, "pogo — compare"),
    ("pogo-search.svg",  ["/"] + list("status:4xx"),          100, 12, "pogo — search"),
    ("pogo-help.svg",    ["?"],                               108, 30, "pogo — help"),
]


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--pogo", default="./bin/pogo", help="path to the pogo binary")
    ap.add_argument("--home", default=os.environ.get("POKE_HOME"),
                    help="POKE_HOME holding the demo history")
    ap.add_argument("--out", default="docs/img", help="output directory")
    ap.add_argument("--only", help="capture a single screenshot by file name")
    args = ap.parse_args()

    if not args.home:
        sys.exit("set --home (or POKE_HOME) to a history directory; see the runbook")

    env = {"POKE_HOME": os.path.abspath(args.home)}
    os.makedirs(args.out, exist_ok=True)

    for name, keys, cols, rows, title in SHOTS:
        if args.only and args.only != name:
            continue
        buf = drive([os.path.abspath(args.pogo)], keys=keys, cols=cols, rows=rows,
                    env=env, settle=0.45)
        path = os.path.join(args.out, name)
        with open(path, "w") as fh:
            fh.write(to_svg(buf, cols, rows, title=title))
        print(f"  {path}")


if __name__ == "__main__":
    main()
