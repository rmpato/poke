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
    ("pogo-list.svg",     [],                                       132, 24, "pogo"),
    ("pogo-apis.svg",     ["A"],                                    132, 24, "pogo — APIs"),
    ("pogo-palette.svg",  ["\x0b"],                                 110, 24, "pogo — commands"),
    # Land on an authenticated request and open the response pane: the masked
    # token and the JSON tree are the two things this screen is for.
    ("pogo-inspect.svg",  ["j", "j", "j", "j", "\r", "3"],           110, 30, "pogo — inspect"),
    ("pogo-edit.svg",     ["j", "j", "j", "e"],                     110, 26, "pogo — edit"),
    ("pogo-timing.svg",   ["j", "\r", "4"],                         110, 22, "pogo — timing"),
    # The two /orders/9021 requests, which the demo server answers differently
    # each time: a diff of the same endpoint against itself is the one worth
    # photographing, and the one the docs describe.
    ("pogo-diff.svg",     ["d", "j", "d"],                          110, 38, "pogo — compare"),
    ("pogo-search.svg",   ["/"] + list("status:4xx"),               124, 16, "pogo — search"),
    ("pogo-env.svg",      ["j", "j", "j", "E"],                     110, 20, "pogo — environments"),
    ("pogo-settings.svg", ["H", "j", "j", "\r"],                    110, 26, "pogo — settings"),
    ("pogo-home.svg",     ["H"],                                    110, 26, "pogo — home"),
    ("pogo-help.svg",     ["?"],                                    150, 38, "pogo — help"),
]

# The walkthrough is only shown on a first run, so it is captured from a
# configuration that has never seen it — separately from everything else, which
# runs with it already dismissed.
# Tall enough that the steps show their full copy rather than collapsing to
# titles — the walkthrough's whole job is the sentences.
WELCOME = ("pogo-welcome.svg", [], 110, 38, "pogo — first run")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--pogo", default="./bin/pogo", help="path to the pogo binary")
    ap.add_argument("--home", default=os.environ.get("POGO_HOME"),
                    help="POGO_HOME holding the demo history")
    ap.add_argument("--out", default="docs/img", help="output directory")
    ap.add_argument("--env-file", help="environments file to use for the capture")
    ap.add_argument("--sandbox-home", help="run with this directory as HOME, so paths on "
                                           "screen read as the documented defaults")
    ap.add_argument("--only", help="capture a single screenshot by file name")
    args = ap.parse_args()

    if not args.home:
        sys.exit("set --home (or POGO_HOME) to a history directory; see the runbook")

    # Run with a sandboxed HOME whose default data directory *is* the demo
    # history. The app then displays "~/.local/share/pogo" — its real default —
    # instead of whatever scratch path the history happens to live in.
    home = os.path.abspath(args.home)
    env = {"POGO_HOME": home}
    config_dir = os.path.join(home, "config")
    if args.sandbox_home:
        sandbox = os.path.abspath(args.sandbox_home)
        target = os.path.join(sandbox, ".local", "share")
        os.makedirs(target, exist_ok=True)
        link = os.path.join(target, "pogo")
        if os.path.islink(link):
            os.unlink(link)
        if not os.path.exists(link):
            os.symlink(home, link)
        # The pty inherits this process's environment, so a POGO_HOME exported
        # in the shell that launched the capture would win over the sandbox and
        # put a scratch path on screen. Clear it explicitly.
        env = {"HOME": sandbox, "POGO_HOME": "", "POGO_CONFIG": ""}
        config_dir = os.path.join(sandbox, ".config", "pogo")
    if args.env_file:
        env["POGO_ENV_FILE"] = os.path.abspath(args.env_file)
    os.makedirs(args.out, exist_ok=True)

    def shoot(name, keys, cols, rows, title):
        buf = drive([os.path.abspath(args.pogo)], keys=keys, cols=cols, rows=rows,
                    env=env, settle=0.45)
        path = os.path.join(args.out, name)
        with open(path, "w") as fh:
            fh.write(to_svg(buf, cols, rows, title=title))
        print(f"  {path}")

    config = os.path.join(config_dir, "config.yaml")
    os.makedirs(config_dir, exist_ok=True)

    # First run, with the walkthrough still to be seen.
    if os.path.exists(config):
        os.remove(config)
    if not args.only or args.only == WELCOME[0]:
        shoot(*WELCOME)

    # Then every other screen, with it dismissed — the state a user is in for
    # all but the first thirty seconds.
    with open(config, "w") as fh:
        fh.write("onboarding_seen: true\n")

    for name, keys, cols, rows, title in SHOTS:
        if args.only and args.only != name:
            continue
        shoot(name, keys, cols, rows, title)


if __name__ == "__main__":
    main()
