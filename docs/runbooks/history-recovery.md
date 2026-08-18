# History that is damaged, enormous, or holds a secret

<sub>[Docs](../README.md) · [Runbooks](README.md)</sub>

History is a plain append-only JSONL file plus a blob directory. That is
deliberate: everything below is doable with standard tools.

```bash
pogo --path          # where it lives
```

## Inspecting without pogo

```bash
cd "$(pogo --path)"
tail -1 history.jsonl | jq .
jq -r 'select(.op=="put") | .entry | "\(.request.method) \(.request.url) \(.exit)"' history.jsonl
```

Each line is one operation: `put` records a capture, `patch` a star or note,
`del` a tombstone. Folding them in order gives the current history, which is
what `store.Load` does.

## Damaged lines

pogo reports damaged records in its header rather than refusing to start, and
skips them. To see what was skipped:

```bash
while read -r line; do
  echo "$line" | jq empty 2>/dev/null || echo "BAD: ${line:0:80}"
done < history.jsonl
```

To drop them permanently, compact — it rewrites the log from the folded state:

```bash
pogo --compact
```

A truncated final line (a crash mid-write) is the usual cause and is harmless:
the record is skipped and everything before it is intact. This is the main
reason the format is append-only.

## A history that has grown too large

```bash
du -sh "$(pogo --path)"
wc -l "$(pogo --path)/history.jsonl"
```

Compaction applies the entry cap (`capture.max_entries`, 5000 by default),
oldest first, and sweeps blobs no surviving entry references. Starred entries
are never dropped by the cap.

```bash
pogo --compact
```

To keep less in future:

```json
{ "capture": { "max_entries": 500, "max_response_body": 262144 } }
```

## A secret you did not mean to store

Assume it is compromised and rotate it. Then remove it:

```bash
# Find the entries.
jq -r 'select(.op=="put") | select(.entry.command.args[]? | contains("sk-live")) | .entry.id' \
  "$(pogo --path)/history.jsonl"
```

Delete each in pogo with `x`, then compact so the original capture records are
gone from the log rather than merely tombstoned:

```bash
pogo --compact
```

Verify:

```bash
grep -c "sk-live" "$(pogo --path)/history.jsonl"   # expect 0
grep -rl "sk-live" "$(pogo --path)/blobs/"          # expect nothing
```

To prevent a repeat, switch redaction to store mode, accepting that replay will
no longer authenticate:

```json
{ "redact": { "mode": "store" } }
```

## Moving history between machines

It is a directory. Copy it.

```bash
tar -czf poke-history.tar.gz -C "$(dirname "$(pogo --path)")" poke
```

Remember what is inside before you put that anywhere:
[../security.md](../security.md).

## Starting over

```bash
rm -rf "$(pogo --path)"
```
