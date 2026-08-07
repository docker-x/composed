# Security Policy

## Supported Versions

`composed` is pre-1.0 and follows [Semantic Versioning](https://semver.org). Only the latest released minor version line receives full security updates; older lines may receive critical backports on a best-effort basis.

| Version | Supported |
|---|---|
| v0.6.x | :white_check_mark: |
| v0.5.x | :warning: (critical fixes only, best effort) |
| < v0.5.0 | :x: |

We update this table when a new minor release is published. If you are running an unsupported version, include that version in your report. Upgrade to the latest release when possible, but do not delay reporting while you upgrade.

## Reporting a Vulnerability

Please do not open a public issue, discussion, or pull request for a security vulnerability. Instead, submit a private report so we can investigate and coordinate disclosure.

**Preferred channel:** [Open a draft GitHub Security Advisory](https://github.com/docker-x/composed/security/advisories/new) for `docker-x/composed`.

When reporting, include:

- A clear description of the vulnerability and its impact.
- Steps to reproduce or a proof-of-concept.
- The affected version(s) and any suggested remediation.
- Whether you would like public credit if we publish an advisory.

**What to expect:**

- **Acknowledgement:** within 5 business days.
- **Status updates:** at least every 14 days, or sooner if there is significant progress.
- **Triage outcome:** we will accept the report, decline it with an explanation, or request more information.
- **Disclosure:** we follow coordinated disclosure. Once a fix is available, we typically publish the advisory within 90 days. We may publish it earlier by mutual agreement. We give credit only with your permission. We ask that you do not disclose the issue before we publish a fix unless mutually agreed.
