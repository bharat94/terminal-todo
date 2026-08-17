# Install terminal-todo

The supported binary distribution is the GitHub-attested release pipeline.
Each release provides Linux, macOS, and Windows archives for amd64 and arm64,
plus SHA-256 checksums, SPDX SBOMs, and GitHub provenance attestations.

The current prerelease is
[`v0.1.0-beta.1`](https://github.com/bharat94/terminal-todo/releases/tag/v0.1.0-beta.1).
It is production-validated within the documented
[beta operating boundary](production-readiness.md), but is not a 1.0 stability
claim.

## Choose an artifact

| Platform | Intel / AMD 64-bit | ARM 64-bit |
|---|---|---|
| Linux | `terminal-todo_0.1.0-beta.1_linux_amd64.tar.gz` | `terminal-todo_0.1.0-beta.1_linux_arm64.tar.gz` |
| macOS | `terminal-todo_0.1.0-beta.1_darwin_amd64.tar.gz` | `terminal-todo_0.1.0-beta.1_darwin_arm64.tar.gz` |
| Windows | `terminal-todo_0.1.0-beta.1_windows_amd64.zip` | `terminal-todo_0.1.0-beta.1_windows_arm64.zip` |

Download the matching archive and `checksums.txt` from the release page.
Apple Silicon machines use `darwin_arm64`; most current Windows and Linux PCs
use `amd64`.

## Verify the download

Compare the digest printed by your platform with the matching line in
`checksums.txt` before extracting the archive.

Linux:

```bash
sha256sum terminal-todo_0.1.0-beta.1_linux_amd64.tar.gz
```

macOS:

```bash
shasum -a 256 terminal-todo_0.1.0-beta.1_darwin_arm64.tar.gz
```

Windows PowerShell:

```powershell
Get-FileHash .\terminal-todo_0.1.0-beta.1_windows_amd64.zip -Algorithm SHA256
```

Operators who verify software supply-chain provenance can additionally use
the [GitHub CLI attestation command](releasing.md#verify-downloaded-artifacts).

## Install on Linux or macOS

Extract the archive, then place `todo` in a directory on `PATH`:

```bash
tar -xzf terminal-todo_0.1.0-beta.1_linux_amd64.tar.gz
sudo install -m 755 todo /usr/local/bin/todo
todo --version
```

Replace the archive name with the one downloaded for your platform. A
user-local installation does not require elevated privileges:

```bash
mkdir -p "$HOME/.local/bin"
install -m 755 todo "$HOME/.local/bin/todo"
```

If `$HOME/.local/bin` is not already on `PATH`, add it through your shell's
normal profile configuration and start a new shell.

## Install on Windows

Expand the zip, move `todo.exe` into a stable directory such as
`%LOCALAPPDATA%\Programs\terminal-todo`, and add that directory to the user
`PATH`. In a new PowerShell window, confirm:

```powershell
todo --version
todo help
```

## Build from source

Source builds require Go 1.26.1 or newer in the Go 1.26 release line:

```bash
git clone https://github.com/bharat94/terminal-todo.git
cd terminal-todo
go mod verify
make build
./todo --version
```

Install to `/usr/local/bin` with `sudo make install`, or choose another prefix:

```bash
make install PREFIX="$HOME/.local"
```

## Install with the script

```bash
curl -fsSL https://raw.githubusercontent.com/bharat94/terminal-todo/master/install.sh | sh
```

The script detects your platform, downloads the matching archive, and verifies
it against the release's `checksums.txt` before installing. A mismatch is a
hard failure: the download is discarded and nothing is installed.

| Variable | Default | Purpose |
|---|---|---|
| `VERSION` | latest release | Install a specific tag, such as `v0.1.0-beta.1` |
| `PREFIX` | `$HOME/.local` | Install into `$PREFIX/bin` |
| `BINDIR` | `$PREFIX/bin` | Install into an exact directory |

Piping a script from the network into a shell means trusting this repository
and the transport. Download and read it first if you would rather not:

```bash
curl -fsSL -O https://raw.githubusercontent.com/bharat94/terminal-todo/master/install.sh
less install.sh
sh install.sh
```

The script verifies checksums but does not verify SBOMs or provenance
attestations. Use a release archive directly when those matter. Windows is not
covered; use the archive instructions above.

## Install with the Go toolchain

```bash
go install github.com/bharat94/terminal-todo/cmd/todo@latest
```

The binary lands in `$(go env GOPATH)/bin`, which must be on your `PATH`.
Replace `@latest` with a tag such as `@v0.1.0-beta.1` to pin a release.

Install `./cmd/todo`, not the module root. Go names an installed binary after
the last element of its import path, so installing the module root produces a
binary called `terminal-todo`. That path still works, and is kept working, for
anyone already using it.

`go install` cannot pass linker flags, so the version is recovered from the
module version the toolchain records in the binary. A tagged install reports
that tag; an install from an untagged commit reports a pseudo-version.

This route does not produce the checksums, SBOMs, or provenance attestations
that release archives carry. Prefer an archive when those matter.

## First project

From the repository whose work you want to coordinate:

```bash
todo init
todo add "Try terminal-todo"
todo status
```

To configure project-scoped Codex and Claude Code integrations:

```bash
todo integrate
todo integrate --check --live
```

Continue with the [quick start](../README.md#quick-start) or the complete
[examples](examples.md).

## Upgrade safely

Before replacing the binary for an important shared project:

```bash
todo backup
todo --version
```

Stop active workers, install the new binary, run `todo doctor`, then restart
workers with the version-matched integration files:

```bash
todo doctor
todo integrate --check
todo integrate
todo integrate --check --live
```

`todo integrate` refuses to overwrite locally modified terminal-todo entries
or skill files without `--force`. Review drift before replacing it. A newer
binary migrates supported older stores under the project lock; see the
[store compatibility contract](compatibility.md#store-compatibility) before
planning a rollback.

## Uninstall

Remove only the installed `todo` or `todo.exe` binary from the directory where
you placed it. Project data is independent and remains in each repository's
`.terminal-todo/` directory.

Do not delete `.terminal-todo/` unless you intentionally want to delete that
project's task graph, history, agent cards, configuration, and backups. Use
`todo backup` first when the state may be needed later.
