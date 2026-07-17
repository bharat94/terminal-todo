# Security policy

terminal-todo stores coordination state on the user's filesystem and may be
invoked by autonomous workers. Security reports involving state integrity,
path handling, command execution, authorization, release artifacts, or
cross-worker isolation are especially welcome.

## Supported versions

Before the first tagged release, security fixes land on `master`. After
releases begin, the latest release line and `master` receive fixes. Older
versions may be asked to upgrade when a safe backport is not practical.

## Report a vulnerability

Use GitHub's private vulnerability reporting:

<https://github.com/bharat94/terminal-todo/security/advisories/new>

Do not open a public issue for an undisclosed vulnerability. Include:

- affected version or commit;
- operating system and filesystem;
- impact and realistic threat model;
- minimal reproduction steps or proof of concept;
- any suggested mitigation;
- whether the issue is already public.

Avoid including real credentials, personal data, or a user's task store.
Synthetic fixtures are preferred.

The maintainer will aim to acknowledge a complete report within five business
days, coordinate validation and remediation, and credit the reporter unless
anonymity is requested. Please allow a reasonable remediation window before
public disclosure.

For the data model, permissions, backup boundary, and known trust assumptions,
see [Security and data lifecycle](docs/security-and-data.md).
