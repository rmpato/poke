# Responding to a security report

<sub>[Docs](../README.md) · [Runbooks](README.md)</sub>

## First, decide whether it is in scope

poke stores your HTTP traffic locally on purpose. [SECURITY.md](../../SECURITY.md)
lists what is expected behavior rather than a vulnerability — chiefly that
tokens appear in `history.jsonl` in the default `display` redaction mode.

A report about that is not a vulnerability, but it is a signal that the
documentation did not reach the reader. Answer it seriously, point at
[../security.md](../security.md), and consider whether the wording needs to be
louder.

Genuinely in scope:

- a secret that survives `redact.mode: "store"` and reaches disk
- files or directories created wider than `0700`/`0600`
- a crafted history file, blob reference or release archive that reads or writes
  outside the poke data directory
- poke altering the request curl sends, or captured data leaving the machine
- the updater installing bytes whose checksum does not match the published
  `checksums.txt`

## Triage

1. **Reproduce it** in a throwaway history directory:
   ```bash
   POKE_HOME=/tmp/poke-triage ./bin/poke ...
   ```
2. **Write a failing test before the fix.** The redaction miss found during
   development (`-H 'Authorization: …'` was cleaned in the header list but left
   in the recorded option list) is now pinned by
   `TestHeaderOptionValuesAreRedacted`. Every fix here should leave a test like
   that behind.
3. **Check for siblings.** A leak in one field usually has relatives: request
   headers, response headers, recorded options, the command line, the URL, the
   request body. Walk all six.

## Fixing and releasing

- Fix on `main`, with the test.
- Cut a patch release ([release.md](release.md)).
- If secrets may already be sitting in users' history files, say so plainly in
  the release notes and tell people how to check and clean up
  ([history-recovery.md](history-recovery.md)). Do not describe a data-exposure
  fix as "improved redaction handling".
- Credit the reporter unless they prefer otherwise.

## What not to do

Do not silently widen the default redaction list and call it a fix — that
changes what replay does for everyone. If the default trade-off is wrong, that
is a deliberate, documented, announced change, not a quiet patch.
