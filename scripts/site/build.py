#!/usr/bin/env python3
"""Build the pogo website into docs/.

    python3 scripts/site/build.py

One generator owns the shell — head, nav, footer — because a nav copied into
four static files drifts the first time a page is added, and the symptom is a
dead link nobody notices. The pages themselves are the fragments in
`fragments/`, which are plain HTML: there is no template language to learn and
no build step in the way of editing a paragraph.

docs/ is what GitHub Pages serves, so the generated files are committed.
"""
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
OUT = os.path.join(ROOT, "docs")

SITE = "https://rmpato.github.io/pogo/"
REPO = "https://github.com/rmpato/pogo"

# The single source of truth for navigation. Adding a page here puts it in the
# header and the footer of every other page at once.
NAV = [
    ("home",    "Home",    "index.html"),
    ("tour",    "Tour",    "tour.html"),
    ("apis",    "APIs",    "apis.html"),
    ("install", "Install", "install.html"),
]

PAGES = [
    {
        "file": "index.html",
        "nav": "home",
        "title": "pogo — curl, but it remembers",
        "description": "pogo runs curl and writes down what happened, then gives you a "
                       "terminal UI over everything it has run — grouped by API, with "
                       "staging and production as environments of the same thing.",
        "fragments": ["home-hero", "home-bento", "home-idea", "home-loop", "home-start"],
        "hero": True,
    },
    {
        "file": "tour.html",
        "nav": "tour",
        "title": "Tour — pogo",
        "description": "Every screen in pogo, one at a time: the request list, APIs and "
                       "environments, inspecting, editing, diffing, search, the palette, "
                       "settings and the home shell.",
        "fragments": ["tour"],
    },
    {
        "file": "apis.html",
        "nav": "apis",
        "title": "APIs and environments — pogo",
        "description": "How pogo works out which API a request belongs to and which "
                       "environment of it, how to correct a guess, and how {{variables}} "
                       "resolve per API under a global environment name.",
        "fragments": ["apis"],
    },
    {
        "file": "install.html",
        "nav": "install",
        "title": "Install and reference — pogo",
        "description": "Install pogo, the whole command line, every keyboard shortcut, "
                       "what lands on disk, and how the wrapper works.",
        "fragments": ["install"],
    },
]

SHELL = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title}</title>
<meta name="description" content="{description}">
<meta name="theme-color" content="#0b0d12">
<link rel="canonical" href="{site}{canonical}">

<meta property="og:type" content="website">
<meta property="og:url" content="{site}{canonical}">
<meta property="og:title" content="{title}">
<meta property="og:description" content="{description}">
<meta property="og:image" content="{site}img/pogo-list.svg">
<meta name="twitter:card" content="summary_large_image">

<link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><rect width='32' height='32' rx='7' fill='%2314161c'/><path d='M9 24V8h6a5 5 0 0 1 0 10H9' stroke='%232E86F0' stroke-width='3' fill='none'/><circle cx='23' cy='21' r='3' fill='%233FB950'/></svg>">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;700&display=swap" rel="stylesheet">
<link rel="stylesheet" href="styles.css">
</head>
<body{body_class}>

<a class="skip" href="#main">Skip to content</a>

<header class="nav">
  <div class="shell nav-inner">
    <a class="brand" href="index.html">pogo</a>
    <span class="spacer"></span>
    <nav class="nav-links" aria-label="Site">
{nav}
      <a class="gh" href="{repo}">
        <svg width="15" height="15" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 0a8 8 0 0 0-2.53 15.59c.4.07.55-.17.55-.38l-.01-1.49c-2.01.37-2.53-.5-2.7-.96-.09-.24-.48-.96-.82-1.15-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.4 7.4 0 0 1 4 0c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48l-.01 2.19c0 .21.15.46.55.38A8 8 0 0 0 8 0Z"/></svg>
        <span>GitHub</span>
      </a>
    </nav>
  </div>
</header>

<main id="main">
{body}
</main>

<footer>
  <div class="shell">
    <div class="links">
      <a href="index.html">Home</a>
      <a href="tour.html">Tour</a>
      <a href="apis.html">APIs</a>
      <a href="install.html">Install</a>
      <a href="{repo}">GitHub</a>
      <a href="{repo}/releases">Releases</a>
      <a href="{repo}/blob/main/docs/README.md">Docs</a>
      <a href="https://github.com/rmpato/whis">whis</a>
    </div>
    <p class="sig">MIT · local-first · curl does the requesting, pogo does the remembering</p>
  </div>
</footer>

<script src="site.js" defer></script>
</body>
</html>
"""


def nav_html(active):
    rows = []
    for ident, label, href in NAV:
        current = ' aria-current="page"' if ident == active else ""
        rows.append('      <a href="%s"%s>%s</a>' % (href, current, label))
    return "\n".join(rows)


def build():
    written = []
    for page in PAGES:
        body = []
        for name in page["fragments"]:
            path = os.path.join(HERE, "fragments", name + ".html")
            with open(path) as handle:
                body.append(handle.read().rstrip("\n"))

        canonical = "" if page["file"] == "index.html" else page["file"]
        html = SHELL.format(
            title=page["title"],
            description=page["description"],
            canonical=canonical,
            site=SITE,
            repo=REPO,
            nav=nav_html(page["nav"]),
            body="\n".join(body),
            body_class=' class="is-home"' if page.get("hero") else "",
        )

        out = os.path.join(OUT, page["file"])
        with open(out, "w") as handle:
            handle.write(html)
        written.append((page["file"], len(html)))

    for name, size in written:
        print("  docs/%-14s %6d bytes" % (name, size))


if __name__ == "__main__":
    missing = [
        name
        for page in PAGES
        for name in page["fragments"]
        if not os.path.exists(os.path.join(HERE, "fragments", name + ".html"))
    ]
    if missing:
        sys.exit("missing fragments: " + ", ".join(missing))
    build()
