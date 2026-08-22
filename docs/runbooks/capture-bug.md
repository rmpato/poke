# "pogo behaves differently from curl"

<sub>[Docs](../README.md) · [Runbooks](README.md)</sub>

This is the report that matters most. pogo's entire premise is that wrapping
curl is invisible, so a difference is a bug in pogo until proven otherwise —
never something to explain away.

## 1. Establish the difference

Get the exact command, then run both:

```bash
curl <args>; echo "curl exit=$?"
pogo curl <args>; echo "pogo exit=$?"
```

Compare stdout, stderr and the exit code separately:

```bash
curl <args> >/tmp/c.out 2>/tmp/c.err; echo $? >/tmp/c.code
pogo curl <args> >/tmp/p.out 2>/tmp/p.err; echo $? >/tmp/p.code
diff /tmp/c.out /tmp/p.out
diff /tmp/c.err /tmp/p.err
diff /tmp/c.code /tmp/p.code
```

## 2. Rule capture in or out

```bash
pogo curl --pogo-no-capture <args>
```

If the difference disappears, capture is responsible. If it persists, pogo is
mangling the arguments — check `internal/curlargs` is not being used for
execution anywhere; it never should be.

## 3. Is it a terminal-only difference?

Most capture differences only appear when stdout is a terminal, because that is
when pogo puts a pipe in front of curl. Reproduce it under a pty rather than
guessing:

```bash
python3 - <<'PY'
import pty, os
pty.spawn(["pogo", "curl", "https://example.com"])
PY
```

The two behaviors pogo deliberately re-implements are:

- **The progress meter.** curl turns it on when output is not a terminal, so
  pogo passes `--no-progress-meter` when stdout *is* one and the body is going
  there. If a meter appears where curl showed none, or vanishes where curl
  showed one, that condition is wrong. See `runner.Run`.
- **The binary output guard.** curl refuses to write binary to a terminal
  unless asked with `-o -`. pogo reproduces the NUL-byte test, curl's warning
  text and exit 23. See `teeWriter` in `internal/runner/tee.go`.

Both are documented in [../architecture.md](../architecture.md). If you find a
third divergence, it is a bug: fix it, or document it there and say why it
cannot be fixed.

## 4. Common culprits

| Symptom | Likely cause |
|---|---|
| Extra progress meter on screen | `--no-progress-meter` not injected; check `bodyOnStdout` and the TTY flag |
| Binary garbage in the terminal | guard not enabled; check `guardTTY` |
| `-w` output missing | pogo overrode a user `--write-out`; `hasWriteOut` should have prevented that |
| `-D` file empty or missing | pogo used its own dump path instead of the user's |
| Interactive stdin broken | pogo wrapped stdin when it should have passed the file through; see `spec.ReadsStdin` |
| Wrong exit code | check the binary guard's forced 23, and signal handling in `internal/cli` |

## 5. Write the test first

Every fix here gets a test in `internal/runner` driving the real curl against an
`httptest` server. If it only reproduces under a terminal, add the assertion at
the `teeWriter` level, where the TTY condition is an ordinary struct field.
