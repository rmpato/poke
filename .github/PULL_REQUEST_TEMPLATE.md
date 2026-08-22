## What this changes

<!-- One or two sentences. What is different after this PR? -->

## Why

<!-- The problem, or the behaviour that was wrong. Link an issue if there is one. -->

## Checklist

- [ ] `make check` passes (`gofmt`, `go vet`, `go test`, `go test -race`)
- [ ] Tests cover the change, or there is a reason they do not
- [ ] Docs updated if behaviour changed (`README.md`, `docs/`)

## If this touches how pogo runs curl

pogo's core promise is that wrapping curl does not change what curl does.

- [ ] The user's own arguments are still passed through verbatim
- [ ] stdout and stderr still carry exactly what curl produced
- [ ] Exit codes are unchanged
- [ ] Any new divergence is documented in `docs/architecture.md`

## If this touches what gets stored

- [ ] `docs/security.md` still describes reality
- [ ] No new secret is written to disk that redaction does not cover

## If this touches the UI

- [ ] Every new screen renders as exactly the terminal's rectangle
      (`internal/tui/render_guard_test.go` has a case for it)
- [ ] No colour is constructed in a screen file; it comes from a theme token
- [ ] New actions are in the command registry, so the palette and the `?`
      reference pick them up without a second edit
