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
# The list is captured wide enough to show the sidebar, because the sidebar is
# how the shape of the history becomes visible.
SHOTS = [
    ("pogo-list.svg",    [],                                        132, 20, "pogo"),
    ("pogo-palette.svg", ["\x0b"],                                  100, 22, "pogo — commands"),
    ("pogo-inspect.svg", ["j", "j", "\r"],                          100, 27, "pogo — inspect"),
    ("pogo-edit.svg",    ["j"] * 8 + ["e"],                         100, 22, "pogo — edit"),
    ("pogo-timing.svg",  ["j", "j", "\r", "4"],                     100, 19, "pogo — timing"),
    ("pogo-diff.svg",    ["j", "j", "j", "d"] + ["j"] * 8 + ["d"],  100, 24, "pogo — compare"),
    ("pogo-search.svg",  ["/"] + list("status:4xx"),                116, 14, "pogo — search"),
    ("pogo-groups.svg",  ["t", "t"],                                132, 22, "pogo — collections"),
    ("pogo-env.svg",     ["E"],                                     100, 18, "pogo — environments"),
    ("pogo-help.svg",    ["?"],                                     140, 34, "pogo — help"),
]


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--pogo", default="./bin/pogo", help="path to the pogo binary")
    ap.add_argument("--home", default=os.environ.get("POKE_HOME"),
                    help="POKE_HOME holding the demo history")
    ap.add_argument("--out", default="docs/img", help="output directory")
    ap.add_argument("--env-file", help="environments file to use for the capture")
    ap.add_argument("--sandbox-home", help="run with this directory as HOME, so paths on "
                                           "screen read as the documented defaults")
    ap.add_argument("--only", help="capture a single screenshot by file name")
    args = ap.parse_args()

    if not args.home:
        sys.exit("set --home (or POKE_HOME) to a history directory; see the runbook")

    # Run with a sandboxed HOME whose default data directory *is* the demo
    # history. The app then displays "~/.local/share/poke" — its real default —
    # instead of whatever scratch path the history happens to live in.
    home = os.path.abspath(args.home)
    env = {"POKE_HOME": home}
    if args.sandbox_home:
        sandbox = os.path.abspath(args.sandbox_home)
        target = os.path.join(sandbox, ".local", "share")
        os.makedirs(target, exist_ok=True)
        link = os.path.join(target, "poke")
        if os.path.islink(link):
            os.unlink(link)
        if not os.path.exists(link):
            os.symlink(home, link)
        env = {"HOME": sandbox}
    if args.env_file:
        env["POKE_ENV_FILE"] = os.path.abspath(args.env_file)
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
