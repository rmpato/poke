# Cutting a release

<sub>[Docs](../README.md) · [Runbooks](README.md)</sub>

Releases are built by goreleaser from a tag. Nothing is uploaded by hand.

## Before you tag

```bash
make check                 # gofmt, vet, test, race
golangci-lint run          # what CI's lint job runs
git status --porcelain     # must be empty
```

Confirm CI is green on `main`:

```bash
gh run list --workflow=ci.yml --limit 1
```

Then dry-run the build, which catches goreleaser configuration errors without
publishing anything:

```bash
goreleaser release --snapshot --clean
ls dist/
```

Check that `dist/` contains one archive per platform and that each archive holds
the `pogo` binary — the install script and the self-updater both look for it by
exact name.

```bash
tar -tzf dist/pogo_*_darwin_arm64.tar.gz
```

## Tag and push

Versions are `vMAJOR.MINOR.PATCH`. Until the on-disk format is stable, stay on
`v0.x` and treat a format change as a minor bump.

```bash
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

The `Release` workflow runs the test suite again, then goreleaser. Watch it:

```bash
gh run watch $(gh run list --workflow=release.yml --limit 1 --json databaseId -q '.[0].databaseId')
```

## After

Verify the two paths users actually take.

```bash
# 1. The install script, into a throwaway directory.
POGO_INSTALL_DIR=/tmp/pogo-verify sh install.sh
/tmp/pogo-verify/pogo --version
/tmp/pogo-verify/pogo curl -sS -o /dev/null -w '%{http_code}\n' https://example.com

# 2. Self-update, from the previous release.
pogo update --check
pogo update
```

Verifying the update path properly needs a build that thinks it is older. Build
one rather than waiting for the next release:

```bash
go build -ldflags "-X github.com/rmpato/pogo/internal/version.Version=0.0.9" \
  -o /tmp/pogo-old/pogo ./cmd/pogo
/tmp/pogo-old/pogo update --check    # should offer the release you just cut
/tmp/pogo-old/pogo update          # should download, verify and replace
/tmp/pogo-old/pogo --version
```

If `pogo update` reports a checksum mismatch, the release assets and
`checksums.txt` disagree. Do not re-upload assets by hand: delete the release and
retag, so the checksums are regenerated together with the archives.

## When a release is wrong

Releases are immutable in practice — people have already downloaded them, and
the self-updater resolves "latest". Do not move a tag.

Ship a patch release instead:

```bash
git revert <bad commit>      # or fix forward
git tag -a v0.2.1 -m "v0.2.1"
git push origin v0.2.1
```

If the release is actively harmful (it corrupts history, it leaks a secret),
also mark the bad one as a pre-release in the GitHub UI so `pogo update` and the
install script stop resolving to it, and say so in the release notes of the
replacement.

## Version numbers in the binary

`Version` and `Commit` are injected at link time. A `go install` build has no
tag, so it reports `dev` plus the VCS revision from the module's build info.
That is expected; `pogo update` on a `dev` build says so and installs the latest
release.
