# Security Policy

## Supported versions

SeedGrid is under active development (V1 / MVP). There are no stable releases with formal
support yet; fixes land on the main branch.

## Reporting a vulnerability

Please report vulnerabilities **privately**, not through public issues. Use GitHub's private
vulnerability reporting on this repository (**Security → Report a vulnerability**).

Include a description, the potential impact, steps to reproduce and the affected version or
commit. You will get an acknowledgement, and the fix will be coordinated with you before any
public disclosure.

## Security principles of the platform

As set out in the official documentation (`docs/14 - Authentication.md`,
`docs/21 - Coding Standards.md`):

- All administrative communication uses TLS.
- Every API requires authentication (except the initial Agent registration, which is public
  by design and protected by a join token).
- No credential is stored in the source code.
- Least privilege for every component.
- Credentials never appear in logs, events, error messages or the audit trail.
- Every administrative operation is auditable.
- Site profiles shared through this repository have no field for credentials; see
  [`profiles/README.md`](profiles/README.md).
