# The curl option table

`internal/curlargs/options.go` records which curl options take a value. poke
uses it to tell a flag's argument from a URL when building a history record.

It is generated, never hand-edited.

## When CI complains

`TestOptionTableMatchesLocalCurl` fails when the table disagrees with the curl
on the machine. That means one of two things:

- **curl changed an option's arity.** Rare, and worth reading about before
  regenerating.
- **The table is stale.** Regenerate it.

The test only fails on options the table already knows. Options a newer curl has
added that the table has never heard of are logged, not failed — the parser
treats unknown options safely, so they degrade a history record's completeness
rather than breaking anything.

## Regenerating

```bash
scripts/gen-curl-options.sh > /tmp/arity.txt
wc -l /tmp/arity.txt          # expect ~300 lines
```

The script asks curl itself rather than scraping help text, because curl's help
output does not mark arguments consistently across versions:

```
$ curl --proxy
curl: option --proxy: requires parameter
```

Then update `internal/curlargs/options.go` from that output, keeping the
supplemental list of options from curl releases newer than the machine's, and:

```bash
gofmt -w internal/curlargs/options.go
go test ./internal/curlargs/
```

## Why a wrong entry is survivable

Nothing in the execution path consults this table. poke passes the user's argv
to curl verbatim, so a wrong entry produces a history record with a missing or
odd-looking URL — never a wrong request. Anything the parser cannot place lands
in `Unrecognized` and pogo says the parse was incomplete.

That property is worth protecting: if you ever find yourself wanting to use
`curlargs` output to build a command to execute, stop.
