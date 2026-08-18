# Cutting a release

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
**both** binaries — the install script and the self-updater both expect that.

```bash
tar -tzf dist/poke_*_darwin_arm64.tar.gz
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
POKE_INSTALL_DIR=/tmp/poke-verify sh install.sh
/tmp/poke-verify/poke --version
/tmp/poke-verify/pogo --version

# 2. Self-update, from the previous release.
poke --check-update
poke --update
```

Verifying the update path properly needs a build that thinks it is older. Build
one rather than waiting for the next release:

```bash
go build -ldflags "-X github.com/rmpato/poke/internal/version.Version=0.0.9" \
  -o /tmp/poke-old/poke ./cmd/poke
/tmp/poke-old/poke --check-update    # should offer the release you just cut
/tmp/poke-old/poke --update          # should download, verify and replace
/tmp/poke-old/poke --version
```

If `--update` reports a checksum mismatch, the release assets and
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
also mark the bad one as a pre-release in the GitHub UI so `--update` and the
install script stop resolving to it, and say so in the release notes of the
replacement.

## Version numbers in the binary

`Version` and `Commit` are injected at link time. A `go install` build has no
tag, so it reports `dev` plus the VCS revision from the module's build info.
That is expected; `--update` on a `dev` build says so and installs the latest
release.
