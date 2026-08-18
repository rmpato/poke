# Environments and variables

<sub>[Docs](README.md) · [Keys](keybindings.md) · **Environments** · [Security](security.md) · [Architecture](architecture.md)</sub>

A request written with variables runs against real values and is **stored with
the braces intact**:

```bash
poke -H "Authorization: Bearer {{token}}" '{{base}}/users/42'
```

curl receives `Authorization: Bearer sk-live-…` and `https://api.example.com/users/42`.
Your history file records:

```json
{"method":"GET","url":"{{base}}/users/42",
 "headers":[{"name":"Authorization","value":"Bearer {{token}}"}]}
```

Two problems solved at once. The token never lands in `history.jsonl` — the
thing [docs/security.md](security.md) warns about most. And a replay six weeks
later resolves the variable *again*, so it uses the token you have now instead
of the expired one you captured.

## Setting them up

Environments live in `environments.json`, beside your config
(`~/.config/poke/environments.json`), written with mode `0600` because it holds
the credentials history does not:

```json
{
  "active": "staging",
  "environments": {
    "prod": {
      "base": "https://api.example.com",
      "token": "sk-live-…"
    },
    "staging": {
      "base": "https://staging.example.com",
      "token": "sk-test-…"
    },
    "local": {
      "base": "http://localhost:8080",
      "token": "dev"
    }
  }
}
```

`POKE_ENV_FILE` moves the file; useful for keeping work and personal
environments apart.

## Choosing one

```bash
poke --poke-env prod '{{base}}/users'    # this command only
POKE_ENV=prod poke '{{base}}/users'      # this shell
```

Otherwise the `active` environment is used. In pogo, press `E` to switch; the
choice is written back to the file, so the next `poke` in your terminal uses it
too. The active environment appears in pogo's header.

## Replaying across environments

This is the reason to bother:

```
E → staging → r
```

Select a request you made against production, switch environment, replay. Same
request, different target, and both are in your history side by side — press `d`
on the two to see exactly how the environments differ.

## Undefined variables

A reference with no value is left exactly as written:

```
$ poke '{{nosuchvar}}/x'
poke: undefined variable(s): {{nosuchvar}}
curl: (3) URL rejected: Bad hostname
```

The alternative — substituting an empty string — turns `{{base}}/users` into
`/users` and produces a failure with no visible cause. In pogo the editor lists
every variable a request references and marks the ones the active environment
cannot supply, before you run it.

## Syntax

`{{name}}`, with optional inner whitespace (`{{ name }}`). Names may contain
letters, digits, `_`, `.` and `-`.

Substitution is textual and applies to every argument, so variables work in
URLs, headers, bodies and any other flag value. There is no recursion: a value
containing `{{...}}` is inserted literally.

A command with no `{{` in it is passed through byte for byte, so enabling
environments cannot change what an existing command does.

## Collections

Stars answer "this one matters". Collections answer "these ones go together":

```
c            file the selected request under a name
collection:auth   filter to it
t            cycle grouping: chronological → by host → by collection
```

A collection is a plain name, not a hierarchy. The aim is finding things again,
not modelling a filing cabinet. Imports get one automatically:

```bash
pogo --import-har ~/Downloads/api.example.com.har --collection checkout
```

## Importing from a browser

Devtools → Network → *Save all as HAR*, then:

```bash
pogo --import-har ~/Downloads/api.example.com.har
```

Every request arrives with its headers and body, rendered as a curl command you
can inspect, edit and replay. This is the short path from "it works in the
browser but not from my terminal" to a diff of the two.

Imported entries are marked `import`, so they are never mistaken for something
poke ran. Browser pseudo-headers (`:authority`, `:method`) and hop-by-hop fields
curl sets itself are dropped; everything else is kept, including the
`Authorization` header — which lands in your history like any other captured
credential.
