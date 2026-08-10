# Releasing terminal-todo

Releases are immutable GitHub artifacts built from semantic version tags. The
release workflow produces the `todo` binary for:

- Linux on amd64 and arm64;
- macOS on amd64 and arm64;
- Windows on amd64 and arm64.

Unix archives use `tar.gz`; Windows archives use `zip`. The workflow is
configured to publish SHA-256 checksums, per-archive SPDX JSON SBOMs, and
GitHub build provenance attestations. Snapshot generation and the first public
tag have validated the archive, SBOM, publication, and OIDC attestation path.

## Published release evidence

[`v0.1.0-beta.1`](https://github.com/bharat94/terminal-todo/releases/tag/v0.1.0-beta.1)
was published from commit `1b2ed698bed52bb9e636854f7d7990b2d039370a` by the
[release workflow](https://github.com/bharat94/terminal-todo/actions/runs/31297477889).
Independent post-publication verification downloaded the release and confirmed:

- every archive and SBOM matches `checksums.txt`;
- all six SBOMs are valid SPDX 2.3 documents;
- the macOS arm64 binary reports `todo v0.1.0-beta.1` and renders help outside
  a project; and
- GitHub provenance verification succeeds for all six platform archives.

The tagged commit also passed the complete required
[CI matrix](https://github.com/bharat94/terminal-todo/actions/runs/31297348237).

## Prepare a release

1. Start from a clean, current `master`.
2. Confirm the intended version follows semantic versioning.
3. Run the complete verification suite:

```bash
go mod verify
go test ./... -race -count=1 -timeout 300s
go vet ./...
goreleaser check
goreleaser release --snapshot --clean
```

4. Extract at least one snapshot archive and verify:

```bash
./todo --version
./todo help
```

5. Review generated archive names, `dist/checksums.txt`, SBOMs, and the
   changelog preview.

## Publish

Create and push an annotated tag. Tag creation is the explicit release
approval boundary:

```bash
git tag -a v0.1.0-beta.1 -m "terminal-todo v0.1.0-beta.1"
git push origin v0.1.0-beta.1
```

Tags matching `vMAJOR.MINOR.PATCH` or a semantic prerelease suffix trigger the
release workflow. GoReleaser tests the tagged commit, builds with CGO disabled
and trimmed source paths, and uploads a draft release. GitHub then attests
every archive listed in `checksums.txt`; only after attestation succeeds does
the workflow publish the release. A failed attestation therefore leaves a
non-public draft for investigation.

Do not move or reuse a published tag. If a release is faulty, document the
problem and publish a new patch version.

## Verify downloaded artifacts

Download an archive, its `.sbom.json`, and `checksums.txt` from the GitHub
release:

```bash
sha256sum --ignore-missing -c checksums.txt
jq -e '.spdxVersion == "SPDX-2.3"' \
  terminal-todo_0.1.0-beta.1_linux_amd64.tar.gz.sbom.json
gh attestation verify --owner bharat94 \
  terminal-todo_0.1.0-beta.1_linux_amd64.tar.gz
```

On macOS, use:

```bash
shasum -a 256 -c checksums.txt
```

The version embedded in the binary must match the release tag:

```bash
todo --version
```

## Rollback and incident handling

GitHub releases are distribution records, not mutable deployment slots.
Never replace an archive under an existing version. For a compromised or
broken artifact:

1. mark the GitHub release as affected and state the impact;
2. rotate any compromised release credentials;
3. fix and verify from a clean checkout;
4. publish a new patch release;
5. document which versions users should stop running.

The release workflow uses only GitHub's short-lived repository token and OIDC
identity. It requires no long-lived signing key.
