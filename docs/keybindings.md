# Keybindings

Every binding is also visible in the app: the footer shows what applies right
now, and `?` opens the full list.

<img src="img/pogo-help.svg" alt="the pogo help screen" width="100%">

## Everywhere

| Key | Action |
|---|---|
| `q`, `ctrl+c` | Quit |
| `esc` | Back — leave a screen, close an overlay, or clear the search |
| `?` | Help |
| `S` | Reveal masked secrets (toggle) |

## The list

| Key | Action |
|---|---|
| `↑` `k` / `↓` `j` | Move |
| `g` / `G` | Top / bottom |
| `ctrl+u` / `ctrl+d` | Half page up / down |
| `⏎` | Inspect — or fold a host group |
| `/` | Search |
| `t` | Group by host |
| `r` | Replay |
| `e` | Edit, then run |
| `y` | Copy menu |
| `s` | Star / unstar |
| `x` | Delete |
| `d` | Mark for comparison; press `d` on a second request to diff |

## Search

Free text matches method, URL, status, note and error. Filters can be combined,
and repeating a filter ORs its values.

| Token | Matches |
|---|---|
| `users/42` | anything containing the text |
| `method:POST` | one method |
| `status:404` | one status code |
| `status:4xx` | a status class |
| `host:api.example.com` | a host (substring) |
| `is:starred` | starred requests |
| `is:failed` | requests that failed or returned ≥ 400 |

Example: `method:POST status:5xx host:api` — failing writes to one host.

Header *values* are deliberately not searched, so a search for `token` does not
match every authenticated request.

## Inspecting

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Next / previous pane |
| `1` – `5` | Overview, Request, Response, Timing, Raw |
| `v` | Cycle the body view: tree → pretty → raw |
| `↑` `↓` | Move the JSON cursor in tree view, otherwise scroll |
| `space` | Fold or unfold the node under the cursor |
| `g` / `G` | Top / bottom |

## Editing

`e` opens the request's curl command in an editable buffer.

| Key | Action |
|---|---|
| `ctrl+r` (or `ctrl+⏎`) | Run it |
| `ctrl+e` | Hand the buffer to `$EDITOR` |
| `esc` | Cancel |

Running an edited command records a **new** entry pointing back at the one it
came from. The original is never modified.

> `ctrl+⏎` only reaches an application in terminals that can distinguish it from
> `⏎`; `ctrl+r` works everywhere, which is why it is the documented binding.

## Copying

`y` opens the menu; press the letter, or move and press `⏎`.

| Key | Copies |
|---|---|
| `c` | The curl command |
| `C` | The curl command with secrets masked — safe to paste into an issue |
| `u` | The URL |
| `h` / `H` | Request / response headers |
| `b` / `r` | Request / response body |
| `j` | The whole entry as JSON |

Copying uses `pbcopy`, `wl-copy`, `xclip` or `xsel`, and falls back to an
OSC 52 escape so it also works over SSH and inside tmux.
