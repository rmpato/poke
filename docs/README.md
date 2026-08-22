# Documentation

New here? The [README](../README.md) explains what pogo is in about thirty
seconds, and the [website](https://rmpato.github.io/poke) shows it working.

## Start here

```bash
curl -fsSL https://raw.githubusercontent.com/rmpato/poke/main/install.sh | sh

pogo curl https://api.github.com/zen   # a request, exactly as curl would run it
pogo                                   # everything it has recorded
```

Press `?` in pogo for the keys. That is the whole product.

## Guides

| | |
|---|---|
| [keybindings.md](keybindings.md) | Every key, and the search syntax |
| [apis.md](apis.md) | How history is grouped, `{{variables}}`, environments, collections, HAR import |
| [security.md](security.md) | What lands on disk, what that exposes, and how to change it |

## Reference

| | |
|---|---|
| [architecture.md](architecture.md) | How pogo wraps curl without changing it, and why storage looks the way it does |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Building, testing, and the three rules that are not negotiable |
| [../SECURITY.md](../SECURITY.md) | Reporting a vulnerability, and what counts as one |

## Operations

[runbooks/](runbooks/) — procedures for the recurring jobs: cutting a release,
regenerating the screenshots, refreshing the curl option table, triaging a
"pogo behaves differently from curl" report, recovering a damaged history file,
and responding to a security report.

## About this directory

`index.html` and `img/` are the published website
([rmpato.github.io/poke](https://rmpato.github.io/poke)), served by GitHub Pages
from `main:/docs`. The screenshots are captured from a running pogo rather than
drawn; see [runbooks/screenshots.md](runbooks/screenshots.md).
