# Security Policy

Muamba is alpha-stage software that downloads remote bytes, establishes
integrity locks, and writes files into repositories. Responsible disclosure is
appreciated.

## Supported versions

| Version | Supported |
| --- | --- |
| Latest `v0.0.x` release | Yes |
| Older releases | No |

During alpha, only the latest tagged release receives security fixes.

## Reporting a vulnerability

Do not open a public issue for security problems.

Report privately through GitHub's
[Report a vulnerability](https://github.com/araihu/muamba/security/advisories/new)
page or email **security@araihu.dev**.

Please include:

- affected command and version or commit;
- impact and expected security boundary;
- minimal reproduction steps or proof of concept;
- operating system and filesystem details when relevant; and
- suggested remediation, if available.

Remove credentials, authorization values, private URLs, and sensitive response
bodies from reports sent through channels you do not fully trust.

## Scope

High-value reports include integrity bypasses, path traversal or symlink escape,
unsafe redirect handling, credential or response-body disclosure, non-atomic
grouped updates, lock corruption, unsafe partial writes, denial of service from
unbounded downloads, and generated embed registries that include unintended
files.

## What to expect

- Acknowledgement within five business days.
- Initial impact and severity assessment after reproduction.
- Coordinated disclosure and credit, unless anonymity is preferred, after a fix
  is available.
